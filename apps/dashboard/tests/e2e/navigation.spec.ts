import { test, expect } from '@playwright/test';

// Every route mounts the app shell, which fires these two calls
// unconditionally (notification bell unseen-count, profile config-status
// banner). Without a backend running in CI (this suite deliberately needs
// none — see feed.spec.ts/sources.spec.ts), they'd otherwise fail with
// ECONNREFUSED on every navigation and can destabilize the shell around the
// headings/links this spec actually asserts on.
test.beforeEach(async ({ page }) => {
  await page.route('**/api/notifications/unseen-count', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ count: 0 }),
    });
  });
  await page.route('**/api/profiles/config/status', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ hasConfig: true, hasExistingContent: true }),
    });
  });
});

test.describe('Navigation', () => {
  test('navigates between all top-level pages', async ({ page }) => {
    await page.goto('/');

    // Feed
    await expect(page.getByRole('link', { name: 'Feed' })).toBeVisible();

    // Tracker
    await page.getByRole('link', { name: 'Tracker' }).click();
    await expect(page).toHaveURL('/tracker');
    await expect(page.getByRole('heading', { level: 1, name: 'Application tracker' })).toBeVisible();

    // Sources
    await page.getByRole('link', { name: 'Sources' }).click();
    await expect(page).toHaveURL('/sources');
    await expect(page.getByRole('heading', { name: 'Job sources' })).toBeVisible();

    // Profile
    await page.getByRole('link', { name: 'Profile' }).click();
    await expect(page).toHaveURL('/profile');
    await expect(page.getByRole('heading', { level: 1, name: /^Profile/ })).toBeVisible();

    // Back to Feed
    await page.getByRole('link', { name: 'Feed' }).click();
    await expect(page).toHaveURL('/');
  });

  test('active nav link has the active style class', async ({ page }) => {
    await page.goto('/tracker');
    const trackerLink = page.getByRole('link', { name: 'Tracker' });
    await expect(trackerLink).toHaveClass(/bg-accent-soft/);
  });
});
