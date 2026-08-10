import { afterEach, describe, expect, test, vi } from "vitest"

import { getLocalSession, getReadiness, tantanRequest } from "./client"

describe("Tantan same-origin API client", () => {
  afterEach(() => vi.unstubAllGlobals())

  test("FR-08 sends readiness through the relative same-origin /api boundary", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          ready: true,
          checks: {
            sqlite: "ok",
            migrations: "ok",
            secretStore: "ok",
            routePolicy: "ok",
            staticAssets: "ok",
          },
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ),
    )
    vi.stubGlobal("fetch", fetchMock)

    await expect(getReadiness()).resolves.toMatchObject({ ready: true })
    const [input, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    const headers = new Headers(init.headers)
    expect(input).toBe("/api/readyz")
    expect(init.credentials).toBe("include")
    expect(init.cache).toBe("no-store")
    expect(headers.get("X-Request-Id")).toBeTruthy()
    expect(headers.get("X-Tantan-Timezone")).toBeTruthy()
    expect(headers.has("Authorization")).toBe(false)
  })

  test("REQ:JOB-01 refreshes the in-memory CSRF token before an early local mutation", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            user: { id: "user-1", name: "Test", email: null, image: null },
            timezone: "Asia/Shanghai",
            csrfToken: "csrf-restored-session-1234567890",
          }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        ),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ jobId: "job-1", state: "queued" }), {
          status: 202,
          headers: { "Content-Type": "application/json" },
        }),
      )
    vi.stubGlobal("fetch", fetchMock)

    await tantanRequest("/api/tantan/v1/sync", {
      method: "POST",
      body: JSON.stringify({ scope: "all" }),
    })

    expect(fetchMock).toHaveBeenCalledTimes(2)
    expect(fetchMock.mock.calls[0]?.[0]).toBe("/api/tantan/v1/session")
    const [input, init] = fetchMock.mock.calls[1] as [string, RequestInit]
    expect(input).toBe("/api/tantan/v1/sync")
    expect(new Headers(init.headers).get("X-CSRF-Token")).toBe("csrf-restored-session-1234567890")
  })

  test("FR-08 stores session CSRF only in memory and attaches it to mutations", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            user: { id: "user-1", name: "Test", email: null, image: null },
            timezone: "Asia/Shanghai",
            csrfToken: "csrf-memory-only-1234567890",
          }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        ),
      )
      .mockResolvedValueOnce(new Response(null, { status: 204 }))
    vi.stubGlobal("fetch", fetchMock)

    await getLocalSession()
    await tantanRequest<void>("/api/tantan/v1/filter", { method: "DELETE" })

    const [input, init] = fetchMock.mock.calls[1] as [string, RequestInit]
    expect(input).toBe("/api/tantan/v1/filter")
    expect(new Headers(init.headers).get("X-CSRF-Token")).toBe("csrf-memory-only-1234567890")
  })

  test("FR-08 rejects absolute or non-api browser destinations before fetch", async () => {
    const fetchMock = vi.fn()
    vi.stubGlobal("fetch", fetchMock)

    await expect(tantanRequest("https://api.folo.is/entries")).rejects.toThrow("relative /api path")
    await expect(tantanRequest("/better-auth/get-session")).rejects.toThrow("relative /api path")
    expect(fetchMock).not.toHaveBeenCalled()
  })
})
