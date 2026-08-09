import { AIProviderForm } from "./AIProviderForm"
import { SettingsPageHeader } from "./SettingsPageHeader"

export function AISettingsPage() {
  return (
    <section className="mx-auto min-h-full w-full max-w-2xl px-4 py-3 sm:px-6">
      <SettingsPageHeader>本地 AI</SettingsPageHeader>
      <p className="mb-4 text-sm leading-6 text-zinc-400">
        翻译、摘要、分类和智能筛选使用 Go 服务端配置的固定 Gemini；Key 永不进入浏览器。
      </p>
      <AIProviderForm />
    </section>
  )
}
