import { describe, expect, it } from "vitest";
import zh from "../messages/zh.json";
import { gapKey, pendingListPath, statementListPath } from "./reconciliation";

describe("Aura reconciliation copy", () => {
  it("uses 对账 for map/h1 and 待对账队列 for the section, never a bare 待对账 heading", () => {
    expect(zh.admin.reconciliation).toBe("对账");
    expect(zh.adminUi.reconciliation).toBe("对账");
    expect(zh.adminUi.pendingList).toBe("待对账队列");
    expect(zh.admin.reconciliation).not.toBe("待对账");
    expect(zh.adminUi.reconciliation).not.toBe("待对账");
    expect(zh.admin.heroPending).toBe("待对账");
  });
});

describe("pendingListPath", () => {
  it("defaults to the pending queue with no fake query", () => {
    expect(pendingListPath({})).toBe("/admin/usage/pending");
    expect(pendingListPath({ status: "pending_reconciliation" })).toBe("/admin/usage/pending");
  });

  it("filters resolved history and time", () => {
    expect(pendingListPath({ status: "voided", from: "2026-09-01", to: "2026-09-09" })).toBe(
      "/admin/usage/pending?status=voided&from=2026-09-01&to=2026-09-09",
    );
  });
});

describe("statementListPath", () => {
  it("filters statements by key model and channel", () => {
    expect(
      statementListPath({ apiKeyId: "key_1", modelId: "tokenhub/echo-1", channelId: "chn_official_a" }),
    ).toBe("/admin/usage?api_key_id=key_1&public_model_id=tokenhub%2Fecho-1&channel_id=chn_official_a");
  });
});

describe("gapKey", () => {
  it("prefers usage id", () => {
    expect(gapKey({ id: "usg_1", request_id: "req_1" })).toBe("usg_1");
    expect(gapKey({ request_id: "req_1" })).toBe("req_1");
  });
});
