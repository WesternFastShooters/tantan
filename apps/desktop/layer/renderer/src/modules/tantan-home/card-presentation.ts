import type { HomeCard } from "~/lib/tantan-api/gen/types"

const safeRemoteURL = (value: string | null) => {
  if (!value) return null
  try {
    const url = new URL(value)
    return url.protocol === "http:" || url.protocol === "https:" ? url.toString() : null
  } catch {
    return null
  }
}

const aspectRatios = {
  article: "4 / 3",
  post: "4 / 3",
  image: "1 / 1",
  video: "16 / 9",
} as const

export const resolveCardPresentation = (card: HomeCard) => ({
  aspectRatio: aspectRatios[card.type],
  cover: safeRemoteURL(card.cover),
})
