import {
  BadRequestException,
  Body,
  Controller,
  Delete,
  Get,
  Param,
  Post,
  Put,
} from '@nestjs/common';
import { asc, desc, eq } from 'drizzle-orm';
import { SearchQuery } from '@job-finder/shared';
import { DbService } from '../../db/db.service';
import { jobSource, savedSearch, sourceRun } from '../../db/schema';
import { IngestionService } from './ingestion.service';

@Controller('searches')
export class SearchesController {
  constructor(
    private readonly dbs: DbService,
    private readonly ingestion: IngestionService,
  ) {}

  private get db() {
    return this.dbs.db;
  }

  @Get()
  list() {
    return this.db.select().from(savedSearch).orderBy(asc(savedSearch.createdAt));
  }

  @Post()
  create(@Body() body: { name: string; query: SearchQuery; cron?: string; enabled?: boolean }) {
    if (!body.name || !body.query?.keywords) {
      throw new BadRequestException('name and query.keywords are required');
    }
    return this.db
      .insert(savedSearch)
      .values({
        name: body.name,
        query: body.query as object,
        ...(body.cron ? { cron: body.cron } : {}),
        enabled: body.enabled ?? true,
      })
      .returning()
      .then(([created]) => created);
  }

  @Put(':id')
  update(
    @Param('id') id: string,
    @Body() body: { name?: string; query?: SearchQuery; cron?: string; enabled?: boolean },
  ) {
    return this.db
      .update(savedSearch)
      .set({
        ...(body.name !== undefined ? { name: body.name } : {}),
        ...(body.query !== undefined ? { query: body.query as object } : {}),
        ...(body.cron !== undefined ? { cron: body.cron } : {}),
        ...(body.enabled !== undefined ? { enabled: body.enabled } : {}),
      })
      .where(eq(savedSearch.id, id))
      .returning()
      .then(([updated]) => updated);
  }

  @Delete(':id')
  async remove(@Param('id') id: string) {
    await this.db.delete(savedSearch).where(eq(savedSearch.id, id));
    return { deleted: true };
  }

  @Post(':id/run')
  run(@Param('id') id: string) {
    return this.ingestion.runSearch(id);
  }

  @Get('runs/recent')
  async recentRuns() {
    const runs = await this.db
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
      .limit(50);
    return runs;
  }
}
