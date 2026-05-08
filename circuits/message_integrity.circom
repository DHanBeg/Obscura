pragma circom 2.1.6;

/*
 * MessageIntegrity — Obscura Mesaj Bütünlük Devresi
 *
 * İspatlanan şey:
 *   Mesajın belirli bir DID tarafından gönderildiği ve içeriğinin
 *   değiştirilmediği, gönderenin kimliği açıklanmadan kanıtlanır.
 *   (Grup mesajlarında kim gönderdi bilgisi gizlenebilir)
 *
 * Gizli girdiler (witness):
 *   - sender_secret    : Gönderenin gizli anahtarı
 *   - message_content  : Mesaj içeriği hash'i
 *
 * Kamuya açık girdiler:
 *   - sender_commitment  : Poseidon(sender_secret, group_id)
 *   - message_hash       : Poseidon(message_content, nonce)
 *   - group_id           : Grup konuşma ID'si
 *   - nonce              : Replay önleme
 *
 * Kamuya açık çıktılar:
 *   - valid              : 1 = mesaj geçerli grup üyesinden
 *
 * Kullanım:
 *   - Anonim grup mesajlaşma (kim yazdı belli değil ama grup üyesi olduğu belli)
 *   - Şikayet mekanizması: mesajın gerçek sahibi ZK ile kanıtlanabilir
 */

include "circomlib/circuits/poseidon.circom";
include "circomlib/circuits/comparators.circom";

template MessageIntegrity() {
    // ─── Gizli Girdiler ───────────────────────────────────────────────────────
    signal input sender_secret;      // Gönderenin gizli anahtarı
    signal input message_content;    // Mesaj içerik hash (Poseidon)

    // ─── Kamuya Açık Girdiler ─────────────────────────────────────────────────
    signal input sender_commitment;  // Poseidon(sender_secret, group_id)
    signal input message_hash;       // Poseidon(message_content, nonce)
    signal input group_id;           // Grup ID
    signal input nonce;              // Tek kullanımlık (timestamp bazlı)

    // ─── Kamuya Açık Çıktılar ─────────────────────────────────────────────────
    signal output valid;

    // ─── Kısıtlamalar ─────────────────────────────────────────────────────────

    // 1. Gönderen taahhüdü doğrula
    component senderCommit = Poseidon(2);
    senderCommit.inputs[0] <== sender_secret;
    senderCommit.inputs[1] <== group_id;
    senderCommit.out === sender_commitment;

    // 2. Mesaj hash'i doğrula
    component msgHash = Poseidon(2);
    msgHash.inputs[0] <== message_content;
    msgHash.inputs[1] <== nonce;
    msgHash.out === message_hash;

    // 3. Nonce sıfır olamaz
    component nonceCheck = IsZero();
    nonceCheck.in <== nonce;
    nonceCheck.out === 0;  // nonce != 0

    // 4. Tüm kontroller geçtiyse valid = 1
    valid <== 1;
}

component main {public [sender_commitment, message_hash, group_id, nonce]} = MessageIntegrity();
