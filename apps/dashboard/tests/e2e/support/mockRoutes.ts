import type { Page } from '@playwright/test';

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

export async function mockAllRoutes(page: Page): Promise<void> {
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
