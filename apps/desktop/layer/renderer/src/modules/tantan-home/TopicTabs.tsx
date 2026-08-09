import type { KeyboardEvent } from "react"

import type { Topic } from "~/lib/tantan-api/gen/types"

interface TopicTabsProps {
  topics: Topic[]
  activeTopicId: string
  onChange: (topicId: string) => void
}

export function TopicTabs({ topics, activeTopicId, onChange }: TopicTabsProps) {
  const moveFocus = (event: KeyboardEvent<HTMLButtonElement>, index: number) => {
    let next = index
    if (event.key === "ArrowRight") next = (index + 1) % topics.length
    else if (event.key === "ArrowLeft") next = (index - 1 + topics.length) % topics.length
    else if (event.key === "Home") next = 0
    else if (event.key === "End") next = topics.length - 1
    else return
    event.preventDefault()
    const topic = topics[next]
    if (!topic) return
    onChange(topic.id)
    event.currentTarget.parentElement
      ?.querySelectorAll<HTMLButtonElement>('[role="tab"]')
      .item(next)
      .focus()
  }

  return (
    <div
      role="tablist"
      aria-label="AI 分类"
      className="flex h-10 items-stretch gap-1 overflow-x-auto px-3 [scrollbar-width:none] sm:px-4"
    >
      {topics.map((topic, index) => {
        const active = topic.id === activeTopicId
        return (
          <button
            type="button"
            role="tab"
            key={topic.id}
            aria-selected={active}
            tabIndex={active ? 0 : -1}
            onClick={() => onChange(topic.id)}
            onKeyDown={(event) => moveFocus(event, index)}
            className="relative flex min-w-max items-center px-3 text-sm font-medium text-zinc-400 outline-none hover:text-zinc-100 focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-orange-500 aria-selected:text-zinc-50"
          >
            {topic.name}
            {active && <span className="absolute inset-x-3 bottom-0 h-0.5 rounded bg-orange-500" />}
          </button>
        )
      })}
    </div>
  )
}
