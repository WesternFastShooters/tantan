export const homeQueryKeys = {
  all: ["home"] as const,
  feed: (topicId: string, filterId: string | null) => ["home", topicId, filterId] as const,
  topics: ["topics"] as const,
}
