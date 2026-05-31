//! Obscura Crypto — Saf Rust, sıfır dış servis bağımlılığı
//! Tüm kriptografik işlemler bu crate içinde kapalı.

pub mod identity;
pub mod x3dh;
pub mod ratchet;
pub mod symmetric;
pub mod prekeys;
pub mod zk_stub;
pub mod mls_basic;  // DEPRECATED — see ADR-0007, kept transiently
pub mod mls;        // NEW: openmls (RFC 9420) per ADR-0007
pub mod mnemonic;   // BIP39 + Obscura identity derivation (spec Bölüm 4.2)
pub mod ffi;

// ─── GPU/FPGA acceleration (FAZ 3) — native-only, gated by `gpu` feature ───
#[cfg(not(target_arch = "wasm32"))]
pub mod gpu;

// ─── WASM bindings — browser ZK prover, gated by `wasm` feature ────────────
#[cfg(feature = "wasm")]
pub mod wasm;

pub use identity::*;
pub use x3dh::*;
pub use ratchet::*;
pub use symmetric::*;
pub use prekeys::*;
