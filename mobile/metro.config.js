// ts-mls (L2, MLS grup mesajlaşma) workspace-dışı vendor izolasyonu.
// Bkz. docs/adr/0019-mobile-mls-ts-port.md (Revizyon 2026-08-13, Decision).
//
// vendor/ts-mls/ repo kökünde, root package.json'daki
// "workspaces": ["frontend","mobile","packages/*"] listesinde KASITLI DEĞİL —
// npm onu hoisting'e hiç dahil etmiyor, mobile'ın kendi @noble/curves@2.2.0'ını
// shadow'lamıyor (spike/ts-mls-isolation branch'inde 3/3 kanıtla doğrulandı).
// Ama bu yüzden Metro da onu normalde HİÇ görmez — watchFolders + extraNodeModules
// elle bağlıyor.
const { getDefaultConfig } = require("expo/metro-config");
const path = require("path");

const config = getDefaultConfig(__dirname);

const vendorRoot = path.resolve(__dirname, "../vendor/ts-mls");
const workspaceRoot = path.resolve(__dirname, "..");

// npm workspaces hoisting: expo-router (ve diğer bağımlılıklar) mobile/node_modules
// yerine root node_modules'a hoisted oluyor. Metro'nun default projectRoot=mobile
// varsayımı bunu görmüyor — HMR entry-point resolution (HmrServer._registerEntryPoint)
// bu yüzden reload'da "Unable to resolve module ./node_modules/expo-router/entry"
// diye uncaught throw atıp tüm Metro process'ini öldürüyordu.
config.watchFolders = [...(config.watchFolders || []), vendorRoot, workspaceRoot];

config.resolver.nodeModulesPaths = [
  path.resolve(__dirname, "node_modules"),
  path.resolve(workspaceRoot, "node_modules"),
];

config.resolver.unstable_enablePackageExports = true;

config.resolver.extraNodeModules = {
  ...(config.resolver.extraNodeModules || {}),
  "ts-mls": path.resolve(vendorRoot, "node_modules/ts-mls"),
  "@hpke/core": path.resolve(vendorRoot, "node_modules/@hpke/core"),
  "@hpke/common": path.resolve(vendorRoot, "node_modules/@hpke/common"),
};

module.exports = config;
