# Job Finder UI Kit — how to build with it

A dark-first Tailwind v4 kit from the Job Finder dashboard. Components are plain
React functions styled with Tailwind utilities that resolve to CSS custom
properties. There is no theme-object API and no `styled` layer — you style your own
layout with the same utility vocabulary the components use.

## Always wrap the screen in `ThemeRoot`

Every colour in this system comes from CSS custom properties declared on `:root`
(dark) and re-declared under `[data-theme='light']`. A screen that is not painted
with the app background renders dark text on white and looks broken.

```jsx
<ThemeRoot>            {/* theme="light" switches the whole subtree */}
  <Surface>
    <h3 className="text-base font-semibold text-foreground">Senior Backend Engineer</h3>
    <p className="mt-2 text-sm text-muted">Acme Corp · Berlin, DE · Hybrid</p>
    <div className="mt-3 flex flex-wrap gap-1.5">
      <Chip tone="green">Go</Chip>
      <Chip tone="slate">Kafka</Chip>
    </div>
    <div className="mt-4 flex justify-end gap-2">
      <Button variant="ghost">Discard</Button>
      <Button variant="primary">Shortlist</Button>
    </div>
  </Surface>
</ThemeRoot>
```

`ThemeRoot` sets `background`, `color`, `color-scheme` and the Inter type stack.
Mount `ToastProvider` once near the root as well if the screen raises toasts.

## The styling idiom: Tailwind utilities over semantic tokens

Never use raw Tailwind palette colours (`bg-slate-800`, `text-gray-400`) — they
ignore the theme. Use the semantic families below; they are the whole colour
vocabulary of this system.

| Family | Utilities | Use for |
|---|---|---|
| Page | `bg-background`, `bg-background-secondary` | the canvas behind everything |
| Panels | `bg-surface`, `bg-surface-secondary`, `bg-surface-tertiary` | cards, controls, inset fills |
| Text | `text-foreground`, `text-muted`, `text-faint` | primary / secondary / tertiary copy |
| Lines | `border-border`, `border-border-strong` | card borders, dividers, control outlines |
| Accent | `bg-accent`, `text-accent`, `bg-accent-soft`, `text-accent-foreground`, `ring-accent` | primary action, focus, selection |
| Status | `success`, `warning`, `danger` in the same shapes — `bg-danger`, `text-danger`, `bg-danger-soft`, `ring-danger/30` | scores, health, errors |

Conventions the components themselves follow, worth matching in your own glue:
soft status fills are `bg-*-soft` + `text-*` + `ring-1 ring-inset ring-*/25..30`;
cards are `rounded-xl border border-border bg-surface p-4`; controls are
`rounded-lg border border-border bg-surface-secondary px-3 py-2 text-sm`; field
labels are `text-xs font-semibold uppercase tracking-wide text-faint`.

Underneath, those utilities read `var(--background)`, `var(--foreground)`,
`var(--surface)`, `var(--surface-secondary)`, `var(--surface-tertiary)`,
`var(--muted)`, `var(--faint)`, `var(--border)`, `var(--border-strong)`,
`var(--separator)`, `var(--overlay)`, `var(--accent)`, `var(--accent-soft)`,
`var(--focus)`, `var(--success)`, `var(--warning)`, `var(--danger)`, their
`*-soft` and `*-foreground` variants, plus `var(--radius)` and `var(--font-sans)`.
Reach for `var(--*)` directly only in inline styles where a utility can't express
the value.

## Where the truth lives

- `_ds/<folder>/styles.css` and its imports (`_ds_bundle.css`, `fonts/fonts.css`) —
  every token declaration and every emitted utility. Read it before inventing a class:
  a utility that isn't in there will not resolve.
- `components/<group>/<Name>/<Name>.prompt.md` and `<Name>.d.ts` — the per-component
  API and usage reference.

## Composition notes

- Form controls (`Input`, `Textarea`, `Select`, `Checkbox`) are thin wrappers over the
  native element and forward every native prop. They carry no label — wrap them in
  `Field` for the label treatment.
- `Stepper` and `RangeSlider` are controlled: they take `value`/`valueMin`/`valueMax`
  and require `onChange` handlers; they hold no internal state.
- `GhostBadge` renders `null` below a score of 50, and `ScoreBadge` renders an em dash
  for `null`/`undefined` — both are deliberate, don't guard around them.
- `Surface` caps at `max-w-3xl` and accepts `as` to change the element (`article`,
  `section`, …).
