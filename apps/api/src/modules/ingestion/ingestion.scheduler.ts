import { Injectable, Logger } from '@nestjs/common';
import { Cron } from '@nestjs/schedule';
import * as cronParser from 'cron-parser';
import { eq } from 'drizzle-orm';
import { DbService } from '../../db/db.service';
import { savedSearch } from '../../db/schema';
import { IngestionService } from './ingestion.service';

// cron-parser v4 exports parseExpression, v5 exports CronExpressionParser.parse
const parseCron: (expr: string, opts?: { currentDate?: Date }) => { prev(): { toDate(): Date } } =
  (cronParser as any).CronExpressionParser?.parse?.bind((cronParser as any).CronExpressionParser) ??
  (cronParser as any).parseExpression;

@Injectable()
export class IngestionScheduler {
  private readonly logger = new Logger(IngestionScheduler.name);

  constructor(
    private readonly dbs: DbService,
    private readonly ingestion: IngestionService,
  ) {}

  /** Every 5 minutes: run any enabled saved search whose cron slot has passed since its last run. */
  @Cron('*/5 * * * *')
  async tick() {
    const searches = await this.dbs.db.select().from(savedSearch).where(eq(savedSearch.enabled, true));
    for (const search of searches) {
      try {
        const prev = parseCron(search.cron, { currentDate: new Date() }).prev().toDate();
        const due = !search.lastRunAt || search.lastRunAt < prev;
        if (due) {
          this.logger.log(`search '${search.name}' due (cron ${search.cron})`);
          await this.ingestion.runSearch(search.id);
        }
      } catch (e) {
        this.logger.error(`scheduler failed for search '${search.name}': ${(e as Error).message}`);
      }
    }
  }
}
