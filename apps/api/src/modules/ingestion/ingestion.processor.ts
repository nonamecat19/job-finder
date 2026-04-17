import { Processor, WorkerHost, InjectQueue } from '@nestjs/bullmq';
import { Logger } from '@nestjs/common';
import { Job as BullJob, Queue } from 'bullmq';
import { createHash } from 'crypto';
import { and, desc, eq, isNotNull } from 'drizzle-orm';
import { NormalizedJob, SearchQuery } from '@job-finder/shared';
import { DbService } from '../../db/db.service';
import { job, jobSource, savedSearch, sourceRun } from '../../db/schema';
import { IngestJobData, MatchJobData, QUEUE_INGEST, QUEUE_MATCH } from '../../common/queues';
import { JobSourceRegistry } from '../job-sources/job-source.registry';
import { JobSourcesService } from '../job-sources/job-sources.service';

const UNHEALTHY_AFTER_CONSECUTIVE_FAILURES = 3;

@Processor(QUEUE_INGEST, { concurrency: 2 })
export class IngestionProcessor extends WorkerHost {
  private readonly logger = new Logger(IngestionProcessor.name);

  constructor(
    private readonly dbs: DbService,
    private readonly registry: JobSourceRegistry,
    private readonly sources: JobSourcesService,
    @InjectQueue(QUEUE_MATCH) private readonly matchQueue: Queue<MatchJobData>,
  ) {
    super();
  }

  private get db() {
    return this.dbs.db;
  }

  async process(bullJob: BullJob<IngestJobData>): Promise<{ found: number; new: number }> {
    const { searchId, sourceKey } = bullJob.data;
    const source = await this.sources.getByKey(sourceKey);
    const [run] = await this.db
      .insert(sourceRun)
      .values({ sourceId: source.id, searchId })
      .returning();

    try {
      const [search] = searchId
        ? await this.db.select().from(savedSearch).where(eq(savedSearch.id, searchId))
        : [null];
      const query = (search?.query ?? { keywords: '' }) as unknown as SearchQuery;
      const adapter = this.registry.get(sourceKey);
      const config = this.sources.decryptConfig(source.config);

      const jobs = await adapter.search(query, config);
      let created = 0;
      for (const j of jobs) {
        if (await this.persistIfNew(j)) created++;
      }

      await this.db
        .update(sourceRun)
        .set({ finishedAt: new Date(), ok: true, found: jobs.length, new: created })
        .where(eq(sourceRun.id, run.id));
      await this.db.update(jobSource).set({ healthy: true }).where(eq(jobSource.key, sourceKey));
      this.logger.log(`${sourceKey}: found ${jobs.length}, new ${created}`);
      return { found: jobs.length, new: created };
    } catch (e) {
      const message = (e as Error).message?.slice(0, 1000);
      await this.db
        .update(sourceRun)
        .set({ finishedAt: new Date(), ok: false, error: message })
        .where(eq(sourceRun.id, run.id));
      await this.flagIfUnhealthy(source.id, sourceKey);
      this.logger.error(`${sourceKey} ingest failed: ${message}`);
      throw e;
    }
  }

  /** Dedup by sha256(company+title+canonicalUrl); returns true if the job is new. */
  private async persistIfNew(newJob: NormalizedJob): Promise<boolean> {
    const canonicalUrl = newJob.url.split('?')[0].replace(/\/+$/, '');
    const dedupeKey = createHash('sha256')
      .update(`${newJob.company.toLowerCase()}|${newJob.title.toLowerCase()}|${canonicalUrl}`)
      .digest('hex');

    const [existing] = await this.db.select({ id: job.id }).from(job).where(eq(job.dedupeKey, dedupeKey));
    if (existing) return false;

    const [created] = await this.db
      .insert(job)
      .values({
        dedupeKey,
        sourceKey: newJob.sourceKey,
        externalId: newJob.externalId ?? null,
        title: newJob.title,
        company: newJob.company,
        location: newJob.location ?? null,
        remote: newJob.remote,
        salaryRaw: newJob.salaryRaw ?? null,
        url: newJob.url,
        description: newJob.description,
        raw: (newJob.raw ?? {}) as object,
        postedAt: newJob.postedAt ? new Date(newJob.postedAt) : null,
      })
      .returning();
    await this.matchQueue.add(
      'match',
      { jobId: created.id },
      { removeOnComplete: 500, removeOnFail: 200, attempts: 2, backoff: { type: 'exponential', delay: 10_000 } },
    );
    return true;
  }

  private async flagIfUnhealthy(sourceId: string, sourceKey: string) {
    const recent = await this.db
      .select({ ok: sourceRun.ok })
      .from(sourceRun)
      .where(and(eq(sourceRun.sourceId, sourceId), isNotNull(sourceRun.ok)))
      .orderBy(desc(sourceRun.startedAt))
      .limit(UNHEALTHY_AFTER_CONSECUTIVE_FAILURES);
    const allFailed =
      recent.length === UNHEALTHY_AFTER_CONSECUTIVE_FAILURES && recent.every((r) => r.ok === false);
    if (allFailed) {
      await this.db.update(jobSource).set({ healthy: false }).where(eq(jobSource.key, sourceKey));
      this.logger.warn(`source ${sourceKey} flagged unhealthy after ${recent.length} consecutive failures`);
    }
  }
}
