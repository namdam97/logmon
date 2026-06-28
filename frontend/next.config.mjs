/** @type {import('next').NextConfig} */

// Đích proxy cho /api (server-side rewrite). Mặc định userservice local.
// Prod đứng sau Nginx reverse proxy nên FE+BE đã same-origin — rewrite này chủ
// yếu cho dev/e2e để trình duyệt thấy FE và API CÙNG origin (localhost:3000):
// điều kiện bắt buộc để CSRF double-submit hoạt động (JS phải đọc được cookie
// lm_csrf, vốn bị chặn nếu BE ở origin khác như :8080).
const API_PROXY_TARGET = process.env.API_PROXY_TARGET ?? "http://localhost:8080";

const nextConfig = {
  reactStrictMode: true,
  // standalone: gói server + deps tối thiểu → image Docker nhỏ, chạy `node server.js`.
  // App Router/RSC cần Node SSR (ADR-036) nên KHÔNG export static.
  output: "standalone",
  async rewrites() {
    return [
      { source: "/api/:path*", destination: `${API_PROXY_TARGET}/api/:path*` },
    ];
  },
};

export default nextConfig;
