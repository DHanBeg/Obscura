"use client";

import { useState, useEffect } from "react";
import { useParams } from "next/navigation";
import { AppShell } from "@/components/AppShell";
import { api } from "@/lib/api";
import { MapPin, Users, Clock, UserCheck, UserMinus, Loader2, QrCode } from "lucide-react";

interface Attendee { did: string; joined_at: string; checked_in?: boolean; }
interface Event {
  id: string; title: string; description?: string;
  location_name?: string; starts_at: string; ends_at?: string;
  capacity?: number; attendee_count?: number; creator_did?: string;
}

export default function EventDetailPage() {
  const { id } = useParams<{ id: string }>();
  const [event, setEvent] = useState<Event | null>(null);
  const [attendees, setAttendees] = useState<Attendee[]>([]);
  const [loading, setLoading] = useState(true);
  const [joining, setJoining] = useState(false);
  const [joined, setJoined] = useState(false);
  const [qrCode, setQrCode] = useState<string | null>(null);

  useEffect(() => { if (id) loadEvent(); }, [id]);

  async function loadEvent() {
    setLoading(true);
    try {
      const [ev, att] = await Promise.all([api.getEvent(id), api.listAttendees(id)]);
      setEvent(ev);
      const list = Array.isArray(att) ? att : att?.attendees ?? [];
      setAttendees(list);
      const myDID = localStorage.getItem("obscura_did");
      if (myDID) setJoined(list.some((a: Attendee) => a.did === myDID));
    } catch { /* ignore */ }
    finally { setLoading(false); }
  }

  async function toggleJoin() {
    setJoining(true);
    try {
      if (joined) { await api.leaveEvent(id); setJoined(false); }
      else { await api.joinEvent(id); setJoined(true); }
      loadEvent();
    } catch { /* ignore */ }
    finally { setJoining(false); }
  }

  async function loadQR() {
    try {
      const res = await api.getCheckinQR(id);
      setQrCode(res?.qr_token ?? null);
    } catch { /* ignore */ }
  }

  if (loading) return (
    <AppShell showBack title="Etkinlik">
      <div className="flex items-center justify-center h-full">
        <Loader2 size={28} className="animate-spin" style={{ color: "var(--accent)" }} />
      </div>
    </AppShell>
  );

  if (!event) return (
    <AppShell showBack title="Etkinlik">
      <div className="flex items-center justify-center h-full">
        <p style={{ color: "var(--text-3)" }}>Etkinlik bulunamadı</p>
      </div>
    </AppShell>
  );

  return (
    <AppShell showBack title={event.title}>
      <div className="scroll-area h-full pb-28">
        <div className="px-4 pt-4 space-y-4 animate-slide-up">

          <div className="card p-5 space-y-3">
            <h1 className="text-xl font-bold" style={{ color: "var(--text-1)", fontFamily: "var(--font-display)" }}>
              {event.title}
            </h1>
            {event.description && (
              <p className="text-sm leading-relaxed" style={{ color: "var(--text-2)" }}>{event.description}</p>
            )}
            <div className="space-y-1.5">
              <div className="flex items-center gap-2">
                <Clock size={14} style={{ color: "var(--text-3)" }} />
                <span className="text-sm" style={{ color: "var(--text-2)" }}>
                  {new Date(event.starts_at).toLocaleString("tr-TR")}
                  {event.ends_at && ` — ${new Date(event.ends_at).toLocaleString("tr-TR")}`}
                </span>
              </div>
              {event.location_name && (
                <div className="flex items-center gap-2">
                  <MapPin size={14} style={{ color: "var(--text-3)" }} />
                  <span className="text-sm" style={{ color: "var(--text-2)" }}>{event.location_name}</span>
                </div>
              )}
              <div className="flex items-center gap-2">
                <Users size={14} style={{ color: "var(--text-3)" }} />
                <span className="text-sm" style={{ color: "var(--text-2)" }}>
                  {attendees.length}{event.capacity ? `/${event.capacity}` : ""} katılımcı
                </span>
              </div>
            </div>
          </div>

          <div className="flex gap-2">
            <button onClick={toggleJoin} disabled={joining}
              className={joined ? "btn-ghost flex-1" : "btn-primary flex-1"}>
              {joining ? <Loader2 size={15} className="animate-spin" /> :
                joined ? <><UserMinus size={15} /> Ayrıl</> : <><UserCheck size={15} /> Katıl</>}
            </button>
            <button onClick={loadQR} className="btn-secondary flex-1">
              <QrCode size={15} /> Check-in QR
            </button>
          </div>

          {qrCode && (
            <div className="card p-4 text-center space-y-2">
              <p className="text-xs font-semibold" style={{ color: "var(--text-2)" }}>Check-in kodu</p>
              <div className="font-mono text-xs break-all px-2 py-2 rounded-xl"
                style={{ background: "var(--surface-3)", color: "var(--accent)" }}>
                {qrCode}
              </div>
              <p className="text-xs" style={{ color: "var(--text-3)" }}>Organizatöre gösterin</p>
            </div>
          )}

          {attendees.length > 0 && (
            <div>
              <div className="section-label mb-2">KATILIMCILAR</div>
              <div className="card overflow-hidden">
                {attendees.map((a, i) => (
                  <div key={a.did} className="flex items-center gap-3 px-4 py-3"
                    style={{ borderBottom: i < attendees.length - 1 ? "1px solid var(--border-1)" : undefined }}>
                    <div className="w-8 h-8 rounded-2xl flex items-center justify-center"
                      style={{ background: "var(--accent-muted)" }}>
                      <Users size={14} style={{ color: "var(--accent)" }} />
                    </div>
                    <div className="flex-1 min-w-0">
                      <p className="text-xs font-mono truncate" style={{ color: "var(--text-2)" }}>
                        {a.did.slice(0, 32)}…
                      </p>
                    </div>
                    {a.checked_in && (
                      <span className="text-xs px-2 py-0.5 rounded-full"
                        style={{ background: "rgba(0,229,160,0.1)", color: "var(--accent)" }}>
                        ✓
                      </span>
                    )}
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>
      </div>
    </AppShell>
  );
}
