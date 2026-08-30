export default function AdminConsole() {
  return (
    <main className="mx-auto flex max-w-5xl flex-col gap-4 px-6 py-12">
      <p className="text-sm uppercase tracking-[0.2em] text-slate-400">平台管理控制台</p>
      <h1 className="text-3xl font-semibold">全局运营</h1>
      <p className="text-slate-300">
        平台管理员可以看全部渠道、改归因（必须写原因并进审计），但不能让普通用户自己改归属。
      </p>
    </main>
  );
}
