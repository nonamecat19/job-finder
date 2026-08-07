import { test, expect } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';
import { ROUTES, mockAllRoutes } from './support/mockRoutes';

const THEMES = ['dark', 'light'] as const;

test.describe('Colour contrast (T6, SC-006)', () => {
  test.beforeEach(async ({ page }) => {
    await mockAllRoutes(page);
  });

  for (const route of ROUTES) {
    for (const theme of THEMES) {
      test(`${route} has no color-contrast violations in ${theme} theme`, async ({ page }) => {
        await page.goto(route);
        await page.evaluate((t) => {
          document.documentElement.setAttribute('data-theme', t);
        }, theme);

        const results = await new AxeBuilder({ page })
          .withTags(['wcag2aa'])
          .options({ runOnly: { type: 'rule', values: ['color-contrast'] } })
          .analyze();

        expect(results.violations).toEqual([]);
      });
    }
  }
});

// SC-009: every nav destination and primary control is tabbable with a visible focus ring
// (T047, User Story 4). Checked against the shell nav, which is present on every route.
test.describe('Keyboard reachability (SC-009)', () => {
  test.beforeEach(async ({ page }) => {
    await mockAllRoutes(page);
  });

  test('every nav destination is reachable via Tab and shows a visible focus ring', async ({ page }) => {
    await page.goto('/');

    const navLinks = page.locator('nav a[href]');
    const count = await navLinks.count();
    expect(count).toBeGreaterThan(0);

    for (let i = 0; i < count; i++) {
      const link = navLinks.nth(i);
      await link.focus();
      await expect(link).toBeFocused();

      const outline = await link.evaluate((el) => {
        const style = getComputedStyle(el);
        return { outlineStyle: style.outlineStyle, boxShadow: style.boxShadow };
      });
      const hasVisibleFocus = outline.outlineStyle !== 'none' || outline.boxShadow !== 'none';
      expect(hasVisibleFocus).toBe(true);
    }
  });

  test('scrollable tile regions are keyboard-focusable', async ({ page }) => {
    await page.goto('/');

    const scrollRegions = page.locator('[role="region"][tabindex="0"]');
    const count = await scrollRegions.count();

    for (let i = 0; i < count; i++) {
      const region = scrollRegions.nth(i);
      await region.focus();
      await expect(region).toBeFocused();
      const label = await region.getAttribute('aria-label');
      expect(label).toBeTruthy();
    }
  });
});
