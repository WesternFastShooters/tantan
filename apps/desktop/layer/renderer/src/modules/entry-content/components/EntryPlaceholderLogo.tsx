import { stopPropagation } from "@follow/utils/dom"

export const EntryPlaceholderLogo = () => (
  <div
    data-hide-in-print
    onContextMenu={stopPropagation}
    className="flex w-full min-w-0 flex-col items-center justify-center gap-3 px-12 pb-6 text-center text-text-secondary"
  >
    <i className="i-mgc-news-cute-re size-14 text-text-tertiary" />
    <p className="text-base font-medium">Select an article to start reading</p>
  </div>
)
