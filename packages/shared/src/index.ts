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
  jobId: string;
  type: DocumentType;
  version: number;
  content: unknown;
  pdfPath: string | null;
  model: string;
  createdAt: string;
}

export interface ProfileDto {
  id: string;
  name: string;
  document: JsonResume;
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
