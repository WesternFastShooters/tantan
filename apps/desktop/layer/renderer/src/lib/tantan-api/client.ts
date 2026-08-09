import type {
  ErrorResponse,
  FoloAuthProvidersResponse,
  FoloEmailLoginRequest,
  FoloSocialStartRequest,
  FoloSocialStartResponse,
  FoloTokenLoginRequest,
  FoloTwoFactorChallengeResponse,
  FoloTwoFactorVerifyRequest,
  ReadinessResponse,
  SessionResponse,
} from "~/lib/tantan-api/gen/types"

let csrfToken: string | null = null

export const getSessionCSRFToken = () => csrfToken

const requestId = () =>
  globalThis.crypto?.randomUUID?.() ?? `web-${Date.now()}-${Math.random().toString(36).slice(2)}`

const timezone = () => Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC"

const isMutation = (method: string) =>
  method === "POST" || method === "PUT" || method === "PATCH" || method === "DELETE"

const isSafeAPIPath = (path: string) =>
  path.startsWith("/api/") &&
  !path.startsWith("//") &&
  !path.includes("\\") &&
  !/[\r\n\0]/u.test(path) &&
  !path.includes("://")

export class TantanAPIError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly code: string | null,
    readonly retryable = false,
    readonly response?: ErrorResponse,
  ) {
    super(message)
    this.name = "TantanAPIError"
  }
}

export const tantanRequest = async <T>(path: string, init: RequestInit = {}): Promise<T> => {
  if (!isSafeAPIPath(path)) {
    throw new Error("Tantan browser requests require a relative /api path")
  }
  const method = (init.method ?? "GET").toUpperCase()
  const headers = new Headers(init.headers)
  headers.set("Accept", "application/json")
  headers.set("X-Request-Id", requestId())
  headers.set("X-Tantan-Timezone", timezone())
  if (init.body && !headers.has("Content-Type")) headers.set("Content-Type", "application/json")
  if (isMutation(method) && csrfToken) headers.set("X-CSRF-Token", csrfToken)
  headers.delete("Authorization")

  const response = await fetch(path, {
    ...init,
    cache: "no-store",
    credentials: "include",
    headers,
    method,
  })

  if (!response.ok) {
    if (response.status === 401) csrfToken = null
    const errorBody = await response
      .clone()
      .json()
      .catch(() => null as ErrorResponse | null)
    throw new TantanAPIError(
      errorBody?.error.message ?? `Tantan API request failed (${response.status})`,
      response.status,
      errorBody?.error.code ?? null,
      errorBody?.error.retryable ?? false,
      errorBody ?? undefined,
    )
  }

  if (response.status === 204) return undefined as T
  return (await response.json()) as T
}

const rememberSession = (session: SessionResponse) => {
  csrfToken = session.csrfToken
  return session
}

const postJSON = <T>(path: string, body: unknown) =>
  tantanRequest<T>(path, { method: "POST", body: JSON.stringify(body) })

export const getReadiness = (signal?: AbortSignal) =>
  tantanRequest<ReadinessResponse>("/api/readyz", { signal })

export const getLocalSession = async (signal?: AbortSignal) =>
  rememberSession(await tantanRequest<SessionResponse>("/api/tantan/v1/session", { signal }))

export const getFoloAuthProviders = (signal?: AbortSignal) =>
  tantanRequest<FoloAuthProvidersResponse>("/api/auth/folo/providers", { signal })

export const startFoloSocialLogin = (body: FoloSocialStartRequest) =>
  postJSON<FoloSocialStartResponse>("/api/auth/folo/social-start", body)

export const signInWithFoloEmail = async (body: FoloEmailLoginRequest) =>
  rememberSession(await postJSON<SessionResponse>("/api/auth/folo/email", body))

export const verifyFoloTwoFactor = async (body: FoloTwoFactorVerifyRequest) =>
  rememberSession(await postJSON<SessionResponse>("/api/auth/folo/two-factor", body))

export const signInWithFoloToken = async (body: FoloTokenLoginRequest) =>
  rememberSession(await postJSON<SessionResponse>("/api/auth/folo/token", body))

export const logoutTantan = async () => {
  await tantanRequest<void>("/api/auth/logout", { method: "POST" })
  csrfToken = null
}

export const isFoloTwoFactorChallenge = (
  error: unknown,
): error is TantanAPIError & { response: FoloTwoFactorChallengeResponse } =>
  error instanceof TantanAPIError &&
  error.status === 409 &&
  error.code === "AUTH_2FA_REQUIRED" &&
  typeof (error.response as FoloTwoFactorChallengeResponse | undefined)?.challenge?.flowId ===
    "string"
