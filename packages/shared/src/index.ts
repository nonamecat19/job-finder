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
  /** jobspy site selector when 'jobspy' is in sources */
  site?: 'linkedin' | 'indeed' | 'glassdoor';
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
  lastRunAt: string | null;
}

export interface SubscriptionInput {
  sourceKey: string;
  url: string;
  name?: string;
  enabled?: boolean;
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

export const ACTIVITY_OPS = ['ingest', 'match', 'generate', 'enrich'] as const;
export type ActivityOp = (typeof ACTIVITY_OPS)[number];

export const ACTIVITY_STATES = ['queued', 'running', 'succeeded', 'failed'] as const;
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
}

export interface ActivityListResponse {
  active: ActivityRunDto[];
  recent: ActivityRunDto[];
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
