import { getTrustedContacts, hasTrustedContact, Contact } from "../trusted-contacts";

function makeContact(overrides: Partial<Contact>): Contact {
  return {
    did: "did:obscura:x",
    display_name: "X",
    username: "x",
    avatar_url: "",
    tier: 1,
    nickname: "",
    added_at: "2026-01-01T00:00:00Z",
    is_trusted: false,
    ...overrides,
  };
}

describe("getTrustedContacts / hasTrustedContact — Madde 13 Adım 4 zemini", () => {
  test("boş liste için boş liste ve false döner", () => {
    expect(getTrustedContacts([])).toEqual([]);
    expect(hasTrustedContact([])).toBe(false);
  });

  test("hiçbiri güvenilmiyorsa boş liste ve false döner", () => {
    const contacts = [makeContact({ did: "a" }), makeContact({ did: "b" })];
    expect(getTrustedContacts(contacts)).toEqual([]);
    expect(hasTrustedContact(contacts)).toBe(false);
  });

  test("yalnızca is_trusted=true olanları filtreler", () => {
    const trusted = makeContact({ did: "a", is_trusted: true });
    const untrusted = makeContact({ did: "b", is_trusted: false });
    const contacts = [trusted, untrusted];

    expect(getTrustedContacts(contacts)).toEqual([trusted]);
    expect(hasTrustedContact(contacts)).toBe(true);
  });

  test("birden fazla güven kişisi varsa hepsini döner", () => {
    const contacts = [
      makeContact({ did: "a", is_trusted: true }),
      makeContact({ did: "b", is_trusted: true }),
      makeContact({ did: "c", is_trusted: false }),
    ];
    expect(getTrustedContacts(contacts).map((c) => c.did)).toEqual(["a", "b"]);
  });
});
