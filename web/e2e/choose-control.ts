import { expect, type Locator } from "@playwright/test";

// Reference fields are one searchable box. Ordinary selects stay native.
export async function chooseControl(locator: Locator, value: string | { label: string }) {
  await expect(locator).toBeEnabled();
  if (await locator.evaluate(element => element.tagName === "SELECT")) {
    await locator.selectOption(typeof value === "string" ? value : { label: value.label });
    return;
  }
  await locator.fill(typeof value === "string" ? value : value.label);
}
