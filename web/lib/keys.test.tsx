/** @vitest-environment jsdom */
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { KeysList, parseAllowlist } from "../app/app/keys";

describe("KeysList", () => {
  it("renders API Key prefix and status for the user console", () => {
    render(
      <KeysList
        items={[{ id: "key_1", name: "default", prefix: "thk_abcd", status: "active", key: "thk_abcdsecret" }]}
      />,
    );
    expect(screen.getByText(/default/)).toBeTruthy();
    expect(screen.getAllByText(/thk_abcd/).length).toBeGreaterThan(0);
    expect(screen.getByText(/active/)).toBeTruthy();
    expect(screen.getByText(/模型白名单：不限制/)).toBeTruthy();
  });

  it("renders a model allowlist and RPM", () => {
    render(
      <KeysList
        items={[
          {
            id: "key_2",
            name: "gemini-only",
            prefix: "thk_gem1",
            status: "active",
            rpm_limit: 30,
            allowlist: ["google/gemini-flash"],
          },
        ]}
      />,
    );
    expect(screen.getByText(/模型白名单：google\/gemini-flash/)).toBeTruthy();
    expect(screen.getByText(/RPM 30/)).toBeTruthy();
  });

  it("parses comma-separated allowlists", () => {
    expect(parseAllowlist(" tokenhub/echo-1 , google/gemini-flash，tokenhub/echo-1 ")).toEqual([
      "tokenhub/echo-1",
      "google/gemini-flash",
      "tokenhub/echo-1",
    ]);
    expect(parseAllowlist("")).toEqual([]);
  });
});
