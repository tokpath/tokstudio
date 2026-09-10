/** @vitest-environment jsdom */
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { withZh } from "@/lib/test-i18n";
import ProfilePanel from "./profile-panel";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("ProfilePanel", () => {
  it("renders real session fields and login-method badges", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => ({
        ok: true,
        json: async () => ({
          user: {
            display_name: "Ada",
            email: "ada@example.test",
            login_methods: ["password", "google"],
          },
        }),
      })),
    );
    render(withZh(<ProfilePanel />));
    await waitFor(() => expect(screen.getByTestId("profile-display-name").textContent).toBe("Ada"));
    expect(screen.getByTestId("profile-email").textContent).toBe("ada@example.test");
    expect(screen.getByTestId("profile-login-methods").querySelector('[data-method="password"]')).toBeTruthy();
    expect(screen.getByTestId("profile-login-methods").querySelector('[data-method="google"]')).toBeTruthy();
  });

  it("shows — when name email or methods are missing", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => ({
        ok: true,
        json: async () => ({ user: { display_name: "", email: "" } }),
      })),
    );
    render(withZh(<ProfilePanel />));
    await waitFor(() => expect(screen.getByTestId("profile-display-name").textContent).toBe("—"));
    expect(screen.getByTestId("profile-email").textContent).toBe("—");
    expect(screen.getByTestId("profile-login-methods").textContent).toContain("—");
  });
});
