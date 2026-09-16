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
});
