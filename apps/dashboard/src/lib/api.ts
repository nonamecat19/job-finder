import type {
  ActivityListResponse,
  ApplicationDto,
  BoardCandidateDto,
  CompanyIntelDto,
  DocumentStatusDto,
  DocumentType,
  EmployerBoardDto,
  FitGapAssessment,
  FreshMatchNotificationDto,
  GeneratedDocumentDto,
  GenerationExportDto,
  GenerationRewriteResponseDto,
  GenerationRunDto,
  HostRetrievalStatusDto,
  InterviewPrepPack,
  JobContactDto,
  JobDto,
  JobListResponse,
  JobSignalDto,
  JobSourceDto,
  KeywordDiffResponse,
  ManualAddResultDto,
  ManualVacancyDraftDto,
  AiFeatureSettingDto,
  OutreachDraftDto,
  OutreachToneOptionDto,
  PostAgeResponseDto,
  PreviewDocumentDto,
  ProfileDto,
  QueueBacklogResponse,
  Resume,
  ResumeDto,
  ResumeShapeConfigDto,
  SummaryModelSettingDto,
  SavedSearchDto,
  SearchQuery,
  SourceRunDto,
  StatsDto,
  SubscriptionDto,
  SubscriptionInput,
} from '@job-finder/shared';

export class ApiError extends Error {
  status: number;

  constructor(status: number, statusText: string, body: string) {
    super(`${status} ${statusText}: ${body.slice(0, 300)}`);
    this.name = 'ApiError';
    this.status = status;
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`/api${path}`, {
    headers: init?.body instanceof FormData ? undefined : { 'Content-Type': 'application/json' },
    ...init,
  });
  if (!res.ok) {
    const body = await res.text().catch(() => '');
    throw new ApiError(res.status, res.statusText, body);
  }
  const text = await res.text();
  return text ? (JSON.parse(text) as T) : (undefined as unknown as T);
}

export interface JobFilters {
  sort?: 'score' | 'date';
  source?: string;
  subscriptionId?: string;
  minScore?: number;
  status?: string;
  remote?: boolean;
  q?: string;
  page?: number;
  showBelowFloor?: boolean;
  includeHidden?: boolean;
  includeApplied?: boolean;
  onlyHidden?: boolean;
  onlyApplied?: boolean;
  onlyBelowFloor?: boolean;
  onlyManual?: boolean;
}

// A manual add answers with its outcome in the body even when the status is a
// 4xx: `failed` and `needs_fill_in` are results the operator acts on, not
// transport errors. Only a 5xx or an unparseable body is thrown.
async function requestManualAdd(path: string, body: unknown): Promise<ManualAddResultDto> {
  const res = await fetch(`/api${path}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
  const text = await res.text();
  if (res.status >= 500 || !text) {
    throw new ApiError(res.status, res.statusText, text);
  }
  const parsed = JSON.parse(text) as ManualAddResultDto;
  if (typeof parsed?.outcome !== 'string') {
    throw new ApiError(res.status, res.statusText, text);
  }
  return parsed;
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
    clear: () => request<{ deleted: number }>(`/jobs`, { method: 'DELETE' }),
    shortlist: (id: string) => request(`/jobs/${id}/shortlist`, { method: 'POST' }),
    hide: (id: string) => request(`/jobs/${id}/hide`, { method: 'POST' }),
    unhide: (id: string) => request(`/jobs/${id}/unhide`, { method: 'POST' }),
    generate: (id: string, type: DocumentType) =>
      request(`/jobs/${id}/generate`, { method: 'POST', body: JSON.stringify({ type }) }),
    documents: (id: string) => request<GeneratedDocumentDto[]>(`/jobs/${id}/documents`),
    documentStatuses: (id: string) => request<DocumentStatusDto[]>(`/jobs/${id}/documents/status`),
    keywordDiff: (id: string) => request<KeywordDiffResponse>(`/jobs/${id}/keyword-diff`),
    interviewPrep: (id: string) => request<InterviewPrepPack>(`/jobs/${id}/interview-prep`),
    ghostScore: (id: string) => request<JobSignalDto>(`/jobs/${id}/ghost-score`, { method: 'POST' }),
    reEnrich: (id: string) => request<{ ok: boolean }>(`/jobs/${id}/re-enrich`, { method: 'POST' }),
    contacts: (id: string) => request<JobContactDto[]>(`/jobs/${id}/contacts`),
    refreshContacts: (id: string) =>
      request<JobContactDto[]>(`/jobs/${id}/contacts/refresh`, { method: 'POST' }),
    addManual: (url: string) => requestManualAdd('/jobs/manual', { url }),
    saveManual: (body: ManualVacancyDraftDto & { title: string; company: string; description: string }) =>
      requestManualAdd('/jobs/manual/fill-in', body),
  },
  coach: {
    assess: (jobId: string) =>
      request<FitGapAssessment>(`/jobs/${jobId}/coach/assess`, { method: 'POST' }),
    assessment: (jobId: string) => request<FitGapAssessment>(`/jobs/${jobId}/coach/assessment`),
  },
  outreach: {
    tones: (jobId: string) => request<OutreachToneOptionDto[]>(`/jobs/${jobId}/outreach/tones`),
    generate: (jobId: string, body: { contactId?: string; tone?: string }) =>
      request<OutreachDraftDto>(`/jobs/${jobId}/outreach/generate`, {
        method: 'POST',
        body: JSON.stringify(body),
      }),
  },
  documents: {
    update: (id: string, text: string) =>
      request<GeneratedDocumentDto>(`/documents/${id}`, {
        method: 'PUT',
        body: JSON.stringify({ text }),
      }),
    pdfUrl: (id: string) => `/api/documents/${id}/pdf`,
    tailor: (body: {
      vacancy: string;
      company?: string;
      title?: string;
      groundingLevel?: string;
      requiredSkills?: string[];
      niceToHave?: string[];
      experienceLevel?: string;
      // 034: omit to use the stored default. Sending one applies it to this
      // run and makes it the new default.
      summaryOptionId?: string;
    }) =>
      request<{ resume: GeneratedDocumentDto; coverLetter: GeneratedDocumentDto | null }>('/documents/tailor', {
        method: 'POST',
        body: JSON.stringify(body),
      }),
    coverLetter: (resumeId: string) =>
      request<GeneratedDocumentDto>(`/documents/${resumeId}/cover-letter`, { method: 'POST' }),
    listAdHoc: () => request<GeneratedDocumentDto[]>('/documents/ad-hoc'),
  },
  profiles: {
    list: () => request<ProfileDto[]>('/profiles'),
    create: (body: { name: string; rendercvYaml?: string; extraNotes?: string }) =>
      request<ProfileDto>('/profiles', { method: 'POST', body: JSON.stringify(body) }),
    update: (id: string, body: { name?: string; rendercvYaml?: string; extraNotes?: string | null }) =>
      request<ProfileDto>(`/profiles/${id}`, { method: 'PUT', body: JSON.stringify(body) }),
    remove: (id: string) => request(`/profiles/${id}`, { method: 'DELETE' }),
    uploadConfig: (file: File) => {
      const fd = new FormData();
      fd.append('file', file);
      return request<ProfileDto>('/profiles/config', { method: 'POST', body: fd });
    },
    configStatus: () => request<{ hasConfig: boolean; hasExistingContent: boolean }>('/profiles/config/status'),
    getResume: (id: string) => request<ResumeDto>(`/profiles/${id}/resume`),
    updateResume: (id: string, resume: Resume) =>
      request<ResumeDto>(`/profiles/${id}/resume`, { method: 'PUT', body: JSON.stringify({ resume }) }),
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
  hosts: {
    retrievalStatus: (host: string) =>
      request<HostRetrievalStatusDto>(`/hosts/${encodeURIComponent(host)}/retrieval-status`),
    clearRungPreference: (host: string) =>
      request<void>(`/hosts/${encodeURIComponent(host)}/clear-rung-preference`, { method: 'POST' }),
    clearCookies: (host: string) =>
      request<void>(`/hosts/${encodeURIComponent(host)}/clear-cookies`, { method: 'POST' }),
    overrideCoolingOff: (host: string) =>
      request<{ remainingSeconds: number }>(`/hosts/${encodeURIComponent(host)}/override-cooling-off`, { method: 'POST' }),
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
    runAll: () =>
      request<{ queued: number }>('/subscriptions/run-all', { method: 'POST' }),
  },
  activity: {
    list: (limit?: number) =>
      request<ActivityListResponse>(`/activity${limit ? `?limit=${limit}` : ''}`),
    retry: (op?: string) =>
      request<{ retried: number; skipped: number }>(`/activity/retry${op ? `?op=${op}` : ''}`, { method: 'POST' }),
    cancel: (id: string) =>
      request<{ cancelled: boolean }>(`/activity/${id}/cancel`, { method: 'POST' }),
    cancelAll: () =>
      request<{ cancelled: number }>('/activity/cancel-all', { method: 'POST' }),
    queues: () => request<QueueBacklogResponse>('/activity/queues'),
  },
  companies: {
    intel: (jobId: string) => request<CompanyIntelDto>(`/companies/${jobId}/intel`),
    refresh: (jobId: string) =>
      request<CompanyIntelDto>(`/companies/${jobId}/intel/refresh`, { method: 'POST' }),
  },
  postage: {
    responseRate: () => request<PostAgeResponseDto>('/postage-response-rate'),
  },
  notifications: {
    list: () => request<FreshMatchNotificationDto[]>('/notifications'),
    markSeen: (id: string) => request<void>(`/notifications/${id}/seen`, { method: 'POST' }),
    unseenCount: () => request<{ count: number }>('/notifications/unseen-count'),
  },
  roster: {
    list: () => request<{ employers: EmployerBoardDto[] }>('/roster'),
    register: (url: string) =>
      request<EmployerBoardDto>('/roster', { method: 'POST', body: JSON.stringify({ url }) }),
    remove: (id: string) => request(`/roster/${id}`, { method: 'DELETE' }),
    candidates: () => request<{ candidates: BoardCandidateDto[] }>('/roster/candidates'),
    accept: (id: string) =>
      request<EmployerBoardDto>(`/roster/candidates/${id}/accept`, { method: 'POST' }),
    reject: (id: string) => request(`/roster/candidates/${id}/reject`, { method: 'POST' }),
    discover: () => request<{ newCandidates: number }>('/roster/discover', { method: 'POST' }),
  },
  settings: {
    getAiFeatures: () => request<AiFeatureSettingDto[]>('/v1/settings/ai-features'),
    putAiFeature: (feature: string, body: Pick<AiFeatureSettingDto, 'enabled' | 'threshold'>) =>
      request<AiFeatureSettingDto>(`/v1/settings/ai-features/${feature}`, {
        method: 'PUT',
        body: JSON.stringify(body),
      }),
    // Resume shape: PUT replaces the whole config (validation is
    // all-or-nothing), DELETE means "drop my overrides" and returns defaults.
    getResumeShape: () => request<ResumeShapeConfigDto>('/v1/settings/resume-shape'),
    putResumeShape: (body: ResumeShapeConfigDto) =>
      request<ResumeShapeConfigDto>('/v1/settings/resume-shape', {
        method: 'PUT',
        body: JSON.stringify(body),
      }),
    resetResumeShape: () =>
      request<ResumeShapeConfigDto>('/v1/settings/resume-shape', { method: 'DELETE' }),
    // Summary model (034). GET returns the whole menu with the current entry
    // marked, so the selector renders from one response and the menu and the
    // selection can never disagree about which options exist.
    getSummaryModel: () => request<SummaryModelSettingDto>('/v1/settings/summary-model'),
    putSummaryModel: (optionId: string) =>
      request<SummaryModelSettingDto>('/v1/settings/summary-model', {
        method: 'PUT',
        body: JSON.stringify({ optionId }),
      }),
  },
  // 042: the resume generation workspace. `start`/`get`/`list`/`remove` are
  // live from Phase 2 (Foundational); `patchItem`/`reorder`/`rerun`/`export`/
  // `exportStatus` are defined now per the contract and 404 until the phase
  // that implements each one lands — that is expected, not a bug.
  generations: {
    start: (body: {
      profileId: string;
      jobId?: string;
      vacancy?: { company: string; title: string; text: string };
      groundingLevel?: string;
      summaryOptionId?: string;
    }) =>
      request<{ runId: string; activityId: string }>('/v1/generations', {
        method: 'POST',
        body: JSON.stringify(body),
      }),
    get: (runId: string) => request<GenerationRunDto>(`/v1/generations/${runId}`),
    list: (params?: { jobId?: string; limit?: number }) => {
      const q = new URLSearchParams();
      if (params?.jobId) q.set('jobId', params.jobId);
      if (params?.limit) q.set('limit', String(params.limit));
      const qs = q.toString();
      return request<GenerationRunDto[]>(`/v1/generations${qs ? `?${qs}` : ''}`);
    },
    patchItem: (
      runId: string,
      itemId: string,
      body: { selected?: boolean; position?: number; text?: string; droppedEntries?: string[] },
    ) =>
      request(`/v1/generations/${runId}/items/${itemId}`, { method: 'PATCH', body: JSON.stringify(body) }),
    rewriteItem: (runId: string, itemId: string) =>
      request<GenerationRewriteResponseDto>(`/v1/generations/${runId}/items/${itemId}/rewrite`, {
        method: 'POST',
      }),
    reorder: (runId: string, sectionId: string, itemIds: string[]) =>
      request(`/v1/generations/${runId}/sections/${sectionId}/order`, {
        method: 'PATCH',
        body: JSON.stringify({ itemIds }),
      }),
    rerun: (runId: string, sections?: string[]) =>
      request<{ runId: string; activityId: string }>(`/v1/generations/${runId}/rerun`, {
        method: 'POST',
        body: JSON.stringify({ sections }),
      }),
    export: (runId: string) =>
      request<GenerationExportDto>(`/v1/generations/${runId}/export`, { method: 'POST' }),
    exportStatus: (runId: string) => request<GenerationExportDto>(`/v1/generations/${runId}/export`),
    // 046: the live preview's YAML source — a pure read, no export-status
    // side effect. See specs/046-real-resume-preview/contracts/preview-document.md.
    previewDocument: (runId: string) =>
      request<PreviewDocumentDto>(`/v1/generations/${runId}/preview-document`),
    remove: (runId: string) => request<void>(`/v1/generations/${runId}`, { method: 'DELETE' }),
  },
};
