import { Inject, Injectable, Logger, NotFoundException } from '@nestjs/common';
import { JsonResume } from '@job-finder/shared';
import { and, desc, eq, max } from 'drizzle-orm';
import { readFile } from 'fs/promises';
import * as path from 'path';
import { parse as yamlParse } from 'yaml';
import { DbService } from '../../db/db.service';
import { application, generatedDocument, job } from '../../db/schema';
import { LLM_PROVIDER, LlmProvider } from '../llm/llm.types';
import { ProfileService } from '../profile/profile.service';
import { jsonResumeSchema } from '../profile/profile.schemas';
import { z } from 'zod';
import { verifyGrounding } from './grounding';
import { HtmlPdfRenderer } from './pdf-renderer';
import { RenderCvRenderer } from './rendercv-renderer';
import {
  GroundingLevel,
  RendercvMaster,
  buildTailorPrompt,
  mergeTailored,
  tailoredSectionsSchema,
} from './rendercv-tailor';
import { verifyRendercvGrounding } from './rendercv-grounding';

const coverLetterSchema = z.object({
  letter: z.string(),
});

const GROUNDING_ATTEMPTS = 2;

@Injectable()
export class GenerationService {
  private readonly logger = new Logger(GenerationService.name);

  constructor(
    private readonly dbs: DbService,
    private readonly profiles: ProfileService,
    private readonly renderer: HtmlPdfRenderer,
    private readonly rendercv: RenderCvRenderer,
    @Inject(LLM_PROVIDER) private readonly llm: LlmProvider,
  ) {}

  private get db() {
    return this.dbs.db;
  }

  private get masterPath(): string {
    return process.env.RESUME_MASTER_PATH ?? path.resolve(process.cwd(), '../../resume/resume.yaml');
  }

  private get defaultGroundingLevel(): GroundingLevel {
    const lvl = process.env.RESUME_GROUNDING_LEVEL;
    return lvl === 'strict' || lvl === 'aggressive' ? lvl : 'moderate';
  }

  /**
   * Ad-hoc rendercv path: takes a pasted vacancy, tailors the RenderCV master and renders
   * a PDF — no DB Job required. This is the entry point opencode / paste-text drives.
   * Returns file paths (not a persisted GeneratedDocument, whose jobId is mandatory).
   */
  async generateRendercvFromText(input: {
    vacancy: string;
    company?: string;
    title?: string;
    groundingLevel?: GroundingLevel;
  }): Promise<{ yamlPath: string; pdfPath: string; groundingLevel: GroundingLevel }> {
    const level = input.groundingLevel ?? this.defaultGroundingLevel;
    const master = yamlParse(await readFile(this.masterPath, 'utf8')) as RendercvMaster;
    const merged = await this.tailorRendercvResume(master, input.vacancy, level);

    const stem = `${input.company ?? 'vacancy'}-${input.title ?? 'resume'}`
      .replace(/[^a-z0-9]+/gi, '_')
      .slice(0, 60)
      .toLowerCase();
    const { yamlPath, pdfPath } = await this.rendercv.render(merged, `${stem}-${Date.now()}`);
    this.logger.log(`rendered tailored resume (${level}) → ${pdfPath}`);
    return { yamlPath, pdfPath, groundingLevel: level };
  }

  /** LLM tailors editable sections, merge is deterministic, grounding is re-checked with retry. */
  private async tailorRendercvResume(
    master: RendercvMaster,
    vacancy: string,
    level: GroundingLevel,
  ): Promise<RendercvMaster> {
    let lastViolations: string[] = [];
    for (let attempt = 0; attempt < GROUNDING_ATTEMPTS; attempt++) {
      const suffix = lastViolations.length
        ? `\n\nYour previous attempt violated grounding rules:\n- ${lastViolations.join('\n- ')}\n` +
          `Regenerate without these violations.`
        : '';
      const payload = await this.llm.completeStructured(
        buildTailorPrompt(master, vacancy, level) + suffix,
        tailoredSectionsSchema,
        { system: 'You are an expert resume writer who never fabricates information.' },
      );
      const merged = mergeTailored(master, payload);
      lastViolations = verifyRendercvGrounding(master, merged, level);
      if (lastViolations.length === 0) return merged;
      this.logger.warn(`rendercv grounding violations (attempt ${attempt + 1}): ${lastViolations.join('; ')}`);
    }
    throw new Error(`tailored rendercv resume failed grounding check: ${lastViolations.join('; ')}`);
  }

  async generate(jobId: string, type: 'resume' | 'cover_letter', profileId?: string) {
    const theJob = await this.db.query.job.findFirst({
      where: eq(job.id, jobId),
      with: { matchResult: true },
    });
    if (!theJob) throw new NotFoundException(`job ${jobId} not found`);
    const profile = profileId ? await this.profiles.get(profileId) : await this.profiles.getDefault();
    const master = profile.document as JsonResume;

    const [{ maxVersion }] = await this.db
      .select({ maxVersion: max(generatedDocument.version) })
      .from(generatedDocument)
      .where(and(eq(generatedDocument.jobId, jobId), eq(generatedDocument.type, type)));
    const version = (maxVersion ?? 0) + 1;

    let content: object;
    let pdfPath: string;
    const baseName = `${theJob.company}-${theJob.title}`.replace(/[^a-z0-9]+/gi, '_').slice(0, 60);

    if (type === 'resume') {
      const tailored = await this.tailorResume(master, profile.extraNotes, theJob);
      content = tailored as object;
      pdfPath = await this.renderer.renderResume(tailored, `${baseName}-resume-v${version}.pdf`);
    } else {
      const letter = await this.writeCoverLetter(master, profile.extraNotes, theJob);
      content = { text: letter };
      pdfPath = await this.renderer.renderCoverLetter(
        letter,
        { name: master.basics?.name, company: theJob.company, title: theJob.title },
        `${baseName}-cover-v${version}.pdf`,
      );
    }

    const [doc] = await this.db
      .insert(generatedDocument)
      .values({ jobId, type, version, content, pdfPath, model: this.llm.modelName })
      .returning();

    // job reached docs_generated stage (only move forward from found/shortlisted)
    if (['found', 'shortlisted'].includes(theJob.status)) {
      await this.db.update(job).set({ status: 'docs_generated' }).where(eq(job.id, jobId));
      await this.db
        .insert(application)
        .values({
          jobId,
          status: 'docs_generated',
          events: [{ status: 'docs_generated', at: new Date().toISOString() }],
        })
        .onConflictDoUpdate({ target: application.jobId, set: { status: 'docs_generated' } });
    }
    return doc;
  }

  private async tailorResume(master: JsonResume, extraNotes: string | null, job: { title: string; company: string; description: string; matchResult: { matchedSkills: unknown; missingSkills: unknown } | null }): Promise<JsonResume> {
    const prompt =
      `Create a tailored resume for this job application by selecting, reordering and rephrasing ` +
      `content from the candidate's master profile.\n\n` +
      `STRICT RULES:\n` +
      `- Use ONLY employers, roles, projects, education, dates and facts present in the master profile.\n` +
      `- Never invent experience, employers, dates, degrees, metrics or technologies.\n` +
      `- You may drop irrelevant entries, reorder, and rephrase highlights to emphasize what the job asks for.\n` +
      `- Copy employer names, institution names, project names and all dates EXACTLY as written in the master profile.\n` +
      `- Keep basics (name, email, phone, url, location) exactly as in the master profile.\n\n` +
      `MASTER PROFILE (JSON Resume):\n${JSON.stringify(master).slice(0, 12000)}\n\n` +
      (extraNotes ? `EXTRA CANDIDATE NOTES:\n${extraNotes.slice(0, 2000)}\n\n` : '') +
      `TARGET JOB:\nTitle: ${job.title}\nCompany: ${job.company}\n` +
      (job.matchResult?.matchedSkills ? `Matched skills: ${JSON.stringify(job.matchResult.matchedSkills)}\n` : '') +
      `Description:\n${job.description.slice(0, 6000)}`;

    let lastViolations: string[] = [];
    for (let attempt = 0; attempt < GROUNDING_ATTEMPTS; attempt++) {
      const suffix = lastViolations.length
        ? `\n\nYour previous attempt violated grounding rules:\n- ${lastViolations.join('\n- ')}\nRegenerate without these violations.`
        : '';
      const tailored = await this.llm.completeStructured(prompt + suffix, jsonResumeSchema, {
        system: 'You are an expert resume writer who never fabricates information.',
      });
      lastViolations = verifyGrounding(master, tailored as JsonResume);
      if (lastViolations.length === 0) return tailored as JsonResume;
      this.logger.warn(`grounding violations (attempt ${attempt + 1}): ${lastViolations.join('; ')}`);
    }
    throw new Error(`tailored resume failed grounding check: ${lastViolations.join('; ')}`);
  }

  private async writeCoverLetter(master: JsonResume, extraNotes: string | null, job: { title: string; company: string; description: string }): Promise<string> {
    const { letter } = await this.llm.completeStructured(
      `Write a short cover letter (maximum 150 words, exactly 3 paragraphs separated by blank lines) ` +
        `for this application.\n\n` +
        `Structure: (1) hook referencing the company and role, (2) 2-3 concrete matching experiences ` +
        `from the candidate's real background, (3) brief close.\n` +
        `STRICT RULES: mention only experience present in the profile below; no invented facts, ` +
        `no clichés like "I am writing to express". Plain text, no salutation placeholders like [Hiring Manager] — ` +
        `use "Hello," if needed.\n\n` +
        `CANDIDATE PROFILE:\n${JSON.stringify(master).slice(0, 8000)}\n\n` +
        (extraNotes ? `EXTRA NOTES:\n${extraNotes.slice(0, 1500)}\n\n` : '') +
        `JOB:\nTitle: ${job.title}\nCompany: ${job.company}\nDescription:\n${job.description.slice(0, 4000)}`,
      coverLetterSchema,
      { system: 'You write concise, concrete, honest cover letters.' },
    );
    return letter.trim();
  }

  async listDocuments(jobId: string) {
    return this.db
      .select()
      .from(generatedDocument)
      .where(eq(generatedDocument.jobId, jobId))
      .orderBy(generatedDocument.type, desc(generatedDocument.version));
  }

  async getDocument(id: string) {
    const [doc] = await this.db.select().from(generatedDocument).where(eq(generatedDocument.id, id));
    if (!doc) throw new NotFoundException(`document ${id} not found`);
    return doc;
  }

  /** Edit cover letter text, then re-render its PDF in place (same version). */
  async updateDocument(id: string, body: { text?: string }) {
    const doc = await this.getDocument(id);
    if (doc.type !== 'cover_letter' || !body.text) {
      throw new NotFoundException('only cover_letter text is editable');
    }
    const [theJob] = await this.db.select().from(job).where(eq(job.id, doc.jobId));
    if (!theJob) throw new NotFoundException(`job ${doc.jobId} not found`);
    const profile = await this.profiles.getDefault();
    const master = profile.document as JsonResume;
    const baseName = `${theJob.company}-${theJob.title}`.replace(/[^a-z0-9]+/gi, '_').slice(0, 60);
    const pdfPath = await this.renderer.renderCoverLetter(
      body.text,
      { name: master.basics?.name, company: theJob.company, title: theJob.title },
      `${baseName}-cover-v${doc.version}.pdf`,
    );
    const [updated] = await this.db
      .update(generatedDocument)
      .set({ content: { text: body.text }, pdfPath })
      .where(eq(generatedDocument.id, id))
      .returning();
    return updated;
  }
}
