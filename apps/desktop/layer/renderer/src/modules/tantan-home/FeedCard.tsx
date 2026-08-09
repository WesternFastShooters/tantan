import { useState } from "react"

import type { FeedbackRequest, HomeCard } from "~/lib/tantan-api/gen/types"
import { EntryLink } from "~/modules/tantan-entry/EntryLink"

import { resolveCardPresentation } from "./card-presentation"

interface FeedCardProps {
  card: HomeCard
  onOpen: () => void
  onFeedback: (card: HomeCard, action: FeedbackRequest["action"], topicId?: string) => void
}

const publishedLabel = (value: string) => {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ""
  return new Intl.DateTimeFormat("zh-CN", { month: "numeric", day: "numeric" }).format(date)
}

export function FeedCard({ card, onOpen, onFeedback }: FeedCardProps) {
  const presentation = resolveCardPresentation(card)
  const [coverFailed, setCoverFailed] = useState(false)
  const [menuOpen, setMenuOpen] = useState(false)
  const cover = coverFailed ? null : presentation.cover

  return (
    <article
      data-testid="home-card"
      data-entry-id={card.entryId}
      data-entry-type={card.type}
      className="group relative overflow-hidden rounded-xl bg-[#17181b] shadow-sm ring-1 ring-white/[0.06] transition-transform duration-200 hover:-translate-y-0.5 hover:ring-white/15"
    >
      <EntryLink
        card={card}
        onClick={onOpen}
        className="block rounded-xl outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-orange-500"
        aria-label={`阅读：${card.title}`}
      >
        {cover && (
          <img
            src={cover}
            alt=""
            loading="lazy"
            decoding="async"
            onError={() => setCoverFailed(true)}
            style={{ aspectRatio: presentation.aspectRatio }}
            className="w-full bg-zinc-900 object-cover"
          />
        )}
        <div className="p-3">
          <div className="mb-2 flex items-center gap-1.5 text-[11px] text-zinc-500">
            <span className="rounded bg-white/5 px-1.5 py-0.5 uppercase">{card.type}</span>
            {card.translated && <span>已翻译</span>}
          </div>
          <h2 className="line-clamp-3 text-sm font-semibold leading-5 text-zinc-100">
            {card.title}
          </h2>
          {card.excerpt && (
            <p className="mt-1.5 line-clamp-2 text-xs leading-5 text-zinc-400">{card.excerpt}</p>
          )}
          <footer className="mt-3 flex min-w-0 items-center gap-2 text-[11px] text-zinc-500">
            {card.source.avatar ? (
              <img
                src={card.source.avatar}
                alt=""
                loading="lazy"
                className="size-5 shrink-0 rounded-full object-cover"
                onError={(event) => {
                  event.currentTarget.hidden = true
                }}
              />
            ) : (
              <span className="flex size-5 shrink-0 items-center justify-center rounded-full bg-white/10 text-[9px] text-zinc-300">
                {card.source.name.slice(0, 1)}
              </span>
            )}
            <span className="min-w-0 flex-1 truncate">{card.source.name}</span>
            <time dateTime={card.publishedAt}>{publishedLabel(card.publishedAt)}</time>
          </footer>
        </div>
      </EntryLink>
      <button
        type="button"
        aria-label={`更多操作：${card.title}`}
        aria-expanded={menuOpen}
        onClick={() => setMenuOpen((value) => !value)}
        className="absolute right-1 top-1 flex size-11 items-center justify-center rounded-full bg-black/45 text-zinc-200 opacity-100 outline-none backdrop-blur focus-visible:ring-2 focus-visible:ring-orange-500 md:opacity-0 md:focus:opacity-100 md:group-hover:opacity-100"
      >
        <i className="i-mgc-more-1-cute-re size-5" aria-hidden />
      </button>
      {menuOpen && (
        <div className="absolute right-2 top-11 z-10 min-w-32 rounded-xl border border-white/10 bg-zinc-900 p-1 shadow-xl">
          <button
            type="button"
            onClick={() => {
              setMenuOpen(false)
              onFeedback(card, "not_interested")
            }}
            className="min-h-10 w-full rounded-lg px-3 text-left text-xs text-zinc-200 outline-none hover:bg-white/10 focus-visible:ring-2 focus-visible:ring-orange-500"
          >
            不感兴趣
          </button>
          <button
            type="button"
            onClick={() => {
              setMenuOpen(false)
              onFeedback(card, "block_source")
            }}
            className="min-h-10 w-full rounded-lg px-3 text-left text-xs text-zinc-200 outline-none hover:bg-white/10 focus-visible:ring-2 focus-visible:ring-orange-500"
          >
            屏蔽 Source：{card.source.name}
          </button>
          {card.topics[0] && (
            <button
              type="button"
              onClick={() => {
                setMenuOpen(false)
                onFeedback(card, "block_topic", card.topics[0]?.id)
              }}
              className="min-h-10 w-full rounded-lg px-3 text-left text-xs text-zinc-200 outline-none hover:bg-white/10 focus-visible:ring-2 focus-visible:ring-orange-500"
            >
              少看 Topic：{card.topics[0].name}
            </button>
          )}
        </div>
      )}
    </article>
  )
}
