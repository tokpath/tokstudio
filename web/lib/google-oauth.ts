/** Next.js 接收 Google `?code=&state=` 的规范路径；Console 也登记这一条 Redirect URI。 */
export const GOOGLE_OAUTH_CALLBACK_PATH = "/login/oauth/google";

export const GOOGLE_OAUTH_NEXT_KEY = "tokenhub_login_next";

export type GoogleAuthStatus = {
  available?: boolean;
  configured?: boolean;
  mock?: boolean;
};

export type GoogleButtonState = {
  disabled: boolean;
  showUnconfigured: boolean;
  labelKey: "google" | "googleRedirecting";
};

/** 未拿到状态或三件套缺失：灰按钮，绝不假装可点再 mock 成功。 */
export function googleButtonState(status: GoogleAuthStatus | null, loading: boolean): GoogleButtonState {
  if (loading) {
    return { disabled: true, showUnconfigured: false, labelKey: "googleRedirecting" };
  }
  if (!status || !status.available) {
    return { disabled: true, showUnconfigured: true, labelKey: "google" };
  }
  return { disabled: false, showUnconfigured: false, labelKey: "google" };
}

const SECRET_LIKE = /(?:ya29\.[^\s]+|1\/\/[^\s]+|mock:[^\s]+|[A-Za-z0-9-_]{24,})/;

function hasSecretLike(value: string): boolean {
  return SECRET_LIKE.test(value);
}

/** 结构化错误可以上屏；授权码 / token 不能出现在 UI。 */
export function sanitizeOAuthError(message: string | undefined | null, fallback: string): string {
  const raw = (message || "").trim();
  if (!raw) {
    return fallback;
  }
  const cleaned = raw.replace(new RegExp(SECRET_LIKE, "g"), "…").trim();
  if (!cleaned || cleaned === "…" || hasSecretLike(cleaned)) {
    return fallback;
  }
  return cleaned;
}

export function oauthFailureHref(message: string, errorCode?: string): string {
  const params = new URLSearchParams();
  params.set("oauth_error", sanitizeOAuthError(message, "Google 登录失败"));
  if (errorCode && !hasSecretLike(errorCode) && errorCode.length < 64) {
    params.set("error_code", errorCode);
  }
  return `/login?${params.toString()}`;
}

const OAUTH_CALLBACK_LOCK_PREFIX = "tokenhub_google_oauth_code:";

/** 同文档内存锁：React 重挂载 / Strict Mode 比 sessionStorage 更可靠。 */
const inflightOAuthCodes = new Set<string>();

/** 同一授权码只允许一个回调兑换；防止 React 重挂载打出第二次 invalid_grant。 */
export function claimOAuthCallback(
  storage: Pick<Storage, "getItem" | "setItem"> | null | undefined,
  code: string,
): boolean {
  if (!code) {
    return true;
  }
  if (inflightOAuthCodes.has(code)) {
    return false;
  }
  if (storage) {
    try {
      const key = OAUTH_CALLBACK_LOCK_PREFIX + code;
      if (storage.getItem(key)) {
        inflightOAuthCodes.add(code);
        return false;
      }
      storage.setItem(key, "pending");
    } catch {
      /* private mode：退回内存锁 */
    }
  }
  inflightOAuthCodes.add(code);
  return true;
}

export function releaseOAuthCallback(
  storage: Pick<Storage, "removeItem"> | null | undefined,
  code: string,
): void {
  if (code) {
    inflightOAuthCodes.delete(code);
  }
  if (!storage || !code) {
    return;
  }
  try {
    storage.removeItem(OAUTH_CALLBACK_LOCK_PREFIX + code);
  } catch {
    /* ignore */
  }
}

/** 仅供单测重置模块级锁。 */
export function resetOAuthCallbackLocksForTests(): void {
  inflightOAuthCodes.clear();
}

export function readStoredNext(storage: Pick<Storage, "getItem" | "removeItem"> | null | undefined): string {
  if (!storage) {
    return "";
  }
  try {
    const value = storage.getItem(GOOGLE_OAUTH_NEXT_KEY) || "";
    storage.removeItem(GOOGLE_OAUTH_NEXT_KEY);
    return value;
  } catch {
    return "";
  }
}

export function storeLoginNext(storage: Pick<Storage, "setItem"> | null | undefined, next: string) {
  if (!storage || !next) {
    return;
  }
  try {
    storage.setItem(GOOGLE_OAUTH_NEXT_KEY, next);
  } catch {
    /* ignore quota / private mode */
  }
}

/** 回调失败但可能已由同伴请求写好 session：短轮询 /v1/me。 */
export async function recoverAuthenticatedSession(options: {
  probe: () => Promise<boolean>;
  attempts?: number;
  delayMs?: number;
  sleep?: (ms: number) => Promise<void>;
}): Promise<boolean> {
  const attempts = options.attempts ?? 6;
  const delayMs = options.delayMs ?? 120;
  const sleep =
    options.sleep ??
    ((ms: number) =>
      new Promise<void>((resolve) => {
        setTimeout(resolve, ms);
      }));
  for (let i = 0; i < attempts; i++) {
    if (await options.probe()) {
      return true;
    }
    if (i + 1 < attempts) {
      await sleep(delayMs);
    }
  }
  return false;
}

/** authentication_error / oauth_state_consumed：宁可先查 session，再决定是否进失败页。 */
export function shouldRecoverOAuthFailure(errorCode?: string | null, errorParam?: string | null): boolean {
  const code = (errorCode || "").trim();
  const param = (errorParam || "").trim();
  if (param === "oauth_state_consumed") {
    return true;
  }
  return code === "authentication_error" || code === "oauth_state_consumed";
}
