// @vitest-environment jsdom
import { useState } from "react";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import { OperatorPanel } from "./operator-resources";
import { OperationForm } from "./operation-form";
import { RecordPicker } from "./record-picker";

afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

it("stores the clicked record and keeps the dialog open", async () => {
  const fetch = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => new Response(JSON.stringify(init?.method === "POST" ? { id: "quote", status: "saved" } : { items: [{ value: "buyer-one", label: "直属渠道" }] }), { status: 200 }));
  vi.stubGlobal("fetch", fetch);
  const cache = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  function Harness() {
    const [open, setOpen] = useState(true);
    return <OperatorPanel title="供货报价" description="选择采购方" open={open} close={() => setOpen(false)}>
      <OperationForm operatorID="operator-one" embedded operation={{ name: "quote", label: "向直属采购方发布供货报价", method: "POST", path: "/api/v1/operators/operator-one/procurement", fields: [{ name: "buyer_id", label: "直属采购方", kind: "text", lookup_url: "/api/v1/operators/operator-one/references/buyers", required: true }] }} />
    </OperatorPanel>;
  }
  render(<QueryClientProvider client={cache}><Harness /></QueryClientProvider>);
  const buyer = await screen.findByRole("combobox", { name: "直属采购方" });
  await waitFor(() => expect((buyer as HTMLInputElement).disabled).toBe(false));
  fireEvent.focus(buyer);
  const option = await screen.findByRole("option", { name: "直属渠道" });
  expect(getComputedStyle(option.closest("[data-record-picker-menu]")!).pointerEvents).toBe("auto");
  await act(async () => { await new Promise(resolve => setTimeout(resolve, 20)); });
  fireEvent.pointerDown(option, { button: 0, ctrlKey: false, bubbles: true });
  fireEvent.click(option);
  await act(async () => { await new Promise(resolve => setTimeout(resolve, 20)); });
  expect(screen.getByRole("dialog", { name: "供货报价" })).toBeTruthy();
  expect((buyer as HTMLInputElement).value).toBe("直属渠道");
  fireEvent.click(screen.getByRole("button", { name: "向直属采购方发布供货报价", exact: true }));
  await waitFor(() => expect(fetch.mock.calls.some(call => call[1]?.method === "POST")).toBe(true));
  expect(JSON.parse(String(fetch.mock.calls.find(call => call[1]?.method === "POST")?.[1]?.body))).toEqual({ buyer_id: "buyer-one" });
});

it("keeps a wheel on the open menu from scrolling the page", async () => {
  const options = Array.from({ length: 12 }, (_, index) => ({ value: `buyer-${index}`, label: `采购方 ${index}` }));
  render(<RecordPicker label="直属采购方" value="" displayLabel="" options={options} placeholder="按名称或编号搜索" emptyLabel="请选择实际业务记录" onQuery={() => undefined} onChoose={() => undefined} />);
  fireEvent.focus(screen.getByRole("combobox", { name: "直属采购方" }));
  const option = await screen.findByRole("option", { name: "采购方 11" });
  const menu = option.closest("[data-record-picker-menu]");
  expect(menu).toBeTruthy();
  const blockPageWheel = vi.fn((event: Event) => event.preventDefault());
  document.addEventListener("wheel", blockPageWheel);
  fireEvent.wheel(option, { deltaY: 80, cancelable: true });
  expect(blockPageWheel).not.toHaveBeenCalled();
  document.removeEventListener("wheel", blockPageWheel);
});
