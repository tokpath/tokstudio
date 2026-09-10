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

const SECRET_LIKE = /(?:ya29\.|1\/\/|mock:|[A-Za-z0-9-_]{24,})/g;

/** 结构化错误可以上屏；授权码 / token 不能出现在 UI。 */
export function sanitizeOAuthError(message: string | undefined | null, fallback: string): string {
  const raw = (message || "").trim();
  if (!raw) {
    return fallback;
  }
  const cleaned = raw.replace(SECRET_LIKE, "…").trim();
  if (!cleaned || cleaned === "…") {
    return fallback;
  }
  return cleaned;
}

export function oauthFailureHref(message: string, errorCode?: string): string {
  const params = new URLSearchParams();
  params.set("oauth_error", sanitizeOAuthError(message, "Google 登录失败"));
  if (errorCode && !SECRET_LIKE.test(errorCode) && errorCode.length < 64) {
    params.set("error_code", errorCode);
  }
  return `/login?${params.toString()}`;
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
