import type { ResponsiveSelectItem } from "@follow/components/ui/select/responsive.js"
import { ResponsiveSelect } from "@follow/components/ui/select/responsive.js"
import { useTranslation } from "react-i18next"

import { setUISetting, useUISettingKey } from "~/atoms/settings/ui"

import { Recommendations } from "./recommendations"

const LanguageOptions = [
  {
    label: "words.all",
    value: "all",
  },
  {
    label: "words.english",
    value: "eng",
  },
  {
    label: "words.french",
    value: "fra",
  },
  {
    label: "words.chinese",
    value: "cmn",
  },
] satisfies ResponsiveSelectItem[]

type Language = "all" | "eng" | "cmn" | "fra"
export function DiscoveryContent() {
  const { t } = useTranslation()
  const { t: tCommon } = useTranslation("common")
  const lang = useUISettingKey("discoverLanguage")
  const handleLangChange = (value: string) => {
    setUISetting("discoverLanguage", value as Language)
  }

  return (
    <div className="relative mx-auto w-full max-w-[880px] space-y-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-1.5 text-sm font-semibold">
          <i className="i-mgc-grid-2-cute-re size-4" />
          <span>{t("words.categories")}</span>
        </div>

        <div className="flex items-center gap-2">
          <span className="shrink-0 text-sm font-medium text-text-secondary">
            {t("words.language")}:
          </span>
          <ResponsiveSelect
            value={lang}
            onValueChange={handleLangChange}
            triggerClassName="h-8 rounded border-0 bg-material-ultra-thin"
            size="sm"
            items={LanguageOptions}
            renderItem={(item) => tCommon(item.label as any)}
            renderValue={(item) => tCommon(item.label as any)}
          />
        </div>
      </div>

      <div className="min-h-[400px] rounded-2xl border border-fill-secondary bg-background/70 p-4 shadow-sm">
        <Recommendations />
      </div>
    </div>
  )
}
