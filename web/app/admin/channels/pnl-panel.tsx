"use client";
import { useViewer } from '@/components/rbac/viewer-context';
import { BrandPnLPanel } from '@/app/channel/brand-pnl-panel';
export function ChannelPnLPanel({ channelID }: { channelID: string }) {
 const viewer=useViewer();
 const scope=viewer.userId?`${viewer.userId}:${typeof window==='undefined'?'':window.location.host}:${channelID}:admin-pnl`:'';
 return <BrandPnLPanel key={scope} scope={scope} ownerID={channelID} path={`/admin/channels/${encodeURIComponent(channelID)}/pnl`}/>;
}
