// Node'un global localStorage'ı --localstorage-file bayrağı olmadan
// setItem'da patlıyor (bkz. test çıktısı). Testler için bellek-içi minimal
// yerine geçen.
if (
  typeof globalThis.localStorage === "undefined" ||
  typeof globalThis.localStorage.setItem !== "function"
) {
  const store = new Map<string, string>();
  (globalThis as unknown as { localStorage: Storage }).localStorage = {
    getItem: (k: string) => (store.has(k) ? store.get(k)! : null),
    setItem: (k: string, v: string) => {
      store.set(k, String(v));
    },
    removeItem: (k: string) => {
      store.delete(k);
    },
    clear: () => {
      store.clear();
    },
    key: (i: number) => Array.from(store.keys())[i] ?? null,
    get length() {
      return store.size;
    },
  } as Storage;
}
