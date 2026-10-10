"use client";
import { useParams } from "next/navigation";
import { AdminShell } from "@/app/admin/shell";
import { PaymentOrderDetail } from "../order-detail";
export default function Page(){const {id}=useParams<{id:string}>();return <AdminShell><PaymentOrderDetail id={id}/></AdminShell>}
