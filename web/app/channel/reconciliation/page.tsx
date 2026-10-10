"use client";
import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { PendingUsageQueue } from "@/components/pending-usage-queue";
import { useViewer } from "@/components/rbac/viewer-context";
export default function ChannelReconciliationPage(){const viewer=useViewer(),router=useRouter();useEffect(()=>{if(!viewer.loading&&viewer.channelType!=="C")router.replace("/channel/usage?tab=requests")},[viewer.loading,viewer.channelType,router]);return viewer.channelType==="C"?<PendingUsageQueue surface="channel"/>:null}
