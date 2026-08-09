import { FeedViewType } from "@follow/constants"

export const SUBSCRIPTION_FILTERS = [
  { view: FeedViewType.Articles, label: "文章" },
  { view: FeedViewType.SocialMedia, label: "动态" },
  { view: FeedViewType.Pictures, label: "图片" },
  { view: FeedViewType.Videos, label: "视频" },
] as const

export const filterFeedSubscriptions = <T extends { view: FeedViewType }>(
  subscriptions: readonly T[],
  view: FeedViewType,
) => subscriptions.filter((subscription) => subscription.view === view)
