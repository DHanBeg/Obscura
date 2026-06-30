import { chromium } from "playwright";

const browser = await chromium.launch();
const ctx = await browser.newContext({ viewport: { width: 390, height: 844 } });
const page = await ctx.newPage();

await page.goto("http://localhost:3000/login", { waitUntil: "networkidle" });
await page.waitForTimeout(1500);
await page.screenshot({ path: "E:/obscura/_screenshots/login_empty.png" });

// Type '90' — Turkey
await page.locator('input[type="tel"]').fill("90");
await page.waitForTimeout(600);
await page.screenshot({ path: "E:/obscura/_screenshots/login_tr.png" });

// Change to '44' — UK
await page.locator('input[type="tel"]').fill("44");
await page.waitForTimeout(600);
await page.screenshot({ path: "E:/obscura/_screenshots/login_uk.png" });

// Change to '1' — USA
await page.locator('input[type="tel"]').fill("1");
await page.waitForTimeout(600);
await page.screenshot({ path: "E:/obscura/_screenshots/login_us.png" });

await browser.close();
console.log("done");
