import {redirect} from "next/navigation";
export default async function ModelPage({params,searchParams}:{params:Promise<{id:string[]}>;searchParams:Promise<{key_id?:string;tab?:string}>}) {const {id}=await params;const {key_id,tab}=await searchParams;const query=new URLSearchParams({model:id.join("/"),tab:tab??"overview"});if(key_id)query.set("key_id",key_id);redirect(`/app/docs?${query}`);}
