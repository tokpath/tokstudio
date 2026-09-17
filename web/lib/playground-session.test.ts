import { describe, expect, it } from "vitest";
import { pickPlaygroundModel } from "./playground-session";

const echo = { id: "tokenhub/echo-1", vendor: "tokenhub", display_name: "Echo", kind: "text" as const };
const image = {
  id: "bytedance/seedream",
  vendor: "bytedance",
  display_name: "Seedream",
  kind: "image" as const,
  capabilities: { supported_endpoints: ["/v1/images/generations"] },
};
const down = { ...echo, id: "tokenhub/old", status: "unavailable" };

describe("pickPlaygroundModel", () => {
  it("rejects a requested id that is not in the loaded catalog", () => {
    expect(pickPlaygroundModel([echo], "google/gemini-flash")).toEqual({ id: "", error: "missing" });
    expect(pickPlaygroundModel([], "tokenhub/echo-1")).toEqual({ id: "", error: "missing" });
  });

  it("does not invent a model when the list is empty", () => {
    expect(pickPlaygroundModel([], undefined)).toEqual({ id: "" });
  });

  it("separates unavailable and non-chat models", () => {
    expect(pickPlaygroundModel([down], "tokenhub/old")).toEqual({ id: "tokenhub/old", error: "unavailable" });
    expect(pickPlaygroundModel([image], "bytedance/seedream")).toEqual({ id: "bytedance/seedream", error: "notChat" });
  });

  it("keeps a catalog-confirmed chat model", () => {
    expect(pickPlaygroundModel([echo, image], "tokenhub/echo-1")).toEqual({ id: "tokenhub/echo-1" });
  });
});
