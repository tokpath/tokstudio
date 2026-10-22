// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import { Playground } from "./playground";
import { apiExample } from "./customer-models";

const response = (body: unknown) => new Response(JSON.stringify(body), { status: 200 });
function model(operation: string) { return { id: "public-model", name: "公开体验模型", capabilities: [operation], status: "active", prices: [] }; }
function show() { const cache = new QueryClient({ defaultOptions: { queries: { retry: false } } }); return { ...render(<QueryClientProvider client={cache}><Playground /></QueryClientProvider>), cache }; }
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

it("sends the speech text for the selected model", async () => {
  const calls: RequestInit[] = [];
  vi.stubGlobal("fetch", vi.fn(async (_path: RequestInfo | URL, init?: RequestInit) => {
    if (init?.method === "POST") { calls.push(init); return response({ request_id: "request-one", status: "succeeded", charged_atg: "0.000000000000000001", media_kind: "audio", result_url: "/api/v1/me/requests/request-one/result", result_expires_at: "2099-01-01T00:00:00Z" }); }
    return response({ items: [model("speech")] });
  }));
  show();
  fireEvent.change(await screen.findByLabelText("公开模型", { exact: true }), { target: { value: "public-model" } });
  fireEvent.change(screen.getByLabelText("能力", { exact: true }), { target: { value: "speech" } });
  fireEvent.change(screen.getByLabelText("合成语音文本"), { target: { value: "你好" } });
  expect(screen.queryByText("模型参数")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "确认计费并发送" }));
  await screen.findByText("本次体验结果");
  expect(JSON.parse(String(calls[0].body))).toEqual({ model: "public-model", capability: "speech", input: "你好" });
  expect(screen.getByRole("link", { name: "核对费用与任务状态" }).getAttribute("href")).toBe("/workspace/requests?record=request-one");
});

it("sends transcription as multipart with the original file and parameter payload", async () => {
  const calls: RequestInit[] = [];
  vi.stubGlobal("fetch", vi.fn(async (_path: RequestInfo | URL, init?: RequestInit) => {
    if (init?.method === "POST") { calls.push(init); return response({ request_id: "transcription-one", status: "succeeded", charged_atg: "0.0001", text: "转写完成" }); }
    return response({ items: [model("transcription")] });
  }));
  show();
  fireEvent.change(await screen.findByLabelText("公开模型", { exact: true }), { target: { value: "public-model" } });
  fireEvent.change(screen.getByLabelText("能力", { exact: true }), { target: { value: "transcription" } });
  const file = new File(["synthetic audio"], "voice.wav", { type: "audio/wav" });
  fireEvent.change(screen.getByLabelText("音频文件"), { target: { files: [file] } });
  fireEvent.submit(screen.getByLabelText("音频文件").closest("form")!);
  await screen.findByText("转写完成");
  const body = calls[0].body as FormData;
  expect(body.get("model")).toBe("public-model");
  expect(body.get("capability")).toBe("transcription");
  expect(body.get("parameters")).toBeNull();
  expect(body.get("file")).toBe(file);
  expect((calls[0].headers as Headers).get("Content-Type")).toBeNull();
});

it("still sends a chat request without asking for vendor parameters", async () => {
  const fetch = vi.fn(async () => response({ items: [model("chat")] }));
  vi.stubGlobal("fetch", fetch);
  show();
  fireEvent.change(await screen.findByLabelText("公开模型", { exact: true }), { target: { value: "public-model" } });
  fireEvent.change(screen.getByLabelText("能力", { exact: true }), { target: { value: "chat" } });
  expect(screen.queryByText(/需要完整结构化参数/)).toBeNull();
  expect((screen.getByRole("button", { name: "确认计费并发送" }) as HTMLButtonElement).disabled).toBe(false);
});

it("replays an unknown experience without releasing its content or creating a second request", async () => {
  const calls: RequestInit[] = [];
  vi.stubGlobal("fetch", vi.fn(async (_path: RequestInfo | URL, init?: RequestInit) => {
    if (init?.method === "POST") { calls.push(init); if (calls.length === 1) throw new TypeError("connection lost"); return response({ request_id: "original-request", status: "succeeded", charged_atg: "0.01", text: "原任务结果" }); }
    return response({ items: [model("chat")] });
  }));
  const { cache } = show();
  fireEvent.change(await screen.findByLabelText("公开模型", { exact: true }), { target: { value: "public-model" } });
  fireEvent.change(screen.getByLabelText("能力", { exact: true }), { target: { value: "chat" } });
  fireEvent.change(screen.getByLabelText("本次输入"), { target: { value: "原内容" } });
  fireEvent.click(screen.getByRole("button", { name: "确认计费并发送" }));
  await screen.findByRole("button", { name: "核实或重试原请求" });
  expect(screen.getByLabelText("本次输入").matches(":disabled")).toBe(true);
  expect((screen.getByRole("button", { name: "清空输入" }) as HTMLButtonElement).disabled).toBe(true);
  act(() => { cache.setQueryData(["my-models"], { items: [] }); });
  fireEvent.click(screen.getByRole("button", { name: "核实或重试原请求" }));
  await screen.findByText("原任务结果");
  await waitFor(() => expect(calls).toHaveLength(2));
  expect(calls[1].body).toBe(calls[0].body);
  expect((calls[1].headers as Headers).get("Idempotency-Key")).toBe((calls[0].headers as Headers).get("Idempotency-Key"));
});

it("builds capability-specific API examples instead of sending every model to chat", () => {
  const expected = { chat: "/chat/completions", responses: "/responses", embedding: "/embeddings", image: "/images/generations", image_generation: "/images/generations", speech: "/audio/speech", transcription: "/audio/transcriptions", video: "/videos", video_generation: "/videos" };
  for (const [operation, endpoint] of Object.entries(expected)) {
    expect(apiExample("https://api.example.test/v1/", "public-model", operation, "curl")).toContain(`https://api.example.test/v1${endpoint}`);
    expect(apiExample("https://api.example.test/v1", "public-model", operation, "python")).toContain("Idempotency-Key");
  }
});

it.each(["image", "video"])("sends the %s prompt without a vendor parameter form", async capability => {
  const calls: RequestInit[] = [];
  vi.stubGlobal("fetch", vi.fn(async (_path: RequestInfo | URL, init?: RequestInit) => {
    if (init?.method === "POST") { calls.push(init); return response({ request_id: "generation-request", status: "succeeded", charged_atg: "0.01", text: "生成完成" }); }
    return response({ items: [model(capability)] });
  }));
  show();
  fireEvent.change(await screen.findByLabelText("公开模型", { exact: true }), { target: { value: "public-model" } });
  fireEvent.change(screen.getByLabelText("能力", { exact: true }), { target: { value: capability } });
  fireEvent.change(screen.getByLabelText("本次输入"), { target: { value: "海边的日出" } });
  expect(screen.queryByText("模型参数")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "确认计费并发送" }));
  await screen.findByText("生成完成");
  expect(JSON.parse(String(calls[0].body))).toEqual({ model: "public-model", capability, input: "海边的日出" });
});
