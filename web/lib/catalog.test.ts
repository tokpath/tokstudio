import { describe, expect, it } from "vitest";
import {
  buildCapabilities,
  extraCapabilitiesJSON,
  formatSellPrice,
  modelEditHref,
  parseSupportedParameters,
  supportedParametersText,
} from "./catalog";

describe("admin model catalog helpers", () => {
  it("builds the edit href from a public id that contains a slash", () => {
    expect(modelEditHref("tokenhub/echo-1")).toBe("/admin/models/tokenhub/echo-1");
  });

  it("formats text and media sell prices", () => {
    expect(formatSellPrice(undefined)).toBe("—");
    expect(formatSellPrice({ input: "0.000001", output: "0.000002", currency: "USD" })).toBe("in 0.000001 / out 0.000002");
    expect(formatSellPrice({ video_second: "0.01", image_count: "0.02" })).toBe("video 0.01 / image 0.02");
  });

  it("round-trips supported parameters and extra capability JSON", () => {
    expect(parseSupportedParameters("stream, tools, json")).toEqual(["stream", "tools", "json"]);
    expect(supportedParametersText({ supported_parameters: ["stream", "vision"] })).toBe("stream, vision");
    expect(extraCapabilitiesJSON({ supported_parameters: ["stream"], output_modality: "video" })).toBe(
      JSON.stringify({ output_modality: "video" }, null, 2),
    );
    expect(buildCapabilities("stream, tools", '{"output_modality":"video"}')).toEqual({
      output_modality: "video",
      supported_parameters: ["stream", "tools"],
    });
  });
});
