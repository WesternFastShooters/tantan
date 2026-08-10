interface ActiveAIFilterBarProps {
  prompt: string | null
  onEdit: () => void
  onReset: () => void
  resetting: boolean
}

export function ActiveAIFilterBar({ prompt, onEdit, onReset, resetting }: ActiveAIFilterBarProps) {
  if (!prompt) return null
  return (
    <div
      data-testid="active-ai-filter"
      className="mx-3 mt-2 flex min-h-10 items-center gap-2 rounded-xl border border-orange-500/25 bg-orange-500/10 px-3 text-xs text-orange-100 sm:mx-4"
    >
      <i className="i-mgc-magic-2-cute-re size-4 shrink-0 text-orange-400" aria-hidden />
      <span className="min-w-0 flex-1 truncate">{prompt}</span>
      <button
        type="button"
        disabled={resetting}
        onClick={onEdit}
        className="min-h-9 shrink-0 rounded-lg px-2 font-medium text-orange-300 outline-none hover:bg-orange-500/15 focus-visible:ring-2 focus-visible:ring-orange-500 disabled:opacity-50"
      >
        编辑筛选
      </button>
      <button
        type="button"
        disabled={resetting}
        onClick={onReset}
        className="min-h-9 shrink-0 rounded-lg px-2 font-medium text-orange-300 outline-none hover:bg-orange-500/15 focus-visible:ring-2 focus-visible:ring-orange-500 disabled:opacity-50"
      >
        重置
      </button>
    </div>
  )
}
