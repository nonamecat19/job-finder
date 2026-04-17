import { Global, Module } from '@nestjs/common';
import { LLM_PROVIDER, LlmProvider } from './llm.types';
import { OllamaProvider } from './ollama.provider';
import { CerebrasProvider } from './cerebras.provider';

@Global()
@Module({
  providers: [
    OllamaProvider,
    CerebrasProvider,
    {
      provide: LLM_PROVIDER,
      useFactory: (ollama: OllamaProvider, cerebras: CerebrasProvider): LlmProvider => {
        const provider = process.env.LLM_PROVIDER ?? 'ollama';
        switch (provider) {
          case 'ollama':
            return ollama;
          case 'cerebras':
            return cerebras;
          default:
            throw new Error(`Unknown LLM_PROVIDER '${provider}' — expected 'ollama' or 'cerebras'`);
        }
      },
      inject: [OllamaProvider, CerebrasProvider],
    },
  ],
  exports: [LLM_PROVIDER],
})
export class LlmModule {}
