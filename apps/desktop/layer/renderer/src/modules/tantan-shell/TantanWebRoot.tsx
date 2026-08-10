import { hydrateDatabaseToStore } from "@follow/store/hydrate"
import { QueryClientProvider } from "@tanstack/react-query"
import { Provider } from "jotai"
import { useEffect, useLayoutEffect, useState } from "react"
import { Outlet } from "react-router"

import { removeAppSkeleton } from "~/lib/app"
import { jotaiStore } from "~/lib/jotai"
import { queryClient } from "~/lib/query-client"

let webStoreInitialization: Promise<void> | null = null

const initializeWebStore = () => {
  webStoreInitialization ??= hydrateDatabaseToStore({ migrateDatabase: true })
  return webStoreInitialization
}

export function TantanWebRoot() {
  const [storeState, setStoreState] = useState<"loading" | "ready" | "error">("loading")

  useLayoutEffect(removeAppSkeleton, [])
  useEffect(() => {
    let active = true
    void initializeWebStore().then(
      () => active && setStoreState("ready"),
      () => active && setStoreState("error"),
    )
    return () => {
      active = false
    }
  }, [])

  return (
    <Provider store={jotaiStore}>
      <QueryClientProvider client={queryClient}>
        {storeState === "ready" ? (
          <Outlet />
        ) : (
          <main className="flex min-h-dvh items-center justify-center bg-[#08090b] px-6 text-center">
            <p role={storeState === "error" ? "alert" : "status"} className="text-sm text-zinc-400">
              {storeState === "error" ? "本地数据初始化失败，请刷新页面重试" : "正在准备本地数据…"}
            </p>
          </main>
        )}
      </QueryClientProvider>
    </Provider>
  )
}
