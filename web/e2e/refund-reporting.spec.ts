import { expect, test } from "@playwright/test";
import { mockViewer } from "./mock-viewer";

test("partner can match a localized reversal to its original request and entry", async ({ page }) => {
  await mockViewer(page, { roles: ["agent"], partner: true });
  await page.route("**/v1/me/referral?**", r => r.fulfill({ json: { item: {
    codes: [], code_links: [], can_create: false, invited_count: 0, can_commission: true, professional_customers: true,
    rules: { spend_minor: 0, topup_minor: 0, gift_minor: 0 },
    progress: { spend_minor: 0, largest_topup_minor: 0, gift_granted_minor: 0, gift_remaining_minor: 0 },
    summary: { earned_minor: 300000, frozen_minor: 0, available_minor: 0, held_minor: 0, settled_minor: 0, paid_minor: 0, reversed_minor: 300000 },
    rewards: [
    { id: "cme_original", request_id: "req_consumption", kind: "direct", status: "reversed", amount_minor: 300000 },
    { id: "cme_reversal", request_id: "req_consumption", reversal_of: "cme_original", kind: "direct", status: "reversed", amount_minor: -300000 },
    ], settlements: [], pagination: { page: 1, page_size: 25, rewards_total: 2, settlements_total: 0 },
  } } }));
  await page.goto("/partner/commissions");
  await expect(page).toHaveURL(/\/app\/referral\?tab=commissions$/);
  const row = page.getByRole("row").filter({ hasText: "cme_reversal" });
  await expect(row).toContainText("佣金冲正");
  await expect(row).toContainText("已冲正");
  await expect(row).toContainText("-$0.30");
  await expect(row).toContainText("req_consumption");
  await expect(row).toContainText("冲正对应原记录: cme_original");
});
