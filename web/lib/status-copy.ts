export type StatusTone = "success" | "warn" | "neutral";

const SUCCESS = new Set([
  "active",
  "completed",
  "succeeded",
  "confirmed",
  "paid",
  "published",
  "fulfilled",
]);

const WARN = new Set([
  "failed",
  "disabled",
  "expired",
  "cancelled",
  "canceled",
  "rejected",
  "voided",
  "pending_review",
  "pending_reconciliation",
]);

const LABEL_KEY: Record<string, string> = {
  active: "stActive",
  disabled: "stDisabled",
  expired: "stExpired",
  completed: "stCompleted",
  succeeded: "stSucceeded",
  started: "stInProgress",
  confirmed: "stConfirmed",
  pending: "stPending",
  voided: "stVoided",
  pending_reconciliation: "stPendingRecon",
  pending_review: "stPendingReview",
  published: "stPublished",
  rejected: "stRejected",
  draft: "stDraft",
  failed: "stFailed",
  cancelled: "stCancelled",
  canceled: "stCancelled",
  queued: "stQueued",
  in_progress: "stInProgress",
  paid: "stPaid",
};

/** Unknown statuses stay neutral so they are never painted as success. */
export function statusTone(status?: string): StatusTone {
  const value = (status || "").trim().toLowerCase();
  if (SUCCESS.has(value)) {
    return "success";
  }
  if (WARN.has(value)) {
    return "warn";
  }
  return "neutral";
}

export function statusLabelKey(status?: string): string | undefined {
  const value = (status || "").trim().toLowerCase();
  return LABEL_KEY[value];
}

export function ownerTypeLabelKey(owner?: string): string | undefined {
  if (owner === "platform") {
    return "ownerPlatform";
  }
  if (owner === "channel") {
    return "ownerChannel";
  }
  return undefined;
}
