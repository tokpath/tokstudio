import { describe, expect, it } from "vitest";
import en from "@/messages/en.json";
import ja from "@/messages/ja.json";
import zh from "@/messages/zh.json";

function userFacingCopy(locale: typeof zh) {
  return JSON.stringify({
    login: locale.login,
    home: locale.home,
    public: locale.public,
    docsUi: locale.docsUi,
    blogUi: locale.blogUi,
    chrome: locale.chrome,
  });
}

describe("user-facing copy", () => {
  it.each([
    ["zh", zh],
    ["en", en],
    ["ja", ja],
  ] as const)("%s does not mention ofox", (_name, locale) => {
    expect(userFacingCopy(locale)).not.toMatch(/ofox/i);
  });

  it("does not render a login lead paragraph key", () => {
    expect(zh.login).not.toHaveProperty("lead");
    expect(en.login).not.toHaveProperty("lead");
    expect(ja.login).not.toHaveProperty("lead");
  });

  it("labels the public header console entry in every locale", () => {
    expect(zh.chrome.console).toBe("控制台");
    expect(en.chrome.console).toBe("Console");
    expect(ja.chrome.console).toBe("コンソール");
  });
});
