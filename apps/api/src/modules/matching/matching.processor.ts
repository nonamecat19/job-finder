import { Processor, WorkerHost } from '@nestjs/bullmq';
import { Logger } from '@nestjs/common';
import { Job as BullJob } from 'bullmq';
import { MatchJobData, QUEUE_MATCH } from '../../common/queues';
import { MatchingService } from './matching.service';

// concurrency 1: local LLM handles one request at a time comfortably
@Processor(QUEUE_MATCH, { concurrency: 1 })
export class MatchingProcessor extends WorkerHost {
  private readonly logger = new Logger(MatchingProcessor.name);

  constructor(private readonly matching: MatchingService) {
    super();
  }

  async process(bullJob: BullJob<MatchJobData>) {
    const { jobId } = bullJob.data;
    try {
      const result = await this.matching.matchJob(jobId);
      return { score: result.score, similarity: result.similarity };
    } catch (e) {
      this.logger.error(`matching job ${jobId} failed: ${(e as Error).message}`);
      throw e;
    }
  }
}
