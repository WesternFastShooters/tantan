export interface PrimaryRoute {
  path: "/" | "/subscriptions" | "/discover" | "/settings"
  label: string
  icon: string
  end?: boolean
}

export const primaryRoutes: readonly PrimaryRoute[] = [
  { path: "/", label: "首页", icon: "i-mgc-home-5-cute-re", end: true },
  { path: "/subscriptions", label: "订阅", icon: "i-mgc-black-board-2-cute-re" },
  { path: "/discover", label: "发现", icon: "i-mgc-search-3-cute-re" },
  { path: "/settings", label: "设置", icon: "i-mgc-settings-1-cute-re" },
] as const
