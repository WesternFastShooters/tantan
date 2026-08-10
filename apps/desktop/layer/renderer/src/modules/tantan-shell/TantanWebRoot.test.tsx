import { hydrateDatabaseToStore } from "@follow/store/hydrate"
import * as React from "react"
import { act } from "react"
import type { Root } from "react-dom/client"
import { createRoot } from "react-dom/client"
import { MemoryRouter, Route, Routes } from "react-router"
import { afterEach, beforeAll, describe, expect, test, vi } from "vitest"

import { TantanWebRoot } from "./TantanWebRoot"

vi.mock("@follow/store/hydrate", () => ({
  hydrateDatabaseToStore: vi.fn(),
}))

vi.mock("~/lib/app", () => ({
  removeAppSkeleton: vi.fn(),
}))

const deferred = <T,>() => {
  let resolve!: (value: T | PromiseLike<T>) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, reject, resolve }
}

describe("TantanWebRoot", () => {
  let container: HTMLElement | null = null
  let root: Root | null = null

  beforeAll(() => {
    ;(globalThis as typeof globalThis & { React: typeof React }).React = React
    ;(
      globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }
    ).IS_REACT_ACT_ENVIRONMENT = true
  })

  afterEach(async () => {
    if (root) await act(async () => root?.unmount())
    container?.remove()
    container = null
    root = null
    vi.clearAllMocks()
  })

  test("BUG:web-store-init waits for the local Folo store database before mounting routes", async () => {
    const hydration = deferred<void>()
    vi.mocked(hydrateDatabaseToStore).mockReturnValue(hydration.promise)
    container = document.createElement("div")
    document.body.append(container)
    root = createRoot(container)

    await act(async () => {
      root?.render(
        <MemoryRouter initialEntries={["/"]}>
          <Routes>
            <Route element={<TantanWebRoot />}>
              <Route index element={<p>Source route mounted</p>} />
            </Route>
          </Routes>
        </MemoryRouter>,
      )
    })

    expect(hydrateDatabaseToStore).toHaveBeenCalledWith({ migrateDatabase: true })
    expect(container.textContent).toContain("正在准备本地数据")
    expect(container.textContent).not.toContain("Source route mounted")

    await act(async () => hydration.resolve())

    expect(container.textContent).toContain("Source route mounted")
    expect(container.textContent).not.toContain("正在准备本地数据")
  })
})
