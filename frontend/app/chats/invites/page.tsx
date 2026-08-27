"use client";

import { useState, useCallback, useEffect } from "react";
import { Users, Info, Loader2 } from "lucide-react";
import { AppShell } from "@/components/AppShell";
import { useStore } from "@/lib/store";
import { useToast } from "@/components/Toast";
import type { PendingWelcome } from "@/lib/mls/joinGroupFlow";

// B10 Faz 1 — mobile/app/(main)/mls-invites.tsx'in web karşılığı. KAPSAM
// SINIRI (B10.2'ye kadar): web'den grup KURULAMAZ, sadece mobil'in kurduğu
// bir gruba davetle katılınabilir — bu kısıt aşağıdaki banner'da AÇIKÇA
// söyleniyor, gizlenmiyor.
export default function MlsInvitesPage() {
  const { user } = useStore();
  const { toast } = useToast();
  const [welcomes, setWelcomes] = useState<PendingWelcome[]>([]);
  const [loading, setLoading] = useState(true);
  const [acceptingId, setAcceptingId] = useState<string | null>(null);
  const [bootstrapWarning, setBootstrapWarning] = useState<string | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    if (user) {
      try {
        const { ensureInvitable } = await import("@/lib/mls/inviteBootstrap");
        await ensureInvitable(user.did);
        setBootstrapWarning(null);
      } catch {
        setBootstrapWarning("Davet edilebilir değilsiniz (KeyPackage yüklenemedi) — tekrar deneyin");
      }
    }
    try {
      const { api } = await import("@/lib/api");
      const list = await api.mlsGetWelcomes();
      setWelcomes(list || []);
    } catch {
      toast("Davetler yüklenemedi", "error");
    } finally {
      setLoading(false);
    }
  }, [user, toast]);

  useEffect(() => { load(); }, [load]);

  const accept = useCallback(async (welcome: PendingWelcome) => {
    if (!user || acceptingId) return;
    setAcceptingId(welcome.id);
    try {
      const { acceptMlsWelcome } = await import("@/lib/mls/joinGroupFlow");
      await acceptMlsWelcome({ ownDid: user.did, welcome });
      setWelcomes((prev) => prev.filter((w) => w.id !== welcome.id));
      toast("Gruba katıldın", "success");
    } catch {
      // MÜHÜR: başarısızlıkta local state SİLİNMEZ (joinGroupFlow.ts) — davet
      // listede kalır, tekrar denenebilir.
      toast("Davet kabul edilemedi, tekrar deneyin", "error");
    } finally {
      setAcceptingId(null);
    }
  }, [user, acceptingId, toast]);

  return (
    <AppShell showBack title="Grup Davetleri">
      <div className="flex flex-col h-full" style={{ background: "var(--bg)" }}>
        <div
          className="mx-4 mt-3 mb-2 flex items-start gap-2 px-3 py-2.5 rounded-2xl flex-shrink-0"
          style={{ background: "var(--surface-2)", border: "1px solid var(--border-1)" }}
        >
          <Info size={14} style={{ color: "var(--text-3)", marginTop: 2, flexShrink: 0 }} />
          <p className="text-[11px] leading-relaxed" style={{ color: "var(--text-3)" }}>
            Web&apos;den grup kurulamaz — sadece mobil uygulamada kurulan bir gruba, oradan gelen davetle katılabilirsin.
          </p>
        </div>

        {bootstrapWarning && (
          <p className="mx-4 mb-2 text-[11px]" style={{ color: "var(--red)" }}>{bootstrapWarning}</p>
        )}

        <div className="flex-1 overflow-y-auto px-4 pb-4">
          {loading && (
            <div className="flex flex-col items-center justify-center py-16">
              <Loader2 size={20} className="animate-spin" style={{ color: "var(--text-3)" }} />
            </div>
          )}

          {!loading && welcomes.length === 0 && (
            <div className="flex flex-col items-center justify-center py-16 text-center px-6">
              <Users size={28} className="mb-3" style={{ color: "var(--text-3)" }} />
              <p className="text-sm font-medium" style={{ color: "var(--text-2)" }}>Bekleyen davet yok</p>
            </div>
          )}

          <div className="flex flex-col gap-2">
            {welcomes.map((w) => (
              <div
                key={w.id}
                className="w-full flex items-center gap-3 px-4 py-3 rounded-2xl"
                style={{ background: "var(--surface-2)", border: "1px solid var(--border-1)" }}
              >
                <div
                  className="w-10 h-10 rounded-full flex items-center justify-center flex-shrink-0"
                  style={{ background: "rgba(0,229,160,0.08)", border: "1px solid rgba(0,229,160,0.15)", color: "var(--em)" }}
                >
                  <Users size={16} />
                </div>
                <p className="flex-1 min-w-0 text-sm font-mono truncate" style={{ color: "var(--text-1)" }}>
                  {w.group_id}
                </p>
                <button
                  onClick={() => accept(w)}
                  disabled={acceptingId === w.id}
                  className="px-3.5 py-2 rounded-full text-[12px] font-semibold flex-shrink-0"
                  style={{ background: "var(--accent)", color: "var(--void)" }}
                >
                  {acceptingId === w.id ? <Loader2 size={13} className="animate-spin" /> : "Katıl"}
                </button>
              </div>
            ))}
          </div>
        </div>
      </div>
    </AppShell>
  );
}
