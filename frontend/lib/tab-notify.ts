// Sekme-içi mesaj bildirimi: başlık sayacı "(2) Obscura", favicon rozeti, kısa ses,
// izin varsa OS bildirimi.
//
// Push / service worker akışından ve Notification izninden BAĞIMSIZ: WS `new_message`'a
// doğrudan bağlanır. Başlık/favicon/ses izin GEREKTİRMEZ; yalnız OS bildirimi
// (tauri.ts showNotification) izin verilmişse gösterilir, verilmemişse sessizce atlanır.
//
// "Arka plan" = sekme gizli VEYA pencere odakta değil (iki pencere yan yanayken alıcı sekme
// görünür ama odakta değildir; yalnız document.hidden bunu kaçırıyordu).
//
// Çekirdek (createTabNotifier) DOM'a dokunmaz: ortam (başlık/favicon/ses/OS) enjekte edilir,
// hata izole edilir (biri patlarsa diğerleri çalışır), birim test edilebilir.

import { showNotification } from "./tauri";

export interface TabNotifyEnv {
  isBackground(): boolean;
  getTitle(): string;
  setTitle(title: string): void;
  /** count > 0: rozetli favicon; null: orijinali geri yükle. */
  setFaviconBadge(count: number | null): void;
  playSound(): void;
  showOsNotification(title: string, body: string): void;
}

export interface IncomingMessage {
  id?: string;
  type?: string;
  fromDid?: string;
  ownDid?: string;
  /** Çözülmüş düz metin (ya da decrypt yer tutucusu). */
  plaintext?: string;
}

const NON_NOTIFYING_TYPES = new Set(["system", "read_receipt"]);
const SEEN_IDS_MAX = 100;
const BODY_MAX_CHARS = 60;
const PLACEHOLDER_PREFIX = "\u{1F512}"; // decryptIncoming başarısızlık yer tutucusu
const TITLE_PREFIX_RE = /^\(\d+\+?\)\s+/;
const MAX_SHOWN = 99;

/** OS bildirim gövdesi: yalnız çözülmüş metin; medya URL'i/yer tutucu düz metin gibi gösterilmez. */
export function notificationBody(type: string | undefined, plaintext: string | undefined): string {
  if (type !== "text") return "Yeni mesaj";
  const text = (plaintext ?? "").replace(/\s+/g, " ").trim();
  if (!text || text.startsWith(PLACEHOLDER_PREFIX)) return "Yeni şifreli mesaj";
  const chars = Array.from(text);
  return chars.length <= BODY_MAX_CHARS ? text : chars.slice(0, BODY_MAX_CHARS).join("") + "…";
}

function stripTitlePrefix(title: string): string {
  return title.replace(TITLE_PREFIX_RE, "");
}

function badgeLabel(count: number): string {
  return count > MAX_SHOWN ? `${MAX_SHOWN}+` : String(count);
}

export interface TabNotifier {
  /** Sayılırsa true. Ön planda / kendi / sistem / tekrar mesajlarda false. */
  onIncoming(msg: IncomingMessage): boolean;
  /** Sekme öne gelince sayacı, başlığı ve favicon'u sıfırlar. */
  reset(): void;
  /** Başka kod başlığı değiştirdiyse (sayfa geçişi) sayaç önekini yeniden uygular. */
  refreshTitle(): void;
  readonly unread: number;
}

function safely(fn: () => void): void {
  try {
    fn();
  } catch {
    // Bildirim yardımcıları asla mesaj akışını bozmaz.
  }
}

export function createTabNotifier(env: TabNotifyEnv): TabNotifier {
  let unread = 0;
  const seenIds: string[] = [];

  const render = () => {
    safely(() => {
      const base = stripTitlePrefix(env.getTitle());
      const next = unread > 0 ? `(${badgeLabel(unread)}) ${base}` : base;
      if (next !== env.getTitle()) env.setTitle(next);
    });
    safely(() => env.setFaviconBadge(unread > 0 ? unread : null));
  };

  return {
    onIncoming(msg) {
      if (msg.type && NON_NOTIFYING_TYPES.has(msg.type)) return false;
      if (msg.fromDid && msg.ownDid && msg.fromDid === msg.ownDid) return false;

      if (msg.id) {
        if (seenIds.includes(msg.id)) return false;
        seenIds.push(msg.id);
        if (seenIds.length > SEEN_IDS_MAX) seenIds.shift();
      }

      let background = false;
      safely(() => {
        background = env.isBackground();
      });
      if (!background) return false;

      unread += 1;
      render();
      safely(() => env.playSound());
      safely(() => env.showOsNotification("Yeni mesaj", notificationBody(msg.type, msg.plaintext)));
      return true;
    },

    reset() {
      if (unread === 0) return;
      unread = 0;
      render();
    },

    refreshTitle() {
      if (unread > 0) render();
    },

    get unread() {
      return unread;
    },
  };
}

// ── Tarayıcı bağlayıcısı (DOM) ────────────────────────────────────────────────

const SOUND_FLAG_KEY = "obscura_notify_sound"; // "off" ile kapatılır (varsayılan açık)
const FAVICON_SIZE = 64;

function soundEnabled(): boolean {
  try {
    return localStorage.getItem(SOUND_FLAG_KEY) !== "off";
  } catch {
    return true;
  }
}

let audioCtx: AudioContext | null = null;

function ensureAudioContext(): AudioContext | null {
  const Ctor: typeof AudioContext | undefined =
    window.AudioContext ?? (window as unknown as { webkitAudioContext?: typeof AudioContext }).webkitAudioContext;
  if (!Ctor) return null;
  audioCtx = audioCtx ?? new Ctor();
  return audioCtx;
}

// Sekme daha önce hiç tıklama/tuş görmediyse AudioContext "suspended" kurulur ve
// resume() (bir kullanıcı jesti eşlik etmeden) tamamlanmayabilir — önceki sürüm bunu
// beklemeden osilatörü hemen başlatıyordu, taze/hiç-tıklanmamış sekmede ses SESSİZCE
// çalmıyordu. Artık ses yalnızca context gerçekten "running" olunca çalınır.
function fireBeep(ctx: AudioContext): void {
  const now = ctx.currentTime;
  const osc = ctx.createOscillator();
  const gain = ctx.createGain();
  osc.type = "sine";
  osc.frequency.setValueAtTime(880, now);
  gain.gain.setValueAtTime(0.0001, now);
  gain.gain.exponentialRampToValueAtTime(0.08, now + 0.02);
  gain.gain.exponentialRampToValueAtTime(0.0001, now + 0.22);
  osc.connect(gain).connect(ctx.destination);
  osc.start(now);
  osc.stop(now + 0.25);
}

// KANIT (gerçek Chrome + gerçek hesap, claude-in-chrome ile canlı ölçüldü, 2026-09-24):
// önceki sürüm burada ctx.resume() çağırıp .then() ile bekliyordu. Sekmede hiç
// jest olmadan gelen mesajlarda bu resume() SONSUZA KADAR askıda kaldı (Chrome bunu
// hiç çözmüyor, jestsiz). wireAudioUnlock() ilk jestte KENDİ resume()'unu çağırınca,
// context "running" olduğu an, playBeep()'in tüm o ana kadar BİRİKMİŞ askıdaki
// .then() zincirleri AYNI ANDA çözülüp geciken bipleri TOPLU/ÇAKIŞIK çaldı —
// mesajdan onlarca saniye sonra, kullanıcı hiçbir şey duymamış gibi hissetti.
// Düzeltme: burada resume() ÇAĞRILMAZ, kuyruğa alınmaz. Yalnız context ZATEN
// "running" ise (yani sekmede daha önce en az bir jest olduysa) hemen çalınır;
// değilse bu mesaj için ses sessizce atlanır (başlık/favicon yine de çalışır) —
// context wireAudioUnlock ile açıldıktan SONRAKİ mesajlar normal çalar.
function playBeep(): void {
  if (!soundEnabled()) return;
  const ctx = ensureAudioContext();
  if (!ctx || ctx.state !== "running") return;
  fireBeep(ctx);
}

// İlk gerçek kullanıcı jestinde (tıklama/tuş) AudioContext'i erkenden oluşturup
// resume eder — böylece bir WS mesajı jestsiz anda gelse bile context o ana kadar
// zaten "running" olur ve playBeep() senkron/anında çalar. Yalnız bir kez kurulur.
let audioUnlockWired = false;
function wireAudioUnlock(): void {
  if (audioUnlockWired || typeof window === "undefined") return;
  audioUnlockWired = true;
  const unlock = () => {
    window.removeEventListener("pointerdown", unlock);
    window.removeEventListener("keydown", unlock);
    const ctx = ensureAudioContext();
    if (ctx && ctx.state === "suspended") void ctx.resume().catch(() => {});
  };
  window.addEventListener("pointerdown", unlock);
  window.addEventListener("keydown", unlock);
}

interface FaviconState {
  links: HTMLLinkElement[];
  originals: string[];
  created: HTMLLinkElement | null;
  baseImage: Promise<HTMLImageElement | null> | null;
  requested: number | null;
}

const favicon: FaviconState = { links: [], originals: [], created: null, baseImage: null, requested: null };

function iconLinks(): HTMLLinkElement[] {
  return Array.from(document.querySelectorAll<HTMLLinkElement>('link[rel~="icon"]'));
}

function loadBaseImage(href: string): Promise<HTMLImageElement | null> {
  return new Promise((resolve) => {
    const img = new Image();
    img.onload = () => resolve(img);
    img.onerror = () => resolve(null);
    img.src = href;
  });
}

function drawBadge(base: HTMLImageElement | null, count: number): string | null {
  const canvas = document.createElement("canvas");
  canvas.width = FAVICON_SIZE;
  canvas.height = FAVICON_SIZE;
  const ctx = canvas.getContext("2d");
  if (!ctx) return null;
  if (base) {
    ctx.drawImage(base, 0, 0, FAVICON_SIZE, FAVICON_SIZE);
  } else {
    ctx.fillStyle = "#0b1b18";
    ctx.fillRect(0, 0, FAVICON_SIZE, FAVICON_SIZE);
  }
  const radius = 20;
  const cx = FAVICON_SIZE - radius;
  const cy = radius;
  ctx.beginPath();
  ctx.arc(cx, cy, radius, 0, Math.PI * 2);
  ctx.fillStyle = "#ff3b4e";
  ctx.fill();
  ctx.lineWidth = 3;
  ctx.strokeStyle = "#ffffff";
  ctx.stroke();
  ctx.fillStyle = "#ffffff";
  ctx.font = "bold 26px sans-serif";
  ctx.textAlign = "center";
  ctx.textBaseline = "middle";
  ctx.fillText(count > 9 ? "9+" : String(count), cx, cy + 1);
  return canvas.toDataURL("image/png");
}

function restoreFavicon(): void {
  favicon.links.forEach((link, i) => {
    link.href = favicon.originals[i];
  });
  if (favicon.created) {
    favicon.created.remove();
    favicon.created = null;
  }
  favicon.links = [];
  favicon.originals = [];
}

function applyFaviconBadge(count: number | null): void {
  favicon.requested = count;
  if (count === null) {
    restoreFavicon();
    return;
  }
  if (favicon.links.length === 0) {
    const links = iconLinks();
    if (links.length === 0) {
      const created = document.createElement("link");
      created.rel = "icon";
      created.href = "/logo.jpeg";
      document.head.appendChild(created);
      favicon.created = created;
      links.push(created);
    }
    favicon.links = links;
    favicon.originals = links.map((l) => l.href);
  }
  favicon.baseImage = favicon.baseImage ?? loadBaseImage(favicon.originals[0] ?? "/logo.jpeg");
  void favicon.baseImage.then((img) => {
    if (favicon.requested === null) return; // bu arada sıfırlandı
    const url = drawBadge(img, favicon.requested);
    if (url) favicon.links.forEach((l) => (l.href = url));
  });
}

let singleton: TabNotifier | null = null;

function browserEnv(): TabNotifyEnv {
  return {
    isBackground: () => document.hidden || !document.hasFocus(),
    getTitle: () => document.title,
    setTitle: (t) => {
      document.title = t;
    },
    setFaviconBadge: applyFaviconBadge,
    playSound: playBeep,
    showOsNotification: (title, body) => {
      showNotification(title, body).catch(() => {});
    },
  };
}

/** Tek örnek + öne gelince sıfırlama + sayfa geçişinde başlık önekini koruma (bir kez kurulur). */
export function getTabNotifier(): TabNotifier | null {
  if (typeof window === "undefined" || typeof document === "undefined") return null;
  if (singleton) return singleton;

  const notifier = createTabNotifier(browserEnv());
  singleton = notifier;
  wireAudioUnlock();

  const resetIfFront = () => {
    if (!document.hidden && document.hasFocus()) notifier.reset();
  };
  document.addEventListener("visibilitychange", resetIfFront);
  window.addEventListener("focus", resetIfFront);

  const titleEl = document.querySelector("title");
  if (titleEl && typeof MutationObserver !== "undefined") {
    new MutationObserver(() => notifier.refreshTitle()).observe(titleEl, {
      childList: true,
      characterData: true,
      subtree: true,
    });
  }
  return notifier;
}

/** AppShell WS `new_message` çağıracak tek giriş noktası. Hata fırlatmaz. */
export function notifyIncomingMessage(msg: IncomingMessage): void {
  try {
    getTabNotifier()?.onIncoming(msg);
  } catch {
    // mesaj akışını asla bozma
  }
}
