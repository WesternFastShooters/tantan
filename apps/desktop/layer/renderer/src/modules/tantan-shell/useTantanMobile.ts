import { useSyncExternalStore } from "react"

const TANTAN_MOBILE_MAX_WIDTH = 767
const mobileMediaQuery = `(max-width: ${TANTAN_MOBILE_MAX_WIDTH}px)`

export const isTantanMobileWidth = (width: number) => width > 0 && width <= TANTAN_MOBILE_MAX_WIDTH

const subscribe = (onChange: () => void) => {
  const media = window.matchMedia(mobileMediaQuery)
  media.addEventListener("change", onChange)
  return () => media.removeEventListener("change", onChange)
}

const getSnapshot = () => window.matchMedia(mobileMediaQuery).matches

export const useTantanMobile = () => useSyncExternalStore(subscribe, getSnapshot, () => false)
