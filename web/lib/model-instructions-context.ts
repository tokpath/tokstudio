import {safeNextPath} from "@/lib/login-next";

export type InstructionsQuery = {model?: string; key_id?: string; tab?: string; tool?: string; protocol?: string; language?: string; return_to?: string; promo?: string; promotion_code?: string};

export function instructionsQuery(query: InstructionsQuery, model: string, tab: string): URLSearchParams {
  const params = new URLSearchParams({model, tab});
  for (const name of ["key_id", "tool", "protocol", "language", "promo", "promotion_code"] as const) {
    if (query[name]) params.set(name, query[name]!);
  }
  const back = safeNextPath(query.return_to);
  if (back) params.set("return_to", back);
  return params;
}
