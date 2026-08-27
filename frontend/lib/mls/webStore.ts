// B10 Faz 1 — mobile/lib/keyValueStore.ts'nin web karşılığı. Ayni KeyValueStore
// arayüzü (getItem/setItem/deleteItem) — mls-store.ts/cryptoBlob.ts bu ikisini
// mobile'daki secureStore/asyncStore'un yerine enjekte eder, MANTIK değişmez.
//
// Tarayıcıda OS keychain/keystore eşdeğeri yok — web'in zaten var olan 1:1
// e2ee katmanı (frontend/lib/e2ee-session.ts) private key materyalini
// localStorage'da tutuyor (şifrelenmemiş blob, ek "secure" katman yok). Aynı
// güvenlik duruşu korunuyor: webSecureStore de localStorage — MLS için var
// olandan daha zayıf bir seçim DEĞİL, tutarlı bir seçim.
//
// webAsyncStore IndexedDB kullanıyor (localStorage'ın ~5-10MB senkron
// sınırından daha büyük/async blob'lar için — grup state'i ratchet tree
// içerir, tek mesajdan büyük olabilir; mobile'da AsyncStorage'ın üstlendiği
// rol bu).
"use client";

export interface KeyValueStore {
  getItem(key: string): Promise<string | null>;
  setItem(key: string, value: string): Promise<void>;
  deleteItem(key: string): Promise<void>;
}

export const webSecureStore: KeyValueStore = {
  async getItem(key) {
    try {
      return localStorage.getItem(key);
    } catch {
      return null;
    }
  },
  async setItem(key, value) {
    try {
      localStorage.setItem(key, value);
    } catch {}
  },
  async deleteItem(key) {
    try {
      localStorage.removeItem(key);
    } catch {}
  },
};

const IDB_NAME = "obscura_mls";
const IDB_STORE = "kv";
const IDB_VERSION = 1;

let dbPromise: Promise<IDBDatabase> | null = null;

function openDb(): Promise<IDBDatabase> {
  if (dbPromise) return dbPromise;
  dbPromise = new Promise((resolve, reject) => {
    const req = indexedDB.open(IDB_NAME, IDB_VERSION);
    req.onupgradeneeded = () => {
      if (!req.result.objectStoreNames.contains(IDB_STORE)) {
        req.result.createObjectStore(IDB_STORE);
      }
    };
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error);
  });
  return dbPromise;
}

async function idbGet(key: string): Promise<string | null> {
  const db = await openDb();
  return new Promise((resolve, reject) => {
    const tx = db.transaction(IDB_STORE, "readonly");
    const req = tx.objectStore(IDB_STORE).get(key);
    req.onsuccess = () => resolve((req.result as string | undefined) ?? null);
    req.onerror = () => reject(req.error);
  });
}

async function idbSet(key: string, value: string): Promise<void> {
  const db = await openDb();
  return new Promise((resolve, reject) => {
    const tx = db.transaction(IDB_STORE, "readwrite");
    tx.objectStore(IDB_STORE).put(value, key);
    tx.oncomplete = () => resolve();
    tx.onerror = () => reject(tx.error);
  });
}

async function idbDelete(key: string): Promise<void> {
  const db = await openDb();
  return new Promise((resolve, reject) => {
    const tx = db.transaction(IDB_STORE, "readwrite");
    tx.objectStore(IDB_STORE).delete(key);
    tx.oncomplete = () => resolve();
    tx.onerror = () => reject(tx.error);
  });
}

export const webAsyncStore: KeyValueStore = {
  async getItem(key) {
    try {
      return await idbGet(key);
    } catch {
      return null;
    }
  },
  async setItem(key, value) {
    try {
      await idbSet(key, value);
    } catch {}
  },
  async deleteItem(key) {
    try {
      await idbDelete(key);
    } catch {}
  },
};
