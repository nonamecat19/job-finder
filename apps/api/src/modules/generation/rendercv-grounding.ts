import {
  GroundingLevel,
  RendercvMaster,
  masterSkillTokens,
  norm,
  tokens,
} from './rendercv-tailor';

/**
 * Post-merge grounding check for the rendercv path. Because {@link mergeTailored} only
 * ever replaces summary/skill-details/highlights and copies companies, dates, education
 * and design verbatim, the structural rules (no invented employer/date/degree) are
 * guaranteed by construction. The one machine-checkable content rule is the STRICT skill
 * constraint: no skill token may appear that is not already in the master. We still
 * defensively verify the merged experience companies are a subset of the master.
 */
export function verifyRendercvGrounding(
  master: RendercvMaster,
  merged: RendercvMaster,
  level: GroundingLevel,
): string[] {
  const violations: string[] = [];

  const masterCompanies = new Set(
    (master.cv?.sections?.experience ?? []).map((e) => norm(e.company)),
  );
  for (const e of merged.cv?.sections?.experience ?? []) {
    if (!masterCompanies.has(norm(e.company))) {
      violations.push(`company "${e.company}" not in master profile`);
    }
  }

  if (level === 'strict') {
    const allowed = masterSkillTokens(master);
    for (const g of merged.cv?.sections?.skills ?? []) {
      for (const t of tokens(g.details)) {
        if (!allowed.has(t)) {
          violations.push(`skill "${t}" (${g.label}) not in master profile (strict grounding)`);
        }
      }
    }
  }

  return violations;
}
