import { QueryClientProvider } from "@tanstack/react-query"
import { Provider } from "jotai"
import { useLayoutEffect } from "react"
import { Outlet } from "react-router"

import { removeAppSkeleton } from "~/lib/app"
import { jotaiStore } from "~/lib/jotai"
import { queryClient } from "~/lib/query-client"

export function TantanWebRoot() {
  useLayoutEffect(removeAppSkeleton, [])

  return (
    <Provider store={jotaiStore}>
      <QueryClientProvider client={queryClient}>
        <Outlet />
      </QueryClientProvider>
    </Provider>
  )
}
