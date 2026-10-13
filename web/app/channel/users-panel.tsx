"use client";
import { useTranslations } from "next-intl";
import { Suspense } from "react";
import { CustomerList } from "@/components/customer-list";
export default function ChannelUsers({channelID,managedChannelID}:{channelID?:string;managedChannelID?:string}){
  const tUI = useTranslations();return <Suspense fallback={<p>{tUI("paymentTasks.loadingCustomer")}</p>}><CustomerList surface="channel" channelID={channelID || managedChannelID}/></Suspense>;}
