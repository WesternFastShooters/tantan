import type { ErrorResponse, ReadinessResponse, SessionResponse } from "~/lib/tantan-api/gen/types"

export const TANTAN_API_ORIGIN = "http://127.0.0.1:3000"

const requestId = () =>
  globalThis.crypto?.randomUUID?.() ?? `web-${Date.now()}-${Math.random().toString(36).slice(2)}`

const timezone = () => Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC"

export class TantanAPIError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly code: string | null,
  ) {
    super(message)
    this.name = "TantanAPIError"
  }
}

export const tantanRequest = async <T>(path: string, init: RequestInit = {}): Promise<T> => {
  const headers = new Headers(init.headers)
  headers.set("Accept", "application/json")
  headers.set("X-Request-Id", requestId())
  headers.set("X-Tantan-Timezone", timezone())
  if (init.body && !headers.has("Content-Type")) headers.set("Content-Type", "application/json")

  const response = await fetch(new URL(path, TANTAN_API_ORIGIN), {
    ...init,
    cache: "no-store",
    credentials: "include",
    headers,
  })

  if (!response.ok) {
    const errorBody = await response
      .clone()
      .json()
      .catch(() => null as ErrorResponse | null)
    throw new TantanAPIError(
      errorBody?.error.message ?? `Local API request failed (${response.status})`,
      response.status,
      errorBody?.error.code ?? null,
    )
  }

  if (response.status === 204) return undefined as T
  return (await response.json()) as T
}

export const getReadiness = (signal?: AbortSignal) =>
  tantanRequest<ReadinessResponse>("/readyz", { signal })

export const getLocalSession = (signal?: AbortSignal) =>
  tantanRequest<SessionResponse>("/tantan/v1/session", { signal })
