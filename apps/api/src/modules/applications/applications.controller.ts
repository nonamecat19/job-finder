import { Body, Controller, Get, Param, Patch, Query } from '@nestjs/common';
import { ApplicationStatus } from '@job-finder/shared';
import { ApplicationsService } from './applications.service';

@Controller()
export class ApplicationsController {
  constructor(private readonly applications: ApplicationsService) {}

  @Get('applications')
  list(@Query('status') status?: string) {
    return this.applications.list(status || undefined);
  }

  @Patch('applications/:id')
  update(
    @Param('id') id: string,
    @Body() body: { status?: ApplicationStatus; notes?: string | null },
  ) {
    return this.applications.update(id, body);
  }

  @Get('stats')
  stats() {
    return this.applications.stats();
  }
}
