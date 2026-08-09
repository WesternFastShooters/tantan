import { FeedViewType } from "@follow/constants"
import type { EntryModel } from "@follow/store/entry/types"
import type { FeedModel } from "@follow/store/feed/types"

import type { HomeCard } from "~/lib/tantan-api/gen/types"

import { EntryLink } from "./EntryLink"

const viewToType = (view: FeedViewType): HomeCard["type"] => {
  if (view === FeedViewType.Pictures) return "image"
  if (view === FeedViewType.Videos) return "video"
  if (view === FeedViewType.SocialMedia) return "post"
  return "article"
}

export const entryToHomeCard = (
  entry: EntryModel,
  feed: FeedModel | undefined,
  view: FeedViewType,
): HomeCard => ({
  entryId: entry.id,
  type: viewToType(view),
  title: entry.title || "无标题",
  excerpt: entry.description ?? null,
  cover: entry.media?.find((media) => media.type === "photo")?.url ?? null,
  source: {
    id: feed?.id ?? entry.feedId ?? "source",
    name: feed?.title ?? "订阅 Source",
    avatar: feed?.image ?? null,
  },
  publishedAt:
    entry.publishedAt instanceof Date ? entry.publishedAt.toISOString() : String(entry.publishedAt),
  topics: [],
  translated: false,
})

export function EntryListRow({
  entry,
  feed,
  view,
  action,
}: {
  entry: EntryModel
  feed?: FeedModel
  view: FeedViewType
  action?: React.ReactNode
}) {
  const card = entryToHomeCard(entry, feed, view)
  return (
    <article className="flex items-center gap-3 rounded-xl border border-white/[0.06] bg-[#17181b] p-3">
      <EntryLink
        card={card}
        className="min-w-0 flex-1 rounded-lg outline-none focus-visible:ring-2 focus-visible:ring-orange-500"
      >
        <p className="truncate text-sm font-medium text-zinc-100">{card.title}</p>
        <p className="mt-1 line-clamp-1 text-xs text-zinc-500">
          {card.source.name} · {new Date(card.publishedAt).toLocaleDateString("zh-CN")}
        </p>
      </EntryLink>
      {action}
    </article>
  )
}
