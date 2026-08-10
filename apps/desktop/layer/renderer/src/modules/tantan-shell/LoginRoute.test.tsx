import * as React from "react"
import { act } from "react"
import type { Root } from "react-dom/client"
import { createRoot } from "react-dom/client"
import { MemoryRouter, Route, Routes } from "react-router"
import { afterEach, beforeAll, beforeEach, describe, expect, test, vi } from "vitest"

import type { SessionResponse } from "~/lib/tantan-api/gen/types"

import { LoginRoute } from "./LoginRoute"

const auth = vi.hoisted(() => ({
  getProviders: vi.fn(),
  getSession: vi.fn(),
  socialStart: vi.fn(),
  email: vi.fn(),
  twoFactor: vi.fn(),
  token: vi.fn(),
  isTwoFactor: vi.fn(),
}))

vi.mock("~/lib/tantan-api/client", () => ({
  TANTAN_API_ORIGIN: "http://127.0.0.1:3000",
  getFoloAuthProviders: auth.getProviders,
  getLocalSession: auth.getSession,
  startFoloSocialLogin: auth.socialStart,
  signInWithFoloEmail: auth.email,
  verifyFoloTwoFactor: auth.twoFactor,
  signInWithFoloToken: auth.token,
  isFoloTwoFactorChallenge: auth.isTwoFactor,
}))

const session: SessionResponse = {
  user: { id: "user-1", name: "Mingrui", email: "mingrui@example.com", image: null },
  timezone: "Asia/Shanghai",
  csrfToken: "csrf-memory-only",
}

const setInputValue = (input: HTMLInputElement, value: string) => {
  const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")?.set
  setter?.call(input, value)
  input.dispatchEvent(new Event("input", { bubbles: true }))
}

const click = async (element: Element | null) => {
  if (!(element instanceof HTMLElement)) throw new Error("click target missing")
  await act(async () => element.click())
}

const renderLogin = async (initialEntry = "/login?returnTo=%2Fsubscriptions") => {
  const container = document.createElement("div")
  document.body.append(container)
  const root = createRoot(container)
  await act(async () => {
    root.render(
      <MemoryRouter initialEntries={[initialEntry]}>
        <Routes>
          <Route path="/login" element={<LoginRoute />} />
          <Route path="/subscriptions" element={<h1>Subscriptions destination</h1>} />
          <Route path="/" element={<h1>Home destination</h1>} />
        </Routes>
      </MemoryRouter>,
    )
  })
  await act(async () => Promise.resolve())
  return { container, root }
}

describe("LoginRoute", () => {
  let container: HTMLElement | null = null
  let root: Root | null = null

  beforeAll(() => {
    ;(globalThis as typeof globalThis & { React: typeof React }).React = React
    ;(
      globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }
    ).IS_REACT_ACT_ENVIRONMENT = true
  })

  beforeEach(() => {
    auth.getSession.mockRejectedValue(new Error("server is not bound"))
  })

  afterEach(async () => {
    if (root) await act(async () => root?.unmount())
    container?.remove()
    container = null
    root = null
    vi.restoreAllMocks()
    vi.clearAllMocks()
  })

  test("FR-02 exposes every Folo login method in server display order", async () => {
    auth.getProviders.mockResolvedValue({
      providers: ["google", "github", "apple", "credential", "token"],
    })
    ;({ container, root } = await renderLogin())

    const labels = Array.from(container.querySelectorAll("button")).map((button) =>
      button.textContent?.trim(),
    )
    expect(labels).toEqual([
      "使用 Google 继续",
      "使用 GitHub 继续",
      "使用 Apple 继续",
      "使用 Email 继续",
      "输入授权令牌继续",
    ])
  })

  test("FR-02 signs in with Email, clears the password and restores a safe returnTo", async () => {
    auth.getProviders.mockResolvedValue({ providers: ["credential"] })
    auth.email.mockResolvedValue(session)
    ;({ container, root } = await renderLogin())

    await click(
      Array.from(container.querySelectorAll("button")).find((button) =>
        button.textContent?.includes("Email"),
      ) ?? null,
    )
    const email = container.querySelector<HTMLInputElement>('input[name="email"]')
    const password = container.querySelector<HTMLInputElement>('input[name="password"]')
    expect(email).not.toBeNull()
    expect(password).not.toBeNull()
    setInputValue(email!, "mingrui@example.com")
    setInputValue(password!, "private-password")
    await act(async () => container!.querySelector("form")?.requestSubmit())

    expect(auth.email).toHaveBeenCalledWith({
      email: "mingrui@example.com",
      password: "private-password",
      returnTo: "/subscriptions",
    })
    expect(password?.value).toBe("")
    expect(container.textContent).toContain("Subscriptions destination")
  })

  test("FR-02 completes an Email TOTP challenge without exposing the pending Folo cookie", async () => {
    const challenge = {
      response: { challenge: { flowId: "flow-1", expiresAt: "2026-08-10T18:00:00Z" } },
    }
    auth.getProviders.mockResolvedValue({ providers: ["credential"] })
    auth.email.mockRejectedValue(challenge)
    auth.isTwoFactor.mockImplementation((value) => value === challenge)
    auth.twoFactor.mockResolvedValue(session)
    ;({ container, root } = await renderLogin())

    await click(
      Array.from(container.querySelectorAll("button")).find((button) =>
        button.textContent?.includes("Email"),
      ) ?? null,
    )
    setInputValue(container.querySelector('input[name="email"]')!, "mingrui@example.com")
    setInputValue(container.querySelector('input[name="password"]')!, "private-password")
    await act(async () => container!.querySelector("form")?.requestSubmit())

    const code = container.querySelector<HTMLInputElement>('input[name="code"]')
    expect(code).not.toBeNull()
    setInputValue(code!, "123456")
    await act(async () => container!.querySelector("form")?.requestSubmit())

    expect(auth.twoFactor).toHaveBeenCalledWith({ flowId: "flow-1", code: "123456" })
    expect(container.textContent).toContain("Subscriptions destination")
    expect(container.innerHTML).not.toContain("cookie")
  })

  test("FR-02 opens only the server-returned official social URL and switches to token handoff", async () => {
    auth.getProviders.mockResolvedValue({ providers: ["google"] })
    auth.socialStart.mockResolvedValue({
      authorizeUrl: "https://app.folo.is/login?provider=google",
      handoff: "one-time-token",
    })
    const open = vi.spyOn(window, "open").mockImplementation(() => null)
    ;({ container, root } = await renderLogin())

    await click(
      Array.from(container.querySelectorAll("button")).find((button) =>
        button.textContent?.includes("Google"),
      ) ?? null,
    )

    expect(auth.socialStart).toHaveBeenCalledWith({ provider: "google" })
    expect(open).toHaveBeenCalledWith(
      "https://app.folo.is/login?provider=google",
      "_blank",
      "noopener,noreferrer",
    )
    expect(container.textContent).toContain("管理员初始化 / 重新连接")
  })

  test("single-user deployment automatically enters on a new phone browser", async () => {
    auth.getSession.mockResolvedValue(session)
    ;({ container, root } = await renderLogin())

    expect(container.textContent).toContain("Subscriptions destination")
    expect(auth.getProviders).not.toHaveBeenCalled()
  })

  test("setup URL bypasses automatic session so the owner can reconnect Folo", async () => {
    auth.getSession.mockResolvedValue(session)
    auth.getProviders.mockResolvedValue({ providers: ["token"] })
    ;({ container, root } = await renderLogin("/login?setup=1"))

    expect(auth.getSession).not.toHaveBeenCalled()
    expect(container.textContent).toContain("管理员初始化 / 重新连接")
    expect(container.querySelector('input[name="token"]')).not.toBeNull()
  })

  test("SEC-03 rejects an external returnTo and submits authorization tokens only in the body", async () => {
    auth.getProviders.mockResolvedValue({ providers: ["token"] })
    auth.token.mockResolvedValue(session)
    ;({ container, root } = await renderLogin("/login?returnTo=https%3A%2F%2Fevil.example"))

    setInputValue(container.querySelector('input[name="token"]')!, "one-time-secret")
    await act(async () => container!.querySelector("form")?.requestSubmit())

    expect(auth.token).toHaveBeenCalledWith({ token: "one-time-secret", returnTo: "/" })
    expect(container.textContent).toContain("Home destination")
    expect(window.location.href).not.toContain("one-time-secret")
  })
})
