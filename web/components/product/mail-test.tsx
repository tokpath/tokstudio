"use client";
import { useI18n } from "@/components/i18n";
import { Button } from "@/components/ui/button";
import { CommandResultSchema } from "@/lib/contracts";
import { ErrorPanel, useCommand } from "./shared";

export function MailTestButton({path}:{path:string}) {
 const {t}=useI18n();
 const command=useCommand(path,CommandResultSchema);
 return <div className="space-y-2"><Button type="button" variant="outline" disabled={command.busy} onClick={()=>{if(!command.unresolved)command.reset();void command.run({});}}>{t(command.busy?"正在发送测试邮件…":command.unresolved?"重试本次测试":"发送测试邮件")}</Button>
 {!!command.error&&<ErrorPanel error={command.error} compact/>}
 {command.result&&<p role="status" className="max-w-lg text-sm text-ink-secondary">{t(command.result.message||"邮件服务器已接受测试邮件，请到当前账号邮箱查收（也请检查垃圾邮件）。")}</p>}</div>;
}
