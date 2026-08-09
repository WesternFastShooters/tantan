import { buildBetterAuthSessionTokenCookieHeader } from "@follow/shared/auth-cookie"
import { IN_ELECTRON } from "@follow/shared/constants"
import { env } from "@follow/shared/env.desktop"
import { whoami } from "@follow/store/user/getters"
import { userActions } from "@follow/store/user/store"
import { createDesktopAPIHeaders } from "@follow/utils/headers"
import { FollowClient } from "@follow-app/client-sdk"
import PKG from "@pkg"

import { setLoginModalShow } from "~/atoms/user"
import {
  isRemovedFoloRoute,
  removedFoloResponse,
} from "~/modules/tantan-policy/removed-folo-routes"

import { ipcServices } from "./client"
import { getAuthSessionToken, getClientId, getSessionId } from "./client-session"
import { getSessionCSRFToken } from "./tantan-api/client"

const isElectronRuntime = () => {
  return IN_ELECTRON || (typeof window !== "undefined" && !!window.electron)
}

const browserFoloAPIURL = () => {
  if (typeof window === "undefined") return "http://127.0.0.1:3000/api/folo"
  return new URL("/api/folo/", window.location.origin).toString()
}

const foloAPIURL = isElectronRuntime() ? env.VITE_API_URL : browserFoloAPIURL()

const fetchWithElectronAuth = async (request: Request) => {
  const requestURL = new URL(request.url)
  const electronRuntime = isElectronRuntime()
  const apiURL = new URL(foloAPIURL)
  if (!electronRuntime) {
    const publicPrefix = apiURL.pathname.endsWith("/") ? apiURL.pathname : `${apiURL.pathname}/`
    if (requestURL.origin !== apiURL.origin || !requestURL.pathname.startsWith(publicPrefix)) {
      return new Response(
        JSON.stringify({
          requestId: "browser-policy",
          error: {
            code: "FOLO_ROUTE_DENIED",
            message: "浏览器只允许同源 /api/folo 请求",
            retryable: false,
          },
        }),
        { status: 403, headers: { "Content-Type": "application/json" } },
      )
    }
  }
  if (electronRuntime && isRemovedFoloRoute(requestURL, apiURL.origin)) {
    return removedFoloResponse()
  }
  const authService = ipcServices?.auth as
    | (NonNullable<typeof ipcServices>["auth"] & {
        fetchWithAuth?: (payload: {
          body?: string
          headers?: Record<string, string>
          method: string
          url: string
        }) => Promise<{
          body: string
          headers: [string, string][]
          status: number
          statusText: string
        }>
      })
    | undefined

  if (!electronRuntime || requestURL.origin !== apiURL.origin || !authService?.fetchWithAuth) {
    return fetch(request)
  }

  const body =
    request.method !== "GET" && request.method !== "HEAD" ? await request.clone().text() : undefined
  const response = await authService.fetchWithAuth({
    body,
    headers: Object.fromEntries(request.headers.entries()),
    method: request.method,
    url: request.url,
  })

  return new Response(response.body, {
    headers: response.headers,
    status: response.status,
    statusText: response.statusText,
  })
}

export const followClient = new FollowClient({
  credentials: "include",
  timeout: 60_000,
  baseURL: foloAPIURL,
  fetch: async (input, options = {}) => {
    let requestURL = input.toString()
    if (!isElectronRuntime() && typeof window !== "undefined") {
      const parsed = new URL(requestURL)
      if (parsed.origin !== window.location.origin) {
        throw new Error("Folo browser requests must remain same-origin")
      }
      if (parsed.pathname !== "/api/folo" && !parsed.pathname.startsWith("/api/folo/")) {
        parsed.pathname = `/api/folo${parsed.pathname}`
      }
      requestURL = parsed.toString()
    }
    const request = new Request(requestURL, {
      ...options,
      cache: "no-store",
    })
    return fetchWithElectronAuth(request)
  },
})

export const followApi = followClient.api
followClient.addRequestInterceptor(async (ctx) => {
  const { options } = ctx
  const headers = new Headers(options.headers)
  headers.set("X-Client-Id", getClientId())
  headers.set("X-Session-Id", getSessionId())

  const authSessionToken = isElectronRuntime() ? getAuthSessionToken() : null
  if (authSessionToken && !headers.has("Cookie") && !headers.has("cookie")) {
    headers.set(
      "Cookie",
      buildBetterAuthSessionTokenCookieHeader(env.VITE_API_URL, authSessionToken),
    )
  }

  if (!isElectronRuntime()) {
    const method = String(options.method ?? "GET").toUpperCase()
    const csrfToken = getSessionCSRFToken()
    if (csrfToken && ["POST", "PUT", "PATCH", "DELETE"].includes(method)) {
      headers.set("X-CSRF-Token", csrfToken)
    }
  }

  const apiHeader = createDesktopAPIHeaders({ version: PKG.version })
  Object.entries(apiHeader).forEach(([key, value]) => {
    headers.set(key, value)
  })

  options.headers = Object.fromEntries(headers.entries())
  return ctx
})

followClient.addResponseInterceptor(async ({ response }) => {
  if (response.status === 401) {
    const authSessionToken = isElectronRuntime() ? getAuthSessionToken() : null
    const shouldPromptForLogin =
      response.url.includes("/better-auth/get-session") || (!whoami() && !authSessionToken)

    if (!shouldPromptForLogin) {
      return response
    }

    userActions.removeCurrentUser()
    if (isElectronRuntime()) setLoginModalShow(true)
    else if (typeof window !== "undefined" && window.location.pathname !== "/login") {
      window.location.assign(`/login?returnTo=${encodeURIComponent(window.location.pathname)}`)
    }
  }
  try {
    const isJSON = response.headers.get("content-type")?.includes("application/json")
    if (!isJSON) return response
    const _json = await response.clone().json()

    const isError = response.status >= 400
    if (!isError) return response
  } catch {
    // ignore
  }

  return response
})
