import type { HomeCard } from "~/lib/tantan-api/gen/types"

export const dedupeHomeCards = (cards: readonly HomeCard[]): HomeCard[] => {
  const seen = new Set<string>()
  return cards.filter((card) => {
    if (seen.has(card.entryId)) return false
    seen.add(card.entryId)
    return true
  })
}

export const homeColumnCount = (viewportWidth: number) => {
  if (viewportWidth >= 1440) return 4
  if (viewportWidth >= 1024) return 3
  return 2
}
