# Instructions

- Following Playwright test failed.
- Explain why, be concise, respect Playwright best practices.
- Provide a snippet of code with the fix, if possible.

# Test info

- Name: smoke.spec.ts >> unauthenticated access redirects to login
- Location: e2e/smoke.spec.ts:26:1

# Error details

```
Error: browserType.launch: Executable doesn't exist at /ms-playwright/chromium_headless_shell-1228/chrome-linux/headless_shell
╔════════════════════════════════════════════════════════╗
║ Looks like Playwright was just updated to 1.61.1.      ║
║ Please update docker image as well.                    ║
║ -  current: mcr.microsoft.com/playwright:v1.48.2-jammy ║
║ - required: mcr.microsoft.com/playwright:v1.61.1-jammy ║
║                                                        ║
║ <3 Playwright Team                                     ║
╚════════════════════════════════════════════════════════╝
```