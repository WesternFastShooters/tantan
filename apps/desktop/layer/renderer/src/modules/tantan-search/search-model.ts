import type { HomeCard } from "~/lib/tantan-api/gen/types"

export const SEARCH_DEBOUNCE_MS = 250

export const buildSearchRequest = (value: string, cursor: string | null) => ({
  q: value.trim(),
  cursor,
  limit: 20 as const,
})

export const mergeSearchPages = (pages: readonly (readonly HomeCard[])[]) => {
  const seen = new Set<string>()
  return pages.flatMap((page) =>
    page.filter((card) => {
      if (seen.has(card.entryId)) return false
      seen.add(card.entryId)
      return true
    }),
  )
}
