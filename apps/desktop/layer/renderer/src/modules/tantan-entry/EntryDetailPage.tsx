import { FeedViewType } from "@follow/constants"
import { useIsEntryStarred } from "@follow/store/collection/hooks"
import { collectionSyncService } from "@follow/store/collection/store"
import { useEntry } from "@follow/store/entry/hooks"
import { unreadSyncService } from "@follow/store/unread/store"
import { useQuery, useQueryClient } from "@tanstack/react-query"
import { useEffect, useMemo, useRef, useState } from "react"
import { useLocation, useNavigate, useParams } from "react-router"

import type { HomeCard } from "~/lib/tantan-api/gen/types"
import { removeEntryFromAllHomeQueries } from "~/modules/tantan-home/home-cache"

import {
  getEntryCollectionStatus,
  getEntryDetail,
  markEntryAsReadDirect,
  updateEntryCollectionDirect,
} from "./entry-api"
import { useEntryEnrichment } from "./useEntryEnrichment"

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
  const detailQuery = useQuery({
    queryKey: ["tantan", "entry", entryId],
    queryFn: ({ signal }) => getEntryDetail(entryId, signal),
    enabled: entryId.length > 0,
    staleTime: 5 * 60_000,
  })
  // A Home card may seed the Folo store with only its excerpt. Prefer the
  // dedicated detail response so the original body remains available when AI
  // is missing or fails.
  const resolvedEntry = detailQuery.data ?? entry
  const enrichment = useEntryEnrichment(entryId)
  const storeStarred = useIsEntryStarred(entryId)
  const collectionStatusQuery = useQuery({
    queryKey: ["tantan", "entry", entryId, "collection"],
    queryFn: ({ signal }) => getEntryCollectionStatus(entryId, signal),
    enabled: entryId.length > 0 && Boolean(resolvedEntry) && !entry,
    staleTime: 30_000,
  })
  const [directStarred, setDirectStarred] = useState<boolean | null>(null)
  const [readError, setReadError] = useState<string | null>(null)
  const [collectionError, setCollectionError] = useState<string | null>(null)
  const [collectionPending, setCollectionPending] = useState(false)
  const [showOriginal, setShowOriginal] = useState(false)
  const attemptedEntryIdRef = useRef<string | null>(null)
  const collectionMutationLocked = useRef(false)

  useEffect(() => {
    if (!resolvedEntry || !entryId || attemptedEntryIdRef.current === entryId) return
    attemptedEntryIdRef.current = entryId
    if (resolvedEntry.read) {
      removeEntryFromAllHomeQueries(queryClient, entryId)
      return
    }
    const markRead = entry
      ? unreadSyncService.markEntryAsRead(entryId)
      : markEntryAsReadDirect(entryId)
    markRead
      .then(() => removeEntryFromAllHomeQueries(queryClient, entryId))
      .catch((error: unknown) => {
        attemptedEntryIdRef.current = null
        setReadError(error instanceof Error ? error.message : "已读同步失败")
      })
  }, [entry, entryId, queryClient, resolvedEntry])

  const sourceURL = safeURL(resolvedEntry?.url)
  const originalContent = useMemo(
    () => resolvedEntry?.content || resolvedEntry?.description || card?.excerpt || "正文暂不可用。",
    [card?.excerpt, resolvedEntry?.content, resolvedEntry?.description],
  )
  const publishedAt =
    card?.publishedAt ||
    (resolvedEntry?.publishedAt instanceof Date
      ? resolvedEntry.publishedAt.toISOString()
      : resolvedEntry?.publishedAt) ||
    null
  const enrichmentData = enrichment.data?.data
  const translationAvailable =
    enrichment.data?.state === "ready" ||
    (enrichment.data?.state !== "failed" && Boolean(card?.translated))
  const translatedContent = translationAvailable
    ? enrichmentData?.contentZh || card?.excerpt || null
    : null
  const translatedTitle = translationAvailable
    ? enrichmentData?.titleZh || card?.title || null
    : null
  const title = showOriginal
    ? resolvedEntry?.title || card?.title || "内容详情"
    : translatedTitle || resolvedEntry?.title || "内容详情"
  const content = showOriginal ? originalContent : translatedContent || originalContent
  const collectionView =
    card?.type === "image"
      ? FeedViewType.Pictures
      : card?.type === "video"
        ? FeedViewType.Videos
        : card?.type === "post"
          ? FeedViewType.SocialMedia
          : FeedViewType.Articles
  const starred = directStarred ?? (entry ? storeStarred : (collectionStatusQuery.data ?? false))

  const toggleCollection = async () => {
    if (!resolvedEntry || collectionMutationLocked.current) return
    collectionMutationLocked.current = true
    setCollectionPending(true)
    setCollectionError(null)
    try {
      if (entry) {
        if (starred) await collectionSyncService.unstarEntry({ entryId })
        else await collectionSyncService.starEntry({ entryId, view: collectionView })
      } else {
        await updateEntryCollectionDirect(entryId, collectionView, !starred)
        setDirectStarred(!starred)
      }
    } catch (error) {
      setCollectionError(error instanceof Error ? error.message : "收藏同步失败")
    } finally {
      collectionMutationLocked.current = false
      setCollectionPending(false)
    }
  }

  return (
    <article className="mx-auto min-h-full w-full max-w-3xl bg-[#08090b] px-4 pb-12 text-zinc-100 sm:px-8">
      <header className="sticky top-0 z-10 -mx-4 flex h-14 items-center gap-2 border-b border-white/[0.06] bg-[#08090b]/95 px-2 backdrop-blur sm:-mx-8 sm:px-6">
        <button
          type="button"
          aria-label="返回首页"
          onClick={() => navigate(-1)}
          className="flex size-11 items-center justify-center rounded-full text-zinc-200 outline-none hover:bg-white/10 focus-visible:ring-2 focus-visible:ring-orange-500"
        >
          <svg
            viewBox="0 0 24 24"
            fill="none"
            className="size-5"
            aria-hidden="true"
            focusable="false"
          >
            <path
              d="m15 18-6-6 6-6"
              stroke="currentColor"
              strokeWidth="2.25"
              strokeLinecap="round"
              strokeLinejoin="round"
            />
          </svg>
        </button>
        <span className="min-w-0 flex-1 truncate text-sm text-zinc-400">{card?.source.name}</span>
        <button
          type="button"
          aria-label={starred ? "取消收藏" : "收藏"}
          disabled={!resolvedEntry || collectionPending || collectionStatusQuery.isPending}
          onClick={toggleCollection}
          className="flex size-11 items-center justify-center rounded-full text-orange-400 outline-none hover:bg-orange-500/10 focus-visible:ring-2 focus-visible:ring-orange-500 disabled:opacity-40"
        >
          <i
            className={starred ? "i-mgc-star-cute-fi size-5" : "i-mgc-star-cute-re size-5"}
            aria-hidden
          />
        </button>
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
          <span>{resolvedEntry?.author || card?.source.name}</span>
          <time dateTime={publishedAt || undefined}>
            {publishedAt ? new Date(publishedAt).toLocaleString("zh-CN") : ""}
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
        {collectionError && (
          <p
            role="alert"
            className="mt-4 rounded-xl border border-red-500/20 bg-red-500/10 p-3 text-sm text-red-300"
          >
            {collectionError}
          </p>
        )}
        {detailQuery.isError && !resolvedEntry && (
          <p
            role="alert"
            className="mt-4 rounded-xl border border-amber-500/20 bg-amber-500/10 p-3 text-sm text-amber-200"
          >
            正文加载失败，正在显示首页摘要。
          </p>
        )}
        <section className="mt-5 rounded-xl border border-white/[0.06] bg-[#17181b] p-4">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <h2 className="font-semibold text-zinc-100">本地 AI</h2>
            {translatedContent && (
              <button
                type="button"
                onClick={() => setShowOriginal((value) => !value)}
                className="min-h-9 rounded-lg bg-white/10 px-3 text-xs outline-none hover:bg-white/15 focus-visible:ring-2 focus-visible:ring-orange-500"
              >
                {showOriginal ? "显示中文" : "显示原文"}
              </button>
            )}
          </div>
          {(enrichment.data?.state === "queued" || enrichment.data?.state === "processing") && (
            <p role="status" className="mt-2 text-sm text-amber-200">
              AI 处理中…原文仍可正常阅读。
            </p>
          )}
          {enrichment.data?.state === "failed" && (
            <p role="status" className="mt-2 text-sm text-red-300">
              AI 处理失败，已显示原文。
            </p>
          )}
          {(enrichment.data?.state === "missing" ||
            enrichment.data?.state === "failed" ||
            enrichment.isError) && (
            <button
              type="button"
              disabled={enrichment.ensuring}
              onClick={enrichment.ensure}
              className="mt-2 min-h-11 rounded-xl bg-orange-500 px-4 text-sm font-semibold text-white outline-none hover:bg-orange-400 focus-visible:ring-2 focus-visible:ring-orange-300 disabled:opacity-50"
            >
              {enrichment.ensuring
                ? "正在启动…"
                : enrichment.data?.state === "failed"
                  ? "重试翻译与摘要"
                  : "生成翻译与摘要"}
            </button>
          )}
          {enrichment.ensureError && (
            <p role="alert" className="mt-2 text-sm text-red-300">
              {enrichment.ensureError.message}
            </p>
          )}
          {enrichmentData?.summaryZh && (
            <div className="mt-3">
              <p className="text-sm leading-6 text-zinc-300">{enrichmentData.summaryZh}</p>
              {enrichmentData.keyPoints.length > 0 && (
                <ul className="mt-2 list-disc space-y-1 pl-5 text-sm text-zinc-400">
                  {enrichmentData.keyPoints.map((point) => (
                    <li key={point}>{point}</li>
                  ))}
                </ul>
              )}
            </div>
          )}
        </section>
        <div className="mt-7 whitespace-pre-wrap break-words text-[15px] leading-8 text-zinc-200">
          {content}
        </div>
      </div>
    </article>
  )
}
