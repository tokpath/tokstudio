export type AdminModel = {
  id: string;
  vendor: string;
  display_name: string;
  status: string;
  sync_state?: string;
  capabilities?: Record<string, unknown>;
  sell_price?: Record<string, unknown>;
  providers?: string[];
};

export function modelEditHref(publicId: string): string {
  return `/admin/models/${publicId}`;
}

export function formatSellPrice(price?: Record<string, unknown> | null): string {
  if (!price) {
    return "—";
  }
  const parts: string[] = [];
  if (price.input != null && String(price.input) !== "") {
    parts.push(`in ${price.input}`);
  }
  if (price.output != null && String(price.output) !== "") {
    parts.push(`out ${price.output}`);
  }
  if (price.video_second != null && String(price.video_second) !== "") {
    parts.push(`video ${price.video_second}`);
  }
  if (price.image_count != null && String(price.image_count) !== "") {
    parts.push(`image ${price.image_count}`);
  }
  if (price.audio_second != null && String(price.audio_second) !== "") {
    parts.push(`audio ${price.audio_second}`);
  }
  return parts.length > 0 ? parts.join(" / ") : "—";
}

export function supportedParametersText(capabilities?: Record<string, unknown> | null): string {
  const raw = capabilities?.supported_parameters;
  if (Array.isArray(raw)) {
    return raw.map(String).join(", ");
  }
  if (typeof raw === "string") {
    return raw;
  }
  return "";
}

export function parseSupportedParameters(value: string): string[] {
  return value
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean);
}

export function extraCapabilitiesJSON(capabilities?: Record<string, unknown> | null): string {
  if (!capabilities) {
    return "";
  }
  const extra = { ...capabilities };
  delete extra.supported_parameters;
  return Object.keys(extra).length > 0 ? JSON.stringify(extra, null, 2) : "";
}

export function buildCapabilities(params: string, extraJSON: string): Record<string, unknown> {
  const extra = extraJSON.trim() ? (JSON.parse(extraJSON) as Record<string, unknown>) : {};
  return { ...extra, supported_parameters: parseSupportedParameters(params) };
}
