import { tantanRequest } from "~/lib/tantan-api/client"
import type { SearchResponse } from "~/lib/tantan-api/gen/types"

import { buildSearchRequest } from "./search-model"

export const searchEntries = ({
  q,
  cursor,
  signal,
}: {
  q: string
  cursor: string | null
  signal?: AbortSignal
}) => {
  const request = buildSearchRequest(q, cursor)
  const search = new URLSearchParams({ q: request.q, limit: String(request.limit) })
  if (request.cursor) search.set("cursor", request.cursor)
  return tantanRequest<SearchResponse>(`/tantan/v1/search?${search.toString()}`, { signal })
}
