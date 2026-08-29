export type Readyz = {
  status: string;
  checks?: Record<string, string>;
  request_id?: string;
};

export function summarizeReady(body: Readyz): string {
  if (body.status === "ready") {
    return "控制面已就绪：数据库、Redis 和迁移都通过了检查。";
  }
  const failed = Object.entries(body.checks ?? {})
    .filter(([, value]) => value !== "ok")
    .map(([key]) => key);
  return failed.length
    ? `还未就绪，失败项：${failed.join("、")}`
    : "服务还在启动，请稍后再看。";
}
