"use client";

import { useState, useEffect } from "react";
import { AppShell } from "@/components/AppShell";
import { api } from "@/lib/api";
import { Calendar, MapPin, Users, Plus, Loader2, Clock } from "lucide-react";
import Link from "next/link";

interface Event {
  id: string;
  title: string;
  description?: string;
  location_name?: string;
  starts_at: string;
  ends_at?: string;
  capacity?: number;
  attendee_count?: number;
  creator_did: string;
}

export default function EventsPage() {
  const [events, setEvents] = useState<Event[]>([]);
  const [loading, setLoading] = useState(true);
  const [showCreate, setShowCreate] = useState(false);
  const [creating, setCreating] = useState(false);
  const [form, setForm] = useState({
    title: "", description: "", location: "",
    starts_at: "", ends_at: "", capacity: "",
  });

  useEffect(() => { loadEvents(); }, []);

  async function loadEvents() {
    setLoading(true);
    try {
      const res = await api.listEvents();
      setEvents(Array.isArray(res) ? res : res?.events ?? []);
    } catch { /* ignore */ }
    finally { setLoading(false); }
  }

  async function handleCreate(e: React.FormEvent) {
    e.preventDefault();
    setCreating(true);
    try {
      await api.createEvent({
        title: form.title,
        description: form.description || undefined,
        location: form.location || undefined,
        starts_at: form.starts_at,
        ends_at: form.ends_at || form.starts_at,
        capacity: form.capacity ? Number(form.capacity) : undefined,
      });
      setShowCreate(false);
      setForm({ title: "", description: "", location: "", starts_at: "", ends_at: "", capacity: "" });
      loadEvents();
    } catch { /* ignore */ }
    finally { setCreating(false); }
  }

  return (
    <AppShell title="Etkinlikler">
      <div className="scroll-area h-full pb-28">
        <div className="px-4 pt-4 space-y-3 animate-slide-up">

          <div className="flex items-center justify-between mb-2">
            <h1 className="page-title">Etkinlikler</h1>
            <button className="btn-primary px-4 py-2 text-sm flex items-center gap-1.5"
              onClick={() => setShowCreate(!showCreate)}>
              <Plus size={15} /> Oluştur
            </button>
          </div>

          {/* Create form */}
          {showCreate && (
            <form onSubmit={handleCreate} className="card p-4 space-y-3">
              <input className="field" placeholder="Etkinlik adı *" required
                value={form.title} onChange={(e) => setForm({ ...form, title: e.target.value })} />
              <textarea className="field resize-none h-20" placeholder="Açıklama"
                value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} />
              <input className="field" placeholder="Konum"
                value={form.location} onChange={(e) => setForm({ ...form, location: e.target.value })} />
              <div className="grid grid-cols-2 gap-2">
                <input className="field" type="datetime-local" placeholder="Başlangıç *" required
                  value={form.starts_at} onChange={(e) => setForm({ ...form, starts_at: e.target.value })} />
                <input className="field" type="datetime-local" placeholder="Bitiş"
                  value={form.ends_at} onChange={(e) => setForm({ ...form, ends_at: e.target.value })} />
              </div>
              <input className="field" type="number" placeholder="Kapasite" min={1}
                value={form.capacity} onChange={(e) => setForm({ ...form, capacity: e.target.value })} />
              <div className="flex gap-2">
                <button type="button" className="btn-ghost flex-1" onClick={() => setShowCreate(false)}>İptal</button>
                <button type="submit" className="btn-primary flex-1" disabled={creating}>
                  {creating ? <Loader2 size={15} className="animate-spin" /> : "Oluştur"}
                </button>
              </div>
            </form>
          )}

          {loading ? (
            <div className="flex items-center justify-center py-16">
              <Loader2 size={24} className="animate-spin" style={{ color: "var(--accent)" }} />
            </div>
          ) : events.length === 0 ? (
            <div className="text-center py-16">
              <Calendar size={40} className="mx-auto mb-3" style={{ color: "var(--text-3)" }} />
              <p style={{ color: "var(--text-3)" }}>Henüz etkinlik yok</p>
            </div>
          ) : (
            events.map((ev) => (
              <Link key={ev.id} href={`/events/${ev.id}`}
                className="card p-4 block hover:opacity-90 transition-opacity">
                <h3 className="font-semibold mb-1 truncate"
                  style={{ color: "var(--text-1)", fontFamily: "var(--font-display)" }}>
                  {ev.title}
                </h3>
                {ev.location_name && (
                  <div className="flex items-center gap-1.5 mb-1">
                    <MapPin size={12} style={{ color: "var(--text-3)" }} />
                    <span className="text-xs truncate" style={{ color: "var(--text-3)" }}>{ev.location_name}</span>
                  </div>
                )}
                <div className="flex items-center gap-3">
                  <div className="flex items-center gap-1.5">
                    <Clock size={12} style={{ color: "var(--text-3)" }} />
                    <span className="text-xs" style={{ color: "var(--text-3)" }}>
                      {new Date(ev.starts_at).toLocaleString("tr-TR", { dateStyle: "short", timeStyle: "short" })}
                    </span>
                  </div>
                  {ev.attendee_count !== undefined && (
                    <div className="flex items-center gap-1.5">
                      <Users size={12} style={{ color: "var(--text-3)" }} />
                      <span className="text-xs" style={{ color: "var(--text-3)" }}>
                        {ev.attendee_count}{ev.capacity ? `/${ev.capacity}` : ""}
                      </span>
                    </div>
                  )}
                </div>
              </Link>
            ))
          )}
        </div>
      </div>
    </AppShell>
  );
}
