import { Masonry } from "@follow/components/ui/masonry/index.js"
import { useEffect, useRef } from "react"

import type { FeedbackRequest, HomeCard, QueueState } from "~/lib/tantan-api/gen/types"

import { FeedCard } from "./FeedCard"

interface MasonryFeedProps {
  cards: HomeCard[]
  queue: QueueState | null
  columns: number
  loading: boolean
  fetchingNext: boolean
  hasNextPage: boolean
  onFetchNext: () => void
  onOpenCard: () => void
  onFeedback: (card: HomeCard, action: FeedbackRequest["action"], topicId?: string) => void
}

export function MasonryFeed({
  cards,
  queue,
  columns,
  loading,
  fetchingNext,
  hasNextPage,
  onFetchNext,
  onOpenCard,
  onFeedback,
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
          {fetchingNext ? "加载更多…" : "今天已经看完"}
        </p>
      )}
    </div>
  )
}
