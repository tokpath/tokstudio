"use client";


export function AdminShell({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex w-full flex-col gap-8">
      {children}
    </div>
  );
}
