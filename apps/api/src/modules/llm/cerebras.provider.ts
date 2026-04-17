import { Injectable, Logger } from '@nestjs/common';
import axios, { AxiosInstance } from 'axios';
import { ZodSchema } from 'zod';
import { zodToJsonSchema } from 'zod-to-json-schema';
import { CompleteOptions, LlmProvider } from './llm.types';
import { OllamaProvider } from './ollama.provider';

const STRUCTURED_RETRIES = 2;

/**
 * Cerebras inference (OpenAI-compatible `/v1/chat/completions`). Very fast hosted models
 * (llama-3.3-70b, qwen-3-32b, ...). Cerebras has no embeddings API, so `embed` is delegated
 * to the local Ollama provider — chat runs on Cerebras, embeddings stay local for matching.
 */
@Injectable()
export class CerebrasProvider implements LlmProvider {
  private readonly logger = new Logger(CerebrasProvider.name);
  private readonly http: AxiosInstance;
  readonly modelName: string;

  constructor(private readonly ollama: OllamaProvider) {
    const apiKey = process.env.CEREBRAS_API_KEY;
    if (!apiKey) throw new Error('CEREBRAS_API_KEY is required when LLM_PROVIDER=cerebras');
    this.http = axios.create({
      baseURL: process.env.CEREBRAS_URL ?? 'https://api.cerebras.ai/v1',
      timeout: 120_000,
      headers: { Authorization: `Bearer ${apiKey}` },
    });
    this.modelName = process.env.CEREBRAS_MODEL ?? 'gpt-oss-120b';
  }

  async complete(prompt: string, opts?: CompleteOptions): Promise<string> {
    const res = await this.http.post('/chat/completions', {
      model: this.modelName,
      stream: false,
      messages: [
        ...(opts?.system ? [{ role: 'system', content: opts.system }] : []),
        { role: 'user', content: prompt },
      ],
      temperature: opts?.temperature ?? 0.3,
      ...(opts?.maxTokens ? { max_completion_tokens: opts.maxTokens } : {}),
    });
    return res.data.choices?.[0]?.message?.content ?? '';
  }

  async completeStructured<T>(prompt: string, schema: ZodSchema<T>, opts?: CompleteOptions): Promise<T> {
    // `as any`: zod generic inference blows TS instantiation depth here
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const jsonSchema = zodToJsonSchema(schema as any, 'output') as {
      definitions?: Record<string, unknown>;
    };
    const schemaText = JSON.stringify(jsonSchema.definitions?.output ?? jsonSchema);
    let lastError = '';
    for (let attempt = 0; attempt <= STRUCTURED_RETRIES; attempt++) {
      const messages = [
        ...(opts?.system ? [{ role: 'system', content: opts.system }] : []),
        {
          role: 'user',
          content:
            `${prompt}\n\nRespond with a single JSON object matching this JSON Schema:\n${schemaText}\n` +
            (lastError
              ? `\nYour previous answer was invalid: ${lastError}\nFix it and answer again with valid JSON only.`
              : ''),
        },
      ];
      const res = await this.http.post('/chat/completions', {
        model: this.modelName,
        stream: false,
        response_format: { type: 'json_object' },
        messages,
        temperature: opts?.temperature ?? 0.1,
      });
      const text: string = res.data.choices?.[0]?.message?.content ?? '';
      try {
        const parsed = JSON.parse(this.stripFences(text));
        const result = schema.safeParse(parsed);
        if (result.success) return result.data;
        lastError = JSON.stringify(result.error.issues.slice(0, 5));
      } catch (e) {
        lastError = `not valid JSON: ${(e as Error).message}`;
      }
      this.logger.warn(`structured output attempt ${attempt + 1} failed: ${lastError}`);
    }
    throw new Error(`LLM structured output failed after ${STRUCTURED_RETRIES + 1} attempts: ${lastError}`);
  }

  /** Cerebras offers no embeddings endpoint — use the local Ollama embedder. */
  async embed(text: string): Promise<number[]> {
    return this.ollama.embed(text);
  }

  private stripFences(text: string): string {
    const t = text.trim();
    const fence = t.match(/^```(?:json)?\s*([\s\S]*?)\s*```$/);
    return fence ? fence[1] : t;
  }
}
