// ---------------------------------------------------------------------------
// @job-finder/shared — single entry point
//
// Generated types flow from Go DTOs via tygo. This file re-exports and
// narrows; it never restates a shape. Hand-written types with no backend
// counterpart live in consumer-only.ts.
//
// Adding a Go DTO field: edit the struct in apps/api/internal/dto/,
// run `make tygo-generate`, done. Only a nullability change touches this file.
// ---------------------------------------------------------------------------

import * as Gen from './generated';
import type { Nullable } from './nullable';

// Re-export the Nullable generic for consumers that need it.
export type { Nullable } from './nullable';

// ---------------------------------------------------------------------------
// Consumer-only types (no Go DTO counterpart)
// ---------------------------------------------------------------------------

export {
  ACTIVITY_OPS,
  ACTIVITY_STATES,
  AI_FEATURE_KEYS,
  type ActivityOp,
  type ActivityState,
  type AiFeatureKey,
  type FitGapProximity,
  type JobContactSource,
  type KeywordPolarity,
  type KeywordDiffResponse,
  type OutreachTone,
  type QuestionCategory,
  type QuestionSource,
  type SubscriptionInput,
} from './consumer-only';

// ---------------------------------------------------------------------------
// Enum literal unions — keep const arrays only where generation cannot
// express the union (tygo emits `string` for these Go const groups).
// ---------------------------------------------------------------------------

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

// SourceKind and DocumentType are already unions in generated.ts (enum_style: union).
export type SourceKind = Gen.SourceKind;
export type DocumentType = Gen.DocumentType;

// PostAgeBucketState: generated has `string`; keep the union.
export type PostAgeBucketState = 'observed' | 'prior' | 'insufficient';

// ---------------------------------------------------------------------------
// Identical shapes — re-export directly from generated
// ---------------------------------------------------------------------------

export type ActivityListResponse = Omit<Gen.ActivityListResponse, 'active' | 'recent'> & {
  active: ActivityRunDto[];
  recent: ActivityRunDto[];
};
export type AdhocVacancyDto = Gen.AdhocVacancyDto;
export type AiFeatureSettingDto = Gen.AiFeatureSettingDto;
export type BaselineSummaryDto = Gen.BaselineSummaryDto;
export type BoardCandidateDto = Nullable<Gen.BoardCandidateDto, 'inferredFromJobId'>;
export type ContactImportResultDto = Gen.ContactImportResultDto;
export type CustomConnection = Gen.CustomConnection;
export type EmployerBoardDto = Nullable<Gen.EmployerBoardDto, 'lastSuccessAt'>;
export type ExportBlockDto = Gen.ExportBlockDto;
export type ExportPdfRequestDto = Gen.ExportPdfRequestDto;
export type GenerateRequestDto = Gen.GenerateRequestDto;
export type GithubSyncResultDto = Omit<Gen.GithubSyncResultDto, 'contact'> & {
  contact: ReferralContactDto;
};
export type GroundingTraceDto = Gen.GroundingTraceDto;
export type HostPacingDto = Gen.HostPacingDto;
export type JobListResponse = Omit<Gen.JobListResponse, 'items'> & { items: JobDto[] };
export type JobSignalDto = Omit<Gen.JobSignalDto, 'signals'> & { signals: GhostSignalBreakdownDto };
export type QueueBacklogResponse = Omit<Gen.QueueBacklogResponse, 'queues'> & { queues: QueueBacklogDto[] };
export type ReferralPathDto = Omit<Gen.ReferralPathDto, 'path'> & { path: ReferralContactDto[] };
export type RendercvSummary = Gen.RendercvSummary;
export type RendercvSummaryExperience = Gen.RendercvSummaryExperience;
export type ResumeDto = Omit<Gen.ResumeDto, 'resume'> & { resume: Resume };
export type ResumeEducation = Gen.ResumeEducation;
export type ResumeProject = Gen.ResumeProject;
export type ResumeSkill = Gen.ResumeSkill;
export type ResumeWork = Gen.ResumeWork;
export type Section = Omit<Gen.Section, 'entryType' | 'entries'> & {
  entryType: EntryType;
  entries: Entry[];
};
export type SocialNetwork = Gen.SocialNetwork;
export type TailorResumeRequestDto = Gen.TailorResumeRequestDto;
export type TraceabilityDto = Gen.TraceabilityDto;

// ---------------------------------------------------------------------------
// Nullability + alias narrowings
// ---------------------------------------------------------------------------

export type ActivityRunDto = Nullable<Gen.ActivityRunDto, 'elapsedMs' | 'error' | 'finishedAt' | 'heartbeatAt' | 'jobId' | 'refId' | 'sourceKey' | 'startedAt' | 'step' | 'timeoutMs'> & {
    op: import('./consumer-only').ActivityOp;
    state: import('./consumer-only').ActivityState;
    meta: Record<string, unknown>;
  };

export type ApplicationDto = Nullable<Omit<Gen.ApplicationDto, 'job'>,
  'notes' | 'appliedAt' | 'events'> & {
    events: { status: string; at: string }[];
    job: JobDto | null;
  };

export type CompanyIntelDto = Nullable<Gen.CompanyIntelDto, 'funding' | 'glassdoorRating' | 'headcount' | 'layoffs' | 'techStack' | 'website'>;

export type DocumentStatusDto = Omit<Gen.DocumentStatusDto, 'type'> & {
  type: DocumentType;
};

export type EditProposalDto = Nullable<Gen.EditProposalDto,
  'droppedReason' | 'acceptedAt' | 'rejectedAt'>;

export type FreshMatchNotificationDto = Nullable<Gen.FreshMatchNotificationDto,
  'jobTitle' | 'company' | 'matchScore'>;

export type GeneratedDocumentDto = Nullable<Gen.GeneratedDocumentDto, 'jobId' | 'pdfPath'> & {
    type: DocumentType;
    content: unknown;
  };

export type GhostSignalBreakdownDto = Nullable<Gen.GhostSignalBreakdownDto, 'alwaysHiringCount' | 'crossBoardCount' | 'daysOpen'> & {
    notes: Record<string, string>;
  };

export type HostRetrievalStatusDto = Nullable<Gen.HostRetrievalStatusDto,
  'lastBlockAt' | 'lastBlockReason' | 'coolingOffUntil'>;

export type JobContactDto = Nullable<Gen.JobContactDto, 'email' | 'linkedInUrl' | 'phone' | 'title'> & {
    source: import('./consumer-only').JobContactSource;
  };

export type JobDto = Nullable<Omit<Gen.JobDto, 'matchResult'>,
  'subscriptionId' | 'location' | 'salaryRaw' | 'postedAt'
  | 'salaryMin' | 'salaryMax'
  | 'salaryCurrency' | 'salaryConfidence' | 'salarySource'
  | 'experienceLevel' | 'experienceMinYears'
  | 'englishLevel' | 'salaryEstimateRaw' | 'salaryEstimateMin'
  | 'salaryEstimateMax' | 'salaryEstimateCurrency'> & {
    status: ApplicationStatus | 'hidden';
    matchResult: MatchResultDto | null;
    ghostSignal?: JobSignalDto | null;
  };

export type JobSourceDto = Omit<Gen.JobSourceDto, 'config'> & {
  config: Record<string, unknown>;
};

export type JsonResume = Gen.JsonResume;

export type MatchResultDto = Nullable<Gen.MatchResultDto, 'matchedSkills' | 'missingSkills' | 'redFlags' | 'score' | 'summary'>;

export type NormalizedJob = Omit<Gen.NormalizedJob, 'raw'> & {
  raw: unknown;
};

export type OutreachDraftDto = Nullable<Omit<Gen.OutreachDraftDto, 'tone'>, 'contactId' | 'contactName'> & {
  tone: import('./consumer-only').OutreachTone;
};

export type OutreachToneOptionDto = Omit<Gen.OutreachToneOptionDto, 'value'> & {
  value: import('./consumer-only').OutreachTone;
};

export type PostAgeBucketDto = Nullable<Gen.PostAgeBucketDto, 'rate'>;

export type PostAgeResponseDto = Nullable<Omit<Gen.PostAgeResponseDto, 'buckets'>, 'thresholdMsg'> & {
  buckets: PostAgeBucketDto[];
};

export type ProfileDto = Nullable<Gen.ProfileDto, 'extraNotes'>;

export type QueueBacklogDto = Nullable<Gen.QueueBacklogDto, 'etaSeconds' | 'providerClass'> & {
    providerClass: 'local' | 'hosted' | null;
  };

export type ReferralContactDto = Nullable<Gen.ReferralContactDto,
  'email' | 'company' | 'role' | 'linkedInUrl' | 'gitHubUsername'>;

export type Resume = Omit<Gen.Resume, 'unrecognized' | 'sections'> & {
  unrecognized?: Record<string, unknown>;
  sections: Section[];
};

export type ResumeBasics = Gen.ResumeBasics;

export type ResumeShapeConfigDto = Gen.ResumeShapeConfigDto;

export type RunVerdictDto = Nullable<Gen.RunVerdictDto, 'blockReason'>;

export type SavedSearchDto = Nullable<Gen.SavedSearchDto, 'lastRunAt'>;

export type SearchQuery = Gen.SearchQuery;

export type SourceRunDto = Nullable<Gen.SourceRunDto, 'blockReason' | 'error' | 'finishedAt' | 'ok' | 'searchId' | 'verdict'>;

export type StatsDto = Omit<Gen.StatsDto, 'pipeline' | 'recentRuns'> & {
  pipeline: Record<string, number>;
  recentRuns: SourceRunDto[];
};

export type SubscriptionDto = Nullable<Gen.SubscriptionDto, 'lastRunAt' | 'name'>;

export type TailoredDraftDto = Nullable<Gen.TailoredDraftDto,
  'jobId' | 'vacancyCompany' | 'vacancyTitle' | 'parentDraftId'
  | 'activityId' | 'exportStatus' | 'exportDocumentId'>;

export type Entry = Omit<Gen.Entry, 'unrecognized'> & {
  unrecognized?: Record<string, unknown>;
};

// ---------------------------------------------------------------------------
// Dto-suffixed generated types re-exported with non-Dto names
// (hand-written index.ts used shorter names; consumers import those)
// ---------------------------------------------------------------------------

export type CompanyNewsItem = Gen.CompanyNewsItemDto;
export type FitGapAssessment = Omit<Gen.FitGapAssessmentDto, 'gaps'> & { gaps: FitGapItem[] };
export type FitGapEvidence = Omit<Gen.FitGapEvidenceDto, 'proximity'> & {
  proximity: import('./consumer-only').FitGapProximity;
};
export type FitGapItem = Omit<Gen.FitGapItemDto, 'adjacentEvidence'> & { adjacentEvidence: FitGapEvidence[] };
export type InterviewPrepMetadata = Gen.InterviewPrepMetadataDto;
export type InterviewPrepPack = Omit<Gen.InterviewPrepPackDto, 'questions'> & { questions: InterviewQuestion[] };
export type InterviewQuestion = Omit<Gen.InterviewQuestionDto, 'category' | 'source'> & {
  category: import('./consumer-only').QuestionCategory;
  source: import('./consumer-only').QuestionSource;
};
export type KeywordDiffMetadata = Gen.KeywordDiffMetadataDto;
export type KeywordDiffTerm = Omit<Gen.KeywordDiffTermDto, 'polarity'> & {
  polarity: import('./consumer-only').KeywordPolarity;
};
export type KeywordGapSummary = Gen.KeywordGapSummaryDto;
export type KeywordRephraseSuggestion = Nullable<Gen.KeywordRephraseSuggestionDto, 'rephrase'>;
export type StoryMapping = Gen.StoryMappingDto;
