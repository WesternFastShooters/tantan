import { tantanRequest } from "~/lib/tantan-api/client"
import type { ContentPoolResponse } from "~/lib/tantan-api/gen/types"

export const getContentPoolPage = ({
  sourceId,
  cursor,
  signal,
}: {
  sourceId: string
  cursor: string | null
  signal?: AbortSignal
}) => {
  const search = new URLSearchParams({ sourceId, limit: "20" })
  if (cursor) search.set("cursor", cursor)
  return tantanRequest<ContentPoolResponse>(`/api/tantan/v1/content-pool?${search.toString()}`, {
    signal,
  })
}
