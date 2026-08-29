/** @vitest-environment jsdom */
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { KeysList } from "../app/app/keys";

describe("KeysList", () => {
  it("renders API Key prefix and status for the user console", () => {
    render(
      <KeysList
        items={[{ id: "key_1", name: "default", prefix: "thk_abcd", status: "active", key: "thk_abcdsecret" }]}
      />,
    );
    expect(screen.getByText(/default/)).toBeTruthy();
    expect(screen.getByText(/thk_abcd/)).toBeTruthy();
    expect(screen.getByText(/active/)).toBeTruthy();
  });
});
