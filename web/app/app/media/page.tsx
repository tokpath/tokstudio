import { ConsolePageHeader } from "@/components/console/page-header";
import MediaPanel from "../media-panel";

export default function MediaPage() {
  return (
    <div className="flex flex-col gap-6">
      <ConsolePageHeader
        eyebrow="MEDIA"
        title="媒体任务"
        description="视频 / 图像任务走同一本账：先 HOLD，完成后再结算。参数与网关字段对齐。"
      />
      <MediaPanel />
    </div>
  );
}
