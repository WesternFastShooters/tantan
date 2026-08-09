import { createBrowserRouter } from "react-router"

import { NotFound } from "./components/common/NotFound"
import { EntryDetailPage } from "./modules/tantan-entry/EntryDetailPage"
import { FavoritesPage } from "./modules/tantan-favorites/FavoritesPage"
import { SearchPage } from "./modules/tantan-search/SearchPage"
import { AISettingsPage } from "./modules/tantan-settings/AISettingsPage"
import { DiscoverRoute } from "./modules/tantan-shell/DiscoverRoute"
import { HomeRoute } from "./modules/tantan-shell/HomeRoute"
import { LoginRoute } from "./modules/tantan-shell/LoginRoute"
import { SettingsRoute } from "./modules/tantan-shell/SettingsRoute"
import { SubscriptionsRoute } from "./modules/tantan-shell/SubscriptionsRoute"
import { TantanAppShell } from "./modules/tantan-shell/TantanAppShell"
import { TantanWebRoot } from "./modules/tantan-shell/TantanWebRoot"
import { SourceDetailPage } from "./modules/tantan-subscriptions/SourceDetailPage"

export const router = createBrowserRouter([
  {
    path: "/",
    Component: TantanWebRoot,
    children: [
      { path: "login", Component: LoginRoute },
      {
        Component: TantanAppShell,
        children: [
          { index: true, Component: HomeRoute },
          { path: "subscriptions", Component: SubscriptionsRoute },
          { path: "discover", Component: DiscoverRoute },
          { path: "settings", Component: SettingsRoute },
          { path: "settings/ai", Component: AISettingsPage },
          { path: "search", Component: SearchPage },
          { path: "favorites", Component: FavoritesPage },
          { path: "entries/:entryId", Component: EntryDetailPage },
          { path: "sources/:sourceId", Component: SourceDetailPage },
        ],
      },
      { path: "*", element: <NotFound /> },
    ],
  },
])
