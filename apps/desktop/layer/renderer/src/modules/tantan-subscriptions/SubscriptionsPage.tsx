import { FeedViewType } from "@follow/constants"
import { useFeedsByIds } from "@follow/store/feed/hooks"
import { useAllFeedSubscription, usePrefetchSubscription } from "@follow/store/subscription/hooks"
import { subscriptionSyncService } from "@follow/store/subscription/store"
import { useMemo, useState } from "react"
import { Link } from "react-router"

import { TantanShellPage } from "~/modules/tantan-shell/TantanAppShell"

import { filterFeedSubscriptions, SUBSCRIPTION_FILTERS } from "./subscription-model"

export function SubscriptionsPage() {
  const [activeView, setActiveView] = useState<FeedViewType>(FeedViewType.Articles)
  const [addOpen, setAddOpen] = useState(false)
  const [url, setURL] = useState("")
  const [mutationError, setMutationError] = useState<string | null>(null)
  const [pendingId, setPendingId] = useState<string | null>(null)
  const subscriptionQuery = usePrefetchSubscription()
  const subscriptions = useAllFeedSubscription()
  const filtered = useMemo(
    () => filterFeedSubscriptions(subscriptions, activeView),
    [activeView, subscriptions],
  )
  const feedIds = useMemo(
    () =>
      filtered.map((subscription) => subscription.feedId).filter((id): id is string => Boolean(id)),
    [filtered],
  )
  const feeds = useFeedsByIds(feedIds)
  const grouped = useMemo(() => {
    const result = new Map<string, typeof filtered>()
    filtered.forEach((subscription) => {
      const category = subscription.category || "未分类"
      result.set(category, [...(result.get(category) ?? []), subscription])
    })
    return result
  }, [filtered])

  const subscribe = async () => {
    const value = url.trim()
    if (!value) return
    setPendingId("add")
    setMutationError(null)
    try {
      await subscriptionSyncService.subscribe({
        url: value,
        view: activeView,
        category: null,
        isPrivate: false,
        hideFromTimeline: false,
        title: null,
        feedId: null,
        listId: undefined,
      })
      setURL("")
      setAddOpen(false)
    } catch (error) {
      setMutationError(error instanceof Error ? error.message : "添加订阅失败")
    } finally {
      setPendingId(null)
    }
  }

  const unsubscribe = async (feedId: string) => {
    setPendingId(feedId)
    setMutationError(null)
    try {
      await subscriptionSyncService.unsubscribe(feedId)
    } catch (error) {
      setMutationError(error instanceof Error ? error.message : "取消订阅失败")
    } finally {
      setPendingId(null)
    }
  }

  return (
    <TantanShellPage>
      <header className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">订阅</h1>
          <p className="mt-1 text-sm text-zinc-400">按内容类型浏览和管理 RSS Source。</p>
        </div>
        <div className="flex gap-2">
          <Link
            to="/favorites"
            className="flex min-h-11 items-center rounded-xl bg-white/10 px-4 text-sm outline-none hover:bg-white/15 focus-visible:ring-2 focus-visible:ring-orange-500"
          >
            收藏
          </Link>
          <button
            type="button"
            onClick={() => setAddOpen((value) => !value)}
            className="min-h-11 rounded-xl bg-orange-500 px-4 text-sm font-semibold text-white outline-none hover:bg-orange-400 focus-visible:ring-2 focus-visible:ring-orange-300"
          >
            添加订阅
          </button>
        </div>
      </header>

      {addOpen && (
        <form
          onSubmit={(event) => {
            event.preventDefault()
            subscribe()
          }}
          className="mt-4 flex flex-col gap-2 rounded-xl border border-white/[0.06] bg-[#17181b] p-3 sm:flex-row"
        >
          <label htmlFor="subscription-url" className="sr-only">
            RSS 或网站地址
          </label>
          <input
            id="subscription-url"
            type="url"
            required
            value={url}
            disabled={pendingId === "add"}
            onChange={(event) => setURL(event.target.value)}
            placeholder="https://example.com/feed.xml"
            className="h-11 min-w-0 flex-1 rounded-xl border border-white/10 bg-zinc-950 px-3 text-sm text-zinc-100 outline-none focus:border-orange-500 focus:ring-1 focus:ring-orange-500"
          />
          <button
            type="submit"
            disabled={pendingId === "add"}
            className="min-h-11 rounded-xl bg-orange-500 px-4 text-sm font-semibold text-white disabled:opacity-50"
          >
            {pendingId === "add" ? "添加中…" : "确认添加"}
          </button>
          <button
            type="button"
            onClick={() => setAddOpen(false)}
            disabled={pendingId === "add"}
            className="min-h-11 rounded-xl px-4 text-sm text-zinc-400 hover:bg-white/10 disabled:opacity-50"
          >
            取消
          </button>
        </form>
      )}

      <div
        role="tablist"
        aria-label="订阅类型"
        className="mt-5 flex gap-1 overflow-x-auto border-b border-white/[0.06]"
      >
        {SUBSCRIPTION_FILTERS.map((filter) => (
          <button
            key={filter.view}
            type="button"
            role="tab"
            aria-selected={filter.view === activeView}
            onClick={() => setActiveView(filter.view)}
            className="relative min-h-11 min-w-20 px-3 text-sm text-zinc-400 outline-none hover:text-zinc-100 focus-visible:ring-2 focus-visible:ring-orange-500 aria-selected:text-zinc-50"
          >
            {filter.label}
            {filter.view === activeView && (
              <span className="absolute inset-x-3 bottom-0 h-0.5 bg-orange-500" />
            )}
          </button>
        ))}
      </div>

      {mutationError && (
        <p role="alert" className="mt-3 rounded-xl bg-red-500/10 p-3 text-sm text-red-300">
          {mutationError}
        </p>
      )}
      {subscriptionQuery.isPending && (
        <div aria-busy="true" className="mt-4 h-52 animate-pulse rounded-xl bg-[#17181b]" />
      )}
      {subscriptionQuery.isError && (
        <p role="alert" className="mt-4 rounded-xl bg-red-500/10 p-3 text-sm text-red-300">
          {subscriptionQuery.error.message}
        </p>
      )}
      {!subscriptionQuery.isPending && filtered.length === 0 && (
        <div className="flex min-h-64 flex-col items-center justify-center text-center">
          <h2 className="font-semibold text-zinc-100">还没有这类订阅</h2>
          <p className="mt-1 text-sm text-zinc-500">
            添加 RSS 地址后，内容会在首页和这里同步出现。
          </p>
        </div>
      )}
      <div className="mt-4 space-y-5">
        {[...grouped.entries()].map(([category, categorySubscriptions]) => (
          <section key={category}>
            <h2 className="mb-2 text-xs font-semibold uppercase tracking-wide text-zinc-500">
              {category}
            </h2>
            <div className="grid gap-2 sm:grid-cols-2">
              {categorySubscriptions.map((subscription) => {
                const feed = feeds.find((item) => item.id === subscription.feedId)
                if (!feed || !subscription.feedId) return null
                return (
                  <article
                    key={subscription.feedId}
                    className="flex min-w-0 items-center gap-3 rounded-xl border border-white/[0.06] bg-[#17181b] p-3"
                  >
                    {feed.image ? (
                      <img
                        src={feed.image}
                        alt=""
                        className="size-10 shrink-0 rounded-lg object-cover"
                      />
                    ) : (
                      <span className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-white/10 text-sm">
                        {(subscription.title || feed.title || "R").slice(0, 1)}
                      </span>
                    )}
                    <Link
                      to={`/sources/${encodeURIComponent(feed.id)}`}
                      className="min-w-0 flex-1 rounded-lg outline-none focus-visible:ring-2 focus-visible:ring-orange-500"
                    >
                      <h3 className="truncate text-sm font-medium text-zinc-100">
                        {subscription.title || feed.title || feed.url}
                      </h3>
                      <p className="mt-1 truncate text-xs text-zinc-500">{feed.url}</p>
                    </Link>
                    <button
                      type="button"
                      aria-label={`取消订阅 ${subscription.title || feed.title}`}
                      disabled={pendingId === feed.id}
                      onClick={() => unsubscribe(feed.id)}
                      className="min-h-11 shrink-0 rounded-lg px-2 text-xs text-zinc-400 outline-none hover:bg-red-500/10 hover:text-red-300 focus-visible:ring-2 focus-visible:ring-red-400 disabled:opacity-50"
                    >
                      取消
                    </button>
                  </article>
                )
              })}
            </div>
          </section>
        ))}
      </div>
    </TantanShellPage>
  )
}
