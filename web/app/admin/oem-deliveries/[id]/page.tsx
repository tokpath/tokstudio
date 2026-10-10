"use client";
import { useParams } from "next/navigation";
import { AdminShell } from "@/app/admin/shell";
import { DeliveryPanel } from "../delivery-panel";
export default function Page(){const {id}=useParams<{id:string}>();return <AdminShell><DeliveryPanel key={id} channelID={id}/></AdminShell>}
