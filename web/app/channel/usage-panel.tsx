"use client";
import { UsageReport } from "@/components/usage-report";
export default function ChannelUsage({channelID}:{channelID?:string}={}){return <UsageReport surface="channel" channelID={channelID}/>}
