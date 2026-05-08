import { test, expect } from '@playwright/test';

test('admin login works and renders desk', async ({ page }) => {
  // Navigate to login
  await page.goto('/login');

  // Verify the page has loaded CSS correctly by checking if the primary login button is visible
  const loginButton = page.locator('.btn-login');
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

  // Wait for redirect to /app
  await expect(page).toHaveURL(/.*\/app.*/, { timeout: 15000 });
  
  // Verify that either the desk navbar OR the setup wizard is rendered
  // (Freshly created sites will show the Setup Wizard instead of the Desk)
  const navbar = page.locator('.navbar');
  const setupWizard = page.locator('.setup-wizard-slide, .setup-wizard-wrapper, h1:has-text("Welcome")');
  
  await expect(navbar.or(setupWizard).first()).toBeVisible({ timeout: 15000 });
});
