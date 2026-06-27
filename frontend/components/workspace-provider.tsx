"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useState,
} from "react";

import {
  type Workspace,
  getWorkspaceID,
  listWorkspaces,
  setWorkspaceID,
} from "@/lib/api";

interface WorkspaceContextValue {
  workspaces: Workspace[];
  current?: Workspace;
  loading: boolean;
  error?: string;
  select: (id: string) => void;
}

const WorkspaceContext = createContext<WorkspaceContextValue | undefined>(undefined);

// WorkspaceProvider nạp danh sách workspace của user, chọn mặc định (đã lưu hoặc
// đầu tiên) và đồng bộ lựa chọn xuống localStorage để api client gắn X-Workspace-ID.
export function WorkspaceProvider({ children }: { children: React.ReactNode }) {
  const [workspaces, setWorkspaces] = useState<Workspace[]>([]);
  const [currentId, setCurrentId] = useState<string | undefined>(getWorkspaceID());
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string>();

  useEffect(() => {
    let alive = true;
    listWorkspaces()
      .then((list) => {
        if (!alive) return;
        setWorkspaces(list);
        const stored = getWorkspaceID();
        const valid = list.find((w) => w.id === stored);
        const chosen = valid?.id ?? list[0]?.id;
        if (chosen) {
          setWorkspaceID(chosen);
          setCurrentId(chosen);
        }
      })
      .catch((e: unknown) => alive && setError(e instanceof Error ? e.message : "lỗi tải workspace"))
      .finally(() => alive && setLoading(false));
    return () => {
      alive = false;
    };
  }, []);

  const select = useCallback((id: string) => {
    setWorkspaceID(id);
    setCurrentId(id);
    // Reload để mọi trang fetch lại dữ liệu theo workspace mới (đơn giản, chắc chắn).
    if (typeof window !== "undefined") window.location.reload();
  }, []);

  const current = workspaces.find((w) => w.id === currentId);

  return (
    <WorkspaceContext.Provider value={{ workspaces, current, loading, error, select }}>
      {children}
    </WorkspaceContext.Provider>
  );
}

// useWorkspace truy cập context workspace; ném lỗi nếu dùng ngoài provider.
export function useWorkspace(): WorkspaceContextValue {
  const ctx = useContext(WorkspaceContext);
  if (!ctx) throw new Error("useWorkspace must be used within WorkspaceProvider");
  return ctx;
}
