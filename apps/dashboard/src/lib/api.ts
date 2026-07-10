import type {
  ApplicationDto,
  DocumentType,
  GeneratedDocumentDto,
  JobDto,
  JobListResponse,
  JobSourceDto,
  JsonResume,
  ProfileDto,
  SavedSearchDto,
  SearchQuery,
  SourceRunDto,
  StatsDto,
  SubscriptionDto,
  SubscriptionInput,
} from '@job-finder/shared';

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`/api${path}`, {
    headers: init?.body instanceof FormData ? undefined : { 'Content-Type': 'application/json' },
    ...init,
  });
  if (!res.ok) {
    const body = await res.text().catch(() => '');
    throw new Error(`${res.status} ${res.statusText}: ${body.slice(0, 300)}`);
  }
  return res.json() as Promise<T>;
}

export interface JobFilters {
  sort?: 'score' | 'date';
  source?: string;
  minScore?: number;
  status?: string;
  remote?: boolean;
  q?: string;
  page?: number;
}

export const api = {
  jobs: {
    list: (f: JobFilters) => {
      const params = new URLSearchParams();
      for (const [k, v] of Object.entries(f)) {
        if (v !== undefined && v !== '' && v !== null) params.set(k, String(v));
      }
      return request<JobListResponse>(`/jobs?${params}`);
    },
    get: (id: string) =>
      request<JobDto & { documents: GeneratedDocumentDto[]; application: ApplicationDto | null }>(
        `/jobs/${id}`,
      ),
    shortlist: (id: string) => request(`/jobs/${id}/shortlist`, { method: 'POST' }),
    hide: (id: string) => request(`/jobs/${id}/hide`, { method: 'POST' }),
    generate: (id: string, type: DocumentType) =>
      request(`/jobs/${id}/generate`, { method: 'POST', body: JSON.stringify({ type }) }),
    documents: (id: string) => request<GeneratedDocumentDto[]>(`/jobs/${id}/documents`),
  },
  documents: {
    update: (id: string, text: string) =>
      request<GeneratedDocumentDto>(`/documents/${id}`, {
        method: 'PUT',
        body: JSON.stringify({ text }),
      }),
    pdfUrl: (id: string) => `/api/documents/${id}/pdf`,
  },
  profiles: {
    list: () => request<ProfileDto[]>('/profiles'),
    create: (body: { name: string; document: JsonResume; extraNotes?: string }) =>
      request<ProfileDto>('/profiles', { method: 'POST', body: JSON.stringify(body) }),
    update: (id: string, body: { name?: string; document?: JsonResume; extraNotes?: string | null }) =>
      request<ProfileDto>(`/profiles/${id}`, { method: 'PUT', body: JSON.stringify(body) }),
    remove: (id: string) => request(`/profiles/${id}`, { method: 'DELETE' }),
    import: (file: File) => {
      const fd = new FormData();
      fd.append('file', file);
      return request<{ draft: JsonResume; textLength: number }>('/profiles/import', {
        method: 'POST',
        body: fd,
      });
    },
  },
  sources: {
    list: () => request<JobSourceDto[]>('/sources'),
    update: (key: string, body: { enabled?: boolean; config?: Record<string, unknown> }) =>
      request<JobSourceDto>(`/sources/${key}`, { method: 'PUT', body: JSON.stringify(body) }),
    test: (key: string) =>
      request<{ ok: boolean; error?: string }>(`/sources/${key}/test`, { method: 'POST' }),
    enrich: (key: string, limit?: number) =>
      request<{ enqueued: number }>(`/sources/${key}/enrich${limit ? `?limit=${limit}` : ''}`, { method: 'POST' }),
  },
  searches: {
    list: () => request<SavedSearchDto[]>('/searches'),
    create: (body: { name: string; query: SearchQuery; cron?: string; enabled?: boolean }) =>
      request<SavedSearchDto>('/searches', { method: 'POST', body: JSON.stringify(body) }),
    update: (id: string, body: Partial<{ name: string; query: SearchQuery; cron: string; enabled: boolean }>) =>
      request<SavedSearchDto>(`/searches/${id}`, { method: 'PUT', body: JSON.stringify(body) }),
    remove: (id: string) => request(`/searches/${id}`, { method: 'DELETE' }),
    run: (id: string) => request<{ enqueued: string[] }>(`/searches/${id}/run`, { method: 'POST' }),
    recentRuns: () => request<SourceRunDto[]>('/searches/runs/recent'),
  },
  applications: {
    list: (status?: string) =>
      request<ApplicationDto[]>(`/applications${status ? `?status=${status}` : ''}`),
    update: (id: string, body: { status?: string; notes?: string | null }) =>
      request<ApplicationDto>(`/applications/${id}`, { method: 'PATCH', body: JSON.stringify(body) }),
  },
  stats: () => request<StatsDto>('/stats'),
  subscriptions: {
    list: (source?: string) =>
      request<SubscriptionDto[]>(`/subscriptions${source ? `?source=${source}` : ''}`),
    create: (body: SubscriptionInput) =>
      request<SubscriptionDto>('/subscriptions', { method: 'POST', body: JSON.stringify(body) }),
    update: (id: string, body: { name?: string; url?: string; enabled?: boolean }) =>
      request<SubscriptionDto>(`/subscriptions/${id}`, { method: 'PUT', body: JSON.stringify(body) }),
    remove: (id: string) =>
      request<{ deleted: boolean }>(`/subscriptions/${id}`, { method: 'DELETE' }),
    run: (id: string) =>
      request<{ queued: boolean }>(`/subscriptions/${id}/run`, { method: 'POST' }),
  },
};
