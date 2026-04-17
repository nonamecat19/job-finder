import { Injectable, Logger } from '@nestjs/common';
import axios, { AxiosInstance } from 'axios';
import { ZodSchema } from 'zod';
import { zodToJsonSchema } from 'zod-to-json-schema';
import { CompleteOptions, LlmProvider } from './llm.types';

const STRUCTURED_RETRIES = 2;

@Injectable()
export class OllamaProvider implements LlmProvider {
  private readonly logger = new Logger(OllamaProvider.name);
  private readonly http: AxiosInstance;
  readonly modelName: string;
  private readonly embedModel: string;

  constructor() {
    this.http = axios.create({
      baseURL: process.env.OLLAMA_URL ?? 'http://localhost:11434',
      timeout: 300_000, // local models are slow
    });
    this.modelName = process.env.LLM_MODEL ?? 'qwen2.5:14b';
    this.embedModel = process.env.EMBED_MODEL ?? 'nomic-embed-text';
  }

  async complete(prompt: string, opts?: CompleteOptions): Promise<string> {
    const res = await this.http.post('/api/chat', {
      model: this.modelName,
      stream: false,
      messages: [
        ...(opts?.system ? [{ role: 'system', content: opts.system }] : []),
        { role: 'user', content: prompt },
      ],
      options: {
        temperature: opts?.temperature ?? 0.3,
        ...(opts?.maxTokens ? { num_predict: opts.maxTokens } : {}),
      },
    });
    return res.data.message?.content ?? '';
  }

  async completeStructured<T>(prompt: string, schema: ZodSchema<T>, opts?: CompleteOptions): Promise<T> {
    // `as any`: zod generic inference blows TS instantiation depth here
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const jsonSchema = zodToJsonSchema(schema as any, 'output') as {
      definitions?: Record<string, unknown>;
    };
    let lastError = '';
    for (let attempt = 0; attempt <= STRUCTURED_RETRIES; attempt++) {
      const messages = [
        ...(opts?.system ? [{ role: 'system', content: opts.system }] : []),
        {
          role: 'user',
          content:
            `${prompt}\n\nRespond with a single JSON object matching this JSON Schema:\n` +
            `${JSON.stringify(jsonSchema.definitions?.output ?? jsonSchema)}\n` +
            (lastError
              ? `\nYour previous answer was invalid: ${lastError}\nFix it and answer again with valid JSON only.`
              : ''),
        },
      ];
      const res = await this.http.post('/api/chat', {
        model: this.modelName,
        stream: false,
        format: 'json',
        messages,
        options: { temperature: opts?.temperature ?? 0.1 },
      });
      const text: string = res.data.message?.content ?? '';
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

  async embed(text: string): Promise<number[]> {
    const res = await this.http.post('/api/embeddings', {
      model: this.embedModel,
      prompt: text.slice(0, 8000),
    });
    const embedding: number[] = res.data.embedding;
    if (!Array.isArray(embedding) || embedding.length === 0) {
      throw new Error('Ollama returned empty embedding');
    }
    return embedding;
  }

  private stripFences(text: string): string {
    const t = text.trim();
    const fence = t.match(/^```(?:json)?\s*([\s\S]*?)\s*```$/);
    return fence ? fence[1] : t;
  }
}
