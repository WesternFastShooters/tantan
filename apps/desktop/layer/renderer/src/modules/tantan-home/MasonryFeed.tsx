import { Masonry } from "@follow/components/ui/masonry/index.js"
import { useEffect, useRef } from "react"

import type {
  FeedbackRequest,
  HomeCard,
  QueueState,
  SyncStatusResponse,
} from "~/lib/tantan-api/gen/types"

import { FeedCard } from "./FeedCard"

interface MasonryFeedProps {
  cards: HomeCard[]
  loadedCount?: number
  queue: QueueState | null
  columns: number
  loading: boolean
  syncing: boolean
  syncError: string | null
  syncProgress: SyncStatusResponse["counts"] | null
  syncRetrying: boolean
  fetchingNext: boolean
  hasNextPage: boolean
  onFetchNext: () => void
  onOpenCard: () => void
  onFeedback: (card: HomeCard, action: FeedbackRequest["action"], topicId?: string) => void
  onRetrySync: () => void
}

export function MasonryFeed({
  cards,
  loadedCount = cards.length,
  queue,
  columns,
  loading,
  syncing,
  syncError,
  syncProgress,
  syncRetrying,
  fetchingNext,
  hasNextPage,
  onFetchNext,
  onOpenCard,
  onFeedback,
  onRetrySync,
}: MasonryFeedProps) {
  const sentinelRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const sentinel = sentinelRef.current
    if (!sentinel || !hasNextPage || fetchingNext) return
    const observer = new IntersectionObserver(
      ([entry]) => {
        if (entry?.isIntersecting) onFetchNext()
      },
      { rootMargin: "500px" },
    )
    observer.observe(sentinel)
    return () => observer.disconnect()
  }, [fetchingNext, hasNextPage, onFetchNext])

  if (loading) {
    return (
      <div
        data-testid="home-feed-loading"
        className="grid gap-2 p-2 sm:gap-3 sm:p-3"
        style={{ gridTemplateColumns: `repeat(${columns}, minmax(0, 1fr))` }}
      >
        {Array.from({ length: columns * 2 }, (_, index) => (
          <div key={index} className="h-56 animate-pulse rounded-xl bg-[#17181b] odd:h-72" />
        ))}
      </div>
    )
  }

  if (cards.length === 0) {
    if (syncing) {
      const hasTotal = Boolean(syncProgress?.total)
      return (
        <div
          role="status"
          className="flex min-h-80 flex-col items-center justify-center px-6 text-center"
        >
          <i
            className="i-mgc-loading-3-cute-re mb-3 size-8 animate-spin text-orange-400"
            aria-hidden
          />
          <h2 className="font-semibold text-zinc-100">正在同步 Folo 内容</h2>
          <p className="mt-1 text-sm text-zinc-500">
            {hasTotal
              ? `${syncProgress?.processed ?? 0} / ${syncProgress?.total ?? 0}`
              : "正在读取你的订阅和最近未读内容…"}
          </p>
        </div>
      )
    }
    if (syncError) {
      return (
        <div
          role="alert"
          className="flex min-h-80 flex-col items-center justify-center px-6 text-center"
        >
          <i className="i-mgc-alert-cute-fi mb-3 size-8 text-red-300" aria-hidden />
          <h2 className="font-semibold text-zinc-100">内容同步失败</h2>
          <p className="mt-1 text-sm text-zinc-500">{syncError}</p>
          <button
            type="button"
            disabled={syncRetrying}
            onClick={onRetrySync}
            className="mt-4 min-h-11 rounded-xl bg-orange-500 px-4 text-sm font-semibold text-white outline-none focus-visible:ring-2 focus-visible:ring-orange-300 disabled:opacity-50"
          >
            {syncRetrying ? "正在重试…" : "重试同步"}
          </button>
        </div>
      )
    }
    if (queue && !queue.finished && queue.unread > 0) {
      return (
        <div
          role="status"
          className="flex min-h-80 flex-col items-center justify-center px-6 text-center"
        >
          <i
            className="i-mgc-translate-2-cute-re mb-3 size-8 animate-pulse text-orange-400"
            aria-hidden
          />
          <h2 className="font-semibold text-zinc-100">正在翻译推荐内容</h2>
          <p className="mt-1 text-sm text-zinc-500">翻译完成后会自动出现在瀑布流中。</p>
        </div>
      )
    }
    return (
      <div className="flex min-h-80 flex-col items-center justify-center px-6 text-center">
        <i className="i-mgc-check-circle-cute-re mb-3 size-8 text-orange-400" aria-hidden />
        <h2 className="font-semibold text-zinc-100">今天已经看完</h2>
        <p className="mt-1 text-sm text-zinc-500">更早的未读内容仍可在订阅页和搜索中找到。</p>
      </div>
    )
  }

  return (
    <div data-testid="masonry-feed" data-columns={columns} className="p-2 sm:p-3">
      <Masonry
        items={cards}
        role="list"
        columnCount={columns}
        columnGutter={columns === 2 ? 8 : 12}
        rowGutter={columns === 2 ? 8 : 12}
        itemKey={(card) => card.entryId}
        itemHeightEstimate={280}
        overscanBy={2}
        render={({ data }) => <FeedCard card={data} onOpen={onOpenCard} onFeedback={onFeedback} />}
      />
      <div ref={sentinelRef} data-testid="home-pagination-sentinel" className="h-1" />
      {(fetchingNext || (!hasNextPage && (queue?.finished || cards.length > 0))) && (
        <p className="py-5 text-center text-xs text-zinc-500">
          {fetchingNext
            ? "加载更多…"
            : queue && loadedCount < queue.unread
              ? "正在翻译更多内容…"
              : "今天已经看完"}
        </p>
      )}
    </div>
  )
}
