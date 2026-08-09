import { useEffect, useRef, useState } from "react"

interface AIFilterSheetProps {
  open: boolean
  pending: boolean
  error: string | null
  onClose: () => void
  onSubmit: (prompt: string) => void
}

export function AIFilterSheet({ open, pending, error, onClose, onSubmit }: AIFilterSheetProps) {
  const [prompt, setPrompt] = useState("")
  const [validation, setValidation] = useState<string | null>(null)
  const inputRef = useRef<HTMLTextAreaElement>(null)

  useEffect(() => {
    if (!open) return
    setValidation(null)
    const frame = requestAnimationFrame(() => inputRef.current?.focus())
    return () => cancelAnimationFrame(frame)
  }, [open])

  useEffect(() => {
    if (!open) return
    const escape = (event: KeyboardEvent) => {
      if (event.key === "Escape" && !pending) onClose()
    }
    document.addEventListener("keydown", escape)
    return () => document.removeEventListener("keydown", escape)
  }, [onClose, open, pending])

  if (!open) return null

  const submit = () => {
    const value = prompt.trim()
    if (value.length < 1 || value.length > 300) {
      setValidation("请输入 1～300 个字符")
      return
    }
    setValidation(null)
    onSubmit(value)
  }

  return (
    <div className="fixed inset-0 z-50 flex items-end justify-center md:items-center">
      <button
        type="button"
        aria-label="关闭 AI 智能筛选"
        onClick={() => !pending && onClose()}
        className="absolute inset-0 bg-black/70"
      />
      <section
        role="dialog"
        aria-modal="true"
        aria-labelledby="ai-filter-title"
        onKeyDown={(event) => {
          if (event.key !== "Tab") return
          const focusable = Array.from(
            event.currentTarget.querySelectorAll<HTMLElement>(
              'button:not(:disabled), textarea:not(:disabled), [href], [tabindex]:not([tabindex="-1"])',
            ),
          )
          const first = focusable[0]
          const last = focusable.at(-1)
          if (!first || !last) return
          if (event.shiftKey && document.activeElement === first) {
            event.preventDefault()
            last.focus()
          } else if (!event.shiftKey && document.activeElement === last) {
            event.preventDefault()
            first.focus()
          }
        }}
        className="relative w-full rounded-t-2xl border border-white/10 bg-[#17181b] p-4 shadow-2xl md:max-w-lg md:rounded-2xl md:p-5"
      >
        <div className="mb-4 flex items-start justify-between gap-3">
          <div>
            <h2 id="ai-filter-title" className="font-semibold text-zinc-50">
              AI 智能筛选
            </h2>
            <p className="mt-1 text-xs leading-5 text-zinc-400">
              描述你想多看和不想看的内容，提交后会重新生成首页。
            </p>
          </div>
          <button
            type="button"
            aria-label="取消"
            disabled={pending}
            onClick={onClose}
            className="flex size-11 shrink-0 items-center justify-center rounded-full text-zinc-400 outline-none hover:bg-white/10 focus-visible:ring-2 focus-visible:ring-orange-500 disabled:opacity-50"
          >
            <i className="i-mgc-close-cute-re size-5" aria-hidden />
          </button>
        </div>
        <label htmlFor="ai-filter-prompt" className="sr-only">
          筛选要求
        </label>
        <textarea
          id="ai-filter-prompt"
          ref={inputRef}
          rows={5}
          maxLength={300}
          value={prompt}
          disabled={pending}
          onChange={(event) => setPrompt(event.target.value)}
          placeholder="例如：最近一周多推 Claude Code 和 Codex，不要融资新闻"
          className="w-full resize-none rounded-xl border border-white/10 bg-black/20 p-3 text-sm leading-6 text-zinc-100 outline-none placeholder:text-zinc-600 focus:border-orange-500 focus:ring-1 focus:ring-orange-500 disabled:opacity-60"
        />
        <div className="mt-1 flex min-h-5 justify-between text-xs">
          <span role="alert" className="text-red-400">
            {validation ?? error}
          </span>
          <span className="text-zinc-500">{prompt.length}/300</span>
        </div>
        <div className="mt-3 flex justify-end gap-2">
          <button
            type="button"
            disabled={pending}
            onClick={onClose}
            className="min-h-11 rounded-xl px-4 text-sm text-zinc-300 outline-none hover:bg-white/10 focus-visible:ring-2 focus-visible:ring-orange-500 disabled:opacity-50"
          >
            取消
          </button>
          <button
            type="button"
            disabled={pending}
            onClick={submit}
            className="min-h-11 rounded-xl bg-orange-500 px-4 text-sm font-semibold text-white outline-none hover:bg-orange-400 focus-visible:ring-2 focus-visible:ring-orange-300 disabled:opacity-50"
          >
            {pending ? "正在生成…" : "生成信息流"}
          </button>
        </div>
      </section>
    </div>
  )
}
