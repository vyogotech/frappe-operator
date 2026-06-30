# Instructions

- Following Playwright test failed.
- Explain why, be concise, respect Playwright best practices.
- Provide a snippet of code with the fix, if possible.

# Test info

- Name: login.spec.ts >> admin login works and renders desk
- Location: login.spec.ts:3:5

# Error details

```
Error: expect(locator).toBeVisible() failed

Locator: locator('button:has-text("Login")')
Expected: visible
Error: strict mode violation: locator('button:has-text("Login")') resolved to 2 elements:
    1) <button type="submit" class="btn btn-sm btn-primary btn-block btn-login">↵⇆⇆Login</button> aka getByRole('button', { name: 'Login' })
    2) <button type="submit" class="btn btn-sm btn-primary btn-block btn-login-with-email-link">Send login link</button> aka getByText('Send login link')

Call log:
  - Expect "toBeVisible" with timeout 10000ms
  - waiting for locator('button:has-text("Login")')

```

# Page snapshot

```yaml
- generic [ref=e1]:
  - navigation [ref=e2]:
    - generic [ref=e3]:
      - link "Home" [ref=e4] [cursor=pointer]:
        - /url: /
      - generic:
        - list
  - main [ref=e7]:
    - generic [ref=e10]:
      - generic [ref=e11]:
        - img [ref=e12]
        - heading "Login to Frappe" [level=4] [ref=e13]
      - form [ref=e15]:
        - generic [ref=e16]:
          - generic [ref=e17]:
            - generic [ref=e18]:
              - generic [ref=e19]: Email
              - generic [ref=e20]:
                - textbox "Email" [active] [ref=e21]:
                  - /placeholder: jane@example.com
                - img [ref=e22]
            - generic [ref=e24]:
              - generic [ref=e25]: Password
              - generic [ref=e26]:
                - textbox "Password" [ref=e27]:
                  - /placeholder: •••••
                - img [ref=e28]
                - generic [ref=e30] [cursor=pointer]: Show
            - paragraph [ref=e31]:
              - link "Forgot Password?" [ref=e32] [cursor=pointer]:
                - /url: "#forgot"
          - button "Login" [ref=e34] [cursor=pointer]
          - generic [ref=e35]:
            - paragraph [ref=e36]: or
            - link "Login with Email Link" [ref=e39] [cursor=pointer]:
              - /url: "#login-with-email-link"
```

# Test source

```ts
  1  | import { test, expect } from '@playwright/test';
  2  | 
  3  | test('admin login works and renders desk', async ({ page }) => {
  4  |   // Navigate to login
  5  |   await page.goto('/login');
  6  | 
  7  |   // Verify the page has loaded CSS correctly by checking if a standard Frappe class or the login button is visible
  8  |   const loginButton = page.locator('button:has-text("Login")');
> 9  |   await expect(loginButton).toBeVisible({ timeout: 10000 });
     |                             ^ Error: expect(locator).toBeVisible() failed
  10 | 
  11 |   // Input credentials
  12 |   await page.fill('#login_email', 'Administrator');
  13 |   
  14 |   const password = process.env.ADMIN_PASSWORD;
  15 |   if (!password) {
  16 |     throw new Error('ADMIN_PASSWORD environment variable is not set!');
  17 |   }
  18 |   await page.fill('#login_password', password);
  19 | 
  20 |   // Click login
  21 |   await loginButton.click();
  22 | 
  23 |   // Wait for redirect to /app (Desk dashboard)
  24 |   await expect(page).toHaveURL(/.*\/app.*/, { timeout: 15000 });
  25 |   
  26 |   // Verify that the desk is actually rendered (a navbar exists)
  27 |   await expect(page.locator('.navbar')).toBeVisible();
  28 | });
  29 | 
```