import type { JobFilters } from './api';

export const queryKeys = {
  jobs: {
    all: ['jobs'] as const,
    list: (filters: JobFilters) => ['jobs', 'list', filters] as const,
    detail: (id: string | undefined) => ['jobs', 'detail', id] as const,
    documents: (id: string | undefined) => ['jobs', 'documents', id] as const,
    keywordDiff: (id: string | undefined) => ['jobs', 'keyword-diff', id] as const,
    interviewPrep: (id: string | undefined) => ['jobs', 'interview-prep', id] as const,
    contacts: (id: string | undefined) => ['jobs', 'contacts', id] as const,
    referralPaths: (id: string | undefined) => ['jobs', 'referral-paths', id] as const,
  },
  coach: {
    all: ['coach'] as const,
    assessment: (jobId: string | undefined) => ['coach', 'assessment', jobId] as const,
  },
  outreach: {
    all: ['outreach'] as const,
    tones: (jobId: string | undefined) => ['outreach', 'tones', jobId] as const,
  },
  sources: {
    all: ['sources'] as const,
    list: ['sources', 'list'] as const,
  },
  searches: {
    all: ['searches'] as const,
    list: ['searches', 'list'] as const,
    runs: ['searches', 'runs', 'recent'] as const,
  },
  profiles: {
    all: ['profiles'] as const,
    list: ['profiles', 'list'] as const,
    configStatus: ['profiles', 'configStatus'] as const,
    resume: (id: string | undefined) => ['profiles', 'resume', id] as const,
  },
  applications: {
    all: ['applications'] as const,
    list: (status?: string) => ['applications', 'list', status ?? 'all'] as const,
  },
  subscriptions: {
    all: ['subscriptions'] as const,
    list: ['subscriptions', 'list'] as const,
  },
  activity: {
    all: ['activity'] as const,
    list: (limit?: number) => ['activity', 'list', limit ?? null] as const,
  },
  documents: {
    adHoc: ['documents', 'ad-hoc'] as const,
  },
  companies: {
    all: ['companies'] as const,
    detail: (jobId: string) => ['companies', 'detail', jobId] as const,
  },
  postage: {
    all: ['postage'] as const,
    responseRate: ['postage', 'response-rate'] as const,
  },
  notifications: {
    all: ['notifications'] as const,
    list: ['notifications', 'list'] as const,
    unseenCount: ['notifications', 'unseen-count'] as const,
  },
  contacts: {
    all: ['contacts'] as const,
  },
  llmSettings: {
    all: ['llmSettings'] as const,
    get: ['llmSettings', 'get'] as const,
    models: ['llmSettings', 'models'] as const,
  },
  aiFeatures: {
    all: ['aiFeatures'] as const,
    get: ['aiFeatures', 'get'] as const,
  },
};
