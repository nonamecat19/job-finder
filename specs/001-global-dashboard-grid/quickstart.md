# Quickstart: Validating the Global Dashboard Grid Layout

**Feature**: Global Dashboard Grid Layout  
**Date**: 2026-07-28  
**Purpose**: Runnable validation scenarios to prove the grid layout works end-to-end across all dashboard pages.

---

## Prerequisites

1. **Backend running**: `make run-backend` (or `docker compose up` for Postgres/Redis/API)
2. **Frontend running**: `make run-frontend` (Vite dev server on `http://localhost:5173`)
3. **Seed data**: At least one job in the feed so you can navigate to a job detail page
4. **Browser with DevTools**: Chrome, Firefox, or Edge with responsive design mode

---

## Scenario 1: Job Detail Page Grid (Primary Validation)

**Goal**: Verify the densest grid layout — the job detail page — renders correctly across all breakpoints.

### Steps

1. Navigate to any job detail page (e.g., `http://localhost:5173/jobs/{any-job-id}`)
2. Open DevTools → Toggle Device Toolbar
3. **Desktop (1280×800)**:
   - Verify 3-column grid is active
   - Verify wide cards (Fit Summary, Ghost Signal, Keyword Diff, Prep Pack) span 2 columns
   - Verify narrow cards (Contacts, Company Intel, Coach, Outreach) sit in 1 column
   - Verify no horizontal scroll (check `document.body.scrollWidth === window.innerWidth`)
   - Verify at least 3 cards are visible without scrolling
4. **Tablet (820×1180)**:
   - Verify grid collapses to 2 columns
   - Verify wide cards become full-width (2 columns)
   - Verify narrow cards pair side-by-side
   - Verify no text truncation or button clipping
5. **Mobile (375×667)**:
   - Verify single-column layout
   - Verify all cards are full-width with safe padding
   - Verify no horizontal scroll
   - Verify all interactive elements (buttons, links, dropdowns) remain tappable

### Expected Outcome

- Grid reflows smoothly at each breakpoint
- No horizontal overflow at any width
- All cards maintain internal padding and Surface styling
- Content is not clipped, truncated, or hidden

---

## Scenario 2: Cross-Page Consistency

**Goal**: Verify all 9 pages use the same grid system and feel visually consistent.

### Steps

1. Visit each page in sequence:
   - Feed (`/`)
   - Job Detail (`/jobs/:id`)
   - Tracker (`/tracker`)
   - Tailor (`/tailor`)
   - Contacts (`/contacts`)
   - Status (`/status`)
   - Sources (`/sources`)
   - Profile (`/profile`)
   - Settings (`/settings`)
2. At each page, check:
   - Gap spacing between cards is identical (~20px on desktop)
   - Card border radius is identical (`rounded-xl`)
   - Card background is identical (`bg-surface`)
   - Grid container behavior is consistent (same breakpoints, same column logic)

### Expected Outcome

- A user can switch between any two pages and perceive the layout as part of the same design system
- No page uses a custom one-off layout that breaks the rhythm

---

## Scenario 3: Virtualized List Coexistence

**Goal**: Verify the feed page's virtualized job list still works correctly inside the grid.

### Steps

1. Navigate to the feed page (`/`)
2. Scroll through the job list (load at least 50 jobs)
3. Verify smooth scrolling with no jank
4. Verify job cards render correctly within the virtualized list container
5. Verify the list container itself is a proper grid item (if placed alongside summary cards)

### Expected Outcome

- Virtualization remains functional; rows are recycled on scroll
- Job cards inside the virtualized list maintain their existing styling
- No console errors from TanStack Virtual

---

## Scenario 4: Sidebar Coexistence

**Goal**: Verify the grid does not interfere with the sticky sidebar.

### Steps

1. On desktop (≥768px), navigate to any page
2. Scroll the main content area down
3. Verify the left sidebar remains sticky and does not scroll with the content
4. Verify the grid content does not overlap or slide under the sidebar
5. Resize the browser slowly from 1920px down to 320px
6. Verify the sidebar collapses to top-header at ≤768px without breaking the grid

### Expected Outcome

- Sidebar and grid are independent layout regions
- No z-index conflicts or overlap
- Smooth transition between sidebar modes

---

## Scenario 5: New Page Integration (Developer Experience)

**Goal**: Verify a developer can add a new page using the grid primitives without custom CSS.

### Steps (Manual Test)

1. Create a mock page component:
   ```tsx
   import { DashboardGrid, GridCard } from '@/components/layout';
   import { Surface } from '@/components/ui';

   export function MockPage() {
     return (
       <DashboardGrid>
         <GridCard span="wide">
           <Surface>Wide card content</Surface>
         </GridCard>
         <GridCard span="narrow">
           <Surface>Narrow card content</Surface>
         </GridCard>
       </DashboardGrid>
     );
   }
   ```
2. Render it in a route
3. Verify it automatically gets the 3-column desktop / 2-column tablet / 1-column mobile behavior
4. Verify no additional layout CSS was needed

### Expected Outcome

- Zero custom CSS required for the new page to achieve consistent grid behavior
- Props `span="wide"` / `span="narrow"` correctly map to responsive column spans

---

## Automated Regression Check (Optional)

If Playwright or Cypress e2e tests exist, add a layout audit test:

```ts
// Pseudo-test for scroll-width validation
test('no horizontal scroll on any page', async ({ page }) => {
  const paths = ['/', '/jobs/test-id', '/tracker', '/contacts', '/sources', '/profile', '/settings', '/status', '/tailor'];
  for (const path of paths) {
    await page.goto(path);
    const scrollWidth = await page.evaluate(() => document.body.scrollWidth);
    const viewportWidth = await page.evaluate(() => window.innerWidth);
    expect(scrollWidth).toBeLessThanOrEqual(viewportWidth);
  }
});
```

---

## Troubleshooting

| Symptom | Likely Cause | Fix |
|---------|-------------|-----|
| Horizontal scroll on mobile | Card content exceeds grid cell width | Add `min-w-0` or `overflow-hidden` to GridCard wrapper; check for unwrapped `<pre>` or long URLs |
| Cards overlap on desktop | Incorrect `col-span` or source order | Verify wide cards are placed where they have room to span; source order must match visual order |
| Virtualized list broken on feed | GridCard interferes with absolute positioning | Keep VirtualList as a single GridCard span="full"; do NOT grid individual list rows |
| Sidebar pushes grid off-screen | Grid container missing max-width constraint | Grid lives inside `<main>` which has `min-w-0`; ensure no fixed-width children inside grid cards |
| Inconsistent gaps between pages | Page using custom gap class instead of DashboardGrid | Refactor to use `<DashboardGrid>` consistently; remove per-page `grid gap-*` classes |
