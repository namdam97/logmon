"use client";

import { useState } from "react";

import { listChannels, testChannel, type Channel } from "@/lib/api";
import { useAsync } from "@/lib/use-async";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

export default function ChannelsPage() {
  const { data, loading, error, reload } = useAsync<Channel[]>(listChannels);
  const [busy, setBusy] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const channels = data ?? [];

  async function onTest(id: string) {
    setBusy(id);
    setNotice(null);
    try {
      await testChannel(id);
      setNotice("Đã gửi tin nhắn thử.");
    } catch (e) {
      setNotice(e instanceof Error ? e.message : "unknown error");
    } finally {
      setBusy(null);
    }
  }

  return (
    <div className="space-y-6">
      <div className="flex items-start justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Kênh báo</h1>
          <p className="text-sm text-muted-foreground">
            Kênh thông báo (Slack/Email/PagerDuty/…) của workspace.
          </p>
        </div>
        <Button variant="outline" size="sm" onClick={() => void reload()}>
          Làm mới
        </Button>
      </div>

      {error && (
        <p className="text-sm text-destructive" role="alert">
          Lỗi: {error}
        </p>
      )}
      {notice && (
        <p className="text-sm text-muted-foreground" role="status">
          {notice}
        </p>
      )}

      <Card>
        <CardHeader>
          <CardTitle>Kênh thông báo</CardTitle>
          <CardDescription>GET /api/v1/notifications/channels</CardDescription>
        </CardHeader>
        <CardContent>
          {loading ? (
            <p className="text-sm text-muted-foreground">Đang tải...</p>
          ) : channels.length === 0 ? (
            <p className="text-sm text-muted-foreground">Chưa có kênh nào.</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Tên</TableHead>
                  <TableHead>Loại</TableHead>
                  <TableHead>Trạng thái</TableHead>
                  <TableHead className="text-right">Hành động</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {channels.map((c) => (
                  <TableRow key={c.id}>
                    <TableCell className="font-medium">{c.name}</TableCell>
                    <TableCell>
                      <Badge variant="secondary">{c.type}</Badge>
                    </TableCell>
                    <TableCell>
                      <Badge variant={c.enabled ? "default" : "outline"}>
                        {c.enabled ? "Bật" : "Tắt"}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-right">
                      <Button
                        variant="secondary"
                        size="sm"
                        disabled={busy === c.id}
                        aria-label={`Gửi thử ${c.name}`}
                        onClick={() => void onTest(c.id)}
                      >
                        {busy === c.id ? "..." : "Gửi thử"}
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
