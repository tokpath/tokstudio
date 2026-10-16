import { Suspense } from "react";
import { StaffInvitation } from "@/components/product/staff-invitation";
export default function Page(){return <Suspense fallback={<p className="p-8">正在核实邀请…</p>}><StaffInvitation/></Suspense>}
