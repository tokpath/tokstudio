import { headers } from "next/headers";
import { ConsolePageHeader } from "@/components/console/page-header";
import { loadCatalog } from "@/lib/catalog";
import { PlaygroundClient } from "./playground-client";

export default async function PlaygroundPage() {
  const host = (await headers()).get("x-tokenhub-host") || "localhost";
  const models = await loadCatalog(host);
  const textModels = models.filter((m) => !m.kind || m.kind === "text").slice(0, 48);

  return (
    <div className="flex flex-col gap-6">
      <ConsolePageHeader
        eyebrow="PLAYGROUND"
        title="快速试用"
        description="选一个模型，发一条消息。走同一套网关和预授权：失败看回单，不另开演示通道。"
      />
      <PlaygroundClient models={textModels} />
    </div>
  );
}
