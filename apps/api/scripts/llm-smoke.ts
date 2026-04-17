/**
 * Smoke test for the Ollama provider. Run with the ollama container up:
 *   pnpm --filter @job-finder/api llm:smoke
 */
import { z } from 'zod';
import { OllamaProvider } from '../src/modules/llm/ollama.provider';

async function main() {
  const llm = new OllamaProvider();

  console.log('model:', llm.modelName);

  const text = await llm.complete('Reply with the single word: pong');
  console.log('complete:', text.trim());

  const schema = z.object({
    language: z.string(),
    isCompiled: z.boolean(),
  });
  const structured = await llm.completeStructured(
    'TypeScript: what language does it compile to, and is it a compiled language?',
    schema,
  );
  console.log('structured:', structured);

  const emb = await llm.embed('senior backend engineer, node.js, postgres');
  console.log('embedding dims:', emb.length);
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
