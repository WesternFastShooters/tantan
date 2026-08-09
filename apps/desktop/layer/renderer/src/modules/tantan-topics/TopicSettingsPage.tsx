import { BlockedSources } from "~/modules/tantan-settings/BlockedSources"
import { SettingsPageHeader } from "~/modules/tantan-settings/SettingsPageHeader"

import { TopicManager } from "./TopicManager"

export function TopicSettingsPage() {
  return (
    <section className="mx-auto min-h-full w-full max-w-3xl px-4 py-3 sm:px-6">
      <SettingsPageHeader>频道管理</SettingsPageHeader>
      <p className="mb-4 text-sm leading-6 text-zinc-400">
        调整首页 Topic 的顺序、固定与显示状态。“推荐”始终保留在首位。
      </p>
      <TopicManager />
      <BlockedSources />
    </section>
  )
}
