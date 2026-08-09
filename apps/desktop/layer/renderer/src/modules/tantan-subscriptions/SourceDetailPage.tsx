import { FeedViewType } from "@follow/constants"
import { useEntriesQuery, useEntryList } from "@follow/store/entry/hooks"
import { useFeedById, usePrefetchFeed } from "@follow/store/feed/hooks"
import { useSubscriptionByFeedId } from "@follow/store/subscription/hooks"
import { subscriptionSyncService } from "@follow/store/subscription/store"
import { useState } from "react"
import { Link, useParams } from "react-router"

import { EntryListRow } from "~/modules/tantan-entry/EntryListRow"

export function SourceDetailPage() {
  const { sourceId = "" } = useParams()
  const feed = useFeedById(sourceId)
  const feedQuery = usePrefetchFeed(sourceId, { enabled: !feed })
  const subscription = useSubscriptionByFeedId(sourceId)
  const view = subscription?.view ?? FeedViewType.Articles
  const entriesQuery = useEntriesQuery({ feedId: sourceId, view, limit: 20 })
  const entries = useEntryList(entriesQuery.entriesIds).filter((entry) => entry !== null)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const toggleSubscription = async () => {
    if (!feed) return
    setPending(true)
    setError(null)
    try {
      if (subscription) {
        await subscriptionSyncService.unsubscribe(sourceId)
      } else {
        await subscriptionSyncService.subscribe({
          url: feed.url,
          view: FeedViewType.Articles,
          category: null,
          isPrivate: false,
          hideFromTimeline: false,
          title: null,
          feedId: sourceId,
          listId: undefined,
        })
      }
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "订阅操作失败")
    } finally {
      setPending(false)
    }
  }

  return (
    <section className="mx-auto min-h-full w-full max-w-3xl px-4 py-3 pb-20 sm:px-6">
      <header className="flex min-h-14 items-center gap-2">
        <Link
          to="/subscriptions"
          aria-label="返回订阅"
          className="flex size-11 items-center justify-center rounded-full text-zinc-300 outline-none hover:bg-white/10 focus-visible:ring-2 focus-visible:ring-orange-500"
        >
          <i className="i-mgc-left-cute-re size-5" aria-hidden />
        </Link>
        <h1 className="min-w-0 flex-1 truncate text-xl font-bold text-zinc-50">
          {feed?.title || "Source"}
        </h1>
        <button
          type="button"
          disabled={!feed || pending}
          onClick={toggleSubscription}
          className="min-h-11 rounded-xl bg-orange-500 px-4 text-sm font-semibold text-white outline-none hover:bg-orange-400 focus-visible:ring-2 focus-visible:ring-orange-300 disabled:opacity-50"
        >
          {pending ? "处理中…" : subscription ? "取消订阅" : "添加订阅"}
        </button>
      </header>
      {feed?.description && (
        <p className="mt-2 text-sm leading-6 text-zinc-400">{feed.description}</p>
      )}
      {error && (
        <p role="alert" className="mt-3 rounded-xl bg-red-500/10 p-3 text-sm text-red-300">
          {error}
        </p>
      )}
      {feedQuery.isError && !feed && (
        <p role="alert" className="mt-3 text-sm text-red-300">
          {feedQuery.error.message}
        </p>
      )}
      <div className="mt-5 flex items-center justify-between">
        <h2 className="font-semibold text-zinc-100">历史内容</h2>
        <span className="text-xs text-zinc-500">已读与未读均可查看</span>
      </div>
      {entriesQuery.isLoading && (
        <div aria-busy="true" className="mt-3 h-52 animate-pulse rounded-xl bg-[#17181b]" />
      )}
      {entriesQuery.error && (
        <p role="alert" className="mt-3 text-sm text-red-300">
          {entriesQuery.error.message}
        </p>
      )}
      <div className="mt-3 space-y-2">
        {entries.map((entry) => (
          <EntryListRow key={entry.id} entry={entry} feed={feed} view={view} />
        ))}
      </div>
      {!entriesQuery.isLoading && entries.length === 0 && (
        <p className="py-16 text-center text-sm text-zinc-500">暂无历史内容</p>
      )}
      {entriesQuery.hasNextPage && (
        <button
          type="button"
          disabled={entriesQuery.isFetchingNextPage}
          onClick={() => entriesQuery.fetchNextPage()}
          className="mx-auto mt-4 flex min-h-11 items-center rounded-xl bg-white/10 px-4 text-sm disabled:opacity-50"
        >
          {entriesQuery.isFetchingNextPage ? "加载中…" : "加载更多"}
        </button>
      )}
    </section>
  )
}
