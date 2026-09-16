import { describe, expect, it } from "vitest";
import { pickPlaygroundModel } from "./playground-session";

describe("pickPlaygroundModel", () => {
  it("keeps the requested model even when it is not in the list", () => {
    expect(pickPlaygroundModel([{ id: "tokenhub/echo-1", vendor: "tokenhub", display_name: "Echo" }], "google/gemini-flash")).toBe(
      "google/gemini-flash",
    );
  });

  it("does not invent a model when the list is empty", () => {
    expect(pickPlaygroundModel([], undefined)).toBe("");
    expect(pickPlaygroundModel([], "tokenhub/echo-1")).toBe("tokenhub/echo-1");
  });
});
