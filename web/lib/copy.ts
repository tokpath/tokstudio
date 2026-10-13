import zh from "@/messages/zh.json";

/** Pure formatters accept the current request's translator; legacy callers use Chinese. */
export type CopyTranslator = (key: string, values?: Record<string, string | number>) => string;

export const defaultCopy: CopyTranslator = (key, values) => {
  let message: unknown = zh;
  for (const part of key.split(".")) message = (message as Record<string, unknown> | undefined)?.[part];
  if (typeof message !== "string") return key;
  return message.replace(/\{(\w+)\}/g, (match, name: string) => values?.[name] == null ? match : String(values[name]));
};
