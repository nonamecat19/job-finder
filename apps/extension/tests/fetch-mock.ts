import { vi, type Mock } from 'vitest';

export type FetchArgs = [input: RequestInfo | URL, init?: RequestInit];
export type FetchMock = Mock<(...args: FetchArgs) => Promise<Response>>;

export function stubFetch(impl: (...args: FetchArgs) => Promise<Response>): FetchMock {
  const mock = vi.fn(impl) as FetchMock;
  vi.stubGlobal('fetch', mock);
  return mock;
}

export function stubFetchSequence(...responses: Response[]): FetchMock {
  let i = 0;
  return stubFetch(async () => responses[Math.min(i++, responses.length - 1)]);
}

export function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
}

export function urlOf(mock: FetchMock, call: number): URL {
  return new URL(String(mock.mock.calls[call][0]));
}
