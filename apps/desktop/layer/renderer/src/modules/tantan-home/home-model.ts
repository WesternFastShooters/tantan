import type { HomeCard, HomeResponse } from "~/lib/tantan-api/gen/types"

export interface HomePageParam {
  cursor: string | null
  generation: string | null
}

export class HomeQueueGenerationChangedError extends Error {
  readonly code = "HOME_QUEUE_GENERATION_CHANGED"

  constructor() {
    super("HOME_QUEUE_GENERATION_CHANGED")
    this.name = "HomeQueueGenerationChangedError"
  }
}

export const assertHomePageGeneration = (
  page: HomeResponse,
  expectedGeneration: string | null,
): HomeResponse => {
  if (expectedGeneration && page.queueGeneration !== expectedGeneration) {
    throw new HomeQueueGenerationChangedError()
  }
  return page
}

export const nextHomePageParam = (page: HomeResponse): HomePageParam | undefined =>
  page.nextCursor ? { cursor: page.nextCursor, generation: page.queueGeneration } : undefined

export const dedupeHomeCards = (cards: readonly HomeCard[]): HomeCard[] => {
  const seen = new Set<string>()
  return cards.filter((card) => {
    if (seen.has(card.entryId)) return false
    seen.add(card.entryId)
    return true
  })
}

export const homeColumnCount = (_viewportWidth: number) => 2
