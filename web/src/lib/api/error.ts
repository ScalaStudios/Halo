export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
  ) {
    super(message);
  }
}

export async function toApiError(res: Response, fallback: string): Promise<ApiError> {
  const body = (await res.json().catch(() => null)) as { error?: { code?: string; message?: string } } | null;
  return new ApiError(res.status, body?.error?.code ?? "ERR_INTERNAL", body?.error?.message ?? fallback);
}
