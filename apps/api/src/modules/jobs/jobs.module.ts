import { BullModule } from '@nestjs/bullmq';
import { Module } from '@nestjs/common';
import { QUEUE_GENERATE } from '../../common/queues';
import { GenerationModule } from '../generation/generation.module';
import { JobsController } from './jobs.controller';
import { JobsService } from './jobs.service';

@Module({
  imports: [BullModule.registerQueue({ name: QUEUE_GENERATE }), GenerationModule],
  controllers: [JobsController],
  providers: [JobsService],
})
export class JobsModule {}
