import { test, expect } from '@playwright/test';

test('admin login works and renders desk', async ({ page }) => {
  // Navigate to login
  await page.goto('/login');

  // Verify the page has loaded CSS correctly by checking if a standard Frappe class or the login button is visible
  const loginButton = page.locator('button:has-text("Login")');
  await expect(loginButton).toBeVisible({ timeout: 10000 });

  // Input credentials
  await page.fill('#login_email', 'Administrator');
  
  const password = process.env.ADMIN_PASSWORD;
  if (!password) {
    throw new Error('ADMIN_PASSWORD environment variable is not set!');
  }
  await page.fill('#login_password', password);

  // Click login
  await loginButton.click();

  // Wait for redirect to /app (Desk dashboard)
  await expect(page).toHaveURL(/.*\/app.*/, { timeout: 15000 });
  
  // Verify that the desk is actually rendered (a navbar exists)
  await expect(page.locator('.navbar')).toBeVisible();
});
