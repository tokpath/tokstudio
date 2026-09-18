/** @vitest-environment jsdom */
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { ViewerProvider, useViewer } from "./viewer-context";
function Probe() { return <pre data-testid="viewer">{JSON.stringify(useViewer())}</pre>; }
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });
async function load(meStatus: number, partnerFails = false) {
  vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
    if (String(input).endsWith("/partner/me")) {
      if (partnerFails) throw new Error("network unavailable");
      return { ok: false, status: 403 };
    }
    return { ok: meStatus === 200, status: meStatus, json: async () => ({ user: { id: "test", roles: ["end_user"] } }) };
  }));
  render(<ViewerProvider><Probe /></ViewerProvider>);
  await waitFor(() => expect(screen.getByTestId("viewer").textContent).toContain('"loading":false'));
  return JSON.parse(screen.getByTestId("viewer").textContent || "{}");
}
it("preserves a valid session when the partner service fails", async () => {
  expect(await load(200, true)).toMatchObject({ signedIn: true, partnerError: true, roles: ["end_user"] });
});
it("distinguishes identity service failure from expired login", async () => {
  expect(await load(503)).toMatchObject({ signedIn: false, error: true });
});
it("recognizes an expired session", async () => {
  expect(await load(401)).toMatchObject({ signedIn: false, error: false });
});
