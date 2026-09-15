export const STORAGE_SOURCE_TITLE = "存储源";
export const STORAGE_LABEL_S3 = "S3";
export const STORAGE_LABEL_UNAVAILABLE = "存储不可用";

export type StorageSource = {
  source?: string;
  ok?: boolean;
  label?: string;
  detail?: string;
};

export type StorageBadgeView = {
  label: string;
  tone: "muted" | "ok" | "unavailable";
  ok: boolean;
};

function stripSilentCheck(label: string): string {
  return label.replace(/[✓✔☑✅]/g, "").trim();
}

/** 只读存储源。失败/缺桶必须是灰色「存储不可用」，禁止成功对勾。 */
export function storageBadge(source?: StorageSource | null): StorageBadgeView {
  if (!source || source.ok !== true || source.source === "unavailable") {
    return { label: STORAGE_LABEL_UNAVAILABLE, tone: "unavailable", ok: false };
  }
  if (source.source === "s3") {
    return { label: STORAGE_LABEL_S3, tone: "ok", ok: true };
  }
  return { label: STORAGE_LABEL_S3, tone: "muted", ok: true };
}

type StorageFactBody = {
  storage?: StorageSource | null;
  error?: { code?: string; message?: string };
};

function asStorageFactBody(body: unknown): StorageFactBody | undefined {
  if (!body || typeof body !== "object") {
    return undefined;
  }
  return body as StorageFactBody;
}

/** 接口失败时也要把徽章打成「存储不可用」，禁止残留绿色/对勾。 */
export function applyStorageFact(body: unknown, responseOK = true): StorageSource | undefined {
  const fact = asStorageFactBody(body);
  const storeFailed =
    !responseOK &&
    (fact?.error?.code === "store_unavailable" ||
      fact?.error?.message === STORAGE_LABEL_UNAVAILABLE ||
      fact?.storage?.ok === false ||
      fact?.storage?.source === "unavailable");
  if (storeFailed) {
    return {
      source: "unavailable",
      ok: false,
      label: STORAGE_LABEL_UNAVAILABLE,
      detail: fact?.storage?.detail,
    };
  }
  if (fact?.storage) {
    const view = storageBadge(fact.storage);
    return {
      source: view.ok ? fact.storage.source : "unavailable",
      ok: view.ok,
      label: stripSilentCheck(view.label),
      detail: fact.storage.detail,
    };
  }
  return undefined;
}

export function forbidsSilentSuccessCheck(view: StorageBadgeView): boolean {
  if (view.ok) {
    return view.label === STORAGE_LABEL_S3 && view.tone !== "unavailable";
  }
  return view.label === STORAGE_LABEL_UNAVAILABLE && view.tone === "unavailable";
}
