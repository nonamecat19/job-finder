import { ZodSchema } from 'zod';

export interface CompleteOptions {
  system?: string;
  temperature?: number;
  maxTokens?: number;
}

export interface LlmProvider {
  readonly modelName: string;
  complete(prompt: string, opts?: CompleteOptions): Promise<string>;
  completeStructured<T>(prompt: string, schema: ZodSchema<T>, opts?: CompleteOptions): Promise<T>;
  embed(text: string): Promise<number[]>;
}

export const LLM_PROVIDER = Symbol('LLM_PROVIDER');
