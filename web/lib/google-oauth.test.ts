import { describe, expect, it } from "vitest";
import {
  GOOGLE_OAUTH_CALLBACK_PATH,
  claimOAuthCallback,
  googleButtonState,
  oauthFailureHref,
  readStoredNext,
  releaseOAuthCallback,
  sanitizeOAuthError,
  storeLoginNext,
} from "./google-oauth";

describe("google oauth helpers", () => {
  it("names the Console Redirect URI path", () => {
    expect(GOOGLE_OAUTH_CALLBACK_PATH).toBe("/login/oauth/google");
  });

  it("disables the button until Google is actually available", () => {
    expect(googleButtonState(null, false)).toEqual({
      disabled: true,
      showUnconfigured: true,
      labelKey: "google",
    });
    expect(googleButtonState({ available: false }, false).disabled).toBe(true);
    expect(googleButtonState({ available: false }, false).showUnconfigured).toBe(true);
    expect(googleButtonState({ available: true }, false)).toEqual({
      disabled: false,
      showUnconfigured: false,
      labelKey: "google",
    });
    expect(googleButtonState({ available: true }, true).labelKey).toBe("googleRedirecting");
    expect(googleButtonState({ available: true }, true).disabled).toBe(true);
  });

  it("strips authorization codes and tokens from UI errors", () => {
    expect(sanitizeOAuthError("ya29.a0AfH6SMC-secret-token", "Google 登录失败")).toBe("Google 登录失败");
    expect(sanitizeOAuthError("mock:user@example.test leaked", "fallback")).not.toContain("mock:");
    expect(sanitizeOAuthError("code=4/0AanRRrs-this-is-long-enough-to-redact", "fallback")).not.toMatch(/0AanRRrs/);
    expect(sanitizeOAuthError("Google 登录失败", "fallback")).toBe("Google 登录失败");
    expect(oauthFailureHref("invalid_grant ya29.abc", "authentication_error")).toContain("/login?");
    expect(oauthFailureHref("invalid_grant ya29.abc")).not.toContain("ya29");
    expect(oauthFailureHref("Google 登录失败", "redirect_uri_mismatch")).toContain("error_code=redirect_uri_mismatch");
  });

  it("claims each authorization code only once", () => {
    const store: Record<string, string> = {};
    const memory = {
      getItem: (key: string) => store[key] ?? null,
      setItem: (key: string, value: string) => {
        store[key] = value;
      },
      removeItem: (key: string) => {
        delete store[key];
      },
    };
    expect(claimOAuthCallback(memory, "code-1")).toBe(true);
    expect(claimOAuthCallback(memory, "code-1")).toBe(false);
    releaseOAuthCallback(memory, "code-1");
    expect(claimOAuthCallback(memory, "code-1")).toBe(true);
  });

  it("round-trips the post-login next path", () => {
    const store: Record<string, string> = {};
    const memory = {
      getItem: (key: string) => store[key] ?? null,
      setItem: (key: string, value: string) => {
        store[key] = value;
      },
      removeItem: (key: string) => {
        delete store[key];
      },
    };
    storeLoginNext(memory, "/app/wallet");
    expect(readStoredNext(memory)).toBe("/app/wallet");
    expect(readStoredNext(memory)).toBe("");
  });
});
