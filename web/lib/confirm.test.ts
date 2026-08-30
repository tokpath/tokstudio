import { describe, expect, it } from "vitest";
import { confirmHeaders } from "./confirm";

describe("confirmHeaders", () => {
  it("sends the TokenHub confirm header", () => {
    expect(confirmHeaders).toMatchObject({
      "Content-Type": "application/json",
      "X-Tokenhub-Confirm": "1",
    });
  });
});
