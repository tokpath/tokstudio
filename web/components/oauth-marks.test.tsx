/** @vitest-environment jsdom */
import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { GitHubMark, GoogleMark } from "./oauth-marks";

describe("oauth brand marks", () => {
  it("renders the GitHub invertocat as a currentColor mark", () => {
    const { container } = render(<GitHubMark />);
    const svg = container.querySelector("svg");
    expect(svg).toBeTruthy();
    expect(svg?.getAttribute("viewBox")).toBe("0 0 24 24");
    expect(svg?.getAttribute("fill")).toBe("currentColor");
    expect(svg?.querySelectorAll("path")).toHaveLength(1);
  });

  it("renders the four-color Google G", () => {
    const { container } = render(<GoogleMark />);
    const fills = [...container.querySelectorAll("path")].map((node) => node.getAttribute("fill"));
    expect(fills).toEqual(["#4285F4", "#34A853", "#FBBC05", "#EA4335"]);
  });
});
