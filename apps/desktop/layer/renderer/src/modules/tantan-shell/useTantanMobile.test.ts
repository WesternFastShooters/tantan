import { describe, expect, test } from "vitest"

import { isTantanMobileWidth } from "./useTantanMobile"

describe("Tantan responsive breakpoint", () => {
  test("REQ:FE-02 switches from Mobile Web to PC Web at 768px", () => {
    expect(isTantanMobileWidth(390)).toBe(true)
    expect(isTantanMobileWidth(767)).toBe(true)
    expect(isTantanMobileWidth(768)).toBe(false)
    expect(isTantanMobileWidth(800)).toBe(false)
  })
})
