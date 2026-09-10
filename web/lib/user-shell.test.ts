import { describe, expect, it } from "vitest";
import {
  BALANCE_PILL_FIELD,
  EMPTY_KEYS_TITLE,
  MISSING_PROFILE,
  avatarInitial,
  balancePillText,
  canSeePlatformAdmin,
  formatAvailableBalance,
  loginMethodsOf,
  profileDash,
  readNailedAvailable,
  shellRole,
} from "./user-shell";

describe("user-shell profile fields", () => {
  it("shows — when display name or email is missing", () => {
    expect(profileDash("")).toBe(MISSING_PROFILE);
    expect(profileDash("   ")).toBe(MISSING_PROFILE);
    expect(profileDash(undefined)).toBe(MISSING_PROFILE);
    expect(profileDash("Ada")).toBe("Ada");
  });

  it("builds an initial from real name or email, never an avatar URL", () => {
    expect(avatarInitial("Ada Lovelace", "ada@example.test")).toBe("A");
    expect(avatarInitial("", "ada@example.test")).toBe("A");
    expect(avatarInitial("", "")).toBe(MISSING_PROFILE);
  });

  it("maps admin console roles to admin and everyone else to user", () => {
    expect(shellRole(["end_user"])).toBe("user");
    expect(shellRole(["platform_admin"])).toBe("admin");
    expect(shellRole(["finance_admin", "end_user"])).toBe("admin");
    expect(shellRole([])).toBe("user");
  });

  it("shows the platform admin menu only for platform_admin", () => {
    expect(canSeePlatformAdmin(["platform_admin"])).toBe(true);
    expect(canSeePlatformAdmin(["platform_admin", "end_user"])).toBe(true);
    expect(canSeePlatformAdmin(["finance_admin"])).toBe(false);
    expect(canSeePlatformAdmin(["ops_admin"])).toBe(false);
    expect(canSeePlatformAdmin(["tech_admin"])).toBe(false);
    expect(canSeePlatformAdmin(["audit_readonly"])).toBe(false);
    expect(canSeePlatformAdmin(["end_user"])).toBe(false);
    expect(canSeePlatformAdmin([])).toBe(false);
    expect(canSeePlatformAdmin(undefined)).toBe(false);
  });
});

describe("user-shell balance pill", () => {
  it("nails GET /v1/me/balance balance.available", () => {
    expect(BALANCE_PILL_FIELD).toBe("available");
    expect(readNailedAvailable({ balance: { available: "12.5", reserved: "3" } })).toBe("12.5");
    expect(readNailedAvailable({ balance: { reserved: "3" } })).toBeUndefined();
  });

  it("formats the nailed field as $x.xx and never invents $0.00 on miss/error", () => {
    expect(formatAvailableBalance("12.5")).toBe("$12.50");
    expect(formatAvailableBalance(0)).toBe("$0.00");
    expect(formatAvailableBalance("0")).toBe("$0.00");
    expect(formatAvailableBalance(undefined)).toBe(MISSING_PROFILE);
    expect(formatAvailableBalance("")).toBe(MISSING_PROFILE);
    expect(formatAvailableBalance("nope")).toBe(MISSING_PROFILE);
    expect(balancePillText("error", "12.5")).toBe(MISSING_PROFILE);
    expect(balancePillText("loading", "12.5")).toBe("");
    expect(balancePillText("ok", undefined)).toBe(MISSING_PROFILE);
    expect(balancePillText("ok", "4")).toBe("$4.00");
  });
});

describe("user-shell keys and login methods", () => {
  it("locks the empty keys copy", () => {
    expect(EMPTY_KEYS_TITLE).toBe("暂无 API 密钥");
  });

  it("only keeps real password/google methods", () => {
    expect(loginMethodsOf(undefined)).toEqual([]);
    expect(loginMethodsOf([])).toEqual([]);
    expect(loginMethodsOf(["password", "github", "google"])).toEqual(["password", "google"]);
  });
});
