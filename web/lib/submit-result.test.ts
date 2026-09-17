import { describe, expect, it, vi, afterEach } from "vitest";
import { copyText, errorMessageFromBody, confirmJsonAction } from "./submit-result";

describe("submit result helpers", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("prefers API error text over the fallback", () => {
    expect(errorMessageFromBody({ error: { message: "当前密码不正确" } }, "改密失败")).toBe("当前密码不正确");
    expect(errorMessageFromBody({}, "改密失败")).toBe("改密失败");
    expect(errorMessageFromBody(null, "改密失败")).toBe("改密失败");
  });

  it("reports clipboard success only after writeText resolves", async () => {
    const writeText = vi.fn(async () => undefined);
    vi.stubGlobal("navigator", { clipboard: { writeText } });
    await expect(copyText("thk_secret")).resolves.toBe(true);
    expect(writeText).toHaveBeenCalledWith("thk_secret");
  });

  it("does not claim success when clipboard write is unavailable or throws", async () => {
    vi.stubGlobal("navigator", {});
    await expect(copyText("thk_secret")).resolves.toBe(false);
    vi.stubGlobal("navigator", {
      clipboard: {
        writeText: vi.fn(async () => {
          throw new Error("denied");
        }),
      },
    });
    await expect(copyText("thk_secret")).resolves.toBe(false);
  });

  it("returns false for API and network failures", async () => {
    const onError = vi.fn();
    const onSuccess = vi.fn();
    await expect(
      confirmJsonAction({
        request: async () =>
          ({
            ok: false,
            json: async () => ({ error: { message: "创建失败" } }),
          }) as Response,
        onError,
        onSuccess,
        failFallback: "失败",
        networkMessage: "网络不可用，请重试。",
      }),
    ).resolves.toBe(false);
    expect(onError).toHaveBeenCalledWith("创建失败");
    expect(onSuccess).not.toHaveBeenCalled();

    onError.mockClear();
    await expect(
      confirmJsonAction({
        request: async () => {
          throw new TypeError("Failed to fetch");
        },
        onError,
        failFallback: "失败",
        networkMessage: "网络不可用，请重试。",
      }),
    ).resolves.toBe(false);
    expect(onError).toHaveBeenCalledWith("网络不可用，请重试。");
  });
});
