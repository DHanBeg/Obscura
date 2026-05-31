// OBS Proof Registry — Immutable ZK proof log (Spec Bölüm 7.3 Adım 7)
// Aztec Noir contract v1.0
//
// Amaç:
//   Kredi tier upgrade, governance vote, location proof gibi event'ler için
//   üretilen ZK kanıtların hash'lerini on-chain immutable kaydetmek.
//   Bir kanıt bir kez kaydedildikten sonra hiçbir şekilde silinemez/değiştirilemez.
//
// Saklanan veriler (her kayıt için):
//   - proof_hash: snarkjs proof'un Poseidon hash'i (Field)
//   - circuit_id: hangi circuit (1=credit, 2=vote, 3=storage, ... — bkz CIRCUIT_*)
//   - user_commitment: kullanıcının kimliğinin commitment'ı (DID hash, anonim)
//   - timestamp: kayıt zamanı
//   - block_height: kayıt edildiği Aztec block (otomatik)
//
// Sorgular:
//   - verify_proof_exists(proof_hash) → bool
//   - get_proof_metadata(proof_hash) → (circuit_id, user_commitment, timestamp)
//   - get_proof_count_by_user(user_commitment) → kaç proof basmış
//   - get_proof_count_by_circuit(circuit_id) → toplam circuit kullanımı
//
// Replay koruması: aynı proof_hash iki kez kayıt edilemez.

contract OBSProofRegistry {
    use dep::aztec::{
        context::{PrivateContext, PublicContext},
        state_vars::{Map, PublicMutable},
        protocol_types::address::AztecAddress,
        hash::pedersen_hash,
    };

    // ── Circuit ID sabitleri (spec aligned) ──────────────────────────────────

    global CIRCUIT_CREDIT_THRESHOLD: Field = 1;
    global CIRCUIT_IDENTITY_PROOF:   Field = 2;
    global CIRCUIT_MESSAGE_INTEGRITY: Field = 3;
    global CIRCUIT_TOKEN_BALANCE:    Field = 4;
    global CIRCUIT_VOTE_PROOF:       Field = 5;
    global CIRCUIT_STORAGE_PROOF:    Field = 6;
    global CIRCUIT_AGE_PROOF:        Field = 7;
    global CIRCUIT_ACTIVITY_PROOF:   Field = 8;
    global CIRCUIT_MSG_COUNT_PROOF:  Field = 9;
    global CIRCUIT_NODE_PROOF:       Field = 10;
    global CIRCUIT_ENDORSEMENT_PROOF: Field = 11;
    global CIRCUIT_STREAK_PROOF:     Field = 12;
    global CIRCUIT_LOCATION_PROOF:   Field = 13;
    global CIRCUIT_RECURSIVE:        Field = 14;
    global CIRCUIT_ZKML_MODERATION:  Field = 15;

    global MAX_CIRCUIT_ID: Field = 100; // güvenlik üst sınırı

    // ── Storage ──────────────────────────────────────────────────────────────

    struct ProofRecord {
        proof_hash:      Field,
        circuit_id:      Field,
        user_commitment: Field,
        timestamp:       u64,
        block_height:    u64,
    }

    struct Storage {
        // proof_hash → ProofRecord (exists check için circuit_id == 0 means absent)
        records:                Map<Field, PublicMutable<ProofRecord>>,

        // Stats
        total_proofs:           PublicMutable<Field>,
        circuit_counts:         Map<Field, PublicMutable<Field>>,    // circuit_id → adet
        user_counts:            Map<Field, PublicMutable<Field>>,    // user_commitment → adet

        // Init flag
        initialized:            PublicMutable<bool>,

        // Whitelist: hangi adresler proof basabilir (örn. node operatörleri)
        // 0x0 yazıldıysa herkese açık (permissionless)
        gatekeeper:             PublicMutable<AztecAddress>,
    }

    // ── Constructor ──────────────────────────────────────────────────────────

    #[aztec(private)]
    fn constructor(gatekeeper: AztecAddress) {
        let selector = compute_selector("_initialize((Field))");
        context.call_public_function(
            context.this_address(),
            selector,
            [gatekeeper.to_field()],
        );
    }

    #[aztec(public)]
    internal fn _initialize(gatekeeper: AztecAddress) {
        assert(!storage.initialized.read(), "OBSProofRegistry: already initialized");
        storage.gatekeeper.write(gatekeeper);
        storage.total_proofs.write(0);
        storage.initialized.write(true);
    }

    // ── Register proof (immutable insert) ────────────────────────────────────

    // register_proof: bir ZK proof hash'ini kayıt eder.
    // - proof_hash mutlaka unique olmalı (replay protection)
    // - circuit_id valid range içinde olmalı
    // - Gatekeeper set edilmişse sadece gatekeeper çağırabilir
    #[aztec(public)]
    fn register_proof(
        proof_hash: Field,
        circuit_id: Field,
        user_commitment: Field,
        timestamp: Field,
    ) {
        assert(proof_hash as u64 > 0, "OBSProofRegistry: proof_hash cannot be zero");
        assert(circuit_id as u64 > 0 && circuit_id as u64 <= MAX_CIRCUIT_ID as u64,
               "OBSProofRegistry: invalid circuit_id");
        assert(user_commitment as u64 > 0, "OBSProofRegistry: user_commitment cannot be zero");

        // Gatekeeper kontrolü (0x0 = permissionless)
        let gk = storage.gatekeeper.read();
        let gk_field = gk.to_field();
        if gk_field as u64 != 0 {
            assert(context.msg_sender() == gk,
                   "OBSProofRegistry: not gatekeeper");
        }

        // Replay koruması: bu hash daha önce kayıt olduysa reddet
        let existing = storage.records.at(proof_hash).read();
        assert(existing.circuit_id as u64 == 0,
               "OBSProofRegistry: proof_hash already registered");

        // Kayıt oluştur
        let record = ProofRecord {
            proof_hash,
            circuit_id,
            user_commitment,
            timestamp: timestamp as u64,
            block_height: context.block_number() as u64,
        };
        storage.records.at(proof_hash).write(record);

        // Sayaçlar
        let prev_total = storage.total_proofs.read();
        storage.total_proofs.write(prev_total + 1);

        let prev_circ = storage.circuit_counts.at(circuit_id).read();
        storage.circuit_counts.at(circuit_id).write(prev_circ + 1);

        let prev_user = storage.user_counts.at(user_commitment).read();
        storage.user_counts.at(user_commitment).write(prev_user + 1);
    }

    // ── Batch register (gas optimization — birden fazla proof tek tx'te) ─────

    // batch_register_proofs: en fazla 16 proof tek transaction'da kayıt eder.
    // Tüm proof'lar aynı circuit'ten gelmek zorunda.
    #[aztec(public)]
    fn batch_register_proofs(
        proof_hashes:    [Field; 16],
        circuit_id:      Field,
        user_commitments: [Field; 16],
        timestamps:      [Field; 16],
        valid_count:     Field, // gerçekten doldurulan eleman sayısı (1..16)
    ) {
        assert(valid_count as u64 > 0 && valid_count as u64 <= 16,
               "OBSProofRegistry: invalid batch size");
        assert(circuit_id as u64 > 0 && circuit_id as u64 <= MAX_CIRCUIT_ID as u64,
               "OBSProofRegistry: invalid circuit_id");

        let gk = storage.gatekeeper.read();
        let gk_field = gk.to_field();
        if gk_field as u64 != 0 {
            assert(context.msg_sender() == gk,
                   "OBSProofRegistry: not gatekeeper");
        }

        let count = valid_count as u64;
        let mut inserted: Field = 0;

        for i in 0..16 {
            if (i as u64) < count {
                let ph = proof_hashes[i];
                let uc = user_commitments[i];
                let ts = timestamps[i];

                assert(ph as u64 > 0, "OBSProofRegistry.batch: zero proof_hash");
                assert(uc as u64 > 0, "OBSProofRegistry.batch: zero commitment");

                let existing = storage.records.at(ph).read();
                assert(existing.circuit_id as u64 == 0,
                       "OBSProofRegistry.batch: duplicate proof_hash");

                let rec = ProofRecord {
                    proof_hash: ph,
                    circuit_id,
                    user_commitment: uc,
                    timestamp: ts as u64,
                    block_height: context.block_number() as u64,
                };
                storage.records.at(ph).write(rec);

                let pu = storage.user_counts.at(uc).read();
                storage.user_counts.at(uc).write(pu + 1);

                inserted += 1;
            }
        }

        let prev_total = storage.total_proofs.read();
        storage.total_proofs.write(prev_total + inserted);

        let prev_circ = storage.circuit_counts.at(circuit_id).read();
        storage.circuit_counts.at(circuit_id).write(prev_circ + inserted);
    }

    // ── Queries ──────────────────────────────────────────────────────────────

    // verify_proof_exists: bir proof_hash'in kayıtlı olup olmadığını döner
    #[aztec(public)]
    fn verify_proof_exists(proof_hash: Field) -> bool {
        let rec = storage.records.at(proof_hash).read();
        rec.circuit_id as u64 > 0
    }

    // get_proof_metadata: kayıtlı bir proof'un metadata'sını döner
    // Yoksa circuit_id == 0 olan boş record döner (caller exists check yapmalı)
    #[aztec(public)]
    fn get_proof_metadata(proof_hash: Field) -> ProofRecord {
        storage.records.at(proof_hash).read()
    }

    // Toplam kayıt sayısı
    #[aztec(public)]
    fn get_total_proofs() -> Field {
        storage.total_proofs.read()
    }

    // Belirli bir circuit için kayıt sayısı
    #[aztec(public)]
    fn get_proof_count_by_circuit(circuit_id: Field) -> Field {
        storage.circuit_counts.at(circuit_id).read()
    }

    // Belirli bir kullanıcı commitment için kayıt sayısı
    #[aztec(public)]
    fn get_proof_count_by_user(user_commitment: Field) -> Field {
        storage.user_counts.at(user_commitment).read()
    }

    // ── Admin ────────────────────────────────────────────────────────────────

    // set_gatekeeper: gatekeeper rolünü değiştir
    // (Sadece mevcut gatekeeper veya gatekeeper == 0x0 ise herkes — initial setup)
    #[aztec(public)]
    fn set_gatekeeper(new_gatekeeper: AztecAddress) {
        let current = storage.gatekeeper.read();
        let current_field = current.to_field();
        if current_field as u64 != 0 {
            assert(context.msg_sender() == current,
                   "OBSProofRegistry.set_gatekeeper: not gatekeeper");
        }
        storage.gatekeeper.write(new_gatekeeper);
    }

    #[aztec(public)]
    fn get_gatekeeper() -> AztecAddress {
        storage.gatekeeper.read()
    }
}
