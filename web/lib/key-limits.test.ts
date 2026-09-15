import { describe, expect, it } from "vitest";
import { optionalPositiveInt, parseAllowlist } from "./key-limits";

describe("optionalPositiveInt", () => {
  it("keeps blanks as empty so the server default can apply", () => {
    expect(optionalPositiveInt("")).toBe("empty");
    expect(optionalPositiveInt("  ")).toBe("empty");
  });

  it("rejects values that used to be silently dropped", () => {
    expect(optionalPositiveInt("abc")).toBe("invalid");
    expect(optionalPositiveInt("0")).toBe("invalid");
    expect(optionalPositiveInt("-1")).toBe("invalid");
    expect(optionalPositiveInt("1.5")).toBe("invalid");
  });

  it("accepts positive integers", () => {
    expect(optionalPositiveInt("60")).toBe(60);
    expect(optionalPositiveInt("1")).toBe(1);
  });
});

describe("parseAllowlist", () => {
  it("parses comma-separated allowlists", () => {
    expect(parseAllowlist(" tokenhub/echo-1 , google/gemini-flash，tokenhub/echo-1 ")).toEqual([
      "tokenhub/echo-1",
      "google/gemini-flash",
      "tokenhub/echo-1",
    ]);
    expect(parseAllowlist("")).toEqual([]);
  });
});
