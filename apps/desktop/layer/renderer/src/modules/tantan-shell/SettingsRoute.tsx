import { TantanShellPage } from "./TantanAppShell"

export function SettingsRoute() {
  return (
    <TantanShellPage>
      <h1 className="text-2xl font-bold tracking-tight">设置</h1>
      <p className="mt-2 text-sm text-zinc-400">AI Provider、Topic 和阅读偏好将在本地保存。</p>
    </TantanShellPage>
  )
}
