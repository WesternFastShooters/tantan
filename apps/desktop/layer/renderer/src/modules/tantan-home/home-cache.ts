import type { InfiniteData, QueryClient } from "@tanstack/react-query"

import type { HomeResponse } from "~/lib/tantan-api/gen/types"

import { homeQueryKeys } from "./query-keys"

export const removeEntryFromAllHomeQueries = (queryClient: QueryClient, entryId: string) => {
  queryClient.setQueriesData<InfiniteData<HomeResponse>>(
    { queryKey: homeQueryKeys.all },
    (current) => {
      if (!current) return current
      const removed = current.pages.some((page) =>
        page.items.some((card) => card.entryId === entryId),
      )
      if (!removed) return current
      const seen = new Set<string>()
      const pages = current.pages.map((page) => ({
        ...page,
        items: page.items.filter((card) => {
          if (card.entryId === entryId) return false
          if (seen.has(card.entryId)) return false
          seen.add(card.entryId)
          return true
        }),
        queue: {
          ...page.queue,
          unread: Math.max(0, page.queue.unread - 1),
          finished: Math.max(0, page.queue.unread - 1) === 0,
        },
      }))
      return { ...current, pages }
    },
  )
}
