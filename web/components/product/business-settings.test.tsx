// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import { BusinessSettings } from "./business-settings";

vi.mock("./context", () => ({ useProduct: () => ({ session: { kind: "staff", workspaces: [{ operator: { id: "op" }, permissions: ["*"] }] } }) }));
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });
function show() {
  const readiness = ["registration", "new_purchase", "renewal", "model_usage", "activity", "settlement"].map(feature => ({ feature, requirements: feature === "registration" ? [{ code: "policy", description: "设置邀请奖励规则", resource: "commissions", ready: false }] : [] }));
  return render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><BusinessSettings operatorID="op" row={{ id: "op", version: 7, values: { settlement: true, readiness } }} /></QueryClientProvider>);
}
it("explains missing configuration and toggles the chosen feature using the current version", async () => {
  const fetch = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) => new Response(JSON.stringify({ id: "op", status: "updated", version: 8 }), { status: 200 }));
  vi.stubGlobal("fetch", fetch); show();
  expect((screen.getByRole("button", { name: "开启 新客户注册" }) as HTMLButtonElement).disabled).toBe(true);
  expect(screen.getByRole("link", { name: "去配置" }).getAttribute("href")).toBe("/manage/op/commissions");
  expect(screen.queryByRole("textbox")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "暂停 资金结算" }));
  await waitFor(() => expect(fetch).toHaveBeenCalledTimes(1));
  expect(JSON.parse(String(fetch.mock.calls[0]?.[1]?.body))).toEqual({ feature: "settlement", enabled: false, expected_version: 7 });
});
it("retries an uncertain toggle with its original body and idempotency key", async () => {
  const fetch = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) => {
    if (fetch.mock.calls.length === 1) throw new TypeError("connection interrupted");
    return new Response(JSON.stringify({ id: "op", status: "updated", version: 8 }), { status: 200 });
  });
  vi.stubGlobal("fetch", fetch); show();
  fireEvent.click(screen.getByRole("button", { name: "开启 模型调用" }));
  await screen.findByRole("button", { name: "重试原操作" });
  expect((screen.getByRole("button", { name: "暂停 资金结算" }) as HTMLButtonElement).disabled).toBe(true);
  fireEvent.click(screen.getByRole("button", { name: "重试原操作" }));
  await waitFor(() => expect(fetch).toHaveBeenCalledTimes(2));
  expect(fetch.mock.calls[1]?.[1]?.body).toBe(fetch.mock.calls[0]?.[1]?.body);
  expect(new Headers(fetch.mock.calls[1]?.[1]?.headers).get("Idempotency-Key")).toBe(new Headers(fetch.mock.calls[0]?.[1]?.headers).get("Idempotency-Key"));
});
