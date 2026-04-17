import { JsonResume } from '@job-finder/shared';

/**
 * Post-generation grounding check: every employer, institution, degree and date
 * in the tailored resume must already exist in the master profile. The LLM may
 * reorder/rephrase/omit — never invent.
 */
export function verifyGrounding(master: JsonResume, tailored: JsonResume): string[] {
  const violations: string[] = [];

  const employers = new Set((master.work ?? []).map((w) => norm(w.name)));
  const workDates = new Set(
    (master.work ?? []).flatMap((w) => [w.startDate, w.endDate].filter(Boolean).map((d) => norm(d!))),
  );
  const institutions = new Set((master.education ?? []).map((e) => norm(e.institution)));
  const eduDates = new Set(
    (master.education ?? []).flatMap((e) => [e.startDate, e.endDate].filter(Boolean).map((d) => norm(d!))),
  );
  const projectNames = new Set((master.projects ?? []).map((p) => norm(p.name)));

  for (const w of tailored.work ?? []) {
    if (!employers.has(norm(w.name))) {
      violations.push(`employer "${w.name}" not in master profile`);
    }
    for (const d of [w.startDate, w.endDate]) {
      if (d && !workDates.has(norm(d))) {
        violations.push(`work date "${d}" (${w.name}) not in master profile`);
      }
    }
  }

  for (const e of tailored.education ?? []) {
    if (!institutions.has(norm(e.institution))) {
      violations.push(`institution "${e.institution}" not in master profile`);
    }
    for (const d of [e.startDate, e.endDate]) {
      if (d && !eduDates.has(norm(d))) {
        violations.push(`education date "${d}" (${e.institution}) not in master profile`);
      }
    }
  }

  for (const p of tailored.projects ?? []) {
    if (!projectNames.has(norm(p.name))) {
      violations.push(`project "${p.name}" not in master profile`);
    }
  }

  return violations;
}

function norm(s: string): string {
  return s.toLowerCase().replace(/\s+/g, ' ').trim();
}
