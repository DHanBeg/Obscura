import { Resvg } from "@resvg/resvg-js";
import { Jimp } from "jimp";
import fs from "fs";

const OUT = "E:/obscura/mobile/assets";

// Chroma-keys logo.jpeg's flat black backdrop to transparency so it can be
// composited onto the splash glow without a visible rectangle seam.
async function cutoutLogo() {
  const img = await Jimp.read(`${OUT}/logo.jpeg`);
  const { data, width, height } = img.bitmap;
  const THRESHOLD = 20;
  const FEATHER = 14; // pixels between THRESHOLD and THRESHOLD+FEATHER get partial alpha
  for (let i = 0; i < data.length; i += 4) {
    const m = Math.max(data[i], data[i + 1], data[i + 2]);
    if (m <= THRESHOLD) {
      data[i + 3] = 0;
    } else if (m <= THRESHOLD + FEATHER) {
      data[i + 3] = Math.round(((m - THRESHOLD) / FEATHER) * 255);
    }
  }
  return img.getBuffer("image/png");
}

// ── Splash — brand mark (logo.jpeg) on radial glow, no text ───────────────────
// The jpeg's flat black backdrop is chroma-keyed to transparent (cutoutLogo)
// so it composites onto the glow without a visible rectangle seam.
function buildSplash(logoPngBase64) {
  const LOGO_W = 1024, LOGO_H = 1536;
  const markW = 620, markH = (LOGO_H / LOGO_W) * markW;
  const markX = (1284 - markW) / 2;
  const markY = 1190 - markH / 2;

  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1284 2778">
<defs>
  <radialGradient id="bgGlow" cx="50%" cy="42%" r="55%">
    <stop offset="0%" stop-color="#0F2E24" stop-opacity="1"/>
    <stop offset="55%" stop-color="#08080E" stop-opacity="1"/>
    <stop offset="100%" stop-color="#050508" stop-opacity="1"/>
  </radialGradient>
  <radialGradient id="markGlow" cx="50%" cy="50%" r="50%">
    <stop offset="0%" stop-color="#00E5A0" stop-opacity="0.30"/>
    <stop offset="65%" stop-color="#00E5A0" stop-opacity="0.07"/>
    <stop offset="100%" stop-color="#00E5A0" stop-opacity="0"/>
  </radialGradient>
</defs>

<rect width="1284" height="2778" fill="url(#bgGlow)"/>

<!-- ambient halo bleeding beyond the mark's edges for depth -->
<circle cx="642" cy="1190" r="520" fill="url(#markGlow)"/>
<circle cx="642" cy="1190" r="${markH / 2 + 60}" fill="none" stroke="#00E5A0" stroke-width="1.5" opacity="0.18"/>

<image x="${markX}" y="${markY}" width="${markW}" height="${markH}"
       href="data:image/png;base64,${logoPngBase64}"/>
</svg>`;
}

// ── App icon — same brand mark as splash (green eagle, logo.jpeg), replacing
// the old hand-drawn white eagle SVG so icon/splash/nav-orb are consistent.
// transparentBg=true for adaptive-icon.png (Android supplies its own background
// layer + safe-zone mask); false for icon.png, which needs an opaque backdrop.
function buildIcon(logoPngBase64, transparentBg) {
  const LOGO_W = 1024, LOGO_H = 1536;
  // Keep the mark inside Android's ~66% adaptive-icon safe zone so launcher
  // masks (circle/squircle/rounded-square) never clip it.
  const markH = 620, markW = (LOGO_W / LOGO_H) * markH;
  const markX = (1024 - markW) / 2;
  const markY = (1024 - markH) / 2;

  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1024 1024">
${transparentBg ? "" : `<rect width="1024" height="1024" fill="#08080E"/>`}
<image x="${markX}" y="${markY}" width="${markW}" height="${markH}"
       href="data:image/png;base64,${logoPngBase64}"/>
</svg>`;
}

// ── Notification icon — egg + cracked shell cap ───────────────────────────────
// White monochrome on transparent (Android tints at runtime)
const egg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 96 96">
<!-- Egg body -->
<ellipse cx="48" cy="60" rx="29" ry="34" fill="white"/>

<!-- Shell cap — broken piece sitting on top of egg -->
<!-- Left half of cap -->
<path d="M19 46 Q22 24 48 20 L44 40 Q31 38 19 46 Z" fill="white"/>
<!-- Right half of cap -->
<path d="M77 46 Q74 24 48 20 L52 40 Q65 38 77 46 Z" fill="white"/>

<!-- Gap/crack between cap halves (dark cutout) -->
<polygon points="48,20 44,40 52,40" fill="#08080E"/>

<!-- Crack lines on egg body -->
<polyline points="36,46 40,53 35,59" fill="none" stroke="#08080E" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"/>
<polyline points="60,46 56,54 61,60" fill="none" stroke="#08080E" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"/>
</svg>`;

function toPng(svg, w, h) {
  return new Resvg(svg, { fitTo: { mode: "width", value: w } }).render().asPng();
}

const cutoutBuffer = await cutoutLogo();
const logoBase64 = cutoutBuffer.toString("base64");
const splash = buildSplash(logoBase64);
const icon = buildIcon(logoBase64, false);
const adaptiveIcon = buildIcon(logoBase64, true);

fs.writeFileSync(`${OUT}/icon.png`,              toPng(icon,  1024, 1024));
fs.writeFileSync(`${OUT}/adaptive-icon.png`,     toPng(adaptiveIcon, 1024, 1024));
fs.writeFileSync(`${OUT}/splash.png`,            toPng(splash, 1284, 2778));
fs.writeFileSync(`${OUT}/notification-icon.png`, toPng(egg,      96,   96));

console.log("icon.png          ✓");
console.log("adaptive-icon.png ✓");
console.log("splash.png        ✓");
console.log("notification-icon ✓");
