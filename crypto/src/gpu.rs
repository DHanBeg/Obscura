//! GPU/FPGA accelerated ZK proving — stub
//!
//! FAZ 3 deliverable: bellperson backend ile CUDA/OpenCL üzerinden
//! Groth16 proving 5-10x hızlanır (özellikle sunucu-tarafı bulk verify).
//!
//! NOT: WASM target'ta derlenmez (`#[cfg(not(target_arch = "wasm32"))]`).
//! NOT: `gpu` feature flag'i olmadan tüm fonksiyonlar `unimplemented!()` döndürür.
//!
//! TODO(FAZ-3): CUDA/OpenCL entegrasyonu
//!   1. bellperson::groth16::create_random_proof_batch_priority ile prover bind
//!   2. EnvConfig ile EC2 p3/p4 vs lokal GPU otomatik tespit
//!   3. CPU fallback path — GPU yoksa rayon ile multi-thread
//!   4. Benchmark: native CPU vs CUDA RTX 3090 vs OpenCL Apple M-series
//!   5. Test vektörleri — `circuits/test/gpu_parity.test.js` ile karşılaştırma

#![cfg(not(target_arch = "wasm32"))]

use thiserror::Error;

#[derive(Debug, Error)]
pub enum GpuError {
    #[error("GPU acceleration not enabled (rebuild with --features gpu)")]
    NotEnabled,
    #[error("No compatible GPU detected")]
    NoDevice,
    #[error("GPU proving failed: {0}")]
    Proving(String),
}

/// GPU acceleration runtime durumu
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum AccelBackend {
    /// CPU-only — varsayılan, her platformda çalışır
    Cpu,
    /// CUDA — NVIDIA GPU
    Cuda,
    /// OpenCL — AMD/Intel/Apple GPU
    OpenCl,
    /// FPGA — Xilinx/Intel FPGA (uzun vadeli)
    Fpga,
}

/// Runtime'da GPU mevcut mu? `gpu` feature olmadan her zaman `Cpu`.
#[must_use]
pub fn detect_backend() -> AccelBackend {
    #[cfg(feature = "gpu")]
    {
        // TODO(FAZ-3): bellperson::gpu::DEVICE_POOL kontrolü
        // bellperson init env vars: BELLMAN_GPU=1, BELLMAN_GPU_FRAMEWORK=cuda|opencl
        if std::env::var("BELLMAN_NO_GPU").is_ok() {
            return AccelBackend::Cpu;
        }
        // Stub: feature açıkken bile gerçek device probe yok — placeholder
        AccelBackend::Cpu
    }
    #[cfg(not(feature = "gpu"))]
    {
        AccelBackend::Cpu
    }
}

/// GPU üzerinde Groth16 proof üret — stub
///
/// `r1cs_bytes`     — derlenmiş circuit (snarkjs/circom çıktısı)
/// `zkey_bytes`     — phase 2 final zkey
/// `witness_bytes`  — circuit witness (private + public inputs)
///
/// Returns: serialize edilmiş Groth16 proof (~192 byte) veya hata.
pub fn prove_groth16_gpu(
    _r1cs_bytes: &[u8],
    _zkey_bytes: &[u8],
    _witness_bytes: &[u8],
) -> Result<Vec<u8>, GpuError> {
    #[cfg(feature = "gpu")]
    {
        // TODO(FAZ-3): bellperson::groth16::create_random_proof_batch_priority(
        //     params, circuits, &mut rng
        // ) — bellperson API'sini circom witness'ına adapte eden bridge gerek.
        // Şu an feature açık bile olsa native bellperson Groth16 + circom köprüsü
        // hazır değil; explicit unimplemented dön ki sessizce CPU'ya düşmesin.
        Err(GpuError::Proving(
            "bellperson<->circom köprüsü FAZ-3'te tamamlanacak".into(),
        ))
    }
    #[cfg(not(feature = "gpu"))]
    {
        Err(GpuError::NotEnabled)
    }
}

/// Bulk verification — N proof'u tek pass'te doğrula
///
/// Sunucu tarafı: gelen tüm credit/identity proof'larını batch'le
/// (Groth16 batch verify ~3-5x daha hızlı).
///
/// Returns: her proof için bool vektörü.
pub fn batch_verify_groth16_gpu(
    proofs: &[Vec<u8>],
    _vkey_bytes: &[u8],
    _public_inputs: &[Vec<String>],
) -> Result<Vec<bool>, GpuError> {
    #[cfg(feature = "gpu")]
    {
        // TODO(FAZ-3): bellperson::groth16::verify_proofs_batch
        // Şu an stub: hep false dön ki yanlış pozitif olmasın
        Ok(vec![false; proofs.len()])
    }
    #[cfg(not(feature = "gpu"))]
    {
        let _ = proofs;
        Err(GpuError::NotEnabled)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn detect_backend_returns_cpu_without_feature() {
        // Default build (`cargo test`) `gpu` feature olmadan koşar
        assert_eq!(detect_backend(), AccelBackend::Cpu);
    }

    #[test]
    fn prove_returns_not_enabled_without_feature() {
        let r = prove_groth16_gpu(&[], &[], &[]);
        assert!(r.is_err());
        #[cfg(not(feature = "gpu"))]
        assert!(matches!(r, Err(GpuError::NotEnabled)));
    }

    #[test]
    fn batch_verify_returns_not_enabled_without_feature() {
        let r = batch_verify_groth16_gpu(&[], &[], &[]);
        #[cfg(not(feature = "gpu"))]
        assert!(matches!(r, Err(GpuError::NotEnabled)));
        #[cfg(feature = "gpu")]
        assert!(r.is_ok());
    }
}
