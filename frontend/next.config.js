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
    if (isServer) {
      config.resolve.fallback = {
        ...config.resolve.fallback,
        crypto: false,
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
      };
    }
    return config;
  },
};
module.exports = nextConfig;
