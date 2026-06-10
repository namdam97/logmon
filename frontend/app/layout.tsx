import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "LogMon",
  description: "LogMon observability admin",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="vi">
      <body className="min-h-screen antialiased">{children}</body>
    </html>
  );
}
