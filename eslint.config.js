// Flat ESLint config for the whole workspace (specs/023-workflow-quality-gates).
//
// Lives at the repository root, not inside apps/dashboard, because it must
// also cover packages/shared with a *different* rule (ignore the
// tygo-generated file there) than the dashboard gets. Two configs that must
// agree about a shared package is exactly the duplication pattern feature
// 024 exists to remove elsewhere — one root config with per-package
// overrides avoids introducing a new instance of it here.
//
// Type-aware rules are deliberately NOT enabled: they require a `project`
// service and are markedly slower, which would blow the <60s budget
// (SC-004). `tsc --noEmit` already runs in CI (frontend typecheck) and
// covers type errors; this config covers style/correctness lint only.
import js from "@eslint/js";
import tseslint from "typescript-eslint";
import reactHooks from "eslint-plugin-react-hooks";
import reactRefresh from "eslint-plugin-react-refresh";

export default tseslint.config(
  {
    // Generated/build output is never held to hand-written-code rules
    // (FR-010) — a linter that flagged sqlcgen-equivalent generated.ts
    // would produce permanent, unfixable noise.
    ignores: [
      "**/dist/**",
      "**/node_modules/**",
      "packages/shared/src/generated.ts",
      "apps/api/**",
    ],
  },
  js.configs.recommended,
  ...tseslint.configs.recommended,
  {
    files: ["apps/dashboard/**/*.{ts,tsx}", "packages/shared/**/*.{ts,tsx}"],
    languageOptions: {
      ecmaVersion: 2022,
      sourceType: "module",
    },
    rules: {
      // Non-null assertions and `any` show up throughout the existing
      // codebase; tightening these is a separate, deliberate follow-up
      // rather than something this adoption change forces through.
      "@typescript-eslint/no-explicit-any": "off",
      "@typescript-eslint/no-non-null-assertion": "off",
      "@typescript-eslint/no-unused-vars": [
        "error",
        { argsIgnorePattern: "^_", varsIgnorePattern: "^_" },
      ],
    },
  },
  {
    files: ["apps/dashboard/**/*.{ts,tsx}"],
    plugins: {
      "react-hooks": reactHooks,
      "react-refresh": reactRefresh,
    },
    rules: {
      ...reactHooks.configs.recommended.rules,
      "react-refresh/only-export-components": [
        "warn",
        { allowConstantExport: true },
      ],
    },
  },
  {
    files: ["**/*.test.{ts,tsx}", "**/test/**/*.{ts,tsx}", "**/tests/**/*.{ts,tsx}"],
    rules: {
      // Test doubles/fixtures routinely export non-component helpers
      // alongside components; the fast-refresh rule doesn't apply to files
      // Vite never hot-reloads.
      "react-refresh/only-export-components": "off",
    },
  },
);
