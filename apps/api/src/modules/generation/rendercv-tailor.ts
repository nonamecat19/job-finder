import { z } from 'zod';

/**
 * RenderCV master is a hand-tuned YAML config (theme, templates, locale, design,
 * settings + the actual CV content). We only ever tailor the *content* sections and
 * leave every structural/design field byte-for-byte intact, so the polished
 * `harvard`-theme output is preserved. Typed loosely because the master carries many
 * rendercv-specific fields we deliberately pass through untouched.
 */
export type RendercvMaster = {
  cv?: {
    sections?: {
      summary?: string[];
      skills?: { label: string; details: string }[];
      experience?: {
        company: string;
        position?: string;
        start_date?: string;
        end_date?: string;
        highlights?: string[];
        [k: string]: unknown;
      }[];
      [k: string]: unknown;
    };
    [k: string]: unknown;
  };
  [k: string]: unknown;
};

export type GroundingLevel = 'strict' | 'moderate' | 'aggressive';

/**
 * The only content the LLM is allowed to produce. Everything else in the master
 * (companies, dates, education, design, templates, locale, header) is preserved by
 * {@link mergeTailored}, never sent back by the model.
 */
export const tailoredSectionsSchema = z.object({
  summary: z.string().describe('one-paragraph professional summary rewritten for the vacancy'),
  skills: z
    .array(
      z.object({
        index: z.number().int().describe('0-based index of the skill group in the master, unchanged'),
        details: z.string().describe('comma-separated skills for this group, reordered/tailored'),
      }),
    )
    .describe('one entry per master skill group, same indexes'),
  experience: z
    .array(
      z.object({
        company: z.string().describe('company name copied EXACTLY from the master'),
        highlights: z.array(z.string()).describe('selected, reordered, rephrased highlights'),
      }),
    )
    .describe('one entry per master experience entry, keyed by company'),
});

export type TailoredSections = z.infer<typeof tailoredSectionsSchema>;

const LEVEL_RULES: Record<GroundingLevel, string> = {
  strict:
    'GROUNDING = STRICT. Use ONLY skills and facts already present in the master profile. ' +
    'You may reorder, trim and rephrase, but you must NOT introduce any technology, tool or ' +
    'skill token that does not already appear in the master. Do not invent achievements.',
  moderate:
    'GROUNDING = MODERATE. You may reorder, trim and rephrase freely, and you MAY add a skill ' +
    'or reframe a highlight for technology that is directly ADJACENT to the existing stack ' +
    '(e.g. the vacancy asks for Terraform and the master already lists AWS, Docker and ' +
    'Kubernetes). Never add technology unrelated to the demonstrated experience, and never ' +
    'invent employers, dates, projects or metrics.',
  aggressive:
    'GROUNDING = AGGRESSIVE. Maximize keyword match with the vacancy: you may add any skills the ' +
    'vacancy requires and frame highlights toward them. Still never invent employers, dates, ' +
    'degrees or numeric metrics that are not in the master.',
};

export function buildTailorPrompt(
  master: RendercvMaster,
  vacancy: string,
  level: GroundingLevel,
): string {
  const sections = master.cv?.sections ?? {};
  const skills = sections.skills ?? [];
  const experience = sections.experience ?? [];

  const skillList = skills
    .map((s, i) => `  [${i}] ${s.label}: ${s.details}`)
    .join('\n');
  const expList = experience
    .map(
      (e) =>
        `  - company: ${e.company}${e.position ? ` (${e.position})` : ''}\n` +
        (e.highlights ?? []).map((h) => `      • ${h}`).join('\n'),
    )
    .join('\n');
  const currentSummary = (sections.summary ?? []).join(' ');

  return (
    `Tailor this candidate's resume content to the target vacancy by selecting, reordering and ` +
    `rephrasing what the master profile already contains.\n\n` +
    `${LEVEL_RULES[level]}\n\n` +
    `HARD RULES (all levels):\n` +
    `- Return skills as one entry per group, using the SAME [index] shown below.\n` +
    `- Return experience keyed by the EXACT company name shown below; do not add or drop companies.\n` +
    `- Emphasize what the vacancy asks for; move the most relevant items first.\n` +
    `- Keep highlights concise, one achievement each, no fabricated numbers.\n\n` +
    `CURRENT SUMMARY:\n${currentSummary}\n\n` +
    `SKILL GROUPS:\n${skillList}\n\n` +
    `EXPERIENCE:\n${expList}\n\n` +
    `TARGET VACANCY:\n${vacancy.slice(0, 6000)}`
  );
}

const norm = (s: string): string => s.toLowerCase().replace(/\s+/g, ' ').trim();

/** Split a comma/slash-separated details string into individual skill tokens. */
function tokens(details: string): string[] {
  return details
    .split(/[,/]/)
    .map((t) => norm(t))
    .filter(Boolean);
}

/**
 * Deep-clone the master and overwrite ONLY summary, skill details and experience
 * highlights. Companies, positions, dates, education, design, templates, locale,
 * settings and header are taken verbatim from the master — the LLM never touches them.
 */
export function mergeTailored(master: RendercvMaster, payload: TailoredSections): RendercvMaster {
  const merged: RendercvMaster = structuredClone(master);
  const sections = merged.cv?.sections;
  if (!sections) return merged;

  sections.summary = [payload.summary.trim()];

  const skills = sections.skills ?? [];
  for (const s of payload.skills) {
    if (s.index >= 0 && s.index < skills.length) {
      skills[s.index] = { ...skills[s.index], details: s.details.trim() };
    }
  }

  const experience = sections.experience ?? [];
  const byCompany = new Map(experience.map((e) => [norm(e.company), e]));
  for (const e of payload.experience) {
    const target = byCompany.get(norm(e.company));
    if (target) target.highlights = e.highlights.map((h) => h.trim()).filter(Boolean);
  }

  return merged;
}

/** Union of every skill token present in the master (across all groups). */
export function masterSkillTokens(master: RendercvMaster): Set<string> {
  const set = new Set<string>();
  for (const g of master.cv?.sections?.skills ?? []) {
    for (const t of tokens(g.details)) set.add(t);
  }
  return set;
}

export { norm, tokens };
