"use client";
import { Suspense,use } from "react";
import { CustomerDetail } from "@/components/customer-detail";
import { AdminShell } from "../../shell";
export default function Page({params}:{params:Promise<{id:string}>}){const {id}=use(params);return <AdminShell><Suspense fallback={<p>正在读取客户…</p>}><CustomerDetail surface="admin" id={id}/></Suspense></AdminShell>;}
