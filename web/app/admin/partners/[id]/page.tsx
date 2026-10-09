"use client";
import { useParams } from "next/navigation";
import { AdminShell } from "../../shell";
import { ProfessionalCustomerDetail } from "@/components/professional-customer-detail";
export default function Page(){const {id}=useParams<{id:string}>();return <AdminShell><ProfessionalCustomerDetail id={id}/></AdminShell>;}
