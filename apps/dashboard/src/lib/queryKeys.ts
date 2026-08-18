import type { JobFilters } from './api';

export const queryKeys = {
  jobs: {
    all: ['jobs'] as const,
    list: (filters: JobFilters) => ['jobs', 'list', filters] as const,

    picker: (filters: JobFilters) => ['jobs', 'picker', filters] as const,
    detail: (id: string | undefined) => ['jobs', 'detail', id] as const,
    documents: (id: string | undefined) => ['jobs', 'documents', id] as const,
    documentStatuses: (id: string | undefined) => ['jobs', 'documents', 'status', id] as const,
    keywordDiff: (id: string | undefined) => ['jobs', 'keyword-diff', id] as const,
    interviewPrep: (id: string | undefined) => ['jobs', 'interview-prep', id] as const,
    contacts: (id: string | undefined) => ['jobs', 'contacts', id] as const,
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
    queues: ['activity', 'queues'] as const,
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
  aiFeatures: {
    all: ['aiFeatures'] as const,
    get: ['aiFeatures', 'get'] as const,
  },
  resumeShape: {
    all: ['resumeShape'] as const,
    get: ['resumeShape', 'get'] as const,
  },
  roster: {
    all: ['roster'] as const,
    list: ['roster', 'list'] as const,
    candidates: ['roster', 'candidates'] as const,
  },
  hosts: {
    all: ['hosts'] as const,
    retrievalStatus: (host: string) => ['hosts', 'retrieval-status', host] as const,
  },
};
