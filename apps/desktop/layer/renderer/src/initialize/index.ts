import { initializeDayjs } from "@follow/components/dayjs"
import { registerGlobalContext } from "@follow/shared/bridge"
import { DEV, ELECTRON_BUILD } from "@follow/shared/constants"
import { hydrateDatabaseToStore } from "@follow/store/hydrate"
import { whoami } from "@follow/store/user/getters"
import { userSyncService } from "@follow/store/user/store"
import { repository } from "@pkg"
import { enableMapSet } from "immer"

import { initI18n } from "~/i18n"
import { settingSyncQueue } from "~/modules/settings/helper/sync-queue"
import { ElectronCloseEvent, ElectronShowEvent } from "~/providers/invalidate-query-provider"

import { appLog } from "../lib/log"
import { registerHistoryStack } from "./history"
import { doMigration } from "./migrates"
import { initializeSettings } from "./settings"

declare global {
  interface Window {
    version: string
  }
}

export const initializeApp = async () => {
  appLog(`${APP_NAME}: Follow everything in one place`, repository.url)

  await apm("hydrateDatabaseToStore", () => {
    return hydrateDatabaseToStore({
      migrateDatabase: true,
    })
  })

  if (DEV) {
    const url = "/favicon-dev.ico"

    // Change favicon
    const $icon = document.head.querySelector("link[rel='icon']")
    if ($icon) {
      $icon.setAttribute("href", url)
    } else {
      const icon = document.createElement("link")
      icon.setAttribute("rel", "icon")
      icon.setAttribute("href", url)
      document.head.append(icon)
    }
  }

  appLog(`Initialize ${APP_NAME}...`)
  window.version = APP_VERSION

  const now = Date.now()
  initializeDayjs()
  registerHistoryStack()

  // Set Environment
  document.documentElement.dataset.buildType = ELECTRON_BUILD ? "electron" : "web"

  // Register global context for electron
  registerGlobalContext({
    /**
     * Electron app only
     */
    onWindowClose() {
      document.dispatchEvent(new ElectronCloseEvent())
    },
    onWindowShow() {
      document.dispatchEvent(new ElectronShowEvent())
    },
  })

  apm("migration", doMigration)

  // Enable Map/Set in immer
  enableMapSet()

  apm("initializeSettings", initializeSettings)

  await apm("i18n", initI18n)
  void apm("setting sync", async () => {
    await settingSyncQueue.init()

    await userSyncService.whoami().catch(() => null)

    if (!whoami()) {
      return
    }
    await settingSyncQueue.syncLocal()
  }).catch((error) => {
    appLog("setting sync failed", error)
  })

  const loadingTime = Date.now() - now
  appLog(`Initialize ${APP_NAME} done,`, `${loadingTime}ms`)
}

const apm = async (label: string, fn: () => Promise<any> | any) => {
  const start = Date.now()
  const result = await fn()
  const end = Date.now()
  appLog(`${label} took ${end - start}ms`)
  return result
}
