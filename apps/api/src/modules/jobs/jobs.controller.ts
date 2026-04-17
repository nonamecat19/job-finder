import { Body, Controller, Get, HttpCode, Param, Post, Query } from '@nestjs/common';
import { GenerateRequestDto } from '@job-finder/shared';
import { GenerationService } from '../generation/generation.service';
import { JobsService } from './jobs.service';

@Controller('jobs')
export class JobsController {
  constructor(
    private readonly jobs: JobsService,
    private readonly generation: GenerationService,
  ) {}

  @Get()
  list(
    @Query('sort') sort?: 'score' | 'date',
    @Query('source') source?: string,
    @Query('minScore') minScore?: string,
    @Query('status') status?: string,
    @Query('remote') remote?: string,
    @Query('q') q?: string,
    @Query('page') page?: string,
    @Query('pageSize') pageSize?: string,
  ) {
    return this.jobs.list({
      sort,
      source: source || undefined,
      minScore: minScore ? Number(minScore) : undefined,
      status: status || undefined,
      remote: remote === undefined || remote === '' ? undefined : remote === 'true',
      q: q || undefined,
      page: page ? Number(page) : undefined,
      pageSize: pageSize ? Number(pageSize) : undefined,
    });
  }

  @Get(':id')
  get(@Param('id') id: string) {
    return this.jobs.get(id);
  }

  @Post(':id/shortlist')
  shortlist(@Param('id') id: string) {
    return this.jobs.shortlist(id);
  }

  @Post(':id/hide')
  hide(@Param('id') id: string) {
    return this.jobs.hide(id);
  }

  @Post(':id/generate')
  @HttpCode(202)
  generate(@Param('id') id: string, @Body() body: GenerateRequestDto) {
    return this.jobs.enqueueGeneration(id, body.type, body.profileId);
  }

  @Get(':id/documents')
  documents(@Param('id') id: string) {
    return this.generation.listDocuments(id);
  }
}
