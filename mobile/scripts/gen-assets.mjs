import { Resvg } from "@resvg/resvg-js";
import fs from "fs";

const OUT = "E:/obscura/mobile/assets";

// ── White Eagle — app icon ────────────────────────────────────────────────────
const eagle = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1024 1024">
<rect width="1024" height="1024" rx="220" fill="#08080E"/>

<!-- Left wing -->
<path d="M512 600 Q380 540 220 480 Q280 510 320 490 Q260 450 160 400
         Q240 440 300 430 Q220 380 140 320 Q250 380 340 390
         Q280 340 230 280 Q340 360 420 400 Q400 350 380 290
         Q440 380 460 460 Q500 540 480 600 Z"
      fill="white"/>

<!-- Right wing -->
<path d="M512 600 Q644 540 804 480 Q744 510 704 490 Q764 450 864 400
         Q784 440 724 430 Q804 380 884 320 Q774 380 684 390
         Q744 340 794 280 Q684 360 604 400 Q624 350 644 290
         Q584 380 564 460 Q524 540 544 600 Z"
      fill="white"/>

<!-- Body -->
<ellipse cx="512" cy="680" rx="140" ry="175" fill="white"/>

<!-- Neck -->
<ellipse cx="512" cy="480" rx="72" ry="90" fill="white"/>

<!-- Head -->
<ellipse cx="512" cy="360" rx="105" ry="98" fill="white"/>

<!-- Beak — hooked eagle beak -->
<path d="M470 388 Q512 378 554 388 L570 408 Q558 430 540 425
         L512 438 L484 425 Q466 430 454 408 Z"
      fill="#F59E0B"/>
<path d="M512 415 Q530 420 545 410" stroke="#CC7A00" stroke-width="4" fill="none" stroke-linecap="round"/>

<!-- Left eye -->
<circle cx="472" cy="338" r="26" fill="#08080E"/>
<circle cx="472" cy="338" r="18" fill="white"/>
<circle cx="472" cy="338" r="10" fill="#08080E"/>
<circle cx="466" cy="332" r="5" fill="white" opacity="0.9"/>

<!-- Right eye -->
<circle cx="552" cy="338" r="26" fill="#08080E"/>
<circle cx="552" cy="338" r="18" fill="white"/>
<circle cx="552" cy="338" r="10" fill="#08080E"/>
<circle cx="546" cy="332" r="5" fill="white" opacity="0.9"/>

<!-- Eye brow ridge — stern eagle look -->
<path d="M446 316 Q472 306 498 314" stroke="#08080E" stroke-width="9" fill="none" stroke-linecap="round"/>
<path d="M526 314 Q552 306 578 316" stroke="#08080E" stroke-width="9" fill="none" stroke-linecap="round"/>

<!-- Tail feathers -->
<path d="M372 840 Q420 800 460 790 Q512 810 512 810 Q512 810 564 790
         Q604 800 652 840 Q596 818 512 825 Q428 818 372 840 Z"
      fill="white"/>

<!-- Left talons -->
<line x1="436" y1="848" x2="412" y2="890" stroke="white" stroke-width="10" stroke-linecap="round"/>
<line x1="436" y1="848" x2="432" y2="896" stroke="white" stroke-width="10" stroke-linecap="round"/>
<line x1="436" y1="848" x2="452" y2="892" stroke="white" stroke-width="10" stroke-linecap="round"/>
<line x1="436" y1="848" x2="464" y2="882" stroke="white" stroke-width="10" stroke-linecap="round"/>

<!-- Right talons -->
<line x1="588" y1="848" x2="564" y2="882" stroke="white" stroke-width="10" stroke-linecap="round"/>
<line x1="588" y1="848" x2="572" y2="892" stroke="white" stroke-width="10" stroke-linecap="round"/>
<line x1="588" y1="848" x2="588" y2="896" stroke="white" stroke-width="10" stroke-linecap="round"/>
<line x1="588" y1="848" x2="612" y2="890" stroke="white" stroke-width="10" stroke-linecap="round"/>

<!-- Green accent rings on eyes -->
<circle cx="472" cy="338" r="24" fill="none" stroke="#00E5A0" stroke-width="3" opacity="0.6"/>
<circle cx="552" cy="338" r="24" fill="none" stroke="#00E5A0" stroke-width="3" opacity="0.6"/>
</svg>`;

// ── Splash ────────────────────────────────────────────────────────────────────
const splash = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1284 2778">
<rect width="1284" height="2778" fill="#08080E"/>
<g transform="translate(252 800) scale(0.762)">
  ${eagle.replace(/<svg[^>]*>/, "").replace("</svg>", "")}
</g>
<text x="642" y="2040" text-anchor="middle"
      font-family="system-ui,-apple-system,sans-serif"
      font-size="110" font-weight="800" letter-spacing="22" fill="white">OBSCURA</text>
<text x="642" y="2120" text-anchor="middle"
      font-family="system-ui,-apple-system,sans-serif"
      font-size="42" letter-spacing="10" fill="#00E5A0" opacity="0.75">SECURE MESSENGER</text>
</svg>`;

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

fs.writeFileSync(`${OUT}/icon.png`,              toPng(eagle,  1024, 1024));
fs.writeFileSync(`${OUT}/adaptive-icon.png`,     toPng(eagle,  1024, 1024));
fs.writeFileSync(`${OUT}/splash.png`,            toPng(splash, 1284, 2778));
fs.writeFileSync(`${OUT}/notification-icon.png`, toPng(egg,      96,   96));

console.log("icon.png          ✓");
console.log("adaptive-icon.png ✓");
console.log("splash.png        ✓");
console.log("notification-icon ✓");
