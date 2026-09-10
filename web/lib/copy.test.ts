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

  it("locks the Google idle button copy", () => {
    expect(zh.login.google).toBe("用 Google 登录");
    expect(zh.login.googleRedirecting).toBe("正在跳转 Google…");
    expect(zh.login.googleUnconfigured).toBe("未配置 Google 登录");
  });

  it("locks user-shell copy and empty keys", () => {
    expect(zh.shell.profile).toBe("个人资料");
    expect(zh.shell.keys).toBe("API 密钥");
    expect(zh.shell.logout).toBe("退出登录");
    expect(zh.user.emptyKeys).toBe("暂无 API 密钥");
    expect(zh.console.profile.title).toBe("个人资料");
  });

  it("labels the public header console entry in every locale", () => {
    expect(zh.chrome.console).toBe("控制台");
    expect(en.chrome.console).toBe("Console");
    expect(ja.chrome.console).toBe("コンソール");
  });
});
