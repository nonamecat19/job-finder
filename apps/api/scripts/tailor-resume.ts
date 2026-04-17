/**
 * Standalone CLI to tailor the RenderCV master to a pasted vacancy — no Nest/DB context,
 * just Ollama + rendercv. This is the opencode `/tailor-resume` fallback when the API is down.
 *
 *   RESUME_GROUNDING_LEVEL=moderate \
 *   pnpm --filter @job-finder/api exec ts-node scripts/tailor-resume.ts "<vacancy text or file path>"
 */
import { existsSync, readFileSync } from 'fs';
import * as path from 'path';
import { parse as yamlParse } from 'yaml';
import { OllamaProvider } from '../src/modules/llm/ollama.provider';
import { CerebrasProvider } from '../src/modules/llm/cerebras.provider';
import { LlmProvider } from '../src/modules/llm/llm.types';
import { RenderCvRenderer } from '../src/modules/generation/rendercv-renderer';
import {
  GroundingLevel,
  RendercvMaster,
  buildTailorPrompt,
  mergeTailored,
  tailoredSectionsSchema,
} from '../src/modules/generation/rendercv-tailor';
import { verifyRendercvGrounding } from '../src/modules/generation/rendercv-grounding';

const GROUNDING_ATTEMPTS = 2;

function resolveLevel(): GroundingLevel {
  const l = process.env.RESUME_GROUNDING_LEVEL;
  return l === 'strict' || l === 'aggressive' ? l : 'moderate';
}

async function main() {
  const arg = process.argv.slice(2).join(' ').trim();
  if (!arg) throw new Error('usage: ts-node scripts/tailor-resume.ts "<vacancy text or file path>"');
  const vacancy = existsSync(arg) ? readFileSync(arg, 'utf8') : arg;

  const masterPath =
    process.env.RESUME_MASTER_PATH ?? path.resolve(process.cwd(), '../../resume/resume.yaml');
  const master = yamlParse(readFileSync(masterPath, 'utf8')) as RendercvMaster;

  const level = resolveLevel();
  const ollama = new OllamaProvider();
  const llm: LlmProvider = process.env.LLM_PROVIDER === 'cerebras' ? new CerebrasProvider(ollama) : ollama;
  const renderer = new RenderCvRenderer();

  let violations: string[] = [];
  let merged: RendercvMaster | null = null;
  for (let attempt = 0; attempt < GROUNDING_ATTEMPTS; attempt++) {
    const suffix = violations.length
      ? `\n\nYour previous attempt violated grounding rules:\n- ${violations.join('\n- ')}\nRegenerate without these violations.`
      : '';
    const payload = await llm.completeStructured(
      buildTailorPrompt(master, vacancy, level) + suffix,
      tailoredSectionsSchema,
      { system: 'You are an expert resume writer who never fabricates information.' },
    );
    merged = mergeTailored(master, payload);
    violations = verifyRendercvGrounding(master, merged, level);
    if (violations.length === 0) break;
    console.error(`grounding violations (attempt ${attempt + 1}): ${violations.join('; ')}`);
    merged = null;
  }
  if (!merged) throw new Error(`failed grounding check: ${violations.join('; ')}`);

  const { yamlPath, pdfPath } = await renderer.render(merged, `tailored-${Date.now()}`);
  console.log(JSON.stringify({ yamlPath, pdfPath, groundingLevel: level }, null, 2));
}

main().catch((e) => {
  console.error(e instanceof Error ? e.message : e);
  process.exit(1);
});
