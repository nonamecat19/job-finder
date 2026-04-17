import { Module } from '@nestjs/common';
import { ScrapingModule } from '../scraping/scraping.module';
import { JOB_SOURCE_ADAPTERS, JobSourceAdapter } from './adapter.interface';
import { AdzunaAdapter } from './adapters/adzuna.adapter';
import { ArbeitnowAdapter } from './adapters/arbeitnow.adapter';
import { DjinniAdapter } from './adapters/djinni.adapter';
import { DouAdapter } from './adapters/dou.adapter';
import { JobSpyAdapter } from './adapters/jobspy.adapter';
import { RemotiveAdapter } from './adapters/remotive.adapter';
import { JobSourceRegistry } from './job-source.registry';
import { JobSourcesController } from './job-sources.controller';
import { JobSourcesService } from './job-sources.service';

// Adding a job site = add its adapter class here. Nothing downstream changes.
const ADAPTERS = [
  AdzunaAdapter,
  RemotiveAdapter,
  ArbeitnowAdapter,
  DjinniAdapter,
  DouAdapter,
  JobSpyAdapter,
];

@Module({
  imports: [ScrapingModule],
  controllers: [JobSourcesController],
  providers: [
    ...ADAPTERS,
    {
      provide: JOB_SOURCE_ADAPTERS,
      useFactory: (...adapters: JobSourceAdapter[]) => adapters,
      inject: ADAPTERS,
    },
    JobSourceRegistry,
    JobSourcesService,
  ],
  exports: [JobSourceRegistry, JobSourcesService],
})
export class JobSourcesModule {}
