/** @vitest-environment jsdom */
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import ExamplesPanel from "./examples-panel";
import { withZh } from "@/lib/test-i18n";

const docs = {
  brand: { api_domain: "api.tokenhub.test" },
  models: ["tokenhub/echo-1"],
  examples: {
    curl: `curl https://api.tokenhub.test/v1/chat/completions -d '{"model":"tokenhub/echo-1"}'`,
    python: "client.chat.completions.create(model='tokenhub/echo-1')",
    node: "client.chat.completions.create({ model: 'tokenhub/echo-1' })",
  },
};

const focusCurl = `curl -sS https://api.tokenhub.test/v1/embeddings -H "Authorization: Bearer \${TOKENHUB_API_KEY}" -d '{"model":"openai/text-embedding-3","input":"hello"}'`;

describe("ExamplesPanel", () => {
  beforeEach(() => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => ({
        ok: true,
        json: async () => docs,
      })),
    );
    vi.stubGlobal("navigator", {
      clipboard: { writeText: vi.fn(async () => undefined) },
    });
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it("keeps a focused model on curl only, and copies that same sample", async () => {
    render(
      withZh(
        <ExamplesPanel requestedModel="openai/text-embedding-3" focusPath="/v1/embeddings" focusCurl={focusCurl} />,
      ),
    );
    const sample = await screen.findByTestId("example-sample");
    await waitFor(() => expect(sample.textContent).toContain("openai/text-embedding-3"));
    expect(sample.getAttribute("data-example-lang")).toBe("curl");
    expect(sample.getAttribute("data-example-model")).toBe("openai/text-embedding-3");
    expect(sample.textContent).toContain("/v1/embeddings");
    expect(sample.textContent).not.toContain("tokenhub/echo-1");
    expect(screen.getByRole("tab", { name: "curl" }).getAttribute("aria-selected")).toBe("true");
    expect(screen.queryByRole("tab", { name: "Python" })).toBeNull();
    expect(screen.queryByRole("tab", { name: "Node.js" })).toBeNull();
    expect(screen.queryByRole("button", { name: "复制 Python" })).toBeNull();
    expect(screen.queryByRole("button", { name: "复制 Node.js" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "复制 curl" }));
    await waitFor(() => expect(navigator.clipboard.writeText).toHaveBeenCalledWith(focusCurl));
    expect(navigator.clipboard.writeText).not.toHaveBeenCalledWith(docs.examples.curl);
  });

  it("switches generic docs tabs and copies the visible language", async () => {
    render(withZh(<ExamplesPanel />));
    const sample = await screen.findByTestId("example-sample");
    await waitFor(() => expect(sample.textContent).toContain("tokenhub/echo-1"));
    expect(sample.getAttribute("data-example-lang")).toBe("curl");
    fireEvent.click(screen.getByRole("tab", { name: "Python" }));
    expect(sample.getAttribute("data-example-lang")).toBe("python");
    expect(sample.textContent).toBe(docs.examples.python);
    fireEvent.click(screen.getByRole("button", { name: "复制 Python" }));
    await waitFor(() => expect(navigator.clipboard.writeText).toHaveBeenCalledWith(docs.examples.python));
    fireEvent.click(screen.getByRole("tab", { name: "Node.js" }));
    expect(sample.getAttribute("data-example-lang")).toBe("node");
    expect(sample.textContent).toBe(docs.examples.node);
    fireEvent.click(screen.getByRole("button", { name: "复制 Node.js" }));
    await waitFor(() => expect(navigator.clipboard.writeText).toHaveBeenCalledWith(docs.examples.node));
  });
});
