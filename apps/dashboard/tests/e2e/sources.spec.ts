import { test, expect } from '@playwright/test';

test.describe('Sources page', () => {
  test('loads and shows the sources heading', async ({ page }) => {
    await page.goto('/sources');
    await expect(page.getByRole('heading', { name: 'Job sources' })).toBeVisible();
  });

  test('displays saved searches section', async ({ page }) => {
    await page.route('**/api/sources', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify([
          { key: 'indeed', kind: 'jobspy', enabled: true, healthy: true, config: {} },
        ]),
      });
    });
    await page.route('**/api/searches', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify([
          { id: 's1', name: 'React jobs', query: { keywords: 'react' }, cron: '0 */6 * * *', enabled: true },
        ]),
      });
    });
    await page.route('**/api/searches/runs/recent', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify([]),
      });
    });

    await page.goto('/sources');
    await expect(page.getByText('React jobs')).toBeVisible();
    await expect(page.getByPlaceholder('keywords*')).toBeVisible();
  });

  test('shows a configured source with test button', async ({ page }) => {
    await page.route('**/api/sources', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify([
          { key: 'adzuna', kind: 'api', enabled: true, healthy: true, config: { appId: 'abc' } },
        ]),
      });
    });
    await page.route('**/api/searches', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify([]),
      });
    });
    await page.route('**/api/searches/runs/recent', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify([]),
      });
    });

    await page.goto('/sources');
    await expect(page.getByText('adzuna')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Test' })).toBeVisible();
  });
});
