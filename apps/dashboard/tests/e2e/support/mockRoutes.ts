import type { Page } from '@playwright/test';

/**
 * The nine top-level routes under test (data-model E4 / SC-001, SC-004, SC-006, SC-010).
 * `/jobs/:id` stands in for the dynamic job-detail route with a fixed id.
 */
export const ROUTES = [
  '/',
  '/jobs/e2e-job-1',
  '/profile',
  '/contacts',
  '/tailor',
  '/sources',
  '/status',
  '/settings',
  '/tracker',
] as const;

const JOB_FIXTURE = {
  id: 'e2e-job-1',
  title: 'Senior React Developer',
  company: 'TestCorp',
  sourceKey: 'test',
  status: 'active',
  url: 'https://example.com/job/1',
  location: 'Remote',
  remote: true,
  description: 'A great job.',
  matchResult: { score: 85, matchedSkills: ['React'], missingSkills: [] },
};

/**
 * Stubs every `/api/**` call the shell and the nine pages can issue, so each route mounts its
 * real content instead of failing on ECONNREFUSED (no backend runs in this suite — see
 * feed.spec.ts/sources.spec.ts). A wildcard catch-all covers anything unmatched with an
 * empty-but-valid JSON body; that's enough for layout/contrast assertions since Tile occupies
 * its grid footprint in every state (loading/empty/error/ready) per contracts/layout-primitives.md.
 */
export async function mockAllRoutes(page: Page): Promise<void> {
  // Registered first so it runs last: Playwright dispatches route handlers in reverse
  // registration order, so this catch-all only fires when none of the specific routes below
  // (registered after it) match. It resolves anything unmodelled to an empty object rather than
  // a network error, so a widget lands in its `empty`/`error` Tile state instead of hanging.
  await page.route('**/api/**', async (route) => {
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({}) });
  });
  await page.route('**/api/profiles/config/status', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ hasConfig: true, hasExistingContent: true }),
    });
  });
  await page.route('**/api/notifications/unseen-count', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ count: 0 }),
    });
  });
  await page.route('**/api/jobs/e2e-job-1', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(JOB_FIXTURE),
    });
  });
  await page.route('**/api/jobs*', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ items: [JOB_FIXTURE], total: 1, page: 1, pageSize: 50 }),
    });
  });
  await page.route('**/api/sources', async (route) => {
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([]) });
  });
  await page.route('**/api/searches*', async (route) => {
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([]) });
  });
  await page.route('**/api/subscriptions', async (route) => {
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([]) });
  });
  await page.route('**/api/roster', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ employers: [] }),
    });
  });
  await page.route('**/api/roster/candidates', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ candidates: [] }),
    });
  });
  await page.route('**/api/applications*', async (route) => {
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([]) });
  });
  await page.route('**/api/tracker*', async (route) => {
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([]) });
  });
  await page.route('**/api/status*', async (route) => {
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({}) });
  });
  await page.route('**/api/settings*', async (route) => {
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({}) });
  });
  await page.route('**/api/profiles*', async (route) => {
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({}) });
  });
  await page.route('**/api/contacts*', async (route) => {
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([]) });
  });
}
