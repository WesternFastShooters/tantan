import * as React from "react"
import { act } from "react"
import type { Root } from "react-dom/client"
import { createRoot } from "react-dom/client"
import { afterEach, beforeAll, describe, expect, test, vi } from "vitest"

import { AIFilterSheet } from "./AIFilterSheet"
import { HomeHeader } from "./HomeHeader"
import { MasonryFeed } from "./MasonryFeed"
import { TopicTabs } from "./TopicTabs"

let container: HTMLDivElement | null = null
let root: Root | null = null

const render = async (node: React.ReactNode) => {
  container = document.createElement("div")
  document.body.append(container)
  root = createRoot(container)
  await act(async () => root?.render(node))
  return container
}

describe("Tantan Home interactions", () => {
  beforeAll(() => {
    ;(globalThis as typeof globalThis & { React: typeof React }).React = React
    ;(
      globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }
    ).IS_REACT_ACT_ENVIRONMENT = true
  })

  afterEach(async () => {
    if (root) await act(async () => root?.unmount())
    container?.remove()
    root = null
    container = null
    vi.clearAllMocks()
  })

  test("REQ:FE-03 ordinary search and AI filter icons have distinct callbacks", async () => {
    const onSearch = vi.fn()
    const onOpenAIFilter = vi.fn()
    const view = await render(<HomeHeader onSearch={onSearch} onOpenAIFilter={onOpenAIFilter} />)

    await act(async () => view.querySelector<HTMLButtonElement>('[aria-label="搜索内容"]')?.click())
    expect(onSearch).toHaveBeenCalledOnce()
    expect(onOpenAIFilter).not.toHaveBeenCalled()

    await act(async () =>
      view.querySelector<HTMLButtonElement>('[aria-label="AI 智能筛选"]')?.click(),
    )
    expect(onOpenAIFilter).toHaveBeenCalledOnce()
  })

  test("REQ:FE-03 Topic tabs support keyboard selection", async () => {
    const onChange = vi.fn()
    const view = await render(
      <TopicTabs
        activeTopicId="recommend"
        onChange={onChange}
        topics={[
          {
            id: "recommend",
            name: "推荐",
            kind: "core",
            fixed: true,
            pinned: true,
            hidden: false,
            unreadCount: 4,
          },
          {
            id: "topic-ai",
            name: "AI",
            kind: "dynamic",
            fixed: false,
            pinned: true,
            hidden: false,
            unreadCount: 2,
          },
        ]}
      />,
    )
    const firstTab = view.querySelector<HTMLButtonElement>('[role="tab"]')
    firstTab?.focus()
    await act(async () =>
      firstTab?.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowRight", bubbles: true })),
    )

    expect(onChange).toHaveBeenCalledWith("topic-ai")
    expect(document.activeElement?.textContent).toContain("AI")
  })

  test("REQ:FE-03 AI Filter Sheet traps focus and submits a trimmed prompt", async () => {
    const onSubmit = vi.fn()
    const view = await render(
      <AIFilterSheet open pending={false} error={null} onClose={vi.fn()} onSubmit={onSubmit} />,
    )
    const textarea = view.querySelector<HTMLTextAreaElement>("textarea")!
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")?.set?.call(
        textarea,
        "  多推 Codex  ",
      )
      textarea.dispatchEvent(new InputEvent("input", { bubbles: true }))
    })
    const generate = [...view.querySelectorAll<HTMLButtonElement>("button")].find((button) =>
      button.textContent?.includes("生成信息流"),
    )!
    await act(async () => generate.click())
    expect(onSubmit).toHaveBeenCalledWith("多推 Codex")

    generate.focus()
    await act(async () =>
      generate.dispatchEvent(new KeyboardEvent("keydown", { key: "Tab", bubbles: true })),
    )
    expect(document.activeElement?.getAttribute("aria-label")).toBe("取消")
  })

  test("REQ:JOB-01 empty Home explains that the first Folo sync is still running", async () => {
    const view = await render(
      <MasonryFeed
        cards={[]}
        queue={null}
        columns={2}
        loading={false}
        syncing
        syncError={null}
        syncProgress={{ processed: 2, total: 10, failed: 0 }}
        syncRetrying={false}
        fetchingNext={false}
        hasNextPage={false}
        onFetchNext={vi.fn()}
        onOpenCard={vi.fn()}
        onFeedback={vi.fn()}
        onRetrySync={vi.fn()}
      />,
    )

    expect(view.textContent).toContain("正在同步 Folo 内容")
    expect(view.textContent).toContain("2 / 10")
    expect(view.textContent).not.toContain("今天已经看完")
  })

  test("REQ:JOB-01 failed first sync offers a retry instead of a finished queue", async () => {
    const onRetrySync = vi.fn()
    const view = await render(
      <MasonryFeed
        cards={[]}
        queue={null}
        columns={2}
        loading={false}
        syncing={false}
        syncError="同步任务未完成，请稍后重试"
        syncProgress={null}
        syncRetrying={false}
        fetchingNext={false}
        hasNextPage={false}
        onFetchNext={vi.fn()}
        onOpenCard={vi.fn()}
        onFeedback={vi.fn()}
        onRetrySync={onRetrySync}
      />,
    )

    expect(view.textContent).toContain("内容同步失败")
    expect(view.textContent).not.toContain("今天已经看完")
    await act(async () =>
      [...view.querySelectorAll<HTMLButtonElement>("button")]
        .find((button) => button.textContent?.includes("重试同步"))
        ?.click(),
    )
    expect(onRetrySync).toHaveBeenCalledOnce()
  })
})
