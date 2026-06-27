"use client";

import { sloCompliance, type SLOCompliance } from "@/lib/api";
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

// budgetVariant: ngân sách lỗi thấp → đỏ, vừa → vàng, cao → xanh (mặc định).
function budgetVariant(pct: number): "destructive" | "default" | "secondary" {
  if (pct <= 10) return "destructive";
  if (pct <= 30) return "default";
  return "secondary";
}

function pct(n: number): string {
  return `${n.toFixed(2)}%`;
}

export default function SLOPage() {
  const { data, loading, error, reload } = useAsync<SLOCompliance[]>(sloCompliance);
  const rows = data ?? [];

  return (
    <div className="space-y-6">
      <div className="flex items-start justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">SLO</h1>
          <p className="text-sm text-muted-foreground">
            Tuân thủ mục tiêu dịch vụ và ngân sách lỗi của workspace.
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

      <Card>
        <CardHeader>
          <CardTitle>Tuân thủ SLO</CardTitle>
          <CardDescription>
            Error budget + burn rate (GET /api/v1/slos/compliance).
          </CardDescription>
        </CardHeader>
        <CardContent>
          {loading ? (
            <p className="text-sm text-muted-foreground">Đang tải...</p>
          ) : rows.length === 0 ? (
            <p className="text-sm text-muted-foreground">Chưa có SLO nào.</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Tên</TableHead>
                  <TableHead>Service</TableHead>
                  <TableHead className="text-right">Mục tiêu</TableHead>
                  <TableHead className="text-right">SLI hiện tại</TableHead>
                  <TableHead className="text-right">Ngân sách còn</TableHead>
                  <TableHead className="text-right">Burn rate</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {rows.map((s) => (
                  <TableRow key={s.id}>
                    <TableCell className="font-medium">{s.name}</TableCell>
                    <TableCell className="text-muted-foreground">{s.service}</TableCell>
                    <TableCell className="text-right">{pct(s.objective)}</TableCell>
                    <TableCell className="text-right">{pct(s.currentSLI)}</TableCell>
                    <TableCell className="text-right">
                      <Badge variant={budgetVariant(s.errorBudgetRemaining)}>
                        {pct(s.errorBudgetRemaining)}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-right">
                      <Badge variant={s.burnRate > 1 ? "destructive" : "secondary"}>
                        {s.burnRate.toFixed(2)}×
                      </Badge>
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
