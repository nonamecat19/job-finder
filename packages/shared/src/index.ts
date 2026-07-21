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
