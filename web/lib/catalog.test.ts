import { describe, expect, it } from "vitest";
import {
  buildCapabilities,
  capabilitiesToForm,
  catalogHref,
  extraCapabilitiesJSON,
  formToCapabilities,
  formatSellPrice,
  KNOWN_PARAMETERS,
  modelEditHref,
  optionUnion,
  parseCatalogSearchParams,
  parseSupportedParameters,
  publicModelsPath,
  resolveVendorInput,
  supportedParametersText,
  vendorLabel,
} from "./catalog";

describe("admin model catalog helpers", () => {
  it("builds the edit href from a public id that contains a slash", () => {
    expect(modelEditHref("tokenhub/echo-1")).toBe("/admin/models/tokenhub/echo-1");
  });

  it("formats text and media sell prices", () => {
    expect(formatSellPrice(undefined)).toBe("—");
    expect(formatSellPrice({ input: "0.000001", output: "0.000002", currency: "USD" })).toBe("in 1/M / out 2/M");
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

  it("edits capabilities via checkboxes without typing parameter names", () => {
    const form = capabilitiesToForm({
      supported_parameters: ["stream", "tools", "custom_foo"],
      supported_endpoints: ["/v1/chat/completions"],
      architecture: {
        modality: "text+image->text",
        input_modalities: ["text", "image"],
        output_modalities: ["text"],
        tokenizer: "qwen",
        instruct_type: null,
      },
      video_attributes: { fps: 24 },
    });
    expect(form.supported_parameters).toEqual(["stream", "tools", "custom_foo"]);
    expect(form.input_modalities).toEqual(["text", "image"]);
    expect(form.tokenizer).toBe("qwen");
    expect(JSON.parse(form.rest_json)).toEqual({
      architecture: { instruct_type: null },
      video_attributes: { fps: 24 },
    });
    expect(optionUnion(KNOWN_PARAMETERS, form.supported_parameters)).toContain("custom_foo");
    const saved = formToCapabilities({ ...form, supported_parameters: ["stream", "reasoning"] });
    expect(saved.supported_parameters).toEqual(["stream", "reasoning"]);
    expect(saved.architecture).toMatchObject({
      input_modalities: ["text", "image"],
      output_modalities: ["text"],
      tokenizer: "qwen",
      modality: "text+image->text",
      instruct_type: null,
    });
    expect(saved.video_attributes).toEqual({ fps: 24 });
  });
});

describe("public catalog query strings", () => {
  it("maps page search params onto the public models API path", () => {
    expect(parseCatalogSearchParams({ vendor: "z-ai", output: "text", q: " glm " })).toEqual({
      vendor: "z-ai",
      kind: "text",
      q: "glm",
    });
    expect(publicModelsPath({ vendor: "z-ai", kind: "text", q: "glm" })).toBe(
      "/v1/public/models?vendor=z-ai&kind=text&q=glm",
    );
    expect(publicModelsPath({ id: "z-ai/glm-5.3-flash" })).toBe(
      "/v1/public/models?id=z-ai%2Fglm-5.3-flash",
    );
    expect(catalogHref("/models", { vendor: "z-ai", kind: "all" })).toBe("/models?vendor=z-ai");
    expect(catalogHref("/app/catalog", {})).toBe("/app/catalog");
  });
});

describe("vendor labels", () => {
  it("shows business names and maps typed labels back to ids", () => {
    expect(vendorLabel("alibaba")).toBe("阿里");
    expect(vendorLabel("")).toBe("—");
    expect(resolveVendorInput("阿里")).toBe("alibaba");
    expect(resolveVendorInput("OpenAI")).toBe("openai");
    expect(resolveVendorInput("tokenhub")).toBe("tokenhub");
  });
});
