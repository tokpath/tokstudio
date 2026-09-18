import { expect, test } from "@playwright/test";
import { mockViewer } from "./mock-viewer";

test("partner can match a localized reversal to its original request and entry", async ({ page }) => {
  await mockViewer(page, { roles: ["agent"], partner: true });
  await page.route("**/v1/partner/commissions", r => r.fulfill({ json: { items: [
    { id: "cme_original", request_id: "req_consumption", kind: "direct", status: "reversed", amount_minor: 300000 },
    { id: "cme_reversal", request_id: "req_consumption", reversal_of: "cme_original", kind: "direct", status: "reversed", amount_minor: -300000 },
  ] } }));
  await page.goto("/partner/commissions");
  const row = page.getByRole("row").filter({ hasText: "cme_reversal" });
  await expect(row).toContainText("佣金冲正");
  await expect(row).toContainText("已冲正");
  await expect(row).toContainText("-$0.30");
  await expect(row).toContainText("req_consumption");
  await expect(row).toContainText("冲正对应原记录: cme_original");
});
