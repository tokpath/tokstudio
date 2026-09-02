"use client";

import { useEffect, useRef } from "react";
import { BarChart3 } from "lucide-react";
import { EmptyState } from "@/components/empty-state";

type EChartsLike = {
  dispose: () => void;
  resize: () => void;
  setOption: (option: object, notMerge?: boolean) => void;
};

/** 控制台共用的 ECharts 容器。无数据时走 empty-state，不画假曲线。 */
export function EChart({
  option,
  emptyTitle,
  emptyDetail,
  testId,
  className = "h-72 w-full",
}: {
  option: object | null;
  emptyTitle: string;
  emptyDetail: string;
  testId?: string;
  className?: string;
}) {
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!option || !ref.current) {
      return;
    }
    let chart: EChartsLike | undefined;
    let cancelled = false;
    const node = ref.current;
    import("echarts").then((echarts) => {
      if (cancelled || !node.isConnected) {
        return;
      }
      chart = echarts.init(node);
      chart.setOption(option, true);
    });
    const onResize = () => chart?.resize();
    window.addEventListener("resize", onResize);
    return () => {
      cancelled = true;
      window.removeEventListener("resize", onResize);
      chart?.dispose();
    };
  }, [option]);

  if (!option) {
    return (
      <div data-testid={testId} className="min-h-48">
        <EmptyState title={emptyTitle} detail={emptyDetail} icon={BarChart3} />
      </div>
    );
  }

  return <div ref={ref} className={className} data-testid={testId} />;
}
