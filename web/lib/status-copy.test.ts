import { describe, expect, it } from "vitest";
import { statusLabelKey, statusTone } from "./status-copy";

describe("status copy", () => {
  it("never marks an unknown status as success", () => {
    expect(statusTone("completed")).toBe("success");
    expect(statusTone("confirmed")).toBe("success");
    expect(statusTone("failed")).toBe("warn");
    expect(statusTone("weird_internal_code")).toBe("neutral");
    expect(statusTone("")).toBe("neutral");
    expect(statusLabelKey("completed")).toBe("stCompleted");
    expect(statusLabelKey("weird_internal_code")).toBeUndefined();
  });
});
