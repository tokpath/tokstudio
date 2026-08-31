import { PartnerBoard } from "./partner-board";

export default function PartnerConsole() {
  return (
    <div className="flex flex-col gap-6">
      <header>
        <p className="th-eyebrow text-ink-mute">PARTNER</p>
        <h1 className="mt-2 text-2xl font-semibold tracking-tight">我的推广范围</h1>
        <p className="mt-2 max-w-2xl text-ink-secondary">
          这里按代理商 / 1 级 KOL / 2 级 KOL 分层。后端再校验角色树，前端隐藏不是安全边界。看不到 prompt，也不能改佣金比例。
        </p>
      </header>
      <PartnerBoard section="all" />
    </div>
  );
}
