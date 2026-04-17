import { Body, Controller, Get, Param, Post, Put } from '@nestjs/common';
import { JobSourcesService } from './job-sources.service';

@Controller('sources')
export class JobSourcesController {
  constructor(private readonly sources: JobSourcesService) {}

  @Get()
  list() {
    return this.sources.list();
  }

  @Put(':key')
  update(
    @Param('key') key: string,
    @Body() body: { enabled?: boolean; config?: Record<string, unknown> },
  ) {
    return this.sources.update(key, body);
  }

  @Post(':key/test')
  test(@Param('key') key: string) {
    return this.sources.test(key);
  }
}
