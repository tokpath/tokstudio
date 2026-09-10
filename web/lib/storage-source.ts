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

export function forbidsSilentSuccessCheck(view: StorageBadgeView): boolean {
  if (view.ok) {
    return view.label === STORAGE_LABEL_S3 && view.tone !== "unavailable";
  }
  return view.label === STORAGE_LABEL_UNAVAILABLE && view.tone === "unavailable";
}
