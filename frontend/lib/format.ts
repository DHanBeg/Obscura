// Time formatting
export function formatTime(dateStr?: string | null): string {
  if (!dateStr) return "";
  const d = new Date(dateStr);
  const now = new Date();
  const diffMs = now.getTime() - d.getTime();
  const diffDays = Math.floor(diffMs / 86400000);

  if (diffDays === 0) {
    return d.toLocaleTimeString("tr-TR", { hour: "2-digit", minute: "2-digit" });
  } else if (diffDays === 1) {
    return "Dün";
  } else if (diffDays < 7) {
    return d.toLocaleDateString("tr-TR", { weekday: "short" });
  } else {
    return d.toLocaleDateString("tr-TR", { day: "numeric", month: "short" });
  }
}

export function formatFullTime(dateStr?: string | null): string {
  if (!dateStr) return "";
  return new Date(dateStr).toLocaleTimeString("tr-TR", { hour: "2-digit", minute: "2-digit" });
}

// Text truncation
export function truncate(str: string, n: number): string {
  return str.length > n ? str.slice(0, n) + "…" : str;
}

// Initials from name
export function initials(name: string): string {
  return name
    .split(" ")
    .slice(0, 2)
    .map((w) => w[0]?.toUpperCase() ?? "")
    .join("");
}

// Tier label
export function tierLabel(tier: number): string {
  return ["", "Başlangıç", "Güvenilir", "Köklü", "Elit", "Usta"][tier] ?? "Bilinmiyor";
}

// Tier color class
export function tierColor(tier: number): string {
  return ["", "text-tier1", "text-tier2", "text-tier3", "text-tier4", "text-tier5"][tier] ?? "text-sub";
}

// Phone mask
export function maskPhone(phone: string): string {
  if (phone.length < 6) return phone;
  return phone.slice(0, -4).replace(/./g, "·") + phone.slice(-4);
}
