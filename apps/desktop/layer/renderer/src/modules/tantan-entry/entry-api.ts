import type { FeedViewType } from "@follow/constants"

import { tantanRequest } from "~/lib/tantan-api/client"

export interface EntryDetail {
  id: string
  title: string
  description: string | null
  content: string | null
  url: string | null
  author: string | null
  publishedAt: string | null
  read: boolean
}

interface FoloEntryResponse {
  data?: {
    entries?: {
      id?: string
      title?: string | null
      description?: string | null
      content?: string | null
      url?: string | null
      author?: string | null
      publishedAt?: string | null
      read?: boolean | null
    }
  }
}

export const getEntryDetail = async (entryId: string, signal?: AbortSignal) => {
  const search = new URLSearchParams({ id: entryId })
  const response = await tantanRequest<FoloEntryResponse>(`/api/folo/entries?${search}`, { signal })
  const entry = response.data?.entries
  if (!entry || entry.id !== entryId) throw new Error("正文不存在")
  return {
    id: entry.id,
    title: entry.title || "内容详情",
    description: entry.description ?? null,
    content: entry.content ?? null,
    url: entry.url ?? null,
    author: entry.author ?? null,
    publishedAt: entry.publishedAt ?? null,
    read: entry.read === true,
  } satisfies EntryDetail
}

export const markEntryAsReadDirect = (entryId: string) =>
  tantanRequest<{ data: null }>("/api/folo/reads", {
    method: "POST",
    body: JSON.stringify({ entryIds: [entryId], isInbox: false }),
  })

export const getEntryCollectionStatus = async (entryId: string, signal?: AbortSignal) => {
  const search = new URLSearchParams({ entryId })
  const response = await tantanRequest<{ data: boolean | null }>(
    `/api/folo/collections?${search}`,
    { signal },
  )
  return response.data === true
}

export const updateEntryCollectionDirect = (
  entryId: string,
  view: FeedViewType,
  starred: boolean,
) =>
  tantanRequest<void>("/api/folo/collections", {
    method: starred ? "POST" : "DELETE",
    body: JSON.stringify(starred ? { entryId, view } : { entryId }),
  })
