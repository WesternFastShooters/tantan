import { FeedViewType } from "@follow/constants"
import { describe, expect, test } from "vitest"

import { filterFeedSubscriptions, SUBSCRIPTION_FILTERS } from "./subscription-model"

describe("Tantan RSS subscription model", () => {
  test("REQ:FE-04 preserves RSS subscriptions while exposing four content filters", () => {
    expect(SUBSCRIPTION_FILTERS).toHaveLength(4)
    const subscriptions = [
      { feedId: "feed-a", view: FeedViewType.Articles },
      { feedId: "feed-b", view: FeedViewType.Pictures },
      { feedId: "feed-c", view: FeedViewType.Videos },
    ]
    const snapshot = structuredClone(subscriptions)

    expect(filterFeedSubscriptions(subscriptions, FeedViewType.Pictures)).toEqual([
      subscriptions[1],
    ])
    expect(subscriptions).toEqual(snapshot)
  })
})
