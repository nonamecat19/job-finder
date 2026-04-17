import { BullModule } from '@nestjs/bullmq';
import { Module } from '@nestjs/common';
import { QUEUE_INGEST, QUEUE_MATCH } from '../../common/queues';
import { JobSourcesModule } from '../job-sources/job-sources.module';
import { IngestionProcessor } from './ingestion.processor';
import { IngestionScheduler } from './ingestion.scheduler';
import { IngestionService } from './ingestion.service';
import { SearchesController } from './searches.controller';

@Module({
  imports: [
    BullModule.registerQueue({ name: QUEUE_INGEST }, { name: QUEUE_MATCH }),
    JobSourcesModule,
  ],
  controllers: [SearchesController],
  providers: [IngestionService, IngestionProcessor, IngestionScheduler],
  exports: [IngestionService],
})
export class IngestionModule {}
