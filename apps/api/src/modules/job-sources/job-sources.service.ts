import { Injectable, Logger, NotFoundException, OnModuleInit } from '@nestjs/common';
import { eq } from 'drizzle-orm';
import { DbService } from '../../db/db.service';
import { jobSource } from '../../db/schema';
import { decryptJson, encryptJson, hasEncryptionKey } from '../../common/crypto';
import { JobSourceRegistry } from './job-source.registry';

const SECRET_KEYS = /cookie|key|secret|token|password/i;

@Injectable()
export class JobSourcesService implements OnModuleInit {
  private readonly logger = new Logger(JobSourcesService.name);

  constructor(
    private readonly dbs: DbService,
    private readonly registry: JobSourceRegistry,
  ) {}

  private get db() {
    return this.dbs.db;
  }

  /** Seed a JobSource row per registered adapter so the dashboard can toggle them. */
  async onModuleInit() {
    for (const adapter of this.registry.all()) {
      await this.db
        .insert(jobSource)
        .values({ key: adapter.key, kind: adapter.kind, config: this.encryptConfig({}) })
        .onConflictDoUpdate({ target: jobSource.key, set: { kind: adapter.kind } });
    }
  }

  private encryptConfig(config: Record<string, unknown>): object {
    if (!hasEncryptionKey()) return config; // dev fallback: plaintext
    return { enc: encryptJson(config) };
  }

  decryptConfig(stored: unknown): Record<string, unknown> {
    if (stored && typeof stored === 'object' && 'enc' in (stored as object)) {
      try {
        return decryptJson((stored as { enc: string }).enc);
      } catch (e) {
        this.logger.error(`config decrypt failed: ${(e as Error).message}`);
        return {};
      }
    }
    return (stored as Record<string, unknown>) ?? {};
  }

  private maskConfig(config: Record<string, unknown>): Record<string, unknown> {
    const masked: Record<string, unknown> = {};
    for (const [k, v] of Object.entries(config)) {
      masked[k] = SECRET_KEYS.test(k) && typeof v === 'string' && v ? '••••••' : v;
    }
    return masked;
  }

  async list() {
    const sources = await this.db.select().from(jobSource).orderBy(jobSource.key);
    return sources.map((s) => ({
      id: s.id,
      key: s.key,
      kind: s.kind,
      enabled: s.enabled,
      healthy: s.healthy,
      config: this.maskConfig(this.decryptConfig(s.config)),
    }));
  }

  async getByKey(key: string) {
    const [source] = await this.db.select().from(jobSource).where(eq(jobSource.key, key));
    if (!source) throw new NotFoundException(`source '${key}' not found`);
    return source;
  }

  async update(key: string, data: { enabled?: boolean; config?: Record<string, unknown> }) {
    const source = await this.getByKey(key);
    let config = this.decryptConfig(source.config);
    if (data.config) {
      // masked values ('••••••') mean "keep existing"
      for (const [k, v] of Object.entries(data.config)) {
        if (v === '••••••') continue;
        if (v === null || v === '') delete config[k];
        else config[k] = v;
      }
    }
    await this.db
      .update(jobSource)
      .set({
        ...(data.enabled !== undefined ? { enabled: data.enabled } : {}),
        ...(data.config ? { config: this.encryptConfig(config) } : {}),
      })
      .where(eq(jobSource.key, key));
    return (await this.list()).find((s) => s.key === key);
  }

  /** Run the adapter's health check (or a tiny search) and persist the result. */
  async test(key: string): Promise<{ ok: boolean; error?: string }> {
    const source = await this.getByKey(key);
    const adapter = this.registry.get(key);
    const config = this.decryptConfig(source.config);
    try {
      const ok = adapter.healthCheck
        ? await adapter.healthCheck(config)
        : (await adapter.search({ keywords: 'developer' }, config)).length >= 0;
      await this.db.update(jobSource).set({ healthy: ok }).where(eq(jobSource.key, key));
      return { ok };
    } catch (e) {
      await this.db.update(jobSource).set({ healthy: false }).where(eq(jobSource.key, key));
      return { ok: false, error: (e as Error).message };
    }
  }
}
