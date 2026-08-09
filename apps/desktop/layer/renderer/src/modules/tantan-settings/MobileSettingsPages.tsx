import { useThemeAtomValue } from "@follow/hooks"
import type { PropsWithChildren } from "react"

import { setGeneralSetting, useGeneralSettingKey } from "~/atoms/settings/general"
import { setUISetting, useUISettingKey } from "~/atoms/settings/ui"
import { useSetTheme } from "~/hooks/common"

import { SettingsPageHeader } from "./SettingsPageHeader"

const Page = ({ title, children }: PropsWithChildren<{ title: string }>) => (
  <section className="mx-auto min-h-full w-full px-4 pb-8 pt-[max(0.5rem,env(safe-area-inset-top))]">
    <SettingsPageHeader>{title}</SettingsPageHeader>
    {children}
  </section>
)

const SettingGroup = ({ title, children }: PropsWithChildren<{ title: string }>) => (
  <section className="mt-5" data-settings-group>
    <h2 className="mb-2 px-1 text-xs font-medium text-zinc-500">{title}</h2>
    <div className="overflow-hidden rounded-2xl bg-white shadow-sm ring-1 ring-zinc-200/70 dark:bg-[#17181b] dark:ring-white/[0.07]">
      {children}
    </div>
  </section>
)

const SettingRow = ({
  label,
  description,
  children,
}: PropsWithChildren<{ label: string; description?: string }>) => (
  <div className="flex min-h-16 items-center gap-3 border-b border-zinc-100 px-4 py-3 last:border-b-0 dark:border-white/[0.06]">
    <div className="min-w-0 flex-1">
      <p className="text-sm font-medium">{label}</p>
      {description && <p className="mt-0.5 text-xs leading-5 text-zinc-500">{description}</p>}
    </div>
    {children}
  </div>
)

const Toggle = ({
  label,
  checked,
  onChange,
}: {
  label: string
  checked: boolean
  onChange: (value: boolean) => void
}) => (
  <button
    type="button"
    role="switch"
    aria-label={label}
    aria-checked={checked}
    onClick={() => onChange(!checked)}
    className="relative h-7 w-12 shrink-0 rounded-full bg-zinc-300 outline-none transition-colors focus-visible:ring-2 focus-visible:ring-orange-500 aria-checked:bg-orange-500 dark:bg-zinc-700"
  >
    <span
      className={`absolute left-1 top-1 size-5 rounded-full bg-white shadow transition-transform ${checked ? "translate-x-5" : ""}`}
    />
  </button>
)

export function GeneralSettingsPage() {
  const unreadOnly = useGeneralSettingKey("unreadOnly")
  const scrollMarkUnread = useGeneralSettingKey("scrollMarkUnread")
  const autoGroup = useGeneralSettingKey("autoGroup")
  const hideAllReadSubscriptions = useGeneralSettingKey("hideAllReadSubscriptions")
  const openLinksInExternalApp = useGeneralSettingKey("openLinksInExternalApp")
  const translation = useGeneralSettingKey("translation")
  const summary = useGeneralSettingKey("summary")

  return (
    <Page title="通用">
      <SettingGroup title="阅读">
        <SettingRow label="仅显示未读" description="进入订阅时间线时优先显示未读内容">
          <Toggle
            label="仅显示未读"
            checked={unreadOnly}
            onChange={(value) => setGeneralSetting("unreadOnly", value)}
          />
        </SettingRow>
        <SettingRow label="滚动时标记已读" description="内容滚出阅读区域后标记为已读">
          <Toggle
            label="滚动时标记已读"
            checked={scrollMarkUnread}
            onChange={(value) => setGeneralSetting("scrollMarkUnread", value)}
          />
        </SettingRow>
        <SettingRow label="在外部应用打开链接">
          <Toggle
            label="在外部应用打开链接"
            checked={openLinksInExternalApp}
            onChange={(value) => setGeneralSetting("openLinksInExternalApp", value)}
          />
        </SettingRow>
      </SettingGroup>
      <SettingGroup title="订阅">
        <SettingRow label="自动分组订阅">
          <Toggle
            label="自动分组订阅"
            checked={autoGroup}
            onChange={(value) => setGeneralSetting("autoGroup", value)}
          />
        </SettingRow>
        <SettingRow label="隐藏全部已读的订阅">
          <Toggle
            label="隐藏全部已读的订阅"
            checked={hideAllReadSubscriptions}
            onChange={(value) => setGeneralSetting("hideAllReadSubscriptions", value)}
          />
        </SettingRow>
      </SettingGroup>
      <SettingGroup title="服务端 AI">
        <SettingRow label="自动翻译" description="使用 Tantan Go 服务配置的 Gemini">
          <Toggle
            label="自动翻译"
            checked={translation}
            onChange={(value) => setGeneralSetting("translation", value)}
          />
        </SettingRow>
        <SettingRow label="自动摘要" description="不调用 Folo 的付费 AI">
          <Toggle
            label="自动摘要"
            checked={summary}
            onChange={(value) => setGeneralSetting("summary", value)}
          />
        </SettingRow>
      </SettingGroup>
    </Page>
  )
}

export function AppearanceSettingsPage() {
  const theme = useThemeAtomValue()
  const setTheme = useSetTheme()
  const uiTextSize = useUISettingKey("uiTextSize")
  const contentLineHeight = useUISettingKey("contentLineHeight")
  const thumbnailRatio = useUISettingKey("thumbnailRatio")
  const reduceMotion = useUISettingKey("reduceMotion")

  return (
    <Page title="外观">
      <SettingGroup title="主题">
        <div className="grid grid-cols-3 gap-1 p-2">
          {(
            [
              ["system", "跟随系统"],
              ["light", "浅色"],
              ["dark", "深色"],
            ] as const
          ).map(([value, label]) => (
            <button
              key={value}
              type="button"
              aria-label={`${label}主题`}
              aria-pressed={theme === value}
              onClick={() => setTheme(value)}
              className="min-h-11 rounded-xl px-2 text-sm text-zinc-500 outline-none focus-visible:ring-2 focus-visible:ring-orange-500 aria-pressed:bg-orange-500 aria-pressed:text-white"
            >
              {label}
            </button>
          ))}
        </div>
      </SettingGroup>
      <SettingGroup title="文字与内容">
        <SettingRow label="界面字号" description={`${uiTextSize}px`}>
          <div className="flex items-center gap-1">
            <button
              type="button"
              aria-label="减小字号"
              onClick={() => setUISetting("uiTextSize", Math.max(14, uiTextSize - 1))}
              className="flex size-11 items-center justify-center rounded-xl bg-zinc-100 text-lg dark:bg-white/10"
            >
              −
            </button>
            <button
              type="button"
              aria-label="增大字号"
              onClick={() => setUISetting("uiTextSize", Math.min(22, uiTextSize + 1))}
              className="flex size-11 items-center justify-center rounded-xl bg-zinc-100 text-lg dark:bg-white/10"
            >
              +
            </button>
          </div>
        </SettingRow>
        <SettingRow label="正文行高" description={contentLineHeight.toFixed(2)}>
          <button
            type="button"
            onClick={() =>
              setUISetting(
                "contentLineHeight",
                contentLineHeight >= 2 ? 1.5 : Number((contentLineHeight + 0.25).toFixed(2)),
              )
            }
            className="min-h-11 rounded-xl bg-zinc-100 px-3 text-sm dark:bg-white/10"
          >
            调整
          </button>
        </SettingRow>
        <SettingRow label="保留图片原始比例">
          <Toggle
            label="保留图片原始比例"
            checked={thumbnailRatio === "original"}
            onChange={(value) => setUISetting("thumbnailRatio", value ? "original" : "square")}
          />
        </SettingRow>
      </SettingGroup>
      <SettingGroup title="辅助功能">
        <SettingRow label="减少动态效果">
          <Toggle
            label="减少动态效果"
            checked={reduceMotion}
            onChange={(value) => setUISetting("reduceMotion", value)}
          />
        </SettingRow>
      </SettingGroup>
    </Page>
  )
}

export function AboutSettingsPage() {
  return (
    <Page title="关于 Tantan">
      <div className="rounded-2xl bg-white p-5 text-sm leading-7 text-zinc-600 shadow-sm ring-1 ring-zinc-200/70 dark:bg-[#17181b] dark:text-zinc-400 dark:ring-white/[0.07]">
        <p className="font-semibold text-zinc-900 dark:text-zinc-100">Tantan Mobile Web/PWA</p>
        <p className="mt-2">
          账号、RSS 与内容来自 Folo；每日推荐、Topic、筛选、翻译和摘要由 Tantan Go 服务处理。
        </p>
        <p className="mt-2">浏览器不会保存模型密钥，也不会调用 Folo 的 AI 或支付接口。</p>
      </div>
    </Page>
  )
}
