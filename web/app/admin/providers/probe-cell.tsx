"use client";

import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { IfCan } from "@/components/rbac/if-can";
import { apiBase } from "@/lib/api";

export function ProbeCell({ id }: { id: string }) {
  const queryClient = useQueryClient();
  const [result, setResult] = useState("");
  return (
    <div className="flex flex-wrap items-center gap-2">
      <IfCan action="providers.health">
        <Button
          size="sm"
          variant="outline"
          onClick={async (event) => {
            event.stopPropagation();
            const res = await fetch(`${apiBase}/admin/providers/${id}/health-check`, {
              method: "POST",
              credentials: "include",
              headers: { "Content-Type": "application/json" },
              body: "{}",
            });
            const body = await res.json();
            setResult(res.ok ? String(body.health ?? "ok") : body.error?.message || "探测失败");
            await queryClient.invalidateQueries();
          }}
        >
          探测
        </Button>
      </IfCan>
      {result ? <span className="text-xs text-ink-secondary">{result}</span> : null}
    </div>
  );
}
