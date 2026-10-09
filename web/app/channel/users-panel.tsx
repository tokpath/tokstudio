"use client";
import { Suspense } from "react";
import { CustomerList } from "@/components/customer-list";
export default function ChannelUsers({channelID,managedChannelID}:{channelID?:string;managedChannelID?:string}){return <Suspense fallback={<p>正在读取客户…</p>}><CustomerList surface="channel" channelID={channelID || managedChannelID}/></Suspense>;}
