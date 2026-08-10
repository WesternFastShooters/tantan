import { FeedViewType } from "@follow/constants"
import { useFeedById, usePrefetchFeed } from "@follow/store/feed/hooks"
import { useSubscriptionByFeedId } from "@follow/store/subscription/hooks"
import { subscriptionSyncService } from "@follow/store/subscription/store"
import { useInfiniteQuery } from "@tanstack/react-query"
import { useMemo, useRef, useState } from "react"
import { Link, useParams } from "react-router"

import type { HomeCard } from "~/lib/tantan-api/gen/types"
import { EntryLink } from "~/modules/tantan-entry/EntryLink"

import { getContentPoolPage } from "./content-pool-api"

const TranslatedPoolRow = ({ card }: { card: HomeCard }) => (
  <article className="overflow-hidden rounded-xl border border-white/[0.06] bg-[#17181b]">
    <EntryLink
      card={card}
      className="flex min-h-28 gap-3 rounded-xl p-3 outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-orange-500"
      aria-label={`阅读：${card.title}`}
    >
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2 text-[11px] text-zinc-500">
          <span className="rounded bg-white/5 px-1.5 py-0.5 uppercase">{card.type}</span>
          <span>{card.translated ? "已翻译" : "中文原文"}</span>
        </div>
        <h3 className="mt-2 line-clamp-2 font-semibold leading-6 text-zinc-100">{card.title}</h3>
        {card.excerpt && (
          <p className="mt-1 line-clamp-2 text-sm leading-6 text-zinc-400">{card.excerpt}</p>
        )}
        <footer className="mt-2 flex gap-2 text-xs text-zinc-500">
          <span className="min-w-0 flex-1 truncate">{card.source.name}</span>
          <time dateTime={card.publishedAt}>
            {new Date(card.publishedAt).toLocaleDateString("zh-CN")}
          </time>
        </footer>
      </div>
      {card.cover && (
        <img
          src={card.cover}
          alt=""
          loading="lazy"
          decoding="async"
          className="h-24 w-20 shrink-0 rounded-lg bg-zinc-900 object-cover"
          onError={(event) => {
            event.currentTarget.hidden = true
          }}
        />
      )}
    </EntryLink>
  </article>
)

export function SourceDetailPage() {
  const { sourceId = "" } = useParams()
  const feed = useFeedById(sourceId)
  const feedQuery = usePrefetchFeed(sourceId, { enabled: !feed })
  const subscription = useSubscriptionByFeedId(sourceId)
  const poolQuery = useInfiniteQuery({
    queryKey: ["content-pool", sourceId],
    queryFn: ({ pageParam, signal }) => getContentPoolPage({ sourceId, cursor: pageParam, signal }),
    initialPageParam: null as string | null,
    getNextPageParam: (page) => page.nextCursor ?? undefined,
    enabled: sourceId.length > 0,
    staleTime: 30_000,
    refetchInterval: (query) => (query.state.data?.pages.at(-1)?.pool.pending ? 3_000 : false),
  })
  const entries = useMemo(() => {
    const seen = new Set<string>()
    return (poolQuery.data?.pages ?? [])
      .flatMap((page) => page.items)
      .filter((entry) => {
        if (seen.has(entry.entryId)) return false
        seen.add(entry.entryId)
        return true
      })
  }, [poolQuery.data?.pages])
  const pool = poolQuery.data?.pages.at(-1)?.pool
  const [pending, setPending] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const mutationLocked = useRef(false)

  const toggleSubscription = async () => {
    if (!feed || mutationLocked.current) return
    mutationLocked.current = true
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
      mutationLocked.current = false
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
      {poolQuery.isPending && (
        <div aria-busy="true" className="mt-3 h-52 animate-pulse rounded-xl bg-[#17181b]" />
      )}
      {poolQuery.error && (
        <p role="alert" className="mt-3 text-sm text-red-300">
          {poolQuery.error.message}
        </p>
      )}
      {pool && pool.pending > 0 && (
        <p role="status" className="mt-3 rounded-xl bg-orange-500/10 p-3 text-sm text-orange-200">
          还有 {pool.pending} 条内容正在翻译
        </p>
      )}
      <div className="mt-3 space-y-2">
        {entries.map((entry) => (
          <TranslatedPoolRow key={entry.entryId} card={entry} />
        ))}
      </div>
      {!poolQuery.isPending && !poolQuery.isError && entries.length === 0 && (
        <p className="py-16 text-center text-sm text-zinc-500">
          {pool?.pending ? "内容翻译完成后会显示在这里" : "暂无历史内容"}
        </p>
      )}
      {poolQuery.hasNextPage && (
        <button
          type="button"
          disabled={poolQuery.isFetchingNextPage}
          onClick={() => poolQuery.fetchNextPage()}
          className="mx-auto mt-4 flex min-h-11 items-center rounded-xl bg-white/10 px-4 text-sm disabled:opacity-50"
        >
          {poolQuery.isFetchingNextPage ? "加载中…" : "加载更多"}
        </button>
      )}
    </section>
  )
}
