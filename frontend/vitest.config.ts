import { defineConfig, configDefaults } from "vitest/config";

// Vitest chỉ chạy unit/integration test của FE; loại trừ thư mục e2e/ (Playwright).
// Playwright dùng `test` riêng của @playwright/test — sẽ ném lỗi nếu vitest nạp nó.
export default defineConfig({
  test: {
    exclude: [...configDefaults.exclude, "e2e/**"],
  },
});
