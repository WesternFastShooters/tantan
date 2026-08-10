import { useEffect, useRef, useState } from "react"

import { AISparklesIcon } from "./AISparklesIcon"

const suggestions = [
  { label: "AI Agent", prompt: "多推 AI Agent" },
  { label: "3D 项目", prompt: "多推 3D 项目" },
  { label: "前端新技术", prompt: "多推前端新技术" },
  { label: "只看英文一手来源", prompt: "只看英文一手来源" },
  { label: "过滤营销内容", prompt: "过滤营销内容" },
] as const

interface AIFilterSheetProps {
  open: boolean
  initialPrompt?: string
  pending: boolean
  error: string | null
  onClose: () => void
  onSubmit: (prompt: string) => void
}

export function AIFilterSheet({
  open,
  initialPrompt = "",
  pending,
  error,
  onClose,
  onSubmit,
}: AIFilterSheetProps) {
  const [prompt, setPrompt] = useState("")
  const [selectedSuggestions, setSelectedSuggestions] = useState<string[]>([])
  const [validation, setValidation] = useState<string | null>(null)
  const inputRef = useRef<HTMLTextAreaElement>(null)

  useEffect(() => {
    if (!open) return
    setPrompt(initialPrompt)
    setSelectedSuggestions([])
    setValidation(null)
    const frame = requestAnimationFrame(() => inputRef.current?.focus())
    return () => cancelAnimationFrame(frame)
  }, [initialPrompt, open])

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
    <div className="fixed inset-0 z-50 flex items-end justify-center">
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
        className="relative max-h-[70dvh] w-full overflow-y-auto rounded-t-3xl border border-white/10 bg-[#1f2025] px-4 pb-[max(1.5rem,env(safe-area-inset-bottom))] shadow-2xl"
      >
        <div className="flex justify-center pb-3 pt-3" aria-hidden>
          <span className="h-1 w-9 rounded-full bg-[#3a3b42]" />
        </div>
        <div className="mb-3 flex items-start justify-between gap-3">
          <div>
            <h2
              id="ai-filter-title"
              className="flex items-center gap-2 text-base font-bold text-zinc-50"
            >
              <AISparklesIcon className="size-4 text-orange-500" /> AI 智能筛选
            </h2>
            <p className="mt-1 text-xs leading-5 text-zinc-400">
              用自然语言描述你想看的内容，AI 将为你生成个性化信息流
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
          className="w-full resize-none rounded-xl border border-white/10 bg-[#17181b] p-3 text-sm leading-6 text-zinc-100 outline-none placeholder:text-zinc-600 focus:border-orange-500 focus:ring-1 focus:ring-orange-500 disabled:opacity-60"
        />
        <div className="mt-1 flex min-h-5 justify-between text-xs">
          <span role="alert" className="text-red-400">
            {validation ?? error}
          </span>
          <span className="text-zinc-500">{prompt.length}/300</span>
        </div>
        <p className="mb-2 mt-3 text-sm font-semibold text-zinc-100">推荐主题</p>
        <div className="flex flex-wrap gap-2">
          {suggestions.map((suggestion) => {
            const selected = selectedSuggestions.includes(suggestion.label)
            return (
              <button
                type="button"
                key={suggestion.label}
                aria-pressed={selected}
                disabled={pending}
                onClick={() => {
                  const next = selected
                    ? selectedSuggestions.filter((item) => item !== suggestion.label)
                    : [...selectedSuggestions, suggestion.label]
                  setSelectedSuggestions(next)
                  setPrompt(
                    suggestions
                      .filter((item) => next.includes(item.label))
                      .map((item) => item.prompt)
                      .join("；"),
                  )
                }}
                className="min-h-9 rounded-full border border-white/10 px-3 text-xs text-zinc-400 outline-none hover:border-orange-500/60 hover:text-orange-400 focus-visible:ring-2 focus-visible:ring-orange-500 disabled:opacity-50 aria-pressed:border-orange-500 aria-pressed:bg-orange-500/10 aria-pressed:text-orange-400"
              >
                {suggestion.label}
              </button>
            )
          })}
        </div>
        <div className="mt-5 flex gap-3">
          <button
            type="button"
            disabled={pending}
            onClick={onClose}
            className="min-h-12 flex-1 rounded-xl bg-[#17181b] px-4 text-sm font-semibold text-zinc-400 outline-none hover:bg-white/10 focus-visible:ring-2 focus-visible:ring-orange-500 disabled:opacity-50"
          >
            取消
          </button>
          <button
            type="button"
            disabled={pending}
            onClick={submit}
            className="flex min-h-12 flex-1 items-center justify-center gap-2 rounded-xl bg-orange-500 px-4 text-sm font-semibold text-white outline-none hover:bg-orange-400 focus-visible:ring-2 focus-visible:ring-orange-300 disabled:opacity-50"
          >
            {!pending && <AISparklesIcon className="size-4" />}
            {pending ? "正在生成…" : "生成信息流"}
          </button>
        </div>
      </section>
    </div>
  )
}
