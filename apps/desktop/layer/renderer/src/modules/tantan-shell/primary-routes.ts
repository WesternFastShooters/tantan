export interface PrimaryRoute {
  path: "/" | "/subscriptions" | "/settings"
  label: string
  icon: string
  end?: boolean
}

export const primaryRoutes: readonly PrimaryRoute[] = [
  { path: "/", label: "首页", icon: "i-mgc-home-4-cute-re", end: true },
  { path: "/subscriptions", label: "订阅", icon: "i-mgc-rss-2-cute-re" },
  { path: "/settings", label: "设置", icon: "i-mgc-settings-7-cute-re" },
] as const
