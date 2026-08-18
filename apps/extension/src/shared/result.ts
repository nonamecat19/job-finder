
export type Result<T> = { ok: true; value: T } | { ok: false; error: AppError };

export type ErrorCode =
  | 'api_unreachable'
  | 'not_found'
  | 'pdf_not_ready'
  | 'no_adapter'
  | 'form_not_open'
  | 'no_file_input'
  | 'no_letter_field'
  | 'bad_request'
  | 'unknown';

export type AppError = {
  code: ErrorCode;

  message: string;

  detail?: string;
};

export function ok<T>(value: T): Result<T> {
  return { ok: true, value };
}

export function err<T = never>(code: ErrorCode, message: string, detail?: string): Result<T> {
  return { ok: false, error: { code, message, detail } };
}

export async function attempt<T>(fn: () => Promise<Result<T>>): Promise<Result<T>> {
  try {
    return await fn();
  } catch (e) {
    return err('unknown', e instanceof Error ? e.message : String(e), e instanceof Error ? e.stack : undefined);
  }
}
