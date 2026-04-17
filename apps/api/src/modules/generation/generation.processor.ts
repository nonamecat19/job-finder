import { Processor, WorkerHost } from '@nestjs/bullmq';
import { Logger } from '@nestjs/common';
import { Job as BullJob } from 'bullmq';
import { GenerateJobData, QUEUE_GENERATE } from '../../common/queues';
import { GenerationService } from './generation.service';

@Processor(QUEUE_GENERATE, { concurrency: 1 })
export class GenerationProcessor extends WorkerHost {
  private readonly logger = new Logger(GenerationProcessor.name);

  constructor(private readonly generation: GenerationService) {
    super();
  }

  async process(bullJob: BullJob<GenerateJobData>) {
    const { jobId, type, profileId } = bullJob.data;
    try {
      const doc = await this.generation.generate(jobId, type, profileId);
      return { documentId: doc.id, version: doc.version };
    } catch (e) {
      this.logger.error(`generation ${type} for job ${jobId} failed: ${(e as Error).message}`);
      throw e;
    }
  }
}
