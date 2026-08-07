import { test, expect } from '@playwright/test';
import { ROUTES, mockAllRoutes } from './support/mockRoutes';

const COLUMN_MATRIX: Array<{ width: number; height: number; columns: number }> = [
  { width: 375, height: 812, columns: 1 },
  { width: 768, height: 1024, columns: 2 },
  { width: 1280, height: 900, columns: 3 },
  { width: 1920, height: 1080, columns: 4 },
  { width: 2560, height: 1440, columns: 5 },
];

const FIT_ROUTES = ['/', '/status', '/tracker'];

test.describe('Dashboard grid layout', () => {
  test.beforeEach(async ({ page }) => {
    await mockAllRoutes(page);
  });

  for (const route of ROUTES) {
    for (const { width, height, columns } of COLUMN_MATRIX) {
      test(`${route} renders ${columns} grid column(s) at ${width}x${height}`, async ({ page }) => {
        await page.setViewportSize({ width, height });
        await page.goto(route);

        const grid = page.locator('[class*="grid-cols-1"]').first();
        await expect(grid).toBeVisible();

        const trackCount = await grid.evaluate((el) => {
          const template = getComputedStyle(el).gridTemplateColumns;
          return template.split(' ').filter(Boolean).length;
        });

        expect(trackCount).toBe(columns);
      });
    }
  }

  // SC-004 / FR-010: no horizontal scroll at any matrix width, plus the 320px floor.
  for (const route of ROUTES) {
    for (const width of [320, 375, 768, 1280, 1920, 2560, 3840]) {
      test(`${route} has no horizontal overflow at ${width}px`, async ({ page }) => {
        await page.setViewportSize({ width, height: 900 });
        await page.goto(route);

        const overflow = await page.evaluate(() => {
          const el = document.documentElement;
          return el.scrollWidth - el.clientWidth;
        });

        expect(overflow).toBeLessThanOrEqual(1); // 1px tolerance for scrollbar rounding
      });
    }
  }

  // FR-011 / Edge Cases: ultra-wide caps at 5 columns and stays centred within --container-dashboard.
  for (const route of ROUTES) {
    test(`${route} caps at 5 columns and stays centred at 3840px`, async ({ page }) => {
      await page.setViewportSize({ width: 3840, height: 1440 });
      await page.goto(route);

      const grid = page.locator('[class*="grid-cols-1"]').first();
      await expect(grid).toBeVisible();

      const { trackCount, centred } = await grid.evaluate((el) => {
        const style = getComputedStyle(el);
        const tracks = style.gridTemplateColumns.split(' ').filter(Boolean).length;
        const rect = el.getBoundingClientRect();
        const viewportWidth = document.documentElement.clientWidth;
        const leftGap = rect.left;
        const rightGap = viewportWidth - rect.right;
        return { trackCount: tracks, centred: Math.abs(leftGap - rightGap) <= 2 };
      });

      expect(trackCount).toBe(5);
      expect(centred).toBe(true);
    });
  }

  // T046 / SC-010: fit-mode pages fill the viewport with no page-level scroll at desktop sizes.
  for (const route of FIT_ROUTES) {
    for (const width of [1920, 2560]) {
      test(`${route} has no page-level scroll in fit mode at ${width}px`, async ({ page }) => {
        await page.setViewportSize({ width, height: 1080 });
        await page.goto(route);

        const overflowY = await page.evaluate(() => {
          const main = document.querySelector('main');
          if (!main) return 0;
          return main.scrollHeight - main.clientHeight;
        });

        expect(overflowY).toBeLessThanOrEqual(1);
      });
    }
  }

  // Edge Cases / FR-020: short viewports revert fit pages to normal document scroll rather
  // than clipping content below the 45rem (720px) height threshold.
  for (const route of FIT_ROUTES) {
    test(`${route} falls back to flow scrolling below the 45rem height threshold`, async ({ page }) => {
      await page.setViewportSize({ width: 1920, height: 600 });
      await page.goto(route);

      const main = page.locator('main');
      await expect(main).toBeVisible();
      const overflowStyle = await main.evaluate((el) => getComputedStyle(el).overflowY);
      expect(overflowStyle).not.toBe('hidden');
    });
  }
});
