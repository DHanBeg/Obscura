import { chromium } from "playwright";
import { mkdirSync } from "fs";

const OUT = "E:/obscura/_screenshots";
mkdirSync(OUT, { recursive: true });

const VIEWPORT = { width: 390, height: 844 };

const MOCK_MESSAGES = [
  {
    id: "msg-1", conv_id: "conv-1",
    from_did: "did:obs:b4e39fa5d2c8g3f1b7e4d0c6f9a2e5b8",
    to_did:   "did:obs:a3f28e9d4c1b7f2e0a6d3c9b5e8f1d4a",
    type: "text", ciphertext: "Merhaba! Bugün nasılsın?",
    status: "read", sent_at: new Date(Date.now() - 600000).toISOString(),
  },
  {
    id: "msg-2", conv_id: "conv-1",
    from_did: "did:obs:a3f28e9d4c1b7f2e0a6d3c9b5e8f1d4a",
    to_did:   "did:obs:b4e39fa5d2c8g3f1b7e4d0c6f9a2e5b8",
    type: "text", ciphertext: "İyiyim, teşekkürler! Proje nasıl gidiyor?",
    status: "read", sent_at: new Date(Date.now() - 540000).toISOString(),
  },
  {
    id: "msg-3", conv_id: "conv-1",
    from_did: "did:obs:b4e39fa5d2c8g3f1b7e4d0c6f9a2e5b8",
    to_did:   "did:obs:a3f28e9d4c1b7f2e0a6d3c9b5e8f1d4a",
    type: "text", ciphertext: "FAZ 2 tamamlandı, şimdi FAZ 3 üzerinde çalışıyoruz. Libp2p entegrasyonu çok iyi gidiyor!",
    status: "read", sent_at: new Date(Date.now() - 480000).toISOString(),
  },
  {
    id: "msg-4", conv_id: "conv-1",
    from_did: "did:obs:a3f28e9d4c1b7f2e0a6d3c9b5e8f1d4a",
    to_did:   "did:obs:b4e39fa5d2c8g3f1b7e4d0c6f9a2e5b8",
    type: "text", ciphertext: "Harika! ZK devreleri de hazır mı?",
    status: "delivered", sent_at: new Date(Date.now() - 360000).toISOString(),
  },
  {
    id: "msg-5", conv_id: "conv-1",
    from_did: "did:obs:b4e39fa5d2c8g3f1b7e4d0c6f9a2e5b8",
    to_did:   "did:obs:a3f28e9d4c1b7f2e0a6d3c9b5e8f1d4a",
    type: "text", ciphertext: "Toplantı saat 15:00'de başlıyor",
    status: "read", sent_at: new Date(Date.now() - 300000).toISOString(),
  },
];

const MOCK_CONVS = [
  { id: "conv-1", name: "Defne Kaya", peer_name: "Defne Kaya",
    peer_did: "did:obs:b4e39fa5d2c8g3f1b7e4d0c6f9a2e5b8", peer_tier: 2,
    last_msg_text: "Toplantı saat 15:00'de başlıyor",
    last_msg_at: new Date(Date.now() - 300000).toISOString(), unread_count: 2, is_group: false },
  { id: "conv-2", name: "Mert Yıldız", peer_name: "Mert Yıldız",
    peer_did: "did:obs:c5f40gb6e3d9h4g2c8f5e1d7g0b3f6c9", peer_tier: 1,
    last_msg_text: "ZK kanıtı doğrulandı ✓",
    last_msg_at: new Date(Date.now() - 3600000).toISOString(), unread_count: 0, is_group: false },
  { id: "conv-3", name: "Proje Ekibi", peer_name: "Proje Ekibi",
    peer_did: null, peer_tier: 4,
    last_msg_text: "FAZ 3 deploy tamamlandı",
    last_msg_at: new Date(Date.now() - 86400000).toISOString(), unread_count: 5, is_group: true },
];

const browser = await chromium.launch({ headless: true });

async function makePage() {
  const ctx = await browser.newContext({
    viewport: VIEWPORT,
    deviceScaleFactor: 2,
    colorScheme: "dark",
    storageState: {
      cookies: [],
      origins: [{
        origin: "http://localhost:3000",
        localStorage: [
          { name: "obscura_token", value: "mock-jwt-token" },
          {
            name: "obscura-store",
            value: JSON.stringify({
              state: {
                user: { did: "did:obs:a3f28e9d4c1b7f2e0a6d3c9b5e8f1d4a", phone: "+90 555 123 45 67", username: "emirhan", tier: 3 },
                conversations: MOCK_CONVS,
                messages: { "conv-1": MOCK_MESSAGES },
                onlineUsers: ["did:obs:b4e39fa5d2c8g3f1b7e4d0c6f9a2e5b8"],
                identity: null,
                ratchets: {},
              },
              version: 0,
            }),
          },
        ],
      }],
    },
  });
  const page = await ctx.newPage();

  // Mock all API calls
  await page.route("**/v1/**", (route) => {
    const url = route.request().url();
    if (url.includes("/conversations") && !url.includes("/messages")) {
      return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ success: true, data: MOCK_CONVS }) });
    }
    if (url.includes("/messages")) {
      return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ success: true, data: MOCK_MESSAGES }) });
    }
    if (url.includes("/users/me")) {
      return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ success: true, data: { did: "did:obs:a3f28e9d4c1b7f2e0a6d3c9b5e8f1d4a", phone: "+90 555 123 45 67", username: "emirhan", tier: 3 } }) });
    }
    if (url.includes("/keys/")) {
      return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ success: true, data: null }) });
    }
    return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ success: true, data: null }) });
  });

  return { page, ctx };
}

// Force inject state + fix display issues
async function injectState(page) {
  await page.evaluate(() => {
    // Fix hidden file input
    document.querySelectorAll('input[type="file"]').forEach(el => {
      el.style.display = "none";
    });
  });
}

// ── SCREEN 1: Chat List ────────────────────────────────────────────────────
{
  console.log("📸 1/7 Chat List...");
  const { page } = await makePage();
  await page.goto("http://localhost:3000/chats", { waitUntil: "networkidle" });
  await page.waitForTimeout(2500);
  await injectState(page);
  await page.screenshot({ path: `${OUT}/01_chat_list.png` });
  await page.context().close();
}

// ── SCREEN 2: Chat Conversation (messages loaded, no E2EE) ─────────────────
{
  console.log("📸 2/7 Chat Conversation (messages)...");
  const { page } = await makePage();
  await page.goto("http://localhost:3000/chats/conv-1", { waitUntil: "networkidle" });
  await page.waitForTimeout(2500);
  await injectState(page);
  await page.screenshot({ path: `${OUT}/02_chat_conv.png` });
  await page.context().close();
}

// ── SCREEN 3: Chat with Encryption Banner (force e2eeReady) ────────────────
{
  console.log("📸 3/7 Encryption Banner...");
  const { page } = await makePage();
  await page.goto("http://localhost:3000/chats/conv-1", { waitUntil: "networkidle" });
  await page.waitForTimeout(2000);

  // Force the encryption banner visible by setting React state via DOM trick
  await page.evaluate(() => {
    // Inject a fake encryption banner directly into DOM to demonstrate the UI
    document.querySelectorAll('input[type="file"]').forEach(el => el.style.display = "none");

    // Find the messages area and inject a banner above it
    const bannerHtml = `
      <div id="injected-banner" style="
        display: flex; align-items: center; justify-content: space-between;
        padding: 0 16px; height: 32px; flex-shrink: 0;
        background: rgba(16,185,129,0.07);
        border-bottom: 1px solid rgba(16,185,129,0.18);
      ">
        <div style="display:flex; align-items:center; gap:8px">
          <svg width="12" height="12" fill="none" stroke="#10B981" stroke-width="2" viewBox="0 0 24 24"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/><polyline points="9 12 11 14 15 10"/></svg>
          <span style="font-size:11px; font-weight:600; letter-spacing:0.05em; color:#10B981; font-family:system-ui">Uçtan Uca Şifreli</span>
          <svg width="9" height="9" fill="none" stroke="#10B981" stroke-width="2" viewBox="0 0 24 24" style="opacity:0.5"><rect x="3" y="11" width="18" height="11" rx="2" ry="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/></svg>
        </div>
        <svg width="11" height="11" fill="none" stroke="rgba(16,185,129,0.6)" stroke-width="2" viewBox="0 0 24 24"><polyline points="6 9 12 15 18 9"/></svg>
      </div>
    `;

    // Insert after the sub-header
    const chatArea = document.querySelector('.scroll-area');
    if (chatArea && chatArea.parentNode) {
      const bannerDiv = document.createElement('div');
      bannerDiv.innerHTML = bannerHtml;
      chatArea.parentNode.insertBefore(bannerDiv.firstElementChild, chatArea);
    }
  });

  await page.waitForTimeout(400);
  await page.screenshot({ path: `${OUT}/03_encryption_banner.png` });
  await page.context().close();
}

// ── SCREEN 4: Cipher Details Expanded ──────────────────────────────────────
{
  console.log("📸 4/7 Cipher Details...");
  const { page } = await makePage();
  await page.goto("http://localhost:3000/chats/conv-1", { waitUntil: "networkidle" });
  await page.waitForTimeout(2000);

  await page.evaluate(() => {
    document.querySelectorAll('input[type="file"]').forEach(el => el.style.display = "none");

    const chatArea = document.querySelector('.scroll-area');
    if (chatArea && chatArea.parentNode) {
      const wrapHtml = `
        <div style="flex-shrink:0">
          <div style="
            display:flex; align-items:center; justify-content:space-between;
            padding:0 16px; height:32px;
            background:rgba(16,185,129,0.07);
            border-bottom:none;
          ">
            <div style="display:flex; align-items:center; gap:8px">
              <svg width="12" height="12" fill="none" stroke="#10B981" stroke-width="2" viewBox="0 0 24 24"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/><polyline points="9 12 11 14 15 10"/></svg>
              <span style="font-size:11px; font-weight:600; color:#10B981; font-family:system-ui; letter-spacing:0.05em">Uçtan Uca Şifreli</span>
              <svg width="9" height="9" fill="none" stroke="#10B981" stroke-width="2" viewBox="0 0 24 24" style="opacity:0.5"><rect x="3" y="11" width="18" height="11" rx="2" ry="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/></svg>
            </div>
            <svg width="11" height="11" fill="none" stroke="rgba(16,185,129,0.6)" stroke-width="2" viewBox="0 0 24 24" style="transform:rotate(180deg)"><polyline points="6 9 12 15 18 9"/></svg>
          </div>
          <div style="
            padding:10px 16px;
            background:rgba(16,185,129,0.04);
            border-bottom:1px solid rgba(16,185,129,0.1);
          ">
            <div style="display:flex; gap:12px; flex-wrap:wrap">
              ${["Signal Protocol","AES-256-GCM","Ed25519","X3DH"].map(l =>
                `<span style="font-size:10px; font-family:monospace; color:rgba(16,185,129,0.6)">${l}</span>`
              ).join("")}
              <span style="font-size:10px; font-family:monospace; color:#404466">did:obs:b4e39fa5d2c8g3…</span>
            </div>
          </div>
        </div>
      `;
      const el = document.createElement('div');
      el.innerHTML = wrapHtml;
      chatArea.parentNode.insertBefore(el.firstElementChild, chatArea);
    }
  });

  await page.waitForTimeout(400);
  await page.screenshot({ path: `${OUT}/04_cipher_expanded.png` });
  await page.context().close();
}

// ── SCREEN 5: DID Expand Panel ─────────────────────────────────────────────
{
  console.log("📸 5/7 DID Expand...");
  const { page } = await makePage();
  await page.goto("http://localhost:3000/chats/conv-1", { waitUntil: "networkidle" });
  await page.waitForTimeout(2000);
  await injectState(page);

  const btn = page.locator('button[aria-label="Kimlik bilgilerini göster"]').first();
  if (await btn.isVisible()) {
    await btn.click({ force: true });
    await page.waitForTimeout(500);
  }
  await page.screenshot({ path: `${OUT}/05_did_expand.png` });
  await page.context().close();
}

// ── SCREEN 6: Login ────────────────────────────────────────────────────────
{
  console.log("📸 6/7 Login...");
  const { page } = await makePage();
  await page.goto("http://localhost:3000/login", { waitUntil: "networkidle" });
  await page.waitForTimeout(1500);
  await page.screenshot({ path: `${OUT}/06_login.png` });
  await page.context().close();
}

// ── SCREEN 7: Settings ─────────────────────────────────────────────────────
{
  console.log("📸 7/7 Settings...");
  const { page } = await makePage();
  await page.goto("http://localhost:3000/settings", { waitUntil: "networkidle" });
  await page.waitForTimeout(1500);
  await page.screenshot({ path: `${OUT}/07_settings.png` });
  await page.context().close();
}

await browser.close();
console.log("✅ Done → E:/obscura/_screenshots/");
