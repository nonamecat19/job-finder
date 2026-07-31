// ---------------------------------------------------------------------------
// Enums
// ---------------------------------------------------------------------------

export const SOURCE_KINDS = ['api', 'scrape', 'sidecar'] as const;
export type SourceKind = (typeof SOURCE_KINDS)[number];

export const APPLICATION_STATUSES = [
  'found',
  'shortlisted',
  'docs_generated',
  'applied',
  'interview',
  'offer',
  'rejected',
] as const;
export type ApplicationStatus = (typeof APPLICATION_STATUSES)[number];

export const DOCUMENT_TYPES = ['resume', 'cover_letter'] as const;
export type DocumentType = (typeof DOCUMENT_TYPES)[number];

// ---------------------------------------------------------------------------
// Job ingestion
// ---------------------------------------------------------------------------

/** Canonical job shape every source adapter must produce. */
export interface NormalizedJob {
  sourceKey: string;
  externalId?: string;
  title: string;
  company: string;
  location?: string;
  remote: boolean;
  salaryRaw?: string;
  url: string;
  description: string;
  postedAt?: string; // ISO date
  raw: unknown;
  experienceLevel?: string;
  experienceMinYears?: number;
  englishLevel?: string;
  salaryEstimateRaw?: string;
  salaryEstimateMin?: number;
  salaryEstimateMax?: number;
  salaryEstimateCurrency?: string;
}

/** Query shape a saved search stores and adapters receive. */
export interface SearchQuery {
  keywords: string;
  location?: string;
  remote?: boolean;
  salaryMin?: number;
  country?: string; // ISO country code for country-scoped APIs (Adzuna)
  /** adapter keys to run; empty/undefined = all enabled sources */
  sources?: string[];
}

// ---------------------------------------------------------------------------
// JSON Resume (subset of the standard schema we use)
// ---------------------------------------------------------------------------

export interface ResumeBasics {
  name?: string;
  label?: string;
  email?: string;
  phone?: string;
  url?: string;
  summary?: string;
  location?: { city?: string; countryCode?: string; region?: string };
  profiles?: { network?: string; username?: string; url?: string }[];
}

export interface ResumeWork {
  name: string; // employer
  position?: string;
  url?: string;
  startDate?: string;
  endDate?: string;
  summary?: string;
  highlights?: string[];
}

export interface ResumeEducation {
  institution: string;
  area?: string;
  studyType?: string;
  startDate?: string;
  endDate?: string;
}

export interface ResumeSkill {
  name: string;
  level?: string;
  keywords?: string[];
}

export interface ResumeProject {
  name: string;
  description?: string;
  url?: string;
  keywords?: string[];
  highlights?: string[];
}

export interface JsonResume {
  basics?: ResumeBasics;
  work?: ResumeWork[];
  education?: ResumeEducation[];
  skills?: ResumeSkill[];
  projects?: ResumeProject[];
  languages?: { language?: string; fluency?: string }[];
  certificates?: { name?: string; issuer?: string; date?: string }[];
}

// ---------------------------------------------------------------------------
// API DTOs
// ---------------------------------------------------------------------------

export interface MatchResultDto {
  id: string;
  jobId: string;
  similarity: number;
  score: number | null;
  matchedSkills: string[] | null;
  missingSkills: string[] | null;
  summary: string | null;
  redFlags: string[] | null;
  model: string;
  createdAt: string;
}

export interface JobDto {
  id: string;
  dedupeKey: string;
  sourceKey: string;
  subscriptionId: string | null;
  title: string;
  company: string;
  location: string | null;
  remote: boolean;
  salaryRaw: string | null;
  url: string;
  description: string;
  descriptionHtml?: string | null;
  postedAt: string | null;
  ingestedAt: string;
  status: ApplicationStatus | 'hidden';
  matchResult?: MatchResultDto | null;
  // Salary inference (spec 006). All five are null together when no source
  // could produce a band (FR-009) — salaryRaw is preserved and displayed
  // alongside regardless (FR-024).
  salaryMin: number | null;
  salaryMax: number | null;
  salaryCurrency: string | null;
  salaryConfidence: number | null;
  salarySource: string | null;
  // Computed against SALARY_FLOOR_USD; true only for a USD band entirely
  // below the floor (FR-016, FR-020 fails open for other currencies).
  salaryBelowFloor: boolean;
  /**
   * GhostSignal is the ghost-job detector's (005) result, when one exists.
   * A job with no ghost result renders exactly as it does today — this
   * field is simply absent, never a zero-valued panel (FR-017, SC-008).
   */
  ghostSignal?: JobSignalDto | null;
  // Djinni scraping enhancement (spec 022)
  experienceLevel?: string | null;
  experienceMinYears?: number | null;
  englishLevel?: string | null;
  salaryEstimateRaw?: string | null;
  salaryEstimateMin?: number | null;
  salaryEstimateMax?: number | null;
  salaryEstimateCurrency?: string | null;
  salaryIsEstimated: boolean;
}

export interface JobListResponse {
  items: JobDto[];
  total: number;
  page: number;
  pageSize: number;
}

export interface GeneratedDocumentDto {
  id: string;
  jobId: string | null;
  type: DocumentType;
  version: number;
  content: unknown;
  pdfPath: string | null;
  model: string;
  company?: string | null;
  title?: string | null;
  vacancy?: string | null;
  createdAt: string;
}

export interface RendercvSummaryExperience {
  company: string;
  highlightCount: number;
}

export interface RendercvSummary {
  name: string;
  headline: string;
  skillGroups: string[];
  experience: RendercvSummaryExperience[];
}

export interface ProfileDto {
  id: string;
  name: string;
  hasConfig: boolean;
  rendercvConfig?: RendercvSummary | null;
  rendercvFull?: Record<string, unknown> | null;
  extraNotes: string | null;
  updatedAt: string;
}

// ---------------------------------------------------------------------------
// Structured, fully-editable Resume (spec 009-editable-resume-profile)
// ---------------------------------------------------------------------------

/** One of the 9 canonical RenderCV entry types. A Section holds entries of
 * exactly one EntryType (see specs/009-editable-resume-profile/data-model.md). */
export const ENTRY_TYPES = [
  'education',
  'experience',
  'normal',
  'publication',
  'one_line',
  'bullet',
  'numbered',
  'reversed_numbered',
  'text',
] as const;
export type EntryType = (typeof ENTRY_TYPES)[number];

export interface SocialNetwork {
  network: string;
  username: string;
}

/** A network not in RenderCV's built-in social_networks list (e.g. Telegram
 * in some configs) — display label, link, and fontawesome icon name. */
export interface CustomConnection {
  placeholder: string;
  url: string;
  icon?: string;
}

/** Which fields are meaningful depends on the parent Section's entryType. */
export interface Entry {
  // education
  institution?: string;
  area?: string;
  degree?: string;
  // experience
  company?: string;
  position?: string;
  // normal (projects)
  name?: string;
  // publication
  title?: string;
  authors?: string[];
  doi?: string;
  journal?: string;
  // publication, normal
  url?: string;
  // one_line (skills)
  label?: string;
  details?: string;
  // bullet
  bullet?: string;
  // numbered
  number?: string;
  // reversed_numbered — value is a talk title/description, not a number
  // (RenderCV naming quirk, preserved as-is for round-trip fidelity).
  reversedNumber?: string;
  // text
  text?: string;
  // education, experience, normal, publication
  date?: string;
  // education, experience, normal
  startDate?: string;
  endDate?: string;
  location?: string;
  // education, experience, normal, publication
  summary?: string;
  // education, experience, normal
  highlights?: string[];
  // Any fields present in imported data that don't map to a field above
  // (FR-009) — never silently dropped.
  unrecognized?: Record<string, unknown>;
}

export interface Section {
  name: string;
  entryType: EntryType;
  entries: Entry[];
}

/** Top-level structured, editable view of a Profile's resume content. */
export interface Resume {
  name: string;
  headline?: string;
  location?: string;
  email?: string;
  phone?: string;
  website?: string;
  photo?: string;
  socialNetworks?: SocialNetwork[];
  customConnections?: CustomConnection[];
  sections: Section[];
  unrecognized?: Record<string, unknown>;
}

export interface ResumeDto {
  resume: Resume;
}

export interface EmployerBoardDto {
  id: string;
  vendor: string;
  employerIdentifier: string;
  displayName: string;
  addedVia: string;
  enabled: boolean;
  lastSuccessAt?: string;
  lastPostingCount: number;
  stale: boolean;
}

export interface BoardCandidateDto {
  id: string;
  vendor: string;
  employerIdentifier: string;
  displayName: string;
  inferredFromJobId?: string;
  state: string;
}

export interface JobSourceDto {
  id: string;
  key: string;
  kind: SourceKind;
  enabled: boolean;
  healthy: boolean;
  /** decrypted config with secret values masked */
  config: Record<string, unknown>;
}

export interface SavedSearchDto {
  id: string;
  name: string;
  query: SearchQuery;
  cron: string;
  enabled: boolean;
  lastRunAt: string | null;
}

export interface SubscriptionDto {
  id: string;
  sourceKey: string;
  name: string | null;
  url: string;
  enabled: boolean;
  /** Cron the ingestion scheduler scrapes this subscription on. */
  cron: string;
  lastRunAt: string | null;
}

export interface SubscriptionInput {
  sourceKey: string;
  url: string;
  name?: string;
  enabled?: boolean;
  /** Omitted means the server default: every six hours. */
  cron?: string;
}

export interface SourceRunDto {
  id: string;
  sourceKey: string;
  searchId: string | null;
  startedAt: string;
  finishedAt: string | null;
  ok: boolean | null;
  found: number;
  new: number;
  error: string | null;
  verdict: string | null;
  blockedCount: number;
  blockReason: string | null;
}

export interface HostPacingDto {
  requestsPerSecond: number;
  intervalSeconds: number;
  source: string;
}

export interface HostRetrievalStatusDto {
  host: string;
  identityVersion: string;
  currentRung: string;
  lastBlockAt: string | null;
  lastBlockReason: string | null;
  coolingOffUntil: string | null;
  crawlDelaySeconds?: number;
  pacing: HostPacingDto;
}

export interface RunVerdictDto {
  verdict: string;
  blockedCount: number;
  blockReason: string | null;
}

export interface ApplicationDto {
  id: string;
  jobId: string;
  status: ApplicationStatus;
  notes: string | null;
  appliedAt: string | null;
  events: { status: string; at: string }[];
  updatedAt: string;
  job?: JobDto;
}

export interface StatsDto {
  jobsTotal: number;
  jobsLast24h: number;
  highFit: number; // score >= 70
  pipeline: Record<string, number>;
  recentRuns: SourceRunDto[];
}

export interface GenerateRequestDto {
  type: DocumentType;
  profileId?: string;
}

// ---------------------------------------------------------------------------
// JD-ATS keyword diff (008)
// ---------------------------------------------------------------------------

export type KeywordPolarity = 'required' | 'preferred';

export interface KeywordDiffTerm {
  term: string;
  canonical: string;
  polarity: KeywordPolarity;
  normalized: string;
  matchType?: string; // 'exact' | 'normalized'
}

export interface KeywordDiffMetadata {
  totalRequired: number;
  totalPreferred: number;
  matchedRequired: number;
  matchedPreferred: number;
  coveragePct: number;
}

/** Advisory, truthful rephrase for a missing-required term. rephrase is null
 * when no honest rephrase is available (reason explains why). */
export interface KeywordRephraseSuggestion {
  term: string;
  canonical: string;
  rephrase: string | null;
  sourceBullet?: string;
  reason?: string;
}

export interface KeywordDiffResponse {
  jobId: string;
  matched: KeywordDiffTerm[];
  missingRequired: KeywordDiffTerm[];
  missingPreferred: KeywordDiffTerm[];
  metadata: KeywordDiffMetadata;
  suggestions: KeywordRephraseSuggestion[];
}

// ---------------------------------------------------------------------------
// Fit-gap coach (009)
// ---------------------------------------------------------------------------

export type FitGapProximity = 'close' | 'moderate' | 'distant';

/** One adjacent profile entry offered as evidence for a missing must-have,
 * with a grounded rephrase truthfully reframing that existing bullet toward
 * the missing term. */
export interface FitGapEvidence {
  sourceEntry: string;
  sourceBullet: string;
  proximity: FitGapProximity;
  rephrase: string;
}

/** One missing must-have with up to 3 adjacent evidence items drawn from the
 * user's profile. noAdjacentEvidence is the honest empty result: nothing in
 * the profile is close enough to cite. */
export interface FitGapItem {
  term: string;
  polarity: 'required';
  adjacentEvidence: FitGapEvidence[];
  noAdjacentEvidence: boolean;
}

/** The fit-gap coach output: "you fail N of M must-haves", plus per-gap
 * adjacent evidence. Served by POST /jobs/{id}/coach/assess (runs a fresh
 * assessment) and GET /jobs/{id}/coach/assessment (the last result computed
 * for this job in this server process). */
export interface FitGapAssessment {
  jobId: string;
  totalMustHaves: number;
  failedMustHaves: number;
  coveragePct: number;
  gaps: FitGapItem[];
}

// ---------------------------------------------------------------------------
// Interview Prep Pack (013)
// ---------------------------------------------------------------------------

export interface InterviewPrepPack {
  jobId: string;
  generatedAt: string;
  questions: InterviewQuestion[];
  companyNews: CompanyNewsItem[];
  keywordGap: KeywordGapSummary;
  metadata: InterviewPrepMetadata;
}

export interface InterviewPrepMetadata {
  totalQuestions: number;
  coveredQuestions: number;
  uncoveredQuestions: number;
  staleNews: boolean;
}

export interface InterviewQuestion {
  id: string;
  text: string;
  category: QuestionCategory;
  source: QuestionSource;
  sourceExcerpt: string;
  mappedStories: StoryMapping[];
}

export type QuestionCategory = 'technical' | 'behavioral' | 'experience' | 'situational' | 'company' | 'gap';

export type QuestionSource = 'required_skill' | 'preferred_skill' | 'responsibility' | 'company_context' | 'generic';

export interface StoryMapping {
  storyId: string;
  storyTitle: string;
  relevanceScore: number;
  matchedSkills: string[];
  excerpt: string;
}

export interface KeywordGapSummary {
  missingRequired: string[];
  missingPreferred: string[];
  coveragePct: number;
  gapAwarenessTips: string[];
}

export interface CompanyNewsItem {
  kind: string;
  label: string;
  value: string;
  source: string;
  fetchedAt: string;
}

export const ACTIVITY_OPS = ['ingest', 'match', 'generate', 'enrich', 'ghost_score', 'salary_infer'] as const;
export type ActivityOp = (typeof ACTIVITY_OPS)[number];

export const ACTIVITY_STATES = [
  'queued',
  'running',
  'succeeded',
  'failed',
  'cancelled',
  'timed_out', // exceeded its task-type deadline (019-ai-job-throughput)
  'interrupted', // worker vanished: crash, shutdown, or power loss (019-ai-job-throughput)
] as const;
export type ActivityState = (typeof ACTIVITY_STATES)[number];

export interface ActivityRunDto {
  id: string;
  op: ActivityOp;
  state: ActivityState;
  label: string;
  step: string | null;
  jobId: string | null;
  sourceKey: string | null;
  refId: string | null;
  error: string | null;
  meta: Record<string, unknown>;
  createdAt: string;
  startedAt: string | null;
  finishedAt: string | null;
  elapsedMs: number | null;
  heartbeatAt: string | null;
  timeoutMs: number | null;
}

export interface ActivityListResponse {
  active: ActivityRunDto[];
  recent: ActivityRunDto[];
}

// 019-ai-job-throughput: per-queue backlog snapshot (GET /api/activity/queues).
export interface QueueBacklogDto {
  queue: string;
  providerClass: 'local' | 'hosted' | null;
  concurrency: number;
  pending: number;
  active: number;
  scheduled: number;
  retry: number;
  archived: number;
  processedPerMinute: number;
  etaSeconds: number | null;
  error?: string;
}

export interface QueueBacklogResponse {
  queues: QueueBacklogDto[];
}

export interface CompanyIntelDto {
  companyName: string;
  website: string | null;
  funding: string | null;
  layoffs: string | null;
  glassdoorRating: number | null;
  headcount: string | null;
  techStack: string | null;
  fetchedAt: string;
  error?: string | null;
}

// ---------------------------------------------------------------------------
// Recruiter / hiring-manager resolution (007)
// ---------------------------------------------------------------------------

export type JobContactSource = 'posting' | 'company-page' | 'linkedin';

export interface JobContactDto {
  id: string;
  name: string;
  title: string | null;
  linkedInUrl: string | null;
  email: string | null;
  phone: string | null;
  source: JobContactSource;
  confidence: number;
  fetchedAt: string;
}

// ---------------------------------------------------------------------------
// Post-age vs Response-rate signal (010)
// ---------------------------------------------------------------------------

export type PostAgeBucketState = 'observed' | 'prior' | 'insufficient';

export interface PostAgeBucketDto {
  bucket: string;
  n: number;
  responses: number;
  rate: number | null;
  state: PostAgeBucketState;
}

export interface PostAgeResponseDto {
  buckets: PostAgeBucketDto[];
  totalApps: number;
  globalState: PostAgeBucketState;
  priorRate: number;
  priorLabel: string;
  thresholdMsg: string | null;
}

// ---------------------------------------------------------------------------
// Fresh-match notifications (010)
// ---------------------------------------------------------------------------

export interface FreshMatchNotificationDto {
  id: string;
  jobId: string;
  matchResultId: string;
  fresh: boolean;
  seen: boolean;
  createdAt: string;
  jobTitle?: string | null;
  company?: string | null;
  matchScore?: number | null;
}

// ---------------------------------------------------------------------------
// Ghost-job detector (005)
// ---------------------------------------------------------------------------

/**
 * The measured evidence behind one ghost score: the four signals (a value
 * or an explicit unknown — never a bare 0), the model's confidence, its
 * plain-English explanation, and per-signal provenance notes.
 */
export interface GhostSignalBreakdownDto {
  repostCount: number;
  daysOpen?: number | null;
  crossBoardCount?: number | null;
  alwaysHiringCount?: number | null;
  confidence: number;
  explanation: string;
  topSignals?: string[];
  notes: Record<string, string>;
}

/**
 * One row of the generic "JobSignal" table. For this feature kind is
 * always "ghost"; the shape is deliberately generic so a future signal
 * kind reuses it.
 */
export interface JobSignalDto {
  id: string;
  jobId: string;
  kind: string;
  score: number;
  model: string;
  createdAt: string;
  signals: GhostSignalBreakdownDto;
}

// ---------------------------------------------------------------------------
// Referral path finder (011)
// ---------------------------------------------------------------------------

/** One hop in a warm-path chain — a contact imported from CSV or discovered via GitHub cross-reference. */
export interface ReferralContactDto {
  id: string;
  name: string;
  email?: string | null;
  company?: string | null;
  role?: string | null;
  linkedInUrl?: string | null;
  gitHubUsername?: string | null;
}

/** One ranked warm path from the user to a contact at the job's company. */
export interface ReferralPathDto {
  path: ReferralContactDto[];
  score: number;
  length: number;
}

/** Outcome of a contacts CSV import (POST /api/contacts/import). */
export interface ContactImportResultDto {
  imported: number;
  skipped: number;
  total: number;
}

/** Outcome of cross-referencing one contact's GitHub followers/following (POST /api/contacts/{id}/github-sync). */
export interface GithubSyncResultDto {
  contact: ReferralContactDto;
  followersScanned: number;
  followingScanned: number;
  connectionsMade: number;
}

// ---------------------------------------------------------------------------
// Post-apply outreach draft generator (012)
// ---------------------------------------------------------------------------

export type OutreachTone = 'warm' | 'direct' | 'formal';

/** One specific claim in an OutreachDraftDto mapped to the company-intel
 * signal that backs it (FR-014) — the Story 3 "verify before you send it"
 * view is built directly from this list. */
export interface GroundingTraceDto {
  claim: string;
  signalKind: string;
  signalValue: string;
}

/** A generated, never-sendable outreach message, served by
 * POST /jobs/{id}/outreach/generate. contactId/contactName are undefined
 * when no resolved contact exists for the job (a neutral salutation was
 * used instead — FR-007). groundingTraces is always present, possibly
 * empty: empty means the draft made no specific claim at all, the honest
 * fallback when no company-intel signal exists (FR-012). The only actions
 * this feature ever offers on a draft are copy and regenerate — there is no
 * send/schedule/deliver action anywhere (FR-002, FR-003). */
export interface OutreachDraftDto {
  jobId: string;
  contactId?: string;
  contactName?: string;
  tone: OutreachTone;
  text: string;
  groundingTraces: GroundingTraceDto[];
  generatedAt: string;
}

/** One offered tone option, served by GET /jobs/{id}/outreach/tones
 * (FR-010, FR-011). */
export interface OutreachToneOptionDto {
  value: OutreachTone;
  label: string;
  default: boolean;
}

// ---------------------------------------------------------------------------
// Cerebras free-tier model toggle (001-cerebras-model-toggle)
// ---------------------------------------------------------------------------

/** One chat task's assigned provider/model, served by GET/PUT
 * /v1/settings/llm. taskKey is "match" | "generation" | "rephrase" | "ghost"
 * | "default"; provider is "ollama" | "cerebras" | "gateway". model is ""
 * when the provider's own default model applies, or the taskKey when provider
 * is "gateway" (the LiteLLM proxy resolves it to the upstream model). */
export interface LlmTaskSettingDto {
  taskKey: string;
  provider: string;
  model: string;
}

/** GET/PUT /v1/settings/llm response. credentialConfigured reflects whether
 * CEREBRAS_API_KEY was set at process start — never the key itself,
 * which never leaves the server. */
export interface LlmSettingsResponseDto {
  credentialConfigured: boolean;
  tasks: LlmTaskSettingDto[];
}

/** One row of the GET /v1/settings/ai-features response / PUT
 * /v1/settings/ai-features/{feature} body: whether an AI feature (resume
 * generation, cover letter generation, salary inference) is auto-enqueued
 * when a job's match score reaches threshold (0-100). Below the threshold,
 * or when disabled, the feature only runs on-demand. Match scoring itself
 * has no entry — it always runs unconditionally. */
export interface AiFeatureSettingDto {
  feature: string;
  enabled: boolean;
  threshold: number;
}

export const AI_FEATURE_KEYS = ['resume', 'cover_letter', 'salary_infer'] as const;
export type AiFeatureKey = (typeof AI_FEATURE_KEYS)[number];

/** One curated Cerebras free-tier model offered in the Settings model
 * selector, served by GET /v1/settings/llm/models. */
export interface CerebrasModelDto {
  id: string;
  label: string;
  isDefault: boolean;
}

/** GET /v1/settings/llm/models response: the curated model list per remote
 * provider. */
export interface LlmModelsResponseDto {
  cerebras: CerebrasModelDto[];
}
