
import * as Gen from './generated';
import type { Nullable } from './nullable';

export type { Nullable } from './nullable';

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

export type SourceKind = Gen.SourceKind;
export type DocumentType = Gen.DocumentType;

export type PostAgeBucketState = 'observed' | 'prior' | 'insufficient';

export type ActivityListResponse = Omit<Gen.ActivityListResponse, 'active' | 'recent'> & {
  active: ActivityRunDto[];
  recent: ActivityRunDto[];
};
export type AdhocVacancyDto = Gen.AdhocVacancyDto;
export type AiFeatureSettingDto = Gen.AiFeatureSettingDto;
export type BoardCandidateDto = Nullable<Gen.BoardCandidateDto, 'inferredFromJobId'>;
export type CustomConnection = Gen.CustomConnection;
export type EmployerBoardDto = Nullable<Gen.EmployerBoardDto, 'lastSuccessAt'>;
export type GenerateRequestDto = Gen.GenerateRequestDto;
export type GroundingTraceDto = Gen.GroundingTraceDto;
// 042: the resume generation workspace's wire shapes. Every pointer field on
// these DTOs already carries `omitempty` (dto/generation_workspace.go), so
// none needs a Nullable<> narrowing here — adding a field there requires zero
// edits here, per 024-FR-003 rule 3.
export type GenerationRunDto = Gen.GenerationRunDto;
export type GenerationVacancyDto = Gen.GenerationVacancyDto;
export type GenerationSectionDto = Gen.GenerationSectionDto;
export type GenerationItemDto = Gen.GenerationItemDto;
export type GenerationExportDto = Gen.GenerationExportDto;
export type OverflowReportDto = Gen.OverflowReportDto;
export type OverflowCandidateDto = Gen.OverflowCandidateDto;
export type StartGenerationRequestDto = Gen.StartGenerationRequestDto;
export type PatchGenerationItemRequestDto = Gen.PatchGenerationItemRequestDto;
export type RerunGenerationRequestDto = Gen.RerunGenerationRequestDto;
export type HostPacingDto = Gen.HostPacingDto;
export type JobListResponse = Omit<Gen.JobListResponse, 'items'> & { items: JobDto[] };
export type JobSignalDto = Omit<Gen.JobSignalDto, 'signals'> & { signals: GhostSignalBreakdownDto };
export type QueueBacklogResponse = Omit<Gen.QueueBacklogResponse, 'queues'> & { queues: QueueBacklogDto[] };
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

// providerClass is permanently 'hosted' for any LLM-backed queue since
// 044-litellm-only-routing removed the second (local/Ollama) inference path.
// The field is kept and its shape unchanged rather than removed — dropping it
// would be a breaking change to a shared type for a value that is now
// constant.
export type QueueBacklogDto = Nullable<Gen.QueueBacklogDto, 'etaSeconds' | 'providerClass'> & {
    providerClass: 'local' | 'hosted' | null;
  };

export type Resume = Omit<Gen.Resume, 'unrecognized' | 'sections'> & {
  unrecognized?: Record<string, unknown>;
  sections: Section[];
};

export type ResumeBasics = Gen.ResumeBasics;

export type ResumeShapeConfigDto = Gen.ResumeShapeConfigDto;
export type SummaryModelOptionDto = Gen.SummaryModelOptionDto;
export type SummaryModelSettingDto = Gen.SummaryModelSettingDto;
export type UpdateSummaryModelRequestDto = Gen.UpdateSummaryModelRequestDto;

export type ManualAddResultDto = Gen.ManualAddResultDto;
export type ManualVacancyDraftDto = Gen.ManualVacancyDraftDto;

export type RunVerdictDto = Nullable<Gen.RunVerdictDto, 'blockReason'>;

export type SavedSearchDto = Nullable<Gen.SavedSearchDto, 'lastRunAt'>;

export type SearchQuery = Gen.SearchQuery;

export type SourceRunDto = Nullable<Gen.SourceRunDto, 'blockReason' | 'error' | 'finishedAt' | 'ok' | 'searchId' | 'subscriptionId' | 'verdict'>;

export type StatsDto = Omit<Gen.StatsDto, 'pipeline' | 'recentRuns'> & {
  pipeline: Record<string, number>;
  recentRuns: SourceRunDto[];
};

export type SubscriptionDto = Nullable<Gen.SubscriptionDto, 'lastRunAt' | 'name'>;

export type Entry = Omit<Gen.Entry, 'unrecognized'> & {
  unrecognized?: Record<string, unknown>;
};

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
