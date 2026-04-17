import { InjectQueue } from '@nestjs/bullmq';
import { Injectable, Logger, NotFoundException } from '@nestjs/common';
import { Queue } from 'bullmq';
import { eq } from 'drizzle-orm';
import { SearchQuery } from '@job-finder/shared';
import { DbService } from '../../db/db.service';
import { jobSource, savedSearch } from '../../db/schema';
import { IngestJobData, QUEUE_INGEST } from '../../common/queues';
import { JobSourceRegistry } from '../job-sources/job-source.registry';

@Injectable()
export class IngestionService {
  private readonly logger = new Logger(IngestionService.name);

  constructor(
    private readonly dbs: DbService,
    private readonly registry: JobSourceRegistry,
    @InjectQueue(QUEUE_INGEST) private readonly ingestQueue: Queue<IngestJobData>,
  ) {}

  private get db() {
    return this.dbs.db;
  }

  /** Enqueue one ingest job per (search × enabled source). */
  async runSearch(searchId: string) {
    const [search] = await this.db.select().from(savedSearch).where(eq(savedSearch.id, searchId));
    if (!search) throw new NotFoundException(`search ${searchId} not found`);

    const query = search.query as unknown as SearchQuery;
    const enabledSources = await this.db.select().from(jobSource).where(eq(jobSource.enabled, true));
    const enabledKeys = new Set(enabledSources.map((s) => s.key));

    const wanted =
      query.sources && query.sources.length > 0 ? query.sources : this.registry.keys();
    const keys = wanted.filter((k) => enabledKeys.has(k));

    for (const sourceKey of keys) {
      await this.ingestQueue.add(
        'ingest',
        { searchId, sourceKey },
        { removeOnComplete: 100, removeOnFail: 100, attempts: 1 },
      );
    }
    await this.db.update(savedSearch).set({ lastRunAt: new Date() }).where(eq(savedSearch.id, searchId));
    this.logger.log(`search '${search.name}' enqueued for sources: ${keys.join(', ') || '(none)'}`);
    return { enqueued: keys };
  }
}
