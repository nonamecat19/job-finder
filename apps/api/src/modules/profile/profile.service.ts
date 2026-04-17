import { BadRequestException, Inject, Injectable, Logger, NotFoundException } from '@nestjs/common';
import { JsonResume } from '@job-finder/shared';
import { desc, eq } from 'drizzle-orm';
import pdfParse from 'pdf-parse';
import { DbService } from '../../db/db.service';
import { profile } from '../../db/schema';
import { LLM_PROVIDER, LlmProvider } from '../llm/llm.types';
import { jsonResumeSchema, ParsedResume } from './profile.schemas';

@Injectable()
export class ProfileService {
  private readonly logger = new Logger(ProfileService.name);

  constructor(
    private readonly dbs: DbService,
    @Inject(LLM_PROVIDER) private readonly llm: LlmProvider,
  ) {}

  private get db() {
    return this.dbs.db;
  }

  list() {
    return this.db.select().from(profile).orderBy(desc(profile.updatedAt));
  }

  async get(id: string) {
    const [found] = await this.db.select().from(profile).where(eq(profile.id, id));
    if (!found) throw new NotFoundException(`profile ${id} not found`);
    return found;
  }

  /** Most recently updated profile — the single-user default for matching/generation. */
  async getDefault() {
    const [found] = await this.db.select().from(profile).orderBy(desc(profile.updatedAt)).limit(1);
    if (!found) throw new NotFoundException('no profile exists yet — create one first');
    return found;
  }

  async create(data: { name: string; document: JsonResume; extraNotes?: string }) {
    if (!data.name) throw new BadRequestException('name is required');
    const [created] = await this.db
      .insert(profile)
      .values({
        name: data.name,
        document: (data.document ?? {}) as object,
        extraNotes: data.extraNotes ?? null,
      })
      .returning();
    await this.refreshEmbedding(created.id).catch((e) =>
      this.logger.warn(`profile embedding failed (ollama down?): ${e.message}`),
    );
    return this.get(created.id);
  }

  async update(id: string, data: { name?: string; document?: JsonResume; extraNotes?: string | null }) {
    await this.get(id);
    await this.db
      .update(profile)
      .set({
        ...(data.name !== undefined ? { name: data.name } : {}),
        ...(data.document !== undefined ? { document: data.document as object } : {}),
        ...(data.extraNotes !== undefined ? { extraNotes: data.extraNotes } : {}),
        updatedAt: new Date(),
      })
      .where(eq(profile.id, id));
    await this.refreshEmbedding(id).catch((e) =>
      this.logger.warn(`profile embedding failed: ${e.message}`),
    );
    return this.get(id);
  }

  async remove(id: string) {
    await this.get(id);
    await this.db.delete(profile).where(eq(profile.id, id));
    return { deleted: true };
  }

  /** PDF upload → text → LLM structured parse. Returns a draft; the user reviews and saves explicitly. */
  async importPdf(buffer: Buffer): Promise<{ draft: ParsedResume; textLength: number }> {
    const parsed = await pdfParse(buffer);
    const text = parsed.text?.trim();
    if (!text || text.length < 50) {
      throw new BadRequestException('could not extract text from PDF (scanned image? try a text-based PDF)');
    }
    const draft = await this.llm.completeStructured(
      `Extract a structured resume from the following raw text of a PDF resume.\n` +
        `Rules: copy facts exactly as written — do NOT invent, embellish, or guess missing employers, dates, or degrees. ` +
        `Omit fields that are not present in the text.\n\nRESUME TEXT:\n${text.slice(0, 15000)}`,
      jsonResumeSchema,
      { system: 'You convert resume text into JSON Resume format. You never fabricate information.' },
    );
    return { draft, textLength: text.length };
  }

  /** Serialize profile into text and store its embedding. */
  async refreshEmbedding(id: string) {
    const found = await this.get(id);
    const text = this.profileToText(found.document as JsonResume, found.extraNotes);
    const embedding = await this.llm.embed(text);
    await this.db
      .update(profile)
      .set({ embedding, embedModel: process.env.EMBED_MODEL ?? 'nomic-embed-text' })
      .where(eq(profile.id, id));
  }

  profileToText(doc: JsonResume, extraNotes?: string | null): string {
    const parts: string[] = [];
    if (doc.basics?.label) parts.push(doc.basics.label);
    if (doc.basics?.summary) parts.push(doc.basics.summary);
    for (const w of doc.work ?? []) {
      parts.push([w.position, 'at', w.name, '—', w.summary, ...(w.highlights ?? [])].filter(Boolean).join(' '));
    }
    for (const s of doc.skills ?? []) {
      parts.push([s.name, ...(s.keywords ?? [])].join(' '));
    }
    for (const p of doc.projects ?? []) {
      parts.push([p.name, p.description, ...(p.keywords ?? [])].filter(Boolean).join(' '));
    }
    if (extraNotes) parts.push(extraNotes);
    return parts.join('\n');
  }
}
