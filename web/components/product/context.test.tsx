// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import { ProductProvider } from "./context";
import { AppearanceForm } from "./branding";
import { PublicContextSchema, type Session } from "@/lib/contracts";

const state = vi.hoisted(() => ({ path: "/manage/oem/branding" }));
vi.mock("next/navigation", () => ({ usePathname: () => state.path, useRouter: () => ({ replace: vi.fn() }) }));
vi.mock("next-themes", () => ({ useTheme: () => ({ theme: "light", setTheme: vi.fn() }) }));

const platform = { id: "platform", name: "平台", kind: "platform" as const, brand_id: "official", theme: { primary_color: "#2563eb", background_color: "#ffffff", text_color: "#111827" } };
const oem = { id: "oem", name: "OEM 品牌", kind: "oem" as const, brand_id: "other-brand", theme: { primary_color: "#ba39e9", background_color: "#808080", text_color: "#172334", logo_url: "/v1/branding/assets/oem-logo/content" } };
const channel = { ...oem, id: "channel", name: "渠道", kind: "channel" as const, theme: { ...oem.theme, primary_color: "#087f5b" } };
const publicContext = PublicContextSchema.parse({ brand: { id: "official", name: "平台品牌", api_base_url: "https://official.test/v1", theme: platform.theme }, operator: platform, theme: platform.theme, availability: [], payment_methods: [] });
const staff = (): Session => ({ kind: "staff", staff: { id: "staff", email: "staff@example.test", display_name: "员工" }, workspaces: [platform, oem, channel].map(operator => ({ operator, permissions: ["*"], can_manage_monetary: false })) });
const clients: QueryClient[] = [];

function mount(session: Session, content: ReactNode = <p>业务页面</p>) {
  const cache = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  clients.push(cache);
  cache.setQueryData(["public-context"], publicContext);
  cache.setQueryData(["session"], session);
  vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => new Response(JSON.stringify(String(input).endsWith("/auth/me") ? session : publicContext), { status: 200 })));
  const tree = () => <QueryClientProvider client={cache}><ProductProvider>{content}</ProductProvider></QueryClientProvider>;
  const view = render(tree());
  return { ...view, cache, refresh: () => view.rerender(tree()) };
}
function surface() { return document.querySelector<HTMLElement>("div.product-theme")!; }

afterEach(() => { cleanup(); clients.splice(0).forEach(client => client.clear()); vi.unstubAllGlobals(); state.path = "/manage/oem/branding"; });

it("uses the current staff workspace theme and authenticated logo on a shared platform hostname", async () => {
  mount(staff());
  await waitFor(() => expect(surface().style.getPropertyValue("--brand")).toBe(oem.theme.primary_color));
  expect(surface().style.getPropertyValue("--branding-background")).toBe(oem.theme.background_color);
  expect(document.body.style.getPropertyValue("--brand")).toBe(oem.theme.primary_color);
  expect(document.body.classList.contains("product-theme-root")).toBe(true);
  expect(document.querySelector("header img")?.getAttribute("src")).toBe("/api/v1/operators/oem/branding/assets/oem-logo/content");
  expect(screen.getByRole("link", { name: "OEM 品牌", exact: true })).toBeTruthy();
});

it("switches platform/OEM/channel themes without leaking workspace appearance into public pages", async () => {
  const view = mount(staff());
  for (const operator of [channel, platform, oem]) {
    state.path = `/manage/${operator.id}/customers`; view.refresh();
    await waitFor(() => expect(surface().style.getPropertyValue("--brand")).toBe(operator.theme.primary_color));
    expect(document.body.style.getPropertyValue("--brand")).toBe(operator.theme.primary_color);
  }
  state.path = "/models"; view.refresh();
  await waitFor(() => expect(surface().style.getPropertyValue("--brand")).toBe(platform.theme.primary_color));
  expect(document.querySelector("header img")).toBeNull();
  view.unmount();
  expect(document.body.style.getPropertyValue("--brand")).toBe("");
  expect(document.body.classList.contains("product-theme-root")).toBe(false);
});

it("updates the whole page and portalled panels immediately after saving appearance", async () => {
  let session = staff();
  const view = mount(session, <AppearanceForm operatorID="oem" row={{ id: "oem", version: 1, values: { ...oem.theme, effective_theme: oem.theme } }} operation={{ name: "theme", label: "编辑网站外观", path: "/api/v1/operators/oem/branding", method: "POST", fields: [] }} onBlockedChange={() => {}} />);
  vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    if (init?.method === "POST") {
      const values = JSON.parse(String(init.body)).values;
      if (session.kind === "staff") session = { ...session, workspaces: session.workspaces.map(workspace => workspace.operator.id === "oem" ? { ...workspace, operator: { ...workspace.operator, theme: values } } : workspace) };
      return new Response(JSON.stringify({ id: "oem", status: "updated" }), { status: 200 });
    }
    return new Response(JSON.stringify(String(input).endsWith("/auth/me") ? session : publicContext), { status: 200 });
  }));
  fireEvent.input(screen.getByLabelText("主色", { exact: true }), { target: { value: "#087f5b" } });
  fireEvent.click(screen.getByRole("button", { name: "保存外观", exact: true }));
  await screen.findByText("网站外观已保存。");
  await waitFor(() => expect(surface().style.getPropertyValue("--brand")).toBe("#087f5b"));
  expect(document.body.style.getPropertyValue("--brand")).toBe("#087f5b");
  expect(view.cache.getQueryData(["session"])).not.toEqual(staff());
});

it("keeps customer channel appearance and uses the public logo scope", async () => {
  state.path = "/workspace";
  mount({ kind: "customer", customer: { id: "customer", email: "customer@example.test", display_name: "客户" }, brand: { ...publicContext.brand, id: channel.brand_id, name: "OEM 品牌" }, operator: channel });
  await waitFor(() => expect(surface().style.getPropertyValue("--brand")).toBe(channel.theme.primary_color));
  expect(document.querySelector("header img")?.getAttribute("src")).toBe("/api/v1/branding/assets/oem-logo/content?operator_id=channel");
});
