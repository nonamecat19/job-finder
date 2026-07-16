import type { JobFilters } from '../api';

export const queryKeys = {
  jobs: {
    all: ['jobs'] as const,
    list: (filters: JobFilters) => ['jobs', 'list', filters] as const,
    detail: (id: string | undefined) => ['jobs', 'detail', id] as const,
    documents: (id: string | undefined) => ['jobs', 'documents', id] as const,
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
};
