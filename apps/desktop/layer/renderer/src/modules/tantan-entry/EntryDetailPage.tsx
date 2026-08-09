import { useEntry, usePrefetchEntryDetail } from "@follow/store/entry/hooks"
import { unreadSyncService } from "@follow/store/unread/store"
import { useQueryClient } from "@tanstack/react-query"
import { useEffect, useMemo, useRef, useState } from "react"
import { useLocation, useNavigate, useParams } from "react-router"

import type { HomeCard } from "~/lib/tantan-api/gen/types"
import { removeEntryFromAllHomeQueries } from "~/modules/tantan-home/home-cache"

const safeURL = (value: string | null | undefined) => {
  if (!value) return null
  try {
    const url = new URL(value)
    return url.protocol === "http:" || url.protocol === "https:" ? url.toString() : null
  } catch {
    return null
  }
}

export function EntryDetailPage() {
  const { entryId = "" } = useParams()
  const navigate = useNavigate()
  const location = useLocation()
  const queryClient = useQueryClient()
  const card = (location.state as { card?: HomeCard } | null)?.card
  const entry = useEntry(entryId, (value) => ({
    title: value.title,
    description: value.description,
    content: value.content,
    url: value.url,
    author: value.author,
    publishedAt: value.publishedAt,
    read: value.read,
  }))
  const detailQuery = usePrefetchEntryDetail(entryId)
  const [readError, setReadError] = useState<string | null>(null)
  const attemptedEntryIdRef = useRef<string | null>(null)

  useEffect(() => {
    if (!entry || !entryId || attemptedEntryIdRef.current === entryId) return
    attemptedEntryIdRef.current = entryId
    if (entry.read) {
      removeEntryFromAllHomeQueries(queryClient, entryId)
      return
    }
    unreadSyncService
      .markEntryAsRead(entryId)
      .then(() => removeEntryFromAllHomeQueries(queryClient, entryId))
      .catch((error: unknown) => {
        attemptedEntryIdRef.current = null
        setReadError(error instanceof Error ? error.message : "已读同步失败")
      })
  }, [entry, entryId, queryClient])

  const sourceURL = safeURL(entry?.url)
  const content = useMemo(
    () => entry?.content || entry?.description || card?.excerpt || "正文暂不可用。",
    [card?.excerpt, entry?.content, entry?.description],
  )
  const title = entry?.title || card?.title || "内容详情"

  return (
    <article className="mx-auto min-h-full w-full max-w-3xl bg-[#08090b] px-4 pb-12 text-zinc-100 sm:px-8">
      <header className="sticky top-0 z-10 -mx-4 flex h-14 items-center gap-2 border-b border-white/[0.06] bg-[#08090b]/95 px-2 backdrop-blur sm:-mx-8 sm:px-6">
        <button
          type="button"
          aria-label="返回首页"
          onClick={() => navigate(-1)}
          className="flex size-11 items-center justify-center rounded-full text-zinc-200 outline-none hover:bg-white/10 focus-visible:ring-2 focus-visible:ring-orange-500"
        >
          <i className="i-mgc-left-cute-re size-5" aria-hidden />
        </button>
        <span className="min-w-0 flex-1 truncate text-sm text-zinc-400">{card?.source.name}</span>
        {sourceURL && (
          <a
            href={sourceURL}
            target="_blank"
            rel="noreferrer noopener"
            className="flex min-h-11 items-center rounded-xl px-3 text-xs text-orange-400 outline-none hover:bg-orange-500/10 focus-visible:ring-2 focus-visible:ring-orange-500"
          >
            原文
          </a>
        )}
      </header>

      <div className="pt-7">
        <p className="text-xs text-orange-400">
          {card?.topics.map((topic) => topic.name).join(" · ")}
        </p>
        <h1 className="mt-2 text-2xl font-bold leading-tight tracking-tight sm:text-3xl">
          {title}
        </h1>
        <div className="mt-3 flex flex-wrap gap-x-3 gap-y-1 text-xs text-zinc-500">
          <span>{entry?.author || card?.source.name}</span>
          <time dateTime={card?.publishedAt}>
            {card?.publishedAt ? new Date(card.publishedAt).toLocaleString("zh-CN") : ""}
          </time>
        </div>
        {readError && (
          <p
            role="alert"
            className="mt-4 rounded-xl border border-red-500/20 bg-red-500/10 p-3 text-sm text-red-300"
          >
            已读同步失败，卡片已保留：{readError}
          </p>
        )}
        {detailQuery.isError && !entry && (
          <p
            role="alert"
            className="mt-4 rounded-xl border border-amber-500/20 bg-amber-500/10 p-3 text-sm text-amber-200"
          >
            正文加载失败，正在显示首页摘要。
          </p>
        )}
        <div className="mt-7 whitespace-pre-wrap break-words text-[15px] leading-8 text-zinc-200">
          {content}
        </div>
      </div>
    </article>
  )
}
