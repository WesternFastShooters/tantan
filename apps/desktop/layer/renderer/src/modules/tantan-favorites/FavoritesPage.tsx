import { FeedViewType } from "@follow/constants"
import { collectionSyncService } from "@follow/store/collection/store"
import { useEntriesQuery, useEntryList } from "@follow/store/entry/hooks"
import type { EntryModel } from "@follow/store/entry/types"
import { useFeedById } from "@follow/store/feed/hooks"
import { useRef, useState } from "react"
import { Link } from "react-router"

import { EntryListRow } from "~/modules/tantan-entry/EntryListRow"

const FavoriteRow = ({ entry }: { entry: EntryModel }) => {
  const feed = useFeedById(entry.feedId)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const mutationLocked = useRef(false)
  return (
    <div>
      <EntryListRow
        entry={entry}
        feed={feed}
        view={FeedViewType.All}
        action={
          <button
            type="button"
            aria-label={`取消收藏 ${entry.title || "无标题"}`}
            disabled={pending}
            onClick={async () => {
              if (mutationLocked.current) return
              mutationLocked.current = true
              setPending(true)
              setError(null)
              try {
                await collectionSyncService.unstarEntry({ entryId: entry.id })
              } catch (reason) {
                setError(reason instanceof Error ? reason.message : "取消收藏失败")
              } finally {
                mutationLocked.current = false
                setPending(false)
              }
            }}
            className="flex size-11 shrink-0 items-center justify-center rounded-lg text-orange-400 outline-none hover:bg-orange-500/10 focus-visible:ring-2 focus-visible:ring-orange-500 disabled:opacity-50"
          >
            <i className="i-mgc-star-cute-fi size-5" aria-hidden />
          </button>
        }
      />
      {error && (
        <p role="alert" className="mt-1 rounded-xl bg-red-500/10 p-2 text-xs text-red-300">
          {error}
        </p>
      )}
    </div>
  )
}

export function FavoritesPage() {
  const entriesQuery = useEntriesQuery({
    feedId: "collections",
    view: FeedViewType.All,
    isCollection: true,
    limit: 20,
  })
  const entries = useEntryList(entriesQuery.entriesIds).filter((entry) => entry !== null)

  return (
    <section className="mx-auto min-h-full w-full max-w-3xl px-4 py-3 pb-20 sm:px-6">
      <header className="flex min-h-14 items-center gap-2">
        <Link
          to="/subscriptions"
          aria-label="返回订阅"
          className="flex size-11 items-center justify-center rounded-full text-zinc-300 outline-none hover:bg-white/10 focus-visible:ring-2 focus-visible:ring-orange-500"
        >
          <i className="i-mgc-left-cute-re size-5" aria-hidden />
        </Link>
        <div>
          <h1 className="text-xl font-bold text-zinc-50">收藏</h1>
          <p className="text-xs text-zinc-500">收藏与已读状态互不影响</p>
        </div>
      </header>
      {entriesQuery.isLoading && (
        <div aria-busy="true" className="mt-3 h-52 animate-pulse rounded-xl bg-[#17181b]" />
      )}
      {entriesQuery.error && (
        <p role="alert" className="mt-3 text-sm text-red-300">
          {entriesQuery.error.message}
        </p>
      )}
      <div className="mt-3 space-y-2">
        {entries.map((entry) => (
          <FavoriteRow key={entry.id} entry={entry} />
        ))}
      </div>
      {!entriesQuery.isLoading && entries.length === 0 && (
        <p className="py-20 text-center text-sm text-zinc-500">还没有收藏内容</p>
      )}
      {entriesQuery.hasNextPage && (
        <button
          type="button"
          disabled={entriesQuery.isFetchingNextPage}
          onClick={() => entriesQuery.fetchNextPage()}
          className="mx-auto mt-4 flex min-h-11 items-center rounded-xl bg-white/10 px-4 text-sm disabled:opacity-50"
        >
          {entriesQuery.isFetchingNextPage ? "加载中…" : "加载更多"}
        </button>
      )}
    </section>
  )
}
