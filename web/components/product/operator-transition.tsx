"use client";
import Link from "next/link";
import { useI18n } from "@/components/i18n";
import { useProduct } from "./context";
import { canReadResource } from "@/lib/navigation";
import { Notice, When } from "./shared";

export function OperatorTransitionNotice({operatorID}:{operatorID:string}) {
 const {t}=useI18n();const {session}=useProduct();
 const workspace=session?.kind==="staff"?session.workspaces.find(w=>w.operator.id===operatorID):undefined;
 const state=workspace?.transition;if(!state)return null;
 const completed=state.state==="completed";
 return <div className="mb-6" role="status"><Notice><p className="font-semibold">{t(completed?"已由{brand}接管，渠道经营已结束":"正在由{brand}接管，新增经营已暂停",{brand:state.brand_name})}</p>
 {completed&&state.completed_at&&<p className="mt-2">{t("接管完成时间")}：<When value={state.completed_at}/></p>}
 <p className="mt-2">{t(completed?"客户服务由承接品牌办理。历史记录、账本和报表仍可查询；历史票据及未结资金事项按原责任继续办理。":"请继续处理已受理的存量事项。新增套餐、采购、营销和恢复营业已暂停，接管进度由所属品牌办理。")}</p>
 {canReadResource(workspace,"takeovers")&&<Link className="mt-2 inline-block underline" href={`/manage/${encodeURIComponent(operatorID)}/takeovers`}>{t("查看停业与接管")}</Link>}
 </Notice></div>;
}
