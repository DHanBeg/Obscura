//! Performance bench for MLS — spec Bölüm 15.2 hedefleri:
//!   - encrypt 1000-üyeli grup: <100ms
//!   - decrypt: <50ms
//!
//! Run: cargo test --release --lib mls::bench:: -- --nocapture --ignored

#![cfg(test)]

use super::*;
use openmls::prelude::tls_codec::{Deserialize, Serialize};
use std::time::Instant;

#[test]
#[ignore] // long-running, run with --ignored
fn bench_1000_member_group_encrypt() {
    println!("Setting up 1000-member MLS group...");

    let p_creator = new_provider();
    let creator = Identity::generate("did:obs:creator", &p_creator).unwrap();
    let mut creator_group = create_group(&creator, &p_creator).unwrap();

    // Generate 999 other members and add them all
    let setup_start = Instant::now();
    for i in 0..999 {
        let p = new_provider();
        let id = Identity::generate(format!("did:obs:m{}", i), &p).unwrap();
        let kp = generate_key_package(&id, &p).unwrap();
        let _ = add_member(&mut creator_group, &creator, kp, &p_creator).unwrap();

        if (i + 1) % 100 == 0 {
            println!("  + {} members ({}ms)", i + 1, setup_start.elapsed().as_millis());
        }
    }
    println!("Setup done in {:?}", setup_start.elapsed());
    println!("Final epoch: {}", creator_group.epoch().as_u64());

    // Bench encrypt
    let plaintext = b"This is a benchmark message in a 1000-member MLS group.";
    let runs = 100;

    let t0 = Instant::now();
    for _ in 0..runs {
        let _ = encrypt_message(&mut creator_group, &creator, plaintext, &p_creator).unwrap();
    }
    let elapsed = t0.elapsed();
    let avg_ms = elapsed.as_micros() as f64 / runs as f64 / 1000.0;
    println!("Encrypt {} msgs: total {:?}, avg {:.2} ms/msg (spec target <100ms)", runs, elapsed, avg_ms);
    assert!(avg_ms < 100.0, "encrypt avg {:.2}ms exceeds spec target 100ms", avg_ms);
}

#[test]
#[ignore] // long-running, run with --ignored
fn bench_5000_member_group_encrypt() {
    println!("Setting up 5000-member MLS group (sequential add_member)...");

    let p_creator = new_provider();
    let creator = Identity::generate("did:obs:creator", &p_creator).unwrap();
    let mut creator_group = create_group(&creator, &p_creator).unwrap();

    let setup_start = Instant::now();
    for i in 0..4999 {
        let p = new_provider();
        let id = Identity::generate(format!("did:obs:m{}", i), &p).unwrap();
        let kp = generate_key_package(&id, &p).unwrap();
        let _ = add_member(&mut creator_group, &creator, kp, &p_creator).unwrap();

        if (i + 1) % 500 == 0 {
            println!("  + {} members ({}ms)", i + 1, setup_start.elapsed().as_millis());
        }
    }
    println!("Setup done in {:?}", setup_start.elapsed());
    println!("Final epoch: {}", creator_group.epoch().as_u64());

    // Bench encrypt
    let plaintext = b"This is a benchmark message in a 5000-member MLS group.";
    let runs = 100;

    let t0 = Instant::now();
    for _ in 0..runs {
        let _ = encrypt_message(&mut creator_group, &creator, plaintext, &p_creator).unwrap();
    }
    let elapsed = t0.elapsed();
    let avg_ms = elapsed.as_micros() as f64 / runs as f64 / 1000.0;
    println!(
        "5000-member encrypt {} msgs: total {:?}, avg {:.3} ms/msg (spec target <100ms)",
        runs, elapsed, avg_ms
    );
    assert!(avg_ms < 100.0, "encrypt avg {:.3}ms exceeds spec target 100ms", avg_ms);
}

#[test]
#[ignore] // long-running, run with --ignored
fn bench_bulk_add_5000() {
    println!("Generating 5000 KeyPackages...");
    let kp_start = Instant::now();

    let p_creator = new_provider();
    let creator = Identity::generate("did:obs:creator", &p_creator).unwrap();

    // Generate 5000 KeyPackages (each member has their own provider/keystore).
    // We hold onto providers in a Vec so they live long enough; we don't need
    // the new members' MlsGroups for the bench.
    let mut kps: Vec<KeyPackage> = Vec::with_capacity(5000);
    let mut _providers: Vec<openmls_rust_crypto::OpenMlsRustCrypto> = Vec::with_capacity(5000);
    for i in 0..5000 {
        let p = new_provider();
        let id = Identity::generate(format!("did:obs:bulk{}", i), &p).unwrap();
        let kp = generate_key_package(&id, &p).unwrap();
        kps.push(kp);
        _providers.push(p);
        if (i + 1) % 500 == 0 {
            println!("  generated {} KPs ({}ms)", i + 1, kp_start.elapsed().as_millis());
        }
    }
    println!("KP generation done in {:?}", kp_start.elapsed());

    // Bulk add — single commit
    let mut group = create_group(&creator, &p_creator).unwrap();
    let bulk_start = Instant::now();
    let (_commit, _welcome) =
        add_members(&mut group, &creator, &kps, &p_creator).unwrap();
    let bulk_elapsed = bulk_start.elapsed();

    println!(
        "Bulk add 5000 members (single commit): {:?}, final epoch {} (size {})",
        bulk_elapsed,
        group.epoch().as_u64(),
        group.members().count()
    );

    // Sanity: epoch should have advanced exactly once
    assert_eq!(group.epoch().as_u64(), 1, "bulk add must produce exactly one commit/epoch");
    assert_eq!(group.members().count(), 5001, "creator + 5000 new members");

    // Sanity: encrypt still meets budget after bulk add
    let plaintext = b"post bulk-add benchmark message";
    let runs = 100;
    let t0 = Instant::now();
    for _ in 0..runs {
        let _ = encrypt_message(&mut group, &creator, plaintext, &p_creator).unwrap();
    }
    let avg_ms = t0.elapsed().as_micros() as f64 / runs as f64 / 1000.0;
    println!(
        "Post-bulk-add 5000-member encrypt avg: {:.3} ms/msg (spec target <100ms)",
        avg_ms
    );
    assert!(avg_ms < 100.0, "encrypt avg {:.3}ms exceeds spec target 100ms", avg_ms);
}

#[test]
#[ignore]
fn bench_decrypt() {
    let p_alice = new_provider();
    let p_bob = new_provider();
    let alice = Identity::generate("did:obs:alice", &p_alice).unwrap();
    let bob = Identity::generate("did:obs:bob", &p_bob).unwrap();

    let bob_kp = generate_key_package(&bob, &p_bob).unwrap();
    let mut g_alice = create_group(&alice, &p_alice).unwrap();
    let (_c, w) = add_member(&mut g_alice, &alice, bob_kp, &p_alice).unwrap();

    let w_bytes = w.tls_serialize_detached().unwrap();
    let in_msg = MlsMessageIn::tls_deserialize_exact(w_bytes.as_slice()).unwrap();
    let welcome = match in_msg.extract() {
        MlsMessageBodyIn::Welcome(w) => w,
        _ => panic!(),
    };
    let mut g_bob = process_welcome(welcome, &p_bob).unwrap();

    // Pre-encrypt N messages
    let plaintext = b"benchmark message body";
    let runs = 1000;
    let mut ciphertexts: Vec<Vec<u8>> = Vec::with_capacity(runs);
    for _ in 0..runs {
        let ct = encrypt_message(&mut g_alice, &alice, plaintext, &p_alice).unwrap();
        ciphertexts.push(ct.tls_serialize_detached().unwrap());
    }

    // Decrypt all
    let t0 = Instant::now();
    for ct_bytes in &ciphertexts {
        let in_msg = MlsMessageIn::tls_deserialize_exact(ct_bytes.as_slice()).unwrap();
        let _ = process_message(&mut g_bob, in_msg, &p_bob).unwrap();
    }
    let elapsed = t0.elapsed();
    let avg_ms = elapsed.as_micros() as f64 / runs as f64 / 1000.0;
    println!("Decrypt {} msgs: total {:?}, avg {:.3} ms/msg (spec target <50ms)", runs, elapsed, avg_ms);
    assert!(avg_ms < 50.0, "decrypt avg {:.3}ms exceeds spec target 50ms", avg_ms);
}
