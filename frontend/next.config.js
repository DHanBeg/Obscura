/** @type {import('next').NextConfig} */
const path = require("path");
const nextConfig = {
  output: "standalone",
  typescript: { ignoreBuildErrors: true },
  webpack: (config, { isServer }) => {
    // Tauri desktop API — only available inside Tauri runtime; stub for web/Docker build
    config.resolve.alias["@tauri-apps/api/core"] = path.resolve(
      __dirname,
      "lib/__stubs__/tauri.js"
    );
    config.resolve.alias["@tauri-apps/api/event"] = path.resolve(
      __dirname,
      "lib/__stubs__/tauri.js"
    );
    // B10 Faz 1 (MLS grup mesajlaşma, docs/adr/0019) — vendor/ts-mls repo
    // kökünde, npm workspace hoisting'e BİLEREK dahil değil (mobile'ın Metro
    // watchFolders/extraNodeModules'una denk webpack alias — bkz. ADR-0019).
    // @hpke/* esm/ build ZORUNLU: script/ (CJS/dnt) build'i webpack'in
    // statik çözemediği computed require() içeriyor — Faz 0.5 spike'ta
    // tarayıcıda "Cannot find module '@hpke/common'" ile kanıtlandı,
    // esm/mod.js'e geçince düzeldi (build PASS ≠ runtime PASS uyarısı buradan).
    config.resolve.alias["ts-mls"] = path.resolve(__dirname, "../vendor/ts-mls/node_modules/ts-mls/dist/src/index.js");
    config.resolve.alias["@hpke/core"] = path.resolve(__dirname, "../vendor/ts-mls/node_modules/@hpke/core/esm/mod.js");
    config.resolve.alias["@hpke/common"] = path.resolve(__dirname, "../vendor/ts-mls/node_modules/@hpke/common/esm/mod.js");
    // ts-mls diğer ciphersuite'ler (PQ/X448/XWing/ChaCha) için lazy
    // `import()` kullanıyor — web SADECE X25519+AES128GCM+Ed25519 kullanıyor
    // (mobile ile aynı, B10 kapsamı), bu peer dep'ler kurulu değil. Metro/jest
    // lazy oldukları için es geçiyor, webpack statik çözümlemede build'i
    // kırıyor (Module not found) — fallback:false ile "modül yok, çalışırsa
    // DependencyError fırlat" davranışı taklit edilir.
    const optionalMlsSuites = {
      "@noble/post-quantum/ml-dsa.js": false,
      "@noble/ciphers/chacha.js": false,
      "@hpke/chacha20poly1305": false,
      "@hpke/dhkem-x448": false,
      "@hpke/hybridkem-x-wing": false,
      "@hpke/ml-kem": false,
    };
    if (isServer) {
      config.resolve.fallback = {
        ...config.resolve.fallback,
        crypto: false,
        ...optionalMlsSuites,
      };
    } else {
      // snarkjs browser polyfills
      config.resolve.fallback = {
        ...config.resolve.fallback,
        fs: false,
        readline: false,
        crypto: false,
        os: false,
        path: false,
        stream: false,
        assert: false,
        constants: false,
        "node:crypto": false,
        "node:fs": false,
        "node:path": false,
        "node:os": false,
        ...optionalMlsSuites,
      };
    }
    return config;
  },
};
module.exports = nextConfig;
