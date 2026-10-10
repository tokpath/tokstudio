"use client";
import { Suspense,use } from "react";
import { CustomerDetail } from "@/components/customer-detail";
export default function Page({params}:{params:Promise<{id:string}>}){const {id}=use(params);return <Suspense fallback={<p>正在读取客户…</p>}><CustomerDetail surface="channel" id={id}/></Suspense>;}
