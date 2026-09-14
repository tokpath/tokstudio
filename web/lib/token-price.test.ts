import { describe, expect, it } from "vitest";
import { formatIOPerMillion, millionDim, perMillionToPerToken, perTokenToPerMillion } from "./token-price";

describe("token price million conversion", () => {
  it("converts form millions into catalog per-token strings", () => {
    expect(perMillionToPerToken("2")).toBe("0.000002");
    expect(perMillionToPerToken("1")).toBe("0.000001");
    expect(perMillionToPerToken("0.7")).toBe("0.0000007");
    expect(perMillionToPerToken("0.4")).toBe("0.0000004");
    expect(perMillionToPerToken("1.4")).toBe("0.0000014");
    expect(perMillionToPerToken("")).toBe("");
  });

  it("converts stored per-token strings back to millions for the form", () => {
    expect(perTokenToPerMillion("0.000002")).toBe("2");
    expect(perTokenToPerMillion("0.000001")).toBe("1");
    expect(perTokenToPerMillion("0.0000007")).toBe("0.7");
    expect(perTokenToPerMillion("0.0000004")).toBe("0.4");
    expect(perTokenToPerMillion("0")).toBe("0");
  });

  it("round-trips echo seed prices", () => {
    for (const million of ["1", "2", "0.7", "1.4", "0.4", "0.8"]) {
      expect(perTokenToPerMillion(perMillionToPerToken(million))).toBe(million);
    }
  });

  it("rejects leftover per-token decimals so zeros are not silently misread", () => {
    expect(() => perMillionToPerToken("0.000002")).toThrow(/每百万/);
    expect(() => millionDim("abc", "")).toThrow(/无效单价/);
  });

  it("builds nested dims for the price-book API", () => {
    expect(millionDim("1", "2")).toEqual({ input: "0.000001", output: "0.000002" });
    expect(millionDim("", "")).toBeUndefined();
  });

  it("formats in/out pairs from price-book rows", () => {
    expect(formatIOPerMillion("0.000001/0.000002")).toBe("1/2");
    expect(formatIOPerMillion("0.0000004/0.0000008")).toBe("0.4/0.8");
  });
});
