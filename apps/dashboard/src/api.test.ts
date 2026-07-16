import { api } from './api'

const mockFetch = vi.fn()
global.fetch = mockFetch

function jsonResponse(data: unknown, status = 200) {
  return {
    ok: status >= 200 && status < 300,
    status,
    statusText: status === 200 ? 'OK' : 'Error',
    json: () => Promise.resolve(data),
    text: () => Promise.resolve(JSON.stringify(data)),
  }
}

beforeEach(() => {
  mockFetch.mockReset()
})

describe('api.jobs', () => {
  it('list calls correct URL with params', async () => {
    const payload = { items: [], total: 0, page: 1, pageSize: 20 }
    mockFetch.mockResolvedValue(jsonResponse(payload))

    const result = await api.jobs.list({ sort: 'score', page: 2, remote: true })

    expect(mockFetch).toHaveBeenCalledOnce()
    const [url, opts] = mockFetch.mock.calls[0]
    expect(url).toContain('/api/jobs?')
    expect(url).toContain('sort=score')
    expect(url).toContain('page=2')
    expect(url).toContain('remote=true')
    expect(opts.headers['Content-Type']).toBe('application/json')
    expect(result).toEqual(payload)
  })

  it('get calls correct URL', async () => {
    const job = { id: 'j1', title: 'Dev' }
    mockFetch.mockResolvedValue(jsonResponse(job))

    await api.jobs.get('j1')

    const [url] = mockFetch.mock.calls[0]
    expect(url).toBe('/api/jobs/j1')
  })

  it('shortlist sends POST', async () => {
    mockFetch.mockResolvedValue(jsonResponse({}))

    await api.jobs.shortlist('j1')

    const [url, opts] = mockFetch.mock.calls[0]
    expect(url).toBe('/api/jobs/j1/shortlist')
    expect(opts.method).toBe('POST')
  })

  it('hide sends POST', async () => {
    mockFetch.mockResolvedValue(jsonResponse({}))

    await api.jobs.hide('j1')

    const [url, opts] = mockFetch.mock.calls[0]
    expect(url).toBe('/api/jobs/j1/hide')
    expect(opts.method).toBe('POST')
  })

  it('generate sends POST with body', async () => {
    mockFetch.mockResolvedValue(jsonResponse({}))

    await api.jobs.generate('j1', 'resume')

    const [url, opts] = mockFetch.mock.calls[0]
    expect(url).toBe('/api/jobs/j1/generate')
    expect(opts.method).toBe('POST')
    expect(JSON.parse(opts.body)).toEqual({ type: 'resume' })
  })
})

describe('api.sources', () => {
  it('list calls correct URL', async () => {
    mockFetch.mockResolvedValue(jsonResponse([]))

    await api.sources.list()

    const [url] = mockFetch.mock.calls[0]
    expect(url).toBe('/api/sources')
  })

  it('update sends PUT', async () => {
    mockFetch.mockResolvedValue(jsonResponse({}))

    await api.sources.update('remotive', { enabled: false })

    const [url, opts] = mockFetch.mock.calls[0]
    expect(url).toBe('/api/sources/remotive')
    expect(opts.method).toBe('PUT')
    expect(JSON.parse(opts.body)).toEqual({ enabled: false })
  })
})

describe('api.profiles', () => {
  it('list calls correct URL', async () => {
    mockFetch.mockResolvedValue(jsonResponse([]))

    await api.profiles.list()

    const [url] = mockFetch.mock.calls[0]
    expect(url).toBe('/api/profiles')
  })

  it('create sends POST', async () => {
    mockFetch.mockResolvedValue(jsonResponse({}))

    await api.profiles.create({ name: 'Test', rendercvYaml: 'cv:\n  name: Test\n' })

    const [url, opts] = mockFetch.mock.calls[0]
    expect(url).toBe('/api/profiles')
    expect(opts.method).toBe('POST')
    const body = JSON.parse(opts.body)
    expect(body.name).toBe('Test')
  })
})

describe('api.searches', () => {
  it('list calls correct URL', async () => {
    mockFetch.mockResolvedValue(jsonResponse([]))

    await api.searches.list()

    const [url] = mockFetch.mock.calls[0]
    expect(url).toBe('/api/searches')
  })
})

describe('api.applications', () => {
  it('list calls correct URL', async () => {
    mockFetch.mockResolvedValue(jsonResponse([]))

    await api.applications.list()

    const [url] = mockFetch.mock.calls[0]
    expect(url).toBe('/api/applications')
  })
})

describe('error handling', () => {
  it('throws on non-ok response', async () => {
    mockFetch.mockResolvedValue(jsonResponse('not found', 404))

    await expect(api.jobs.get('missing')).rejects.toThrow('404')
  })
})
