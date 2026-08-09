export const homeQueryKeys = {
  all: ["home"] as const,
  scope: (topicId: string, filterId: string | null) => ["home", topicId, filterId] as const,
  feed: (topicId: string, filterId: string | null, generation: string | null) =>
    ["home", topicId, filterId, generation] as const,
  topics: ["topics"] as const,
}
