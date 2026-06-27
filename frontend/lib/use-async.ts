"use client";

import { useCallback, useEffect, useRef, useState } from "react";

// useAsync chạy một hàm async khi mount + cung cấp reload(), kèm trạng thái
// loading/error. Chặn setState sau unmount. deps thay đổi → fetch lại.
export function useAsync<T>(fn: () => Promise<T>, deps: unknown[] = []) {
  const [data, setData] = useState<T>();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string>();
  const mounted = useRef(true);

  const reload = useCallback(async () => {
    setLoading(true);
    setError(undefined);
    try {
      const d = await fn();
      if (mounted.current) setData(d);
    } catch (e) {
      if (mounted.current) setError(e instanceof Error ? e.message : "unknown error");
    } finally {
      if (mounted.current) setLoading(false);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps);

  useEffect(() => {
    mounted.current = true;
    void reload();
    return () => {
      mounted.current = false;
    };
  }, [reload]);

  return { data, loading, error, reload };
}
