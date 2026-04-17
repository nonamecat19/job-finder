import { BullModule } from '@nestjs/bullmq';
import { Controller, Get, Module } from '@nestjs/common';
import { ConfigModule } from '@nestjs/config';
import { ScheduleModule } from '@nestjs/schedule';
import { redisConnection } from './common/queues';
import { DbModule } from './db/db.module';
import { ApplicationsModule } from './modules/applications/applications.module';
import { GenerationModule } from './modules/generation/generation.module';
import { IngestionModule } from './modules/ingestion/ingestion.module';
import { JobSourcesModule } from './modules/job-sources/job-sources.module';
import { JobsModule } from './modules/jobs/jobs.module';
import { LlmModule } from './modules/llm/llm.module';
import { MatchingModule } from './modules/matching/matching.module';
import { ProfileModule } from './modules/profile/profile.module';
import { ScrapingModule } from './modules/scraping/scraping.module';

@Controller('health')
class HealthController {
  @Get()
  health() {
    return { ok: true, uptime: process.uptime() };
  }
}

@Module({
  imports: [
    ConfigModule.forRoot({ isGlobal: true }),
    ScheduleModule.forRoot(),
    BullModule.forRoot({ connection: redisConnection() }),
    DbModule,
    LlmModule,
    ScrapingModule,
    ProfileModule,
    JobSourcesModule,
    IngestionModule,
    MatchingModule,
    GenerationModule,
    JobsModule,
    ApplicationsModule,
  ],
  controllers: [HealthController],
})
export class AppModule {}
