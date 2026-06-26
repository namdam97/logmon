import { test, expect } from "@playwright/test";

// Backend base URL (userservice). FE gọi cùng giá trị qua NEXT_PUBLIC_API_BASE_URL.
const API = process.env.E2E_API_BASE_URL ?? "http://localhost:8080";
const PASSWORD = "password123";

// Đăng ký + đăng nhập một user mới, trả về sau khi đã ở trang Tổng quan.
async function loginFresh(page: import("@playwright/test").Page, request: import("@playwright/test").APIRequestContext) {
  const email = `e2e-alerts-${Date.now()}@example.com`;
  const created = await request.post(`${API}/api/v1/users`, {
    data: { email, password: PASSWORD },
  });
  expect(created.ok()).toBeTruthy();

  await page.goto("/login");
  await page.getByLabel("Email").fill(email);
  await page.getByLabel("Mật khẩu").fill(PASSWORD);
  await page.getByRole("button", { name: "Đăng nhập" }).click();
  await expect(page.getByRole("heading", { name: "Tổng quan" })).toBeVisible();
}

test("điều hướng tới trang Cảnh báo và render 2 bảng", async ({
  page,
  request,
}) => {
  await loginFresh(page, request);

  await page.getByRole("link", { name: "Cảnh báo" }).click();
  await expect(page).toHaveURL(/\/alerts$/);
  await expect(page.getByRole("heading", { name: "Cảnh báo" })).toBeVisible();

  // Hai khối card luôn hiển thị (kể cả khi rỗng).
  await expect(page.getByText("Đang kích hoạt")).toBeVisible();
  await expect(page.getByText("Rule cảnh báo")).toBeVisible();
});
