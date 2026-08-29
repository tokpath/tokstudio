import ChannelCommissions from "./commissions";

export default function ChannelConsole() {
  return (
    <main className="mx-auto flex max-w-5xl flex-col gap-4 px-6 py-12">
      <p className="text-sm uppercase tracking-[0.2em] text-slate-400">渠道控制台</p>
      <h1 className="text-3xl font-semibold">本渠道数据</h1>
      <p className="text-slate-300">
        渠道管理员只能看到自己渠道的用户和推广数据。后端会再校验 scope，前端隐藏菜单不是安全边界。
      </p>
      <ChannelCommissions />
    </main>
  );
}
