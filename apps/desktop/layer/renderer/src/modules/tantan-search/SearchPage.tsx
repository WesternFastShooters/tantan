import { useInfiniteQuery } from "@tanstack/react-query"
import { useEffect, useMemo, useRef, useState } from "react"
import { useLocation, useNavigate, useSearchParams } from "react-router"

import type { HomeCard } from "~/lib/tantan-api/gen/types"
import { EntryLink } from "~/modules/tantan-entry/EntryLink"

import { searchEntries } from "./api"
import { mergeSearchPages, SEARCH_DEBOUNCE_MS } from "./search-model"

const SearchResultCard = ({ card }: { card: HomeCard }) => (
  <article className="rounded-xl border border-white/[0.06] bg-[#17181b] p-4">
    <EntryLink
      card={card}
      className="block rounded-lg outline-none focus-visible:ring-2 focus-visible:ring-orange-500"
    >
      <p className="text-xs text-orange-400">
        {card.topics.map((topic) => topic.name).join(" · ")}
      </p>
      <h2 className="mt-1 line-clamp-2 font-semibold leading-6 text-zinc-100">{card.title}</h2>
      {card.excerpt && (
        <p className="mt-2 line-clamp-2 text-sm leading-6 text-zinc-400">{card.excerpt}</p>
      )}
      <footer className="mt-3 flex justify-between gap-3 text-xs text-zinc-500">
        <span className="truncate">{card.source.name}</span>
        <span>{card.translated ? "含中文译文" : card.type}</span>
      </footer>
    </EntryLink>
  </article>
)

export function SearchPage() {
  const navigate = useNavigate()
  const location = useLocation()
  const [searchParams] = useSearchParams()
  const urlQuery = searchParams.get("q") ?? ""
  const [input, setInput] = useState(urlQuery)
  const [query, setQuery] = useState(urlQuery.trim())
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    const timer = window.setTimeout(() => setQuery(input.trim()), SEARCH_DEBOUNCE_MS)
    return () => window.clearTimeout(timer)
  }, [input])

  useEffect(() => {
    const current = new URLSearchParams(location.search).get("q") ?? ""
    if (current === query) return
    const next = query ? `/search?q=${encodeURIComponent(query)}` : "/search"
    navigate(next, { replace: true, state: location.state })
  }, [location.search, location.state, navigate, query])

  const validQuery = query.length >= 1 && query.length <= 200
  const resultQuery = useInfiniteQuery({
    queryKey: ["search", query],
    queryFn: ({ pageParam, signal }) => searchEntries({ q: query, cursor: pageParam, signal }),
    initialPageParam: null as string | null,
    getNextPageParam: (page) => page.nextCursor ?? undefined,
    enabled: validQuery,
    staleTime: 5 * 60_000,
  })
  const results = useMemo(
    () => mergeSearchPages(resultQuery.data?.pages.map((page) => page.items) ?? []),
    [resultQuery.data?.pages],
  )
  const indexStatus = resultQuery.data?.pages.at(-1)?.indexStatus

  return (
    <section className="mx-auto min-h-full w-full max-w-4xl bg-[#08090b] px-3 pb-20 sm:px-6">
      <header className="sticky top-0 z-20 -mx-3 border-b border-white/[0.06] bg-[#08090b]/95 px-3 py-3 backdrop-blur sm:-mx-6 sm:px-6">
        <div className="flex items-center gap-2">
          <button
            type="button"
            aria-label="返回"
            onClick={() => navigate(-1)}
            className="flex size-11 shrink-0 items-center justify-center rounded-full text-zinc-300 outline-none hover:bg-white/10 focus-visible:ring-2 focus-visible:ring-orange-500"
          >
            <i className="i-mgc-arrow-left-cute-re size-5" aria-hidden />
          </button>
          <search className="relative min-w-0 flex-1">
            <label htmlFor="tantan-search" className="sr-only">
              搜索订阅内容
            </label>
            <i
              className="i-mgc-search-2-cute-re pointer-events-none absolute left-3 top-1/2 size-5 -translate-y-1/2 text-zinc-500"
              aria-hidden
            />
            <input
              id="tantan-search"
              ref={inputRef}
              type="search"
              autoFocus
              maxLength={200}
              value={input}
              onChange={(event) => setInput(event.target.value)}
              placeholder="搜索标题、正文、译文、Source、Topic、Tag"
              className="h-11 w-full rounded-xl border border-white/10 bg-[#17181b] pl-10 pr-10 text-sm text-zinc-100 outline-none placeholder:text-zinc-600 focus:border-orange-500 focus:ring-1 focus:ring-orange-500"
            />
            {input && (
              <button
                type="button"
                aria-label="清空搜索"
                onClick={() => {
                  setInput("")
                  inputRef.current?.focus()
                }}
                className="absolute right-0 top-0 flex size-11 items-center justify-center text-zinc-500 outline-none hover:text-zinc-200 focus-visible:ring-2 focus-visible:ring-orange-500"
              >
                <i className="i-mgc-close-cute-re size-4" aria-hidden />
              </button>
            )}
          </search>
        </div>
      </header>

      {!query && <p className="py-20 text-center text-sm text-zinc-500">输入关键词开始搜索</p>}
      {query.length > 200 && (
        <p role="alert" className="py-10 text-center text-sm text-red-300">
          搜索词不能超过 200 个字符
        </p>
      )}
      {validQuery && resultQuery.isPending && (
        <div aria-busy="true" className="grid gap-3 py-4 sm:grid-cols-2">
          {Array.from({ length: 6 }, (_, index) => (
            <div key={index} className="h-40 animate-pulse rounded-xl bg-[#17181b]" />
          ))}
        </div>
      )}
      {resultQuery.isError && (
        <div role="alert" className="py-16 text-center">
          <p className="text-sm text-red-300">{resultQuery.error.message}</p>
          <button
            type="button"
            onClick={() => resultQuery.refetch()}
            className="mt-3 min-h-11 rounded-xl bg-white/10 px-4 text-sm"
          >
            重试
          </button>
        </div>
      )}
      {indexStatus === "building" && (
        <p role="status" className="my-3 rounded-xl bg-amber-500/10 p-3 text-sm text-amber-200">
          搜索索引仍在构建，结果可能暂时不完整。
        </p>
      )}
      {validQuery && !resultQuery.isPending && !resultQuery.isError && (
        <div aria-live="polite">
          <p className="py-3 text-xs text-zinc-500">找到 {results.length} 条结果</p>
          {results.length === 0 ? (
            <p className="py-20 text-center text-sm text-zinc-500">没有找到相关内容</p>
          ) : (
            <div className="grid gap-3 sm:grid-cols-2">
              {results.map((card) => (
                <SearchResultCard key={card.entryId} card={card} />
              ))}
            </div>
          )}
          {resultQuery.hasNextPage && (
            <button
              type="button"
              disabled={resultQuery.isFetchingNextPage}
              onClick={() => resultQuery.fetchNextPage()}
              className="mx-auto mt-5 flex min-h-11 items-center rounded-xl bg-white/10 px-4 text-sm outline-none hover:bg-white/15 focus-visible:ring-2 focus-visible:ring-orange-500 disabled:opacity-50"
            >
              {resultQuery.isFetchingNextPage ? "加载中…" : "加载更多"}
            </button>
          )}
        </div>
      )}
    </section>
  )
}
