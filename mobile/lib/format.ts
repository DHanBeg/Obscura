export function formatTime(dateStr?: string | null): string {
  if (!dateStr) return "";
  const d = new Date(dateStr);
  const now = new Date();
  const diffDays = Math.floor((now.getTime() - d.getTime()) / 86400000);
  if (diffDays === 0) return d.toLocaleTimeString("tr-TR", { hour: "2-digit", minute: "2-digit" });
  if (diffDays === 1) return "Dün";
  if (diffDays < 7) return d.toLocaleDateString("tr-TR", { weekday: "short" });
  return d.toLocaleDateString("tr-TR", { day: "numeric", month: "short" });
}

export function formatFullTime(dateStr?: string | null): string {
  if (!dateStr) return "";
  return new Date(dateStr).toLocaleTimeString("tr-TR", { hour: "2-digit", minute: "2-digit" });
}

export function truncate(str: string, n: number): string {
  return str.length > n ? str.slice(0, n) + "…" : str;
}

export function initials(name: string): string {
  return name.split(" ").slice(0, 2).map((w) => w[0]?.toUpperCase() ?? "").join("");
}

export function tierLabel(tier: number): string {
  return ["", "Başlangıç", "Güvenilir", "Köklü", "Elit", "Usta"][tier] ?? "Bilinmiyor";
}

export function tierColor(tier: number): string {
  return ["", "#888888", "#4a9eff", "#a78bfa", "#f59e0b", "#5ec46e"][tier] ?? "#888888";
}
