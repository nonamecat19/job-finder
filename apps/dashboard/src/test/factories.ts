import type {
  ApplicationDto,
  CompanyIntelDto,
  GeneratedDocumentDto,
  JobDto,
  JobListResponse,
  JobSourceDto,
  MatchResultDto,
  ProfileDto,
  SavedSearchDto,
  SourceRunDto,
  StatsDto,
  SubscriptionDto,
} from '@job-finder/shared'

export function mockMatchResult(overrides: Partial<MatchResultDto> = {}): MatchResultDto {
  return {
    id: 'match-1',
    jobId: 'test-job-1',
    similarity: 0.85,
    score: 85,
    matchedSkills: ['React', 'TypeScript'],
    missingSkills: ['Go'],
    summary: 'Strong fit for this role',
    redFlags: null,
    model: 'test-model',
    createdAt: '2025-01-15T12:00:00Z',
    ...overrides,
  }
}

export function mockJob(overrides: Partial<JobDto> = {}): JobDto {
  return {
    id: 'test-job-1',
    dedupeKey: 'abc123',
    sourceKey: 'remotive',
    title: 'Senior React Developer',
    company: 'Acme Corp',
    location: 'Remote',
    remote: true,
    salaryRaw: '$100k-$150k',
    url: 'https://example.com/job/1',
    description: 'Build amazing things',
    postedAt: '2025-01-15T00:00:00Z',
    ingestedAt: '2025-01-15T12:00:00Z',
    status: 'found',
    matchResult: mockMatchResult(),
    ...overrides,
  }
}

export function mockJobListResponse(overrides: Partial<JobListResponse> = {}): JobListResponse {
  return {
    items: [mockJob()],
    total: 1,
    page: 1,
    pageSize: 20,
    ...overrides,
  }
}

export function mockSource(overrides: Partial<JobSourceDto> = {}): JobSourceDto {
  return {
    id: 'src-1',
    key: 'remotive',
    kind: 'api',
    enabled: true,
    healthy: true,
    config: {},
    ...overrides,
  }
}

export function mockProfile(overrides: Partial<ProfileDto> = {}): ProfileDto {
  return {
    id: 'profile-1',
    name: 'My profile',
    hasConfig: true,
    rendercvConfig: { name: 'John Doe', headline: 'Software Engineer', skillGroups: [], experience: [] },
    extraNotes: null,
    updatedAt: '2025-01-15T12:00:00Z',
    ...overrides,
  }
}

export function mockSavedSearch(overrides: Partial<SavedSearchDto> = {}): SavedSearchDto {
  return {
    id: 'search-1',
    name: 'React jobs',
    query: { keywords: 'React developer' },
    cron: '0 */6 * * *',
    enabled: true,
    lastRunAt: null,
    ...overrides,
  }
}

export function mockSubscription(overrides: Partial<SubscriptionDto> = {}): SubscriptionDto {
  return {
    id: 'sub-1',
    sourceKey: 'remotive',
    name: 'Remotive feed',
    url: 'https://remotive.com/rss',
    enabled: true,
    lastRunAt: null,
    ...overrides,
  }
}

export function mockApplication(overrides: Partial<ApplicationDto> = {}): ApplicationDto {
  return {
    id: 'app-1',
    jobId: 'test-job-1',
    status: 'shortlisted',
    notes: null,
    appliedAt: null,
    events: [{ status: 'found', at: '2025-01-15T00:00:00Z' }],
    updatedAt: '2025-01-15T12:00:00Z',
    job: mockJob(),
    ...overrides,
  }
}

export function mockDocument(overrides: Partial<GeneratedDocumentDto> = {}): GeneratedDocumentDto {
  return {
    id: 'doc-1',
    jobId: 'test-job-1',
    type: 'resume',
    version: 1,
    content: { text: 'Resume content' },
    pdfPath: '/path/to/resume.pdf',
    model: 'test-model',
    createdAt: '2025-01-15T12:00:00Z',
    ...overrides,
  }
}

export function mockSourceRun(overrides: Partial<SourceRunDto> = {}): SourceRunDto {
  return {
    id: 'run-1',
    sourceKey: 'remotive',
    searchId: 'search-1',
    startedAt: '2025-01-15T12:00:00Z',
    finishedAt: '2025-01-15T12:01:00Z',
    ok: true,
    found: 10,
    new: 3,
    error: null,
    ...overrides,
  }
}

export function mockStats(overrides: Partial<StatsDto> = {}): StatsDto {
  return {
    jobsTotal: 100,
    jobsLast24h: 5,
    highFit: 12,
    pipeline: { found: 80, shortlisted: 15, applied: 5 },
    recentRuns: [mockSourceRun()],
    ...overrides,
  }
}

export function mockCompanyIntel(overrides: Partial<CompanyIntelDto> = {}): CompanyIntelDto {
  return {
    companyName: 'Acme Corp',
    website: 'https://acme.example.com',
    funding: 'Series B',
    layoffs: null,
    glassdoorRating: 3.8,
    headcount: '200-500',
    techStack: 'React, TypeScript, Go',
    fetchedAt: new Date().toISOString(),
    ...overrides,
  }
}
