import { describe, expect, it, vi } from "vitest";
import { loginHref } from "./login-next";
import {
  CONSOLE_ENTRY_PATH,
  consoleHomeForRoles,
  consoleHomeForViewer,
  playgroundHref,
  resolveConsoleHref,
  resolveStartUsingHref,
} from "./console-home";

function jsonResponse(ok: boolean, body: unknown, status = ok ? 200 : 403) {
  return {
    ok,
    status,
    json: async () => body,
  } as Response;
}

describe("consoleHomeForRoles", () => {
  it("sends platform and ops roles to admin", () => {
    expect(consoleHomeForRoles(["platform_admin"])).toBe("/admin");
    expect(consoleHomeForRoles(["end_user", "finance_admin"])).toBe("/admin");
    expect(consoleHomeForRoles(["ops_admin"])).toBe("/admin");
    expect(consoleHomeForRoles(["tech_admin"])).toBe("/admin");
    expect(consoleHomeForRoles(["audit_readonly"])).toBe("/admin");
  });

  it("sends channel admins to the channel console", () => {
    expect(consoleHomeForRoles(["channel_admin"])).toBe("/channel");
    expect(consoleHomeForRoles(["end_user", "channel_admin"])).toBe("/channel");
  });

  it("prefers admin when a user also has channel_admin", () => {
    expect(consoleHomeForRoles(["channel_admin", "platform_admin"])).toBe("/admin");
  });

  it("falls back to the user console", () => {
    expect(consoleHomeForRoles(["end_user"])).toBe("/app");
    expect(consoleHomeForRoles([])).toBe("/app");
    expect(consoleHomeForRoles(undefined)).toBe("/app");
  });
});

describe("consoleHomeForViewer", () => {
  it("keeps admin and channel ahead of partner", () => {
    expect(consoleHomeForViewer({ roles: ["platform_admin"], isPartner: true })).toBe("/admin");
    expect(consoleHomeForViewer({ roles: ["channel_admin"], isPartner: true })).toBe("/channel");
  });

  it("sends acquisition partners to the partner console", () => {
    expect(consoleHomeForViewer({ roles: ["end_user"], isPartner: true })).toBe("/partner");
    expect(consoleHomeForViewer({ roles: ["end_user"], isPartner: false })).toBe("/app");
  });
});

describe("resolveConsoleHref", () => {
  it("sends guests to login with the console entry as next", async () => {
    const fetcher = vi.fn().mockResolvedValue(jsonResponse(false, { error: { message: "未授权" } }));
    await expect(resolveConsoleHref(fetcher)).resolves.toBe(loginHref(CONSOLE_ENTRY_PATH));
    expect(fetcher).toHaveBeenCalledTimes(1);
  });

  it("routes signed-in roles without asking partner/me", async () => {
    const fetcher = vi.fn().mockResolvedValue(jsonResponse(true, { user: { roles: ["platform_admin"] } }));
    await expect(resolveConsoleHref(fetcher)).resolves.toBe("/admin");
    expect(fetcher).toHaveBeenCalledTimes(1);
  });

  it("checks partner/me for ordinary users", async () => {
    const fetcher = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse(true, { user: { roles: ["end_user"] } }))
      .mockResolvedValueOnce(jsonResponse(true, { role_type: "agent" }));
    await expect(resolveConsoleHref(fetcher)).resolves.toBe("/partner");
    expect(fetcher).toHaveBeenCalledTimes(2);
  });

  it("keeps ordinary users on /app when they are not partners", async () => {
    const fetcher = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse(true, { user: { roles: ["end_user"] } }))
      .mockResolvedValueOnce(jsonResponse(false, { error: { message: "不是推广主体" } }));
    await expect(resolveConsoleHref(fetcher)).resolves.toBe("/app");
  });

  it("falls back to login when /v1/me cannot be reached", async () => {
    const fetcher = vi.fn().mockRejectedValue(new Error("offline"));
    await expect(resolveConsoleHref(fetcher)).resolves.toBe(loginHref(CONSOLE_ENTRY_PATH));
  });
});

describe("playgroundHref", () => {
  it("encodes model ids so slashes stay in the query", () => {
    expect(playgroundHref()).toBe("/app/playground");
    expect(playgroundHref("tokenhub/echo-1")).toBe("/app/playground?model=tokenhub%2Fecho-1");
  });

  it("keeps catalog filters on the return path", () => {
    expect(playgroundHref("tokenhub/echo-1", "/app/catalog?kind=text")).toBe(
      "/app/playground?model=tokenhub%2Fecho-1&from=%2Fapp%2Fcatalog%3Fkind%3Dtext",
    );
  });
});

describe("resolveStartUsingHref", () => {
  it("sends guests to login with playground as next", async () => {
    const fetcher = vi.fn().mockResolvedValue(jsonResponse(false, { error: { message: "未授权" } }));
    await expect(resolveStartUsingHref("tokenhub/echo-1", fetcher)).resolves.toBe(
      loginHref("/app/playground?model=tokenhub%2Fecho-1"),
    );
  });

  it("sends signed-in users to media for an image model", async () => {
    const fetcher = vi.fn().mockResolvedValue(jsonResponse(true, { user: { roles: ["end_user"] } }));
    await expect(
      resolveStartUsingHref({ id: "bytedance/seedream", kind: "image" }, fetcher),
    ).resolves.toBe("/app/media?model=bytedance%2Fseedream&kind=image");
  });
});
