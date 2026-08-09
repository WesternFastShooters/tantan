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
