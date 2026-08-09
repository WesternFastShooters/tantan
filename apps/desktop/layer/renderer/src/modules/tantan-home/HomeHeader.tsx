interface HomeHeaderProps {
  onSearch: () => void
  onOpenAIFilter: () => void
}

export function HomeHeader({ onSearch, onOpenAIFilter }: HomeHeaderProps) {
  return (
    <header className="flex h-14 items-center justify-between gap-3 px-3 sm:px-4">
      <div className="min-w-0">
        <h1 className="truncate text-lg font-bold tracking-tight text-zinc-50">今日推荐</h1>
        <p className="hidden text-xs text-zinc-500 sm:block">最近 7 天未读 · 每日稳定队列</p>
      </div>
      <div className="flex shrink-0 items-center gap-1">
        <button
          type="button"
          aria-label="搜索内容"
          onClick={onSearch}
          className="flex size-11 items-center justify-center rounded-full text-zinc-300 outline-none hover:bg-white/10 focus-visible:ring-2 focus-visible:ring-orange-500"
        >
          <i className="i-mgc-search-2-cute-re size-5" aria-hidden />
        </button>
        <button
          type="button"
          aria-label="AI 智能筛选"
          onClick={onOpenAIFilter}
          className="flex size-11 items-center justify-center rounded-full text-orange-400 outline-none hover:bg-orange-500/10 focus-visible:ring-2 focus-visible:ring-orange-500"
        >
          <i className="i-mgc-sparkles-2-cute-re size-5" aria-hidden />
        </button>
      </div>
    </header>
  )
}
