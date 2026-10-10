"use client";
import { PolicyEditor } from "@/app/admin/commission/policy-editor";
import { useViewer } from "@/components/rbac/viewer-context";
import { canChannelAction } from "@/lib/rbac";
export default function ChannelRules() {
  const viewer = useViewer();
  return <PolicyEditor prefix="/channel" canEdit={viewer.channelType === 'C' && canChannelAction('finance', viewer)} />;
}
