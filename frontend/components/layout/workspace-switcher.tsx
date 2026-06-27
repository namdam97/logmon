"use client";

import { Check, ChevronsUpDown } from "lucide-react";

import { useWorkspace } from "@/components/workspace-provider";

// WorkspaceSwitcher cho phép đổi workspace đang thao tác. Dùng <select> gốc cho
// đơn giản + khả dụng (a11y) — đổi sẽ reload để dữ liệu fetch lại theo tenant.
export function WorkspaceSwitcher() {
  const { workspaces, current, loading, select } = useWorkspace();

  if (loading) {
    return <span className="text-sm text-muted-foreground">Đang tải workspace…</span>;
  }
  if (workspaces.length === 0) {
    return <span className="text-sm text-muted-foreground">Chưa có workspace</span>;
  }

  return (
    <div className="relative flex items-center gap-2 rounded-md border px-2 py-1">
      <ChevronsUpDown className="h-3.5 w-3.5 text-muted-foreground" aria-hidden />
      <label className="sr-only" htmlFor="workspace-switcher">
        Workspace
      </label>
      <select
        id="workspace-switcher"
        className="bg-transparent text-sm outline-none"
        value={current?.id ?? ""}
        onChange={(e) => select(e.target.value)}
      >
        {workspaces.map((w) => (
          <option key={w.id} value={w.id}>
            {w.name}
          </option>
        ))}
      </select>
      {current ? <Check className="h-3.5 w-3.5 text-muted-foreground" aria-hidden /> : null}
    </div>
  );
}
