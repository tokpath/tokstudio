import { RequestDetail } from "@/components/request-detail";
export default async function Page({params}:{params:Promise<{id:string}>}){const{id}=await params;return <RequestDetail surface="user" id={id}/> }
