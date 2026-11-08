"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useI18n } from "@/components/i18n";
import { Button } from "@/components/ui/button";
import { request } from "@/lib/api-client";
import { CommandResultSchema, ResourcePageSchema, type Order } from "@/lib/contracts";
import { PaymentAction } from "./customer-plans";
import { ErrorPanel, Notice, Status, useCommand } from "./shared";

// The purchase and its checkout are durable before contacting the provider.
// Read that exact purchase after an ambiguous response; never create another
// checkout merely to discover whether the original payment finished.
export function ProcurementPayment({ path, orderID, initialOrder, error, onClose }: {
  path: string;
  orderID: string;
  initialOrder?: Order;
  error?: unknown;
  onClose?: () => void;
}) {
  const { t } = useI18n();
  const cache = useQueryClient();
  const base = path.replace(/\/[^/]+\/(checkout|query_payment)$/, "");
  const original = useQuery({
    queryKey: ["procurement-payment", base, orderID],
    queryFn: async ({ signal }) => {
      const page = await request(`${base}?view=buyer&section=orders&record=${encodeURIComponent(orderID)}`, ResourcePageSchema, { signal });
      const order = page.items.find(item => item.id === orderID);
      if (!order) throw new Error("记录不存在或不属于当前范围");
      return order;
    },
    retry: false,
    refetchInterval: query => ["completed", "closed", "exception"].includes(query.state.data?.status || "") ? false : 5000,
  });
  const reconcile = useCommand(`${base}/${encodeURIComponent(orderID)}/query_payment`, CommandResultSchema);
  const status = original.data?.status;
  const completed = status === "completed";
  const paid = status === "paid";
  const ended = completed || status === "closed" || status === "exception";
  function close() {
    // Refresh the list beneath the panel too, including after a lost response.
    void cache.invalidateQueries({ predicate: query => query.queryKey[0] !== "procurement-payment" });
    onClose?.();
  }
  return <div className="space-y-5">
    <div role="status" className="rounded-control border border-hairline p-5">
      <h3 className="font-semibold">{t("原采购订单状态")}</h3>
      <p className="mt-2 break-all font-mono text-xs text-ink-mute">{orderID}</p>
      <div className="mt-3">{status ? <Status value={status} /> : <p className="text-sm">{t("正在读取原订单…")}</p>}</div>
      <p className="mt-3 text-sm leading-7 text-ink-secondary">{t(completed ? "采购付款与库存交付已完成，无须再次付款。" : paid ? "已足额核实收款，正在处理库存交付，无须再次付款。" : ended ? "请返回采购订单查看最新记录及后续办理事项。" : "系统会自动更新原订单状态。可以关闭面板或刷新页面，稍后从原订单继续核实；请勿另付一笔。")}</p>
    </div>
    {!!error && !paid && !ended && <ErrorPanel error={error} />}
    {original.isError && <ErrorPanel error={original.error} retry={() => void original.refetch()} />}
    {/* A current server read must still permit payment before showing a saved form. */}
    {status === "awaiting_payment" && original.data?.allowed_actions?.includes("resume_payment") && initialOrder?.payment_action && <PaymentAction order={initialOrder} />}
    {!ended && <Notice>{t("查询只核实原付款，不会发起新的扣款。")}</Notice>}
    {!!reconcile.error && !paid && !ended && <ErrorPanel error={reconcile.error} />}
    <div className="flex flex-wrap gap-3">
      {!ended && <Button type="button" variant="outline" disabled={reconcile.busy} onClick={async () => { await reconcile.run({}); await original.refetch(); }}>{reconcile.busy ? t("正在核实原交易…") : t("查询原付款与交付")}</Button>}
      {onClose && <Button type="button" onClick={close}>{t("返回采购订单")}</Button>}
    </div>
  </div>;
}
