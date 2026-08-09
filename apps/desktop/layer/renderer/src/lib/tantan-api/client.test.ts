import { afterEach, describe, expect, test, vi } from "vitest"

import { getLocalSession, getReadiness, TANTAN_API_ORIGIN } from "./client"

describe("Tantan local API client", () => {
  afterEach(() => vi.unstubAllGlobals())

  test("REQ:FE-02 sends readiness only to loopback with private request headers", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          ready: true,
          checks: { sqlite: "ok", migrations: "ok", keychain: "ok" },
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ),
    )
    vi.stubGlobal("fetch", fetchMock)

    await expect(getReadiness()).resolves.toMatchObject({ ready: true })
    const [input, init] = fetchMock.mock.calls[0] as [URL, RequestInit]
    const headers = new Headers(init.headers)
    expect(input.origin).toBe(TANTAN_API_ORIGIN)
    expect(input.pathname).toBe("/readyz")
    expect(init.credentials).toBe("include")
    expect(init.cache).toBe("no-store")
    expect(headers.get("X-Request-Id")).toBeTruthy()
    expect(headers.get("X-Tantan-Timezone")).toBeTruthy()
  })

  test("REQ:FE-02 exposes a stable typed 401 for the session gate", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(
          JSON.stringify({
            requestId: "req-401",
            error: { code: "AUTH_REQUIRED", message: "Authentication required" },
          }),
          { status: 401, headers: { "Content-Type": "application/json" } },
        ),
      ),
    )

    await expect(getLocalSession()).rejects.toMatchObject({
      status: 401,
      code: "AUTH_REQUIRED",
    })
  })
})
