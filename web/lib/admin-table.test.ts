import { describe, expect, it } from "vitest";
import { paginate, tableRows } from "./admin-table";

describe("admin table helpers", () => {
  it("projects rows and paginates", () => {
    const items = [
      { id: "a", name: "Echo" },
      { id: "b", name: "Gemini" },
    ];
    expect(tableRows(items, [{ key: "name", header: "Name" }])).toEqual([["Echo"], ["Gemini"]]);
    expect(paginate(items, 2, 1)).toEqual([{ id: "b", name: "Gemini" }]);
  });
});
