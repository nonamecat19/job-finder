import { BadRequestException, Injectable, NotFoundException } from '@nestjs/common';
import { APPLICATION_STATUSES, ApplicationStatus } from '@job-finder/shared';
import { asc, count, desc, eq, gte, ne } from 'drizzle-orm';
import { DbService } from '../../db/db.service';
import { application, job, jobSource, matchResult, sourceRun } from '../../db/schema';

@Injectable()
export class ApplicationsService {
  constructor(private readonly dbs: DbService) {}

  private get db() {
    return this.dbs.db;
  }

  list(status?: string) {
    return this.db.query.application.findMany({
      where: status ? eq(application.status, status) : undefined,
      with: { job: { with: { matchResult: true } } },
      orderBy: (a, { desc }) => [desc(a.updatedAt)],
    });
  }

  async update(id: string, data: { status?: ApplicationStatus; notes?: string | null }) {
    const [app] = await this.db.select().from(application).where(eq(application.id, id));
    if (!app) throw new NotFoundException(`application ${id} not found`);

    if (data.status && !APPLICATION_STATUSES.includes(data.status)) {
      throw new BadRequestException(`invalid status '${data.status}'`);
    }

    const events = (app.events as { status: string; at: string }[]) ?? [];
    if (data.status && data.status !== app.status) {
      events.push({ status: data.status, at: new Date().toISOString() });
    }

    const [updated] = await this.db
      .update(application)
      .set({
        ...(data.status ? { status: data.status, events } : {}),
        ...(data.notes !== undefined ? { notes: data.notes } : {}),
        ...(data.status === 'applied' && !app.appliedAt ? { appliedAt: new Date() } : {}),
      })
      .where(eq(application.id, id))
      .returning();
    if (data.status) {
      await this.db.update(job).set({ status: data.status }).where(eq(job.id, app.jobId));
    }
    return { ...updated, job: (await this.db.select().from(job).where(eq(job.id, app.jobId)))[0] };
  }

  async stats() {
    const [[{ jobsTotal }], [{ jobsLast24h }], [{ highFit }], runs] = await Promise.all([
      this.db.select({ jobsTotal: count() }).from(job).where(ne(job.status, 'hidden')),
      this.db
        .select({ jobsLast24h: count() })
        .from(job)
        .where(gte(job.ingestedAt, new Date(Date.now() - 24 * 3600 * 1000))),
      this.db.select({ highFit: count() }).from(matchResult).where(gte(matchResult.score, 70)),
      this.db
        .select({
          id: sourceRun.id,
          sourceKey: jobSource.key,
          searchId: sourceRun.searchId,
          startedAt: sourceRun.startedAt,
          finishedAt: sourceRun.finishedAt,
          ok: sourceRun.ok,
          found: sourceRun.found,
          new: sourceRun.new,
          error: sourceRun.error,
        })
        .from(sourceRun)
        .innerJoin(jobSource, eq(sourceRun.sourceId, jobSource.id))
        .orderBy(desc(sourceRun.startedAt))
        .limit(10),
    ]);
    const pipelineRaw = await this.db
      .select({ status: application.status, count: count() })
      .from(application)
      .groupBy(application.status)
      .orderBy(asc(application.status));

    const pipeline: Record<string, number> = {};
    for (const row of pipelineRaw) pipeline[row.status] = row.count;

    return {
      jobsTotal,
      jobsLast24h,
      highFit,
      pipeline,
      recentRuns: runs,
    };
  }
}
