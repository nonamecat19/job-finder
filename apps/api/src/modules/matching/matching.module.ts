import { BullModule } from '@nestjs/bullmq';
import { Module } from '@nestjs/common';
import { QUEUE_MATCH } from '../../common/queues';
import { ProfileModule } from '../profile/profile.module';
import { MatchingProcessor } from './matching.processor';
import { MatchingService } from './matching.service';

@Module({
  imports: [BullModule.registerQueue({ name: QUEUE_MATCH }), ProfileModule],
  providers: [MatchingService, MatchingProcessor],
  exports: [MatchingService],
})
export class MatchingModule {}
