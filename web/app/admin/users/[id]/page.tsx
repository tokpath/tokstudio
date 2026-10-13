"use client";
import { useTranslations } from "next-intl";
import { Suspense,use } from "react";
import { CustomerDetail } from "@/components/customer-detail";
import { AdminShell } from "../../shell";
export default function Page({params}:{params:Promise<{id:string}>}){
  const tUI = useTranslations();const {id}=use(params);return <AdminShell><Suspense fallback={<p>{tUI("paymentTasks.loadingCustomer")}</p>}><CustomerDetail surface="admin" id={id}/></Suspense></AdminShell>;}
