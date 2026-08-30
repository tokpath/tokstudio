/** 即使页面是纸，代码也落在碳面上，像一张回单。 */
export function CodeBlock({ children }: { children: string }) {
  return (
    <pre className="overflow-x-auto rounded-control bg-code px-4 py-4 font-mono text-[13px] leading-relaxed text-code-ink">
      {children}
    </pre>
  );
}
