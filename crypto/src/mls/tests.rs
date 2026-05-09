//! End-to-end MLS group flow tests
//! Spec: docs/spec/obscura_spec_v3.txt Bölüm 6.3

use super::*;
use openmls::prelude::tls_codec::{Deserialize, Serialize};

/// Extract Welcome from an outgoing MLS message; panics if not a Welcome.
fn welcome_out(msg: MlsMessageOut) -> Welcome {
    let bytes = msg.tls_serialize_detached().expect("serialize welcome");
    let in_msg = MlsMessageIn::tls_deserialize_exact(bytes.as_slice()).expect("deserialize");
    match in_msg.extract() {
        MlsMessageBodyIn::Welcome(w) => w,
        other => panic!("expected Welcome, got {:?}", other),
    }
}

#[test]
fn full_two_party_flow() {
    // Setup: Alice and Bob each have their own provider + identity
    let alice_provider = new_provider();
    let bob_provider = new_provider();

    let alice = Identity::generate("did:obs:alice", &alice_provider).unwrap();
    let bob = Identity::generate("did:obs:bob", &bob_provider).unwrap();

    // 1. Bob publishes a KeyPackage so Alice can add him
    let bob_kp = generate_key_package(&bob, &bob_provider).unwrap();

    // 2. Alice creates a group
    let mut alice_group = create_group(&alice, &alice_provider).unwrap();
    assert_eq!(alice_group.epoch().as_u64(), 0);

    // 3. Alice adds Bob (epoch 0 → 1)
    let (_commit, welcome_msg) =
        add_member(&mut alice_group, &alice, bob_kp, &alice_provider).unwrap();
    assert_eq!(alice_group.epoch().as_u64(), 1);

    // 4. Bob processes the Welcome → joins the group
    let welcome = welcome_out(welcome_msg);
    let mut bob_group = process_welcome(welcome, &bob_provider).unwrap();
    assert_eq!(bob_group.epoch().as_u64(), 1);

    // 5. Alice sends an encrypted message
    let plaintext = b"hello bob, this is end-to-end encrypted via MLS";
    let ciphertext = encrypt_message(&mut alice_group, &alice, plaintext, &alice_provider)
        .unwrap();

    // 6. Bob decrypts
    let serialized: Vec<u8> = ciphertext
        .tls_serialize_detached()
        .expect("serialize");
    let in_msg = MlsMessageIn::tls_deserialize_exact(serialized.as_slice())
        .expect("deserialize");

    let decrypted = process_message(&mut bob_group, in_msg, &bob_provider)
        .unwrap()
        .expect("application message expected");

    assert_eq!(decrypted, plaintext);
}

#[test]
fn three_party_message_flow() {
    let p_alice = new_provider();
    let p_bob = new_provider();
    let p_carol = new_provider();

    let alice = Identity::generate("did:obs:alice", &p_alice).unwrap();
    let bob = Identity::generate("did:obs:bob", &p_bob).unwrap();
    let carol = Identity::generate("did:obs:carol", &p_carol).unwrap();

    let bob_kp = generate_key_package(&bob, &p_bob).unwrap();
    let carol_kp = generate_key_package(&carol, &p_carol).unwrap();

    // Alice creates group
    let mut g_alice = create_group(&alice, &p_alice).unwrap();

    // Add Bob (epoch 1)
    let (_c1, w1) = add_member(&mut g_alice, &alice, bob_kp, &p_alice).unwrap();
    let mut g_bob = process_welcome(welcome_out(w1), &p_bob).unwrap();

    // Add Carol (epoch 2 — both Alice's group and Bob's group must process this)
    let (commit2, w2) = add_member(&mut g_alice, &alice, carol_kp, &p_alice).unwrap();
    let mut g_carol = process_welcome(welcome_out(w2), &p_carol).unwrap();

    // Bob processes the commit to advance to epoch 2
    let commit_bytes = commit2.tls_serialize_detached().unwrap();
    let commit_in = MlsMessageIn::tls_deserialize_exact(commit_bytes.as_slice()).unwrap();
    process_message(&mut g_bob, commit_in, &p_bob).unwrap();

    // All at epoch 2
    assert_eq!(g_alice.epoch().as_u64(), 2);
    assert_eq!(g_bob.epoch().as_u64(), 2);
    assert_eq!(g_carol.epoch().as_u64(), 2);

    // Alice sends a message; Bob and Carol decrypt
    let pt = b"hi all from alice";
    let ct = encrypt_message(&mut g_alice, &alice, pt, &p_alice).unwrap();
    let bytes = ct.tls_serialize_detached().unwrap();

    let in_bob = MlsMessageIn::tls_deserialize_exact(bytes.as_slice()).unwrap();
    let dec_bob = process_message(&mut g_bob, in_bob, &p_bob).unwrap().unwrap();
    assert_eq!(dec_bob, pt);

    let in_carol = MlsMessageIn::tls_deserialize_exact(bytes.as_slice()).unwrap();
    let dec_carol = process_message(&mut g_carol, in_carol, &p_carol).unwrap().unwrap();
    assert_eq!(dec_carol, pt);
}
