import { InjectQueue } from '@nestjs/bullmq';
import { Injectable, NotFoundException } from '@nestjs/common';
import { Queue } from 'bullmq';
import { and, count, desc, eq, gte, ilike, ne, or, sql } from 'drizzle-orm';
import { DbService } from '../../db/db.service';
import { application, job, matchResult } from '../../db/schema';
import { GenerateJobData, QUEUE_GENERATE } from '../../common/queues';

export interface JobListParams {
  sort?: 'score' | 'date';
  source?: string;
  minScore?: number;
  status?: string;
  remote?: boolean;
  q?: string;
  page?: number;
  pageSize?: number;
}

@Injectable()
export class JobsService {
  constructor(
    private readonly dbs: DbService,
    @InjectQueue(QUEUE_GENERATE) private readonly generateQueue: Queue<GenerateJobData>,
  ) {}

  private get db() {
    return this.dbs.db;
  }

  async list(params: JobListParams) {
    const page = Math.max(1, params.page ?? 1);
    const pageSize = Math.min(100, params.pageSize ?? 25);

    const conditions = [
      params.source ? eq(job.sourceKey, params.source) : undefined,
      params.status ? eq(job.status, params.status) : ne(job.status, 'hidden'),
      params.remote !== undefined ? eq(job.remote, params.remote) : undefined,
      params.q
        ? or(
            ilike(job.title, `%${params.q}%`),
            ilike(job.company, `%${params.q}%`),
            ilike(job.description, `%${params.q}%`),
          )
        : undefined,
      params.minScore !== undefined ? gte(matchResult.score, params.minScore) : undefined,
    ].filter((c): c is NonNullable<typeof c> => c !== undefined);
    const where = and(...conditions);

    const orderBy =
      params.sort === 'date'
        ? [desc(job.ingestedAt)]
        : [sql`${matchResult.score} DESC NULLS LAST`, desc(job.ingestedAt)];

    const [items, [{ total }]] = await Promise.all([
      this.db
        .select({ job, matchResult })
        .from(job)
        .leftJoin(matchResult, eq(matchResult.jobId, job.id))
        .where(where)
        .orderBy(...orderBy)
        .offset((page - 1) * pageSize)
        .limit(pageSize)
        .then((rows) => rows.map((r) => ({ ...r.job, matchResult: r.matchResult }))),
      this.db
        .select({ total: count() })
        .from(job)
        .leftJoin(matchResult, eq(matchResult.jobId, job.id))
        .where(where),
    ]);
    return { items, total, page, pageSize };
  }

  async get(id: string) {
    const found = await this.db.query.job.findFirst({
      where: eq(job.id, id),
      with: {
        matchResult: true,
        documents: { orderBy: (d, { desc }) => [desc(d.createdAt)] },
        application: true,
      },
    });
    if (!found) throw new NotFoundException(`job ${id} not found`);
    return found;
  }

  async shortlist(id: string) {
    const found = await this.get(id);
    await this.db.update(job).set({ status: 'shortlisted' }).where(eq(job.id, id));
    await this.db
      .insert(application)
      .values({
        jobId: id,
        status: 'shortlisted',
        events: [{ status: 'shortlisted', at: new Date().toISOString() }],
      })
      .onConflictDoUpdate({ target: application.jobId, set: { status: 'shortlisted' } });
    return this.get(found.id);
  }

  async hide(id: string) {
    await this.get(id);
    const [updated] = await this.db.update(job).set({ status: 'hidden' }).where(eq(job.id, id)).returning();
    return updated;
  }

  /** Async generation: enqueue and return 202-style payload; dashboard polls documents. */
  async enqueueGeneration(id: string, type: 'resume' | 'cover_letter', profileId?: string) {
    await this.get(id);
    const bullJob = await this.generateQueue.add(
      'generate',
      { jobId: id, type, profileId },
      { removeOnComplete: 200, removeOnFail: 100, attempts: 1 },
    );
    return { queued: true, queueJobId: bullJob.id, type };
  }
}
