/** @type {import('next').NextConfig} */
const nextConfig = {
  output: "standalone",
  // E2EE modülü yalnızca client-side çalışır
  webpack: (config, { isServer }) => {
    if (isServer) {
      // Sunucu tarafında WebCrypto kullanan modülleri dışla
      config.resolve.fallback = {
        ...config.resolve.fallback,
        crypto: false,
      };
    }
    return config;
  },
};
module.exports = nextConfig;
