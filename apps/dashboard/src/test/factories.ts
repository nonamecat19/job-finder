import type {
  ApplicationDto,
  CompanyIntelDto,
  FreshMatchNotificationDto,
  GeneratedDocumentDto,
  JobDto,
  JobListResponse,
  JobSourceDto,
  MatchResultDto,
  PostAgeBucketDto,
  PostAgeResponseDto,
  ProfileDto,
  ReferralContactDto,
  ReferralPathDto,
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

export function mockPostAgeBucket(overrides: Partial<PostAgeBucketDto> = {}): PostAgeBucketDto {
  return {
    bucket: 'fresh',
    n: 12,
    responses: 5,
    rate: 0.42,
    state: 'observed',
    ...overrides,
  }
}

export function mockPostAgeResponse(overrides: Partial<PostAgeResponseDto> = {}): PostAgeResponseDto {
  return {
    buckets: [
      mockPostAgeBucket({ bucket: 'fresh', n: 12, responses: 5, rate: 0.42, state: 'observed' }),
      mockPostAgeBucket({ bucket: 'recent', n: 8, responses: 2, rate: 0.25, state: 'observed' }),
      mockPostAgeBucket({ bucket: 'aging', n: 3, responses: 0, rate: null, state: 'insufficient' }),
      mockPostAgeBucket({ bucket: 'stale', n: 2, responses: 0, rate: null, state: 'insufficient' }),
    ],
    totalApps: 25,
    globalState: 'observed',
    priorRate: 0.2,
    priorLabel: 'Typical baseline — not yet your data',
    thresholdMsg: null,
    ...overrides,
  }
}

export function mockReferralContact(overrides: Partial<ReferralContactDto> = {}): ReferralContactDto {
  return {
    id: 'contact-1',
    name: 'Jane Doe',
    email: 'jane@example.com',
    company: 'Acme Corp',
    role: 'Engineering Manager',
    linkedInUrl: 'https://linkedin.com/in/janedoe',
    gitHubUsername: 'janedoe',
    ...overrides,
  }
}

export function mockReferralPath(overrides: Partial<ReferralPathDto> = {}): ReferralPathDto {
  return {
    path: [mockReferralContact({ id: 'me', name: 'You' }), mockReferralContact()],
    score: 0.72,
    length: 2,
    ...overrides,
  }
}

export function mockFreshMatchNotification(overrides: Partial<FreshMatchNotificationDto> = {}): FreshMatchNotificationDto {
  return {
    id: 'notif-1',
    jobId: 'test-job-1',
    matchResultId: 'match-1',
    fresh: true,
    seen: false,
    createdAt: new Date().toISOString(),
    jobTitle: 'Senior React Developer',
    company: 'Acme Corp',
    matchScore: 85,
    ...overrides,
  }
}
