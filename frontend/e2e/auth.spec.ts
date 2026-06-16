import { test, expect } from "@playwright/test";

// Backend base URL (userservice). FE gọi cùng giá trị này qua NEXT_PUBLIC_API_BASE_URL.
const API = process.env.E2E_API_BASE_URL ?? "http://localhost:8080";
const PASSWORD = "password123";

test("trang login render đầy đủ", async ({ page }) => {
  await page.goto("/login");
  // CardTitle render <div> (shadcn), không phải heading role → khớp bằng text.
  await expect(page.getByText("Đăng nhập LogMon")).toBeVisible();
  await expect(page.getByLabel("Email")).toBeVisible();
  await expect(page.getByLabel("Mật khẩu")).toBeVisible();
  await expect(page.getByRole("button", { name: "Đăng nhập" })).toBeVisible();
});

test("vào dashboard khi chưa đăng nhập → redirect /login", async ({ page }) => {
  await page.goto("/");
  await expect(page).toHaveURL(/\/login$/);
});

test("sai mật khẩu → hiện lỗi, không vào được dashboard", async ({
  page,
  request,
}) => {
  const email = `e2e-bad-${Date.now()}@example.com`;
  const created = await request.post(`${API}/api/v1/users`, {
    data: { email, password: PASSWORD },
  });
  expect(created.ok()).toBeTruthy();

  await page.goto("/login");
  await page.getByLabel("Email").fill(email);
  await page.getByLabel("Mật khẩu").fill("wrong-password-x");
  await page.getByRole("button", { name: "Đăng nhập" }).click();

  await expect(page.getByText(/Lỗi:/)).toBeVisible();
  await expect(page).toHaveURL(/\/login$/);
});

test("đăng ký + login → dashboard → hồ sơ hiển thị email", async ({
  page,
  request,
}) => {
  const email = `e2e-${Date.now()}@example.com`;
  const created = await request.post(`${API}/api/v1/users`, {
    data: { email, password: PASSWORD },
  });
  expect(created.ok()).toBeTruthy();

  await page.goto("/login");
  await page.getByLabel("Email").fill(email);
  await page.getByLabel("Mật khẩu").fill(PASSWORD);
  await page.getByRole("button", { name: "Đăng nhập" }).click();

  // Vào trang Tổng quan sau khi đăng nhập.
  await expect(page).toHaveURL("http://localhost:3000/");
  await expect(page.getByRole("heading", { name: "Tổng quan" })).toBeVisible();

  // Điều hướng sang Hồ sơ và thấy email của mình.
  await page.getByRole("link", { name: "Hồ sơ" }).click();
  await expect(page).toHaveURL(/\/profile$/);
  await expect(page.getByText(email)).toBeVisible();
});

test("đăng xuất → xoá phiên, quay lại /login", async ({ page, request }) => {
  const email = `e2e-logout-${Date.now()}@example.com`;
  const created = await request.post(`${API}/api/v1/users`, {
    data: { email, password: PASSWORD },
  });
  expect(created.ok()).toBeTruthy();

  await page.goto("/login");
  await page.getByLabel("Email").fill(email);
  await page.getByLabel("Mật khẩu").fill(PASSWORD);
  await page.getByRole("button", { name: "Đăng nhập" }).click();
  await expect(page.getByRole("heading", { name: "Tổng quan" })).toBeVisible();

  await page.getByRole("button", { name: "Đăng xuất" }).click();
  await expect(page).toHaveURL(/\/login$/);

  // Phiên đã bị xoá → vào lại dashboard vẫn bị chặn về /login.
  await page.goto("/");
  await expect(page).toHaveURL(/\/login$/);
});
