import { Inject, Injectable, Logger, NotFoundException } from '@nestjs/common';
import { cosineDistance, eq, isNotNull, sql } from 'drizzle-orm';
import { z } from 'zod';
import { JsonResume } from '@job-finder/shared';
import { DbService } from '../../db/db.service';
import { job, matchResult, profile } from '../../db/schema';
import { LLM_PROVIDER, LlmProvider } from '../llm/llm.types';
import { ProfileService } from '../profile/profile.service';

const fitSchema = z.object({
  score: z.number().min(0).max(100),
  matchedSkills: z.array(z.string()),
  missingSkills: z.array(z.string()),
  summary: z.string(),
  redFlags: z.array(z.string()),
});

@Injectable()
export class MatchingService {
  private readonly logger = new Logger(MatchingService.name);

  constructor(
    private readonly dbs: DbService,
    private readonly profiles: ProfileService,
    @Inject(LLM_PROVIDER) private readonly llm: LlmProvider,
  ) {}

  private get db() {
    return this.dbs.db;
  }

  private get threshold(): number {
    return Number(process.env.MATCH_SIMILARITY_THRESHOLD ?? 0.35);
  }

  /** Stage 1 (embedding prefilter) + stage 2 (LLM analysis) for one job. */
  async matchJob(jobId: string) {
    const [theJob] = await this.db.select().from(job).where(eq(job.id, jobId));
    if (!theJob) throw new NotFoundException(`job ${jobId} not found`);
    const prof = await this.profiles.getDefault();

    // ensure job embedding
    const jobText = `${theJob.title} at ${theJob.company}\n${theJob.description}`.slice(0, 8000);
    const jobEmbedding = await this.llm.embed(jobText);
    await this.db.update(job).set({ embedding: jobEmbedding }).where(eq(job.id, jobId));

    // ensure profile embedding exists
    const [profHas] = await this.db
      .select({ has: isNotNull(profile.embedding) })
      .from(profile)
      .where(eq(profile.id, prof.id));
    if (!profHas?.has) {
      await this.profiles.refreshEmbedding(prof.id);
    }

    const [sim] = await this.db
      .select({
        similarity: sql<number>`1 - (${cosineDistance(profile.embedding, jobEmbedding)})`,
      })
      .from(profile)
      .where(eq(profile.id, prof.id));
    const similarity = Number(sim?.similarity ?? 0);

    if (similarity < this.threshold) {
      this.logger.log(`job ${jobId} prefiltered out (similarity ${similarity.toFixed(3)})`);
      return this.saveResult(jobId, { similarity, score: null });
    }

    const doc = prof.document as JsonResume;
    const profileText = this.profiles.profileToText(doc, prof.extraNotes);
    const fit = await this.llm.completeStructured(
      `Rate how well this candidate fits this job.\n\n` +
        `CANDIDATE PROFILE:\n${profileText.slice(0, 6000)}\n\n` +
        `JOB POSTING:\nTitle: ${theJob.title}\nCompany: ${theJob.company}\n` +
        `Location: ${theJob.location ?? 'n/a'} (remote: ${theJob.remote})\n` +
        `Description:\n${theJob.description.slice(0, 6000)}\n\n` +
        `Scoring guide: 90-100 near-perfect fit; 70-89 strong fit, minor gaps; ` +
        `50-69 partial fit, notable gaps; below 50 poor fit. ` +
        `matchedSkills/missingSkills = concrete skills from the job description. ` +
        `redFlags = concerns like seniority mismatch, hard requirements the candidate lacks, ` +
        `suspicious posting. summary = 2-3 sentences.`,
      fitSchema,
      { system: 'You are a precise technical recruiter. Judge only from the given profile and job text.' },
    );

    return this.saveResult(jobId, {
      similarity,
      score: Math.round(fit.score),
      matchedSkills: fit.matchedSkills,
      missingSkills: fit.missingSkills,
      summary: fit.summary,
      redFlags: fit.redFlags,
    });
  }

  private saveResult(
    jobId: string,
    data: {
      similarity: number;
      score: number | null;
      matchedSkills?: string[];
      missingSkills?: string[];
      summary?: string;
      redFlags?: string[];
    },
  ) {
    const payload = {
      similarity: data.similarity,
      score: data.score,
      matchedSkills: data.matchedSkills ?? undefined,
      missingSkills: data.missingSkills ?? undefined,
      summary: data.summary ?? null,
      redFlags: data.redFlags ?? undefined,
      model: this.llm.modelName,
    };
    return this.db
      .insert(matchResult)
      .values({ jobId, ...payload })
      .onConflictDoUpdate({ target: matchResult.jobId, set: payload })
      .returning()
      .then(([row]) => row);
  }
}
