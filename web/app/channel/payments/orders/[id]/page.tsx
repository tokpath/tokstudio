"use client";
import { useParams } from "next/navigation";
import { PaymentOrderDetail } from "@/app/admin/payments/order-detail";
export default function Page(){const {id}=useParams<{id:string}>();return <PaymentOrderDetail id={id} oem/>}
