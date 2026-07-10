import { test, expect } from '@playwright/test';

test.describe('Navigation', () => {
  test('navigates between all top-level pages', async ({ page }) => {
    await page.goto('/');

    // Feed
    await expect(page.getByRole('link', { name: 'Feed' })).toBeVisible();

    // Tracker
    await page.getByRole('link', { name: 'Tracker' }).click();
    await expect(page).toHaveURL('/tracker');
    await expect(page.getByRole('heading', { name: 'Application tracker' })).toBeVisible();

    // Sources
    await page.getByRole('link', { name: 'Sources' }).click();
    await expect(page).toHaveURL('/sources');
    await expect(page.getByRole('heading', { name: 'Job sources' })).toBeVisible();

    // Profile
    await page.getByRole('link', { name: 'Profile' }).click();
    await expect(page).toHaveURL('/profile');
    await expect(page.getByRole('heading', { name: 'Master profile' })).toBeVisible();

    // Back to Feed
    await page.getByRole('link', { name: 'Feed' }).click();
    await expect(page).toHaveURL('/');
  });

  test('active nav link has the active style class', async ({ page }) => {
    await page.goto('/tracker');
    const trackerLink = page.getByRole('link', { name: 'Tracker' });
    await expect(trackerLink).toHaveClass(/bg-sky-600/);
  });
});
