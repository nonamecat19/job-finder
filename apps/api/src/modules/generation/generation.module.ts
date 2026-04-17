import { BullModule } from '@nestjs/bullmq';
import { Module } from '@nestjs/common';
import { QUEUE_GENERATE } from '../../common/queues';
import { ProfileModule } from '../profile/profile.module';
import { ScrapingModule } from '../scraping/scraping.module';
import { DocumentsController } from './documents.controller';
import { GenerationProcessor } from './generation.processor';
import { GenerationService } from './generation.service';
import { HtmlPdfRenderer, PDF_RENDERER } from './pdf-renderer';
import { RenderCvRenderer } from './rendercv-renderer';

@Module({
  imports: [BullModule.registerQueue({ name: QUEUE_GENERATE }), ProfileModule, ScrapingModule],
  controllers: [DocumentsController],
  providers: [
    HtmlPdfRenderer,
    RenderCvRenderer,
    { provide: PDF_RENDERER, useExisting: HtmlPdfRenderer },
    GenerationService,
    GenerationProcessor,
  ],
  exports: [GenerationService],
})
export class GenerationModule {}
