import { test, expect } from '@playwright/test';

// The feed routes sit behind RequireProfileConfig (app/routes.tsx), and the
// shell fires the notification bell's unseen-count on every page, so without
// a backend in CI both must be mocked or the feed renders the "set up your
// profile first" empty state instead of the job list.
test.beforeEach(async ({ page }) => {
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
});

test.describe('Feed page', () => {
  test('loads and shows the feed heading', async ({ page }) => {
    await page.goto('/');
    await expect(page.getByRole('link', { name: 'Feed' })).toBeVisible();
  });

  test('displays search input and filters', async ({ page }) => {
    await page.route('**/api/jobs*', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ items: [], total: 0, page: 1, pageSize: 50 }),
      });
    });
    await page.route('**/api/sources', async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([]) });
    });
    await page.route('**/api/subscriptions', async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([]) });
    });

    await page.goto('/');
    const searchInput = page.getByPlaceholder('Search title/company…');
    await expect(searchInput).toBeVisible();

    await searchInput.fill('react');
    await expect(searchInput).toHaveValue('react');
  });

  test('navigates to job detail when clicking a job link', async ({ page }) => {
    // Intercept the API so there's at least one job
    await page.route('**/api/jobs*', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          items: [
            {
              id: 'test-1',
              title: 'Senior React Developer',
              company: 'TestCorp',
              sourceKey: 'test',
              status: 'active',
              url: 'https://example.com/job/1',
              location: 'Remote',
              remote: true,
              matchResult: { score: 85, matchedSkills: ['React'], missingSkills: [] },
            },
          ],
          total: 1,
          page: 1,
          pageSize: 50,
        }),
      });
    });
    await page.route('**/api/sources', async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([]) });
    });
    await page.route('**/api/subscriptions', async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([]) });
    });

    await page.goto('/');
    const jobLink = page.getByText('Senior React Developer');
    await expect(jobLink).toBeVisible();

    await jobLink.click();
    await expect(page).toHaveURL(/\/jobs\/test-1/);
  });

  test('shows empty state when no jobs', async ({ page }) => {
    await page.route('**/api/jobs*', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ items: [], total: 0, page: 1, pageSize: 50 }),
      });
    });
    await page.route('**/api/sources', async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([]) });
    });
    await page.route('**/api/subscriptions', async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([]) });
    });

    await page.goto('/');
    await expect(page.getByText(/no jobs yet/i)).toBeVisible();
  });
});
