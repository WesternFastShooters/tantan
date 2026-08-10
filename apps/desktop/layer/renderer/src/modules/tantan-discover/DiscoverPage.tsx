import { FeedViewType } from "@follow/constants"
import { useSubscriptionByFeedId } from "@follow/store/subscription/hooks"
import { subscriptionSyncService } from "@follow/store/subscription/store"
import { useRef, useState } from "react"
import { Link } from "react-router"

import { tantanRequest } from "~/lib/tantan-api/client"
import { TantanShellPage } from "~/modules/tantan-shell/TantanAppShell"

type FeedDiscoveryResult = {
  type: "feed"
  id: string
  url: string
  title: string | null
  description: string | null
  siteUrl: string | null
  image: string | null
}

type DiscoveryItem = {
  feed?: FeedDiscoveryResult
  subscriptionCount?: number
  updatesPerWeek?: number
}

type DiscoveryResponse = {
  code: number
  data: DiscoveryItem[]
}

const FeedResult = ({ item }: { item: DiscoveryItem }) => {
  const feed = item.feed
  const subscription = useSubscriptionByFeedId(feed?.id ?? "")
  const [pending, setPending] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [imageFailed, setImageFailed] = useState(false)
  const mutationLocked = useRef(false)

  if (!feed) return null

  const toggleSubscription = async () => {
    if (mutationLocked.current) return
    mutationLocked.current = true
    setPending(true)
    setError(null)
    try {
      if (subscription) {
        await subscriptionSyncService.unsubscribe(feed.id)
      } else {
        await subscriptionSyncService.subscribe({
          url: feed.url,
          view: FeedViewType.Articles,
          category: null,
          isPrivate: false,
          hideFromTimeline: false,
          title: null,
          feedId: feed.id,
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
    <article className="rounded-2xl bg-white p-4 shadow-sm ring-1 ring-zinc-200/70 dark:bg-[#17181b] dark:ring-white/[0.07]">
      <div className="flex items-start gap-3">
        {feed.image && !imageFailed ? (
          <img
            src={feed.image}
            alt=""
            className="size-12 shrink-0 rounded-xl object-cover"
            loading="lazy"
            onError={() => setImageFailed(true)}
          />
        ) : (
          <span className="flex size-12 shrink-0 items-center justify-center rounded-xl bg-orange-500/10 text-lg font-semibold text-orange-500">
            {(feed.title || "R").slice(0, 1)}
          </span>
        )}
        <Link
          to={`/sources/${encodeURIComponent(feed.id)}`}
          className="min-w-0 flex-1 rounded-lg outline-none focus-visible:ring-2 focus-visible:ring-orange-500"
        >
          <h2 className="line-clamp-2 font-semibold text-zinc-900 dark:text-zinc-100">
            {feed.title || feed.url}
          </h2>
          <p className="mt-1 truncate text-xs text-zinc-500">{feed.siteUrl || feed.url}</p>
        </Link>
        <button
          type="button"
          disabled={pending}
          onClick={() => void toggleSubscription()}
          className="min-h-11 shrink-0 rounded-full border border-orange-500 px-4 text-sm font-semibold text-orange-500 outline-none focus-visible:ring-2 focus-visible:ring-orange-500 disabled:opacity-50"
        >
          {pending ? "处理中…" : subscription ? "已订阅" : "订阅"}
        </button>
      </div>
      {feed.description && (
        <p className="mt-3 line-clamp-3 text-sm leading-6 text-zinc-600 dark:text-zinc-400">
          {feed.description}
        </p>
      )}
      <div className="mt-3 flex gap-3 text-xs text-zinc-500">
        {typeof item.subscriptionCount === "number" && <span>{item.subscriptionCount} 人订阅</span>}
        {typeof item.updatesPerWeek === "number" && <span>每周约 {item.updatesPerWeek} 篇</span>}
      </div>
      {error && (
        <p role="alert" className="mt-3 rounded-xl bg-red-500/10 p-2 text-sm text-red-500">
          {error}
        </p>
      )}
    </article>
  )
}

export function DiscoverPage() {
  const [keyword, setKeyword] = useState("")
  const [results, setResults] = useState<DiscoveryItem[]>([])
  const [searched, setSearched] = useState(false)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const requestGeneration = useRef(0)

  const search = async () => {
    const value = keyword.trim()
    if (!value || pending) return
    const generation = ++requestGeneration.current
    setPending(true)
    setError(null)
    try {
      const response = await tantanRequest<DiscoveryResponse>("/api/folo/discover", {
        method: "POST",
        body: JSON.stringify({ keyword: value, target: "feeds" }),
      })
      if (generation !== requestGeneration.current) return
      setResults(response.data.filter((item) => item.feed?.type === "feed"))
      setSearched(true)
    } catch (reason) {
      if (generation !== requestGeneration.current) return
      setError(reason instanceof Error ? reason.message : "搜索 Source 失败")
      setResults([])
      setSearched(true)
    } finally {
      if (generation === requestGeneration.current) setPending(false)
    }
  }

  return (
    <TantanShellPage>
      <header>
        <h1 className="text-2xl font-bold tracking-tight">发现</h1>
        <p className="mt-1 text-sm text-zinc-500">查找网站、播客和 RSS Source</p>
        <form
          className="relative mt-4"
          onSubmit={(event) => {
            event.preventDefault()
            void search()
          }}
        >
          <label htmlFor="discover-keyword" className="sr-only">
            搜索网站或 RSS
          </label>
          <i
            className="i-mgc-search-3-cute-re pointer-events-none absolute left-4 top-1/2 size-5 -translate-y-1/2 text-zinc-500"
            aria-hidden
          />
          <input
            id="discover-keyword"
            type="search"
            value={keyword}
            onChange={(event) => setKeyword(event.target.value)}
            placeholder="输入网站名、关键词或 RSS 地址"
            autoComplete="off"
            className="h-12 w-full rounded-2xl border border-zinc-200 bg-white pl-11 pr-20 text-base outline-none focus:border-orange-500 focus:ring-2 focus:ring-orange-500/20 dark:border-white/10 dark:bg-[#17181b]"
          />
          <button
            type="submit"
            disabled={pending || !keyword.trim()}
            className="absolute right-1.5 top-1/2 min-h-9 -translate-y-1/2 rounded-xl bg-orange-500 px-3 text-sm font-semibold text-white disabled:opacity-40"
          >
            {pending ? "搜索中" : "搜索"}
          </button>
        </form>
      </header>

      {error && (
        <div role="alert" className="mt-4 rounded-2xl bg-red-500/10 p-4 text-sm text-red-500">
          {error}
        </div>
      )}
      {pending && (
        <div aria-busy="true" className="mt-5 space-y-3">
          <div className="h-32 animate-pulse rounded-2xl bg-zinc-200 dark:bg-white/10" />
          <div className="h-32 animate-pulse rounded-2xl bg-zinc-200 dark:bg-white/10" />
        </div>
      )}
      {!pending && results.length > 0 && (
        <div className="mt-5 space-y-3">
          {results.map((item) => (
            <FeedResult key={item.feed?.id} item={item} />
          ))}
        </div>
      )}
      {!pending && searched && !error && results.length === 0 && (
        <div className="flex min-h-64 flex-col items-center justify-center text-center">
          <i className="i-mgc-rss-cute-fi size-10 text-zinc-400" aria-hidden />
          <h2 className="mt-3 font-semibold">没有找到可订阅的 Source</h2>
          <p className="mt-1 text-sm text-zinc-500">可以尝试粘贴完整的 RSS 地址。</p>
        </div>
      )}
      {!pending && !searched && (
        <div className="flex min-h-72 flex-col items-center justify-center text-center">
          <span className="flex size-16 items-center justify-center rounded-3xl bg-orange-500/10">
            <i className="i-mgc-compass-3-cute-re size-8 text-orange-500" aria-hidden />
          </span>
          <h2 className="mt-4 font-semibold">发现新的信息源</h2>
          <p className="mt-1 max-w-64 text-sm leading-6 text-zinc-500">
            搜索后可直接订阅，内容会同步到订阅页和首页推荐。
          </p>
        </div>
      )}
    </TantanShellPage>
  )
}
