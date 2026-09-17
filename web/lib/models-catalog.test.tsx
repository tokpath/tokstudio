/** @vitest-environment jsdom */
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ModelsCatalog } from "../components/models-catalog";
import { withZh } from "./test-i18n";

vi.mock("next/navigation", () => ({
  useRouter: () => ({ replace: vi.fn(), push: vi.fn() }),
}));

describe("ModelsCatalog", () => {
  afterEach(() => {
    cleanup();
  });

  it("offers try now into playground with catalog filters", () => {
    render(
      withZh(
        <ModelsCatalog
          basePath="/app/catalog"
          query={{ kind: "text" }}
          facets={{ kinds: [{ id: "text", count: 1 }], vendors: [{ id: "google", count: 1 }] }}
          models={[
            {
              id: "google/gemini-flash",
              vendor: "google",
              display_name: "Gemini Flash",
              description: "适合短回复",
              sell_price: { input: 1, output: 2 },
              kind: "text",
            },
          ]}
        />,
      ),
    );
    const tryLink = screen.getByRole("link", { name: "立即试用" });
    expect(tryLink.getAttribute("href")).toContain("/app/playground?model=google%2Fgemini-flash");
    expect(tryLink.getAttribute("href")).toContain("from=");
    expect(screen.getByText("适合短回复")).toBeTruthy();
  });

  it("sends image models to media with the model id", () => {
    render(
      withZh(
        <ModelsCatalog
          basePath="/app/catalog"
          query={{ kind: "image" }}
          facets={{ kinds: [{ id: "image", count: 1 }], vendors: [{ id: "bytedance", count: 1 }] }}
          models={[
            {
              id: "bytedance/seedream",
              vendor: "bytedance",
              display_name: "Seedream",
              kind: "image",
            },
          ]}
        />,
      ),
    );
    expect(screen.getByRole("link", { name: "立即试用" }).getAttribute("href")).toContain("/app/media?model=bytedance%2Fseedream");
    expect(screen.getByRole("link", { name: "立即试用" }).getAttribute("href")).toContain("kind=image");
  });

  it("sends video models to media with the model id", () => {
    render(
      withZh(
        <ModelsCatalog
          basePath="/app/catalog"
          query={{ kind: "video" }}
          facets={{ kinds: [{ id: "video", count: 1 }], vendors: [{ id: "bytedance", count: 1 }] }}
          models={[
            {
              id: "bytedance/seedance",
              vendor: "bytedance",
              display_name: "Seedance",
              kind: "video",
            },
          ]}
        />,
      ),
    );
    expect(screen.getByRole("link", { name: "立即试用" }).getAttribute("href")).toContain("/app/media?model=bytedance%2Fseedance");
    expect(screen.getByRole("link", { name: "立即试用" }).getAttribute("href")).toContain("kind=video");
  });

  it("does not show an empty catalog when loading failed", () => {
    render(
      withZh(
        <ModelsCatalog
          basePath="/models"
          query={{}}
          facets={{ kinds: [], vendors: [] }}
          models={[]}
          loadOk={false}
          loadMessage="catalog down"
        />,
      ),
    );
    expect(screen.getByText("目录加载失败")).toBeTruthy();
    expect(screen.getByText("catalog down")).toBeTruthy();
    expect(screen.queryByText("没有匹配的模型")).toBeNull();
    expect(screen.getByRole("button", { name: "重试" })).toBeTruthy();
  });
});
