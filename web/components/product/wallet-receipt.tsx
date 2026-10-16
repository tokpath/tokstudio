"use client";
import {useState,type FormEvent} from "react";
import {CommandResultSchema} from "@/lib/contracts";
import {Input} from "@/components/ui/input";
import {Button} from "@/components/ui/button";
import {ErrorPanel,Notice,useCommand} from "./shared";

export function WalletReceipt({path}:{path:string}){
 const [hash,setHash]=useState(""),[index,setIndex]=useState("0x0");const command=useCommand(path,CommandResultSchema);
 const safe=path.startsWith("/api/v1/me/orders/")||path.startsWith("/api/v1/operators/");
 async function submit(event:FormEvent){event.preventDefault();if(safe)await command.run({transaction_hash:hash,log_index:index})}
 return <form onSubmit={submit} className="mt-5 space-y-4 border-t border-hairline pt-5"><Notice>已转账后可提交查找线索。系统只读核实准确网络、合约、收款地址、Transfer事件和确认数；交易哈希不是付款成功凭据，足额核实后仍须等待原订单交付。</Notice><div className="grid gap-4 sm:grid-cols-2"><label className="text-sm">实际转账交易哈希<Input className="mt-2" required pattern="0x[0-9a-fA-F]{64}" value={hash} onChange={e=>setHash(e.target.value.trim())} disabled={command.busy||command.unresolved}/></label><label className="text-sm">ERC20 Transfer日志索引<Input className="mt-2" required pattern="0x[0-9a-fA-F]+" value={index} onChange={e=>setIndex(e.target.value.trim())} disabled={command.busy||command.unresolved}/></label></div>{!!command.error&&<ErrorPanel error={command.error}/>}<Button type="submit" variant="outline" disabled={!safe||command.busy}>{command.busy?"正在只读核实链上事实…":command.unresolved?"重试原查找线索":"提交链上查找线索"}</Button>{command.result&&<p className="text-sm">本次链上事实已核实并保留，请刷新原订单核对收款和交付。状态：{command.result.status}</p>}</form>
}
