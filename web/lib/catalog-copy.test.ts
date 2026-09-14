import { describe, expect, it } from "vitest";
import {
  filterPublicModelOptions,
  slugifyCatalogId,
  suggestProviderSlug,
  suggestPublicId,
  suggestPublicIdFromDisplay,
} from "./catalog-copy";

describe("catalog copy helpers", () => {
  it("rewrites the public model prefix when the vendor changes", () => {
    expect(suggestPublicId("alibaba", "", "tokenhub")).toBe("alibaba/");
    expect(suggestPublicId("alibaba", "tokenhub/", "tokenhub")).toBe("alibaba/");
    expect(suggestPublicId("alibaba", "tokenhub/echo-1", "tokenhub")).toBe("alibaba/echo-1");
    expect(suggestPublicId("alibaba", "custom/keep", "tokenhub")).toBe("custom/keep");
  });

  it("fills public model ids from display names until the operator edits them", () => {
    expect(slugifyCatalogId("HappyHorse 1.0")).toBe("happyhorse-1.0");
    expect(suggestPublicIdFromDisplay("alibaba", "HappyHorse 1.0", "alibaba/", "")).toBe("alibaba/happyhorse-1.0");
    expect(suggestPublicIdFromDisplay("alibaba", "HappyHorse 2.0", "alibaba/happyhorse-1.0", "happyhorse-1.0")).toBe(
      "alibaba/happyhorse-2.0",
    );
    expect(suggestPublicIdFromDisplay("alibaba", "Other", "alibaba/custom-id", "happyhorse-1.0")).toBe("alibaba/custom-id");
    expect(suggestPublicId("", "keep-me")).toBe("keep-me");
    expect(suggestPublicIdFromDisplay("", "HappyHorse 1.0", "", "")).toBe("");
  });

  it("fills provider slugs from names until the operator edits them", () => {
    expect(suggestProviderSlug("OpenAI", "", "")).toBe("openai");
    expect(suggestProviderSlug("OpenAI Direct", "openai", "openai")).toBe("openai-direct");
    expect(suggestProviderSlug("Renamed", "keep-me", "openai")).toBe("keep-me");
  });

  it("filters public models by display name, vendor, or id", () => {
    const items = [
      { id: "alibaba/happyhorse-1.0", display_name: "HappyHorse 1.0", vendor: "alibaba" },
      { id: "tokenhub/echo-1", display_name: "Echo", vendor: "tokenhub" },
    ];
    expect(filterPublicModelOptions(items, "happy").map((item) => item.id)).toEqual(["alibaba/happyhorse-1.0"]);
    expect(filterPublicModelOptions(items, "tokenhub").map((item) => item.id)).toEqual(["tokenhub/echo-1"]);
  });
});
