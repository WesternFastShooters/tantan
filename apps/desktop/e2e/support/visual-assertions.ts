import type { Locator, Page } from "@playwright/test"
import { expect } from "@playwright/test"

export const expectVisibleIconGlyph = async (control: Locator) => {
  await expect(control).toBeVisible()
  const glyph = control.locator('svg, i[aria-hidden="true"], i[aria-hidden]').first()
  await expect(glyph).toBeVisible()
  const visual = await glyph.evaluate((element) => {
    const rect = element.getBoundingClientRect()
    const style = getComputedStyle(element)
    const isSVG = element instanceof SVGElement
    const hasVectorShape = isSVG
      ? Boolean(element.querySelector("path[d], circle, rect, line, polyline, polygon"))
      : false
    const hasMask =
      style.maskImage !== "none" || style.getPropertyValue("-webkit-mask-image") !== "none"
    const hasBackground = style.backgroundImage !== "none"
    return {
      width: rect.width,
      height: rect.height,
      hasPaint: hasVectorShape || hasMask || hasBackground,
    }
  })

  expect(visual.width).toBeGreaterThanOrEqual(16)
  expect(visual.height).toBeGreaterThanOrEqual(16)
  expect(visual.hasPaint).toBe(true)
}

export const expectDialogToSpanViewport = async (page: Page, dialog: Locator) => {
  await expect(dialog).toBeVisible()
  const box = await dialog.boundingBox()
  const viewport = page.viewportSize()
  expect(box).not.toBeNull()
  expect(viewport).not.toBeNull()
  expect(Math.abs(box!.x)).toBeLessThanOrEqual(1)
  expect(Math.abs(box!.width - viewport!.width)).toBeLessThanOrEqual(1)
}
