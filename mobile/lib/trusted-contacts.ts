// Güven kişisi yardımcıları — Madde 13 (Bölüm 6). contacts.tsx'teki
// işaretleme UI'ı ve panik ekranı (Adım 5) aynı Contact tipini ve filtreleri
// kullanır — tek kaynak.

export interface Contact {
  did: string;
  odi?: string;
  display_name: string;
  username: string;
  avatar_url: string;
  tier: number;
  nickname: string;
  added_at: string;
  is_trusted: boolean;
}

export function getTrustedContacts(contacts: Contact[]): Contact[] {
  return contacts.filter((c) => c.is_trusted);
}

export function hasTrustedContact(contacts: Contact[]): boolean {
  return getTrustedContacts(contacts).length > 0;
}
