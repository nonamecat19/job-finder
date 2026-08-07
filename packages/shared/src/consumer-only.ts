

export const ACTIVITY_OPS = ['ingest', 'match', 'generate', 'enrich', 'ghost_score', 'salary_infer'] as const;
export type ActivityOp = (typeof ACTIVITY_OPS)[number];

export const ACTIVITY_STATES = [
  'queued',
  'running',
  'succeeded',
  'failed',
  'cancelled',
  'timed_out',
  'interrupted',
] as const;
export type ActivityState = (typeof ACTIVITY_STATES)[number];

export const AI_FEATURE_KEYS = ['resume', 'cover_letter', 'salary_infer'] as const;
export type AiFeatureKey = (typeof AI_FEATURE_KEYS)[number];

export type FitGapProximity = 'close' | 'moderate' | 'distant';

export type JobContactSource = 'posting' | 'company-page' | 'linkedin';

export type KeywordPolarity = 'required' | 'preferred';

export type OutreachTone = 'warm' | 'direct' | 'formal';

export type QuestionCategory = 'technical' | 'behavioral' | 'experience' | 'situational' | 'company' | 'gap';

export type QuestionSource = 'required_skill' | 'preferred_skill' | 'responsibility' | 'company_context' | 'generic';

export interface SubscriptionInput {
  sourceKey: string;
  url: string;
  name?: string;
  enabled?: boolean;
  cron?: string;
}

export interface KeywordDiffResponse {
  jobId: string;
  matched: KeywordDiffTerm[];
  missingRequired: KeywordDiffTerm[];
  missingPreferred: KeywordDiffTerm[];
  metadata: KeywordDiffMetadataDto;
  suggestions: KeywordRephraseSuggestion[];
}

import type { KeywordDiffTermDto, KeywordDiffMetadataDto } from './generated';
export type { KeywordDiffTermDto, KeywordDiffMetadataDto };

import type { KeywordRephraseSuggestion, KeywordDiffTerm } from './index';
