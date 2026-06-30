# Instructions

- Following Playwright test failed.
- Explain why, be concise, respect Playwright best practices.
- Provide a snippet of code with the fix, if possible.

# Test info

- Name: login.spec.ts >> admin login works and renders desk
- Location: login.spec.ts:3:5

# Error details

```
Error: page.goto: net::ERR_INVALID_ARGUMENT at http://localhost:8080/login
Call log:
  - navigating to "http://localhost:8080/login", waiting until "load"

```

# Test source

```ts
  1  | import { test, expect } from '@playwright/test';
  2  | 
  3  | test('admin login works and renders desk', async ({ page }) => {
  4  |   // Set host header so Frappe routes to the correct site
  5  |   await page.setExtraHTTPHeaders({ 'Host': 'upgrade.test.local' });
  6  |   // Navigate to login
> 7  |   await page.goto('/login');
     |              ^ Error: page.goto: net::ERR_INVALID_ARGUMENT at http://localhost:8080/login
  8  | 
  9  |   // Verify the page has loaded CSS correctly by checking if a standard Frappe class or the login button is visible
  10 |   const loginButton = page.locator('button:has-text("Login")');
  11 |   await expect(loginButton).toBeVisible({ timeout: 10000 });
  12 | 
  13 |   // Input credentials
  14 |   await page.fill('#login_email', 'Administrator');
  15 |   
  16 |   const password = process.env.ADMIN_PASSWORD;
  17 |   if (!password) {
  18 |     throw new Error('ADMIN_PASSWORD environment variable is not set!');
  19 |   }
  20 |   await page.fill('#login_password', password);
  21 | 
  22 |   // Click login
  23 |   await loginButton.click();
  24 | 
  25 |   // Wait for redirect to /app (Desk dashboard)
  26 |   await expect(page).toHaveURL(/.*\/app.*/, { timeout: 15000 });
  27 |   
  28 |   // Verify that the desk is actually rendered (a navbar exists)
  29 |   await expect(page.locator('.navbar')).toBeVisible();
  30 | });
  31 | 
```