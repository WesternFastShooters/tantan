import { Auth } from "@follow/shared/auth"
import { buildBetterAuthSessionTokenCookieHeader } from "@follow/shared/auth-cookie"
import { IN_ELECTRON } from "@follow/shared/constants"
import { env } from "@follow/shared/env.desktop"
import { createDesktopAPIHeaders } from "@follow/utils/headers"
import PKG from "@pkg"

import { getAuthSessionToken } from "./client-session"

const headers = createDesktopAPIHeaders({ version: PKG.version })
const electronRuntime = IN_ELECTRON || (typeof window !== "undefined" && !!window.electron)
const browserOrigin =
  typeof window === "undefined" ? "http://127.0.0.1:3000" : window.location.origin
const authAPIURL = electronRuntime
  ? env.VITE_API_URL
  : new URL("/api/folo/", browserOrigin).toString()
const authWebURL = electronRuntime ? env.VITE_WEB_URL : browserOrigin

const auth = new Auth({
  apiURL: authAPIURL,
  webURL: authWebURL,
  fetchOptions: {
    headers,
    onRequest: (context) => {
      const authSessionToken = IN_ELECTRON ? getAuthSessionToken() : null
      if (authSessionToken) {
        context.headers.set(
          "Cookie",
          buildBetterAuthSessionTokenCookieHeader(env.VITE_API_URL, authSessionToken),
        )
      }
    },
  },
})

export const { authClient } = auth

// @keep-sorted
export const {
  changeEmail,
  changePassword,
  deleteUserCustom,
  getAccountInfo,
  getProviders,
  getSession,
  linkSocial,
  listAccounts,
  oneTimeToken,
  resetPassword,
  sendVerificationEmail,
  signIn,
  signOut,
  signUp,
  twoFactor,
  unlinkAccount,
  updateUser,
} = auth.authClient

export const forgetPassword = auth.authClient.requestPasswordReset

export const { loginHandler } = auth
