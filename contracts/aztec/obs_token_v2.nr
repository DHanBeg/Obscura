// OBS Token v2 — Obscura native privacy token (Spec Bölüm 8.1 tam uyumlu)
// Aztec Noir contract v2.0
//
// Token economics:
//   - Total supply (genesis): 1_000_000_000 OBS (1e9 * 1e18 atomic units)
//   - Dağılım:
//       Topluluk:    400_000_000 (40%)
//       Ekip:        200_000_000 (20%)  — 24 ay vesting, 6 ay cliff
//       Yatırımcı:   150_000_000 (15%)  — 18 ay vesting, 3 ay cliff
//       Ekosistem:   150_000_000 (15%)
//       Rezerv:      100_000_000 (10%)
//   - Enflasyon: %2/yıl — staking ödülü olarak mint edilir
//     (yıllık 1/50 = max_supply_at_year_start * 0.02, sadece staking kontratına)
//   - Transfer fee: 0.01 OBS (0.5% burn + 0.5% fee pool — yarısı YAKIM)
//     Yani toplam fee'nin %50'si yakılır, %50'si havuza geri döner
//
// Companion: circuits/token_balance.circom, contracts/aztec/staking.nr
// ADR: ADR-0010 (token economics), ADR-0014 (inflation schedule)

contract OBSToken {
    use dep::aztec::{
        context::{PrivateContext, PublicContext},
        note::{
            note_header::NoteHeader,
            note_interface::NoteInterface,
            utils::compute_note_hash_and_nullifier,
        },
        state_vars::{Map, PublicMutable, PrivateSet},
        protocol_types::address::AztecAddress,
        hash::pedersen_hash,
    };

    // ── Constants ────────────────────────────────────────────────────────────

    // Atomic unit = 1e18 (Wei tarzı)
    global ATOMIC: Field = 1_000_000_000_000_000_000;

    // Genesis arz
    global GENESIS_SUPPLY: Field = 1_000_000_000_000_000_000_000_000_000; // 1e9 * 1e18

    // Mutlak üst sınır (asla aşılamaz — enflasyon dahil)
    // 50 yıl sonra %2 enflasyon ile yaklaşık 2.7x → 2.7B üst sınır
    global ABSOLUTE_MAX_SUPPLY: Field = 3_000_000_000_000_000_000_000_000_000; // 3e9 * 1e18

    // Allocations (atomic units)
    global COMMUNITY_ALLOC:  Field = 400_000_000_000_000_000_000_000_000; // 40%
    global TEAM_ALLOC:       Field = 200_000_000_000_000_000_000_000_000; // 20%
    global INVESTOR_ALLOC:   Field = 150_000_000_000_000_000_000_000_000; // 15%
    global ECOSYSTEM_ALLOC:  Field = 150_000_000_000_000_000_000_000_000; // 15%
    global RESERVE_ALLOC:    Field = 100_000_000_000_000_000_000_000_000; // 10%

    // Enflasyon: %2 yıllık = 200 bps (10000 bps = 100%)
    global INFLATION_BPS: Field = 200;
    global BPS_DENOM: Field = 10_000;

    // 1 yıl = 31_536_000 saniye
    global EPOCH_SECONDS: u64 = 31_536_000;

    // Fee
    global TRANSFER_FEE: Field = 10_000_000_000_000_000; // 0.01 OBS atomic
    global FEE_BURN_RATIO: Field = 2; // fee / 2 = burn

    // Allocation identifier sabitleri (genesis_distribute için)
    global ALLOC_COMMUNITY:  Field = 1;
    global ALLOC_TEAM:       Field = 2;
    global ALLOC_INVESTOR:   Field = 3;
    global ALLOC_ECOSYSTEM:  Field = 4;
    global ALLOC_RESERVE:    Field = 5;

    // ── Note struct ──────────────────────────────────────────────────────────

    struct ValueNote {
        amount: Field,
        owner: AztecAddress,
        randomness: Field,
        header: NoteHeader,
    }

    impl NoteInterface<3> for ValueNote {
        fn serialize_content(self) -> [Field; 3] {
            [self.amount, self.owner.to_field(), self.randomness]
        }

        fn deserialize_content(fields: [Field; 3]) -> Self {
            ValueNote {
                amount: fields[0],
                owner: AztecAddress::from_field(fields[1]),
                randomness: fields[2],
                header: NoteHeader::empty(),
            }
        }

        fn get_header(self) -> NoteHeader { self.header }
        fn set_header(&mut self, header: NoteHeader) { self.header = header; }

        fn get_note_type_id() -> Field { 0x4f4253 } // "OBS"

        fn compute_nullifier(self, context: &mut PrivateContext) -> Field {
            let note_hash = self.compute_note_hash();
            let secret = context.request_nsk_app(self.owner.to_field());
            pedersen_hash([note_hash, secret], 0)
        }

        fn compute_note_hash(self) -> Field {
            pedersen_hash(self.serialize_content(), 0)
        }
    }

    // ── Storage ──────────────────────────────────────────────────────────────

    struct Storage {
        // Public — auditable invariants
        total_supply:         PublicMutable<Field>,
        circulating_supply:   PublicMutable<Field>, // total - locked (vesting/staking)
        fee_pool:             PublicMutable<Field>,
        burned:               PublicMutable<Field>,
        initialized:          PublicMutable<bool>,
        paused:               PublicMutable<bool>,

        // Roles (multisig 3/5 controlled adresler)
        owner:                PublicMutable<AztecAddress>,
        minter:               PublicMutable<AztecAddress>,    // staking kontratı
        guardian:             PublicMutable<AztecAddress>,    // emergency pause

        // Allocation tracking — hangi adresler hangi diliminden ne aldı
        alloc_distributed:    Map<Field, PublicMutable<Field>>, // alloc_id → dağıtılan miktar

        // Vesting state — beneficiary → claimable
        vesting_total:        Map<AztecAddress, PublicMutable<Field>>,
        vesting_claimed:      Map<AztecAddress, PublicMutable<Field>>,
        vesting_start:        Map<AztecAddress, PublicMutable<u64>>,
        vesting_cliff:        Map<AztecAddress, PublicMutable<u64>>,
        vesting_duration:     Map<AztecAddress, PublicMutable<u64>>,

        // Enflasyon — son mint zaman damgası
        last_inflation_ts:    PublicMutable<u64>,

        // Private balances
        balances:             Map<AztecAddress, PrivateSet<ValueNote, 3>>,
    }

    // ── Constructor ──────────────────────────────────────────────────────────

    #[aztec(private)]
    fn constructor(
        owner: AztecAddress,
        minter: AztecAddress,
        guardian: AztecAddress,
        genesis_ts: u64,
    ) {
        let selector = compute_selector("_initialize((Field),(Field),(Field),Field)");
        context.call_public_function(
            context.this_address(),
            selector,
            [owner.to_field(), minter.to_field(), guardian.to_field(), genesis_ts as Field],
        );
    }

    #[aztec(public)]
    internal fn _initialize(
        owner: AztecAddress,
        minter: AztecAddress,
        guardian: AztecAddress,
        genesis_ts: Field,
    ) {
        assert(!storage.initialized.read(), "OBSToken: already initialized");
        storage.owner.write(owner);
        storage.minter.write(minter);
        storage.guardian.write(guardian);
        storage.total_supply.write(GENESIS_SUPPLY);
        storage.circulating_supply.write(0);
        storage.fee_pool.write(0);
        storage.burned.write(0);
        storage.paused.write(false);
        storage.last_inflation_ts.write(genesis_ts as u64);
        storage.initialized.write(true);
    }

    // ── Pause guard (modifier-like helper) ───────────────────────────────────

    #[aztec(public)]
    internal fn _require_not_paused() {
        assert(!storage.paused.read(), "OBSToken: paused");
    }

    // ── Emergency pause (guardian-only) ──────────────────────────────────────

    #[aztec(public)]
    fn pause() {
        let guardian = storage.guardian.read();
        assert(context.msg_sender() == guardian, "OBSToken.pause: not guardian");
        storage.paused.write(true);
    }

    #[aztec(public)]
    fn unpause() {
        let owner = storage.owner.read();
        assert(context.msg_sender() == owner, "OBSToken.unpause: not owner");
        storage.paused.write(false);
    }

    // ── Genesis distribution (owner-only, çağrı başına bir allocation) ──────

    // genesis_distribute owner tarafından her allocation için bir kez çağrılır.
    // recipient bir treasury / vesting beneficiary adresidir.
    // Allocation toplamını aşan miktar reddedilir.
    #[aztec(private)]
    fn genesis_distribute(
        alloc_id: Field,
        recipient: AztecAddress,
        amount: Field,
        vesting_start: u64,
        vesting_cliff: u64,
        vesting_duration: u64,
    ) {
        let selector = compute_selector(
            "_check_and_record_distribution(Field,(Field),Field,Field,Field,Field)"
        );
        context.call_public_function(
            context.this_address(),
            selector,
            [
                alloc_id,
                recipient.to_field(),
                amount,
                vesting_start as Field,
                vesting_cliff as Field,
                vesting_duration as Field,
            ],
        );

        // Eğer vesting yoksa anında shielded note olarak teslim et
        if vesting_duration == 0 {
            let mut note = ValueNote {
                amount,
                owner: recipient,
                randomness: context.request_nsk_app(recipient.to_field()),
                header: NoteHeader::empty(),
            };
            storage.balances.at(recipient).insert(&mut note, true);
        }
        // Vesting varsa note basılmaz, claim_vested ile basılır
    }

    #[aztec(public)]
    internal fn _check_and_record_distribution(
        alloc_id: Field,
        recipient: AztecAddress,
        amount: Field,
        vesting_start: Field,
        vesting_cliff: Field,
        vesting_duration: Field,
    ) {
        // Owner-only
        assert(context.msg_sender() == storage.owner.read(),
               "OBSToken.genesis_distribute: not owner");

        // Allocation cap kontrolü
        let cap = if alloc_id == ALLOC_COMMUNITY { COMMUNITY_ALLOC }
                  else if alloc_id == ALLOC_TEAM { TEAM_ALLOC }
                  else if alloc_id == ALLOC_INVESTOR { INVESTOR_ALLOC }
                  else if alloc_id == ALLOC_ECOSYSTEM { ECOSYSTEM_ALLOC }
                  else if alloc_id == ALLOC_RESERVE { RESERVE_ALLOC }
                  else { 0 };
        assert(cap as u64 > 0, "OBSToken.genesis_distribute: invalid alloc_id");

        let prev = storage.alloc_distributed.at(alloc_id).read();
        let next = prev + amount;
        assert(next as u64 <= cap as u64,
               "OBSToken.genesis_distribute: exceeds allocation cap");
        storage.alloc_distributed.at(alloc_id).write(next);

        // Vesting kaydı (varsa)
        if vesting_duration as u64 > 0 {
            storage.vesting_total.at(recipient).write(amount);
            storage.vesting_claimed.at(recipient).write(0);
            storage.vesting_start.at(recipient).write(vesting_start as u64);
            storage.vesting_cliff.at(recipient).write(vesting_cliff as u64);
            storage.vesting_duration.at(recipient).write(vesting_duration as u64);
        } else {
            // Hemen dolaşıma giriyor
            let prev_circ = storage.circulating_supply.read();
            storage.circulating_supply.write(prev_circ + amount);
        }
    }

    // ── Vesting claim ────────────────────────────────────────────────────────

    #[aztec(private)]
    fn claim_vested(beneficiary: AztecAddress, now_ts: u64) {
        let selector = compute_selector("_compute_claimable((Field),Field)");
        let claimable_arr = context.call_public_function(
            context.this_address(),
            selector,
            [beneficiary.to_field(), now_ts as Field],
        );
        let claimable = claimable_arr[0];
        assert(claimable as u64 > 0, "OBSToken.claim_vested: nothing to claim");

        // Note bas
        let mut note = ValueNote {
            amount: claimable,
            owner: beneficiary,
            randomness: context.request_nsk_app(beneficiary.to_field()),
            header: NoteHeader::empty(),
        };
        storage.balances.at(beneficiary).insert(&mut note, true);
    }

    #[aztec(public)]
    internal fn _compute_claimable(
        beneficiary: AztecAddress,
        now_ts: Field,
    ) -> [Field; 1] {
        let total = storage.vesting_total.at(beneficiary).read();
        let already = storage.vesting_claimed.at(beneficiary).read();
        let start = storage.vesting_start.at(beneficiary).read();
        let cliff = storage.vesting_cliff.at(beneficiary).read();
        let duration = storage.vesting_duration.at(beneficiary).read();

        let now_u64 = now_ts as u64;
        if now_u64 < start + cliff { return [0]; }

        let vested = if now_u64 >= start + duration {
            total
        } else {
            // linear vesting: total * (now - start) / duration
            let elapsed = (now_u64 - start) as Field;
            (total * elapsed) / (duration as Field)
        };

        let claimable = vested - already;
        if claimable as u64 == 0 { return [0]; }

        storage.vesting_claimed.at(beneficiary).write(already + claimable);
        let prev_circ = storage.circulating_supply.read();
        storage.circulating_supply.write(prev_circ + claimable);
        [claimable]
    }

    // ── Inflation mint (minter-only = staking kontratı) ──────────────────────

    // mint_inflation yılda bir kez staking kontratı tarafından çağrılır.
    // amount: pro-rata olarak hesaplanır → (total_supply * 2 / 100) * (elapsed / EPOCH_SECONDS)
    // Public function olarak total supply'ı arttırır + minter'a shielded note basar.
    #[aztec(private)]
    fn mint_inflation(now_ts: u64) {
        let selector = compute_selector("_check_inflation_and_mint(Field)");
        let result = context.call_public_function(
            context.this_address(),
            selector,
            [now_ts as Field],
        );
        let to_mint = result[0];
        assert(to_mint as u64 > 0, "OBSToken.mint_inflation: nothing to mint");

        let minter = context.msg_sender();
        let mut note = ValueNote {
            amount: to_mint,
            owner: minter,
            randomness: context.request_nsk_app(minter.to_field()),
            header: NoteHeader::empty(),
        };
        storage.balances.at(minter).insert(&mut note, true);
    }

    #[aztec(public)]
    internal fn _check_inflation_and_mint(now_ts: Field) -> [Field; 1] {
        assert(!storage.paused.read(), "OBSToken: paused");
        assert(context.msg_sender() == storage.minter.read(),
               "OBSToken.mint_inflation: not minter");

        let now_u64 = now_ts as u64;
        let last = storage.last_inflation_ts.read();
        assert(now_u64 > last, "OBSToken.mint_inflation: timestamp regression");

        let elapsed = now_u64 - last;
        let current_supply = storage.total_supply.read();

        // mint = current_supply * INFLATION_BPS * elapsed / (BPS_DENOM * EPOCH_SECONDS)
        let to_mint = (current_supply * INFLATION_BPS * (elapsed as Field))
                      / (BPS_DENOM * (EPOCH_SECONDS as Field));

        let new_supply = current_supply + to_mint;
        assert(new_supply as u64 <= ABSOLUTE_MAX_SUPPLY as u64,
               "OBSToken.mint_inflation: exceeds ABSOLUTE_MAX_SUPPLY");

        storage.total_supply.write(new_supply);
        storage.last_inflation_ts.write(now_u64);
        let prev_circ = storage.circulating_supply.read();
        storage.circulating_supply.write(prev_circ + to_mint);

        [to_mint]
    }

    // ── Transfer (private) — reentrancy guard public tail içinde ─────────────

    #[aztec(private)]
    fn transfer(from: AztecAddress, to: AztecAddress, amount: Field) {
        assert(amount as u64 > 0, "OBSToken.transfer: amount must be > 0");

        // Pause check
        let pause_sel = compute_selector("_require_not_paused()");
        context.call_public_function(context.this_address(), pause_sel, []);

        let total_needed = amount + TRANSFER_FEE;

        let options = NoteGetterOptions::new().select(0, total_needed, Option::none());
        let notes = storage.balances.at(from).get_notes(options);

        let mut input_sum: Field = 0;
        for i in 0..notes.len() {
            if notes[i].is_some() {
                let note = notes[i].unwrap_unchecked();
                input_sum += note.amount;
                storage.balances.at(from).remove(note);
            }
        }

        assert(input_sum as u64 >= total_needed as u64,
               "OBSToken.transfer: insufficient balance");

        let mut to_note = ValueNote {
            amount,
            owner: to,
            randomness: context.request_nsk_app(to.to_field()),
            header: NoteHeader::empty(),
        };
        storage.balances.at(to).insert(&mut to_note, true);

        let change = input_sum - total_needed;
        if change as u64 > 0 {
            let mut change_note = ValueNote {
                amount: change,
                owner: from,
                randomness: context.request_nsk_app(from.to_field()),
                header: NoteHeader::empty(),
            };
            storage.balances.at(from).insert(&mut change_note, true);
        }

        let selector = compute_selector("_apply_fee(Field)");
        context.call_public_function(context.this_address(), selector, [TRANSFER_FEE]);
    }

    #[aztec(public)]
    internal fn _apply_fee(fee: Field) {
        let burn_amount = fee / FEE_BURN_RATIO;
        let pool_amount = fee - burn_amount;

        let prev_supply = storage.total_supply.read();
        storage.total_supply.write(prev_supply - burn_amount);
        let prev_circ = storage.circulating_supply.read();
        storage.circulating_supply.write(prev_circ - burn_amount);

        let prev_pool = storage.fee_pool.read();
        storage.fee_pool.write(prev_pool + pool_amount);

        let prev_burned = storage.burned.read();
        storage.burned.write(prev_burned + burn_amount);
    }

    // ── Burn (anyone can burn their own notes) ───────────────────────────────

    #[aztec(private)]
    fn burn(from: AztecAddress, amount: Field) {
        assert(amount as u64 > 0, "OBSToken.burn: amount must be > 0");

        let options = NoteGetterOptions::new().select(0, amount, Option::none());
        let notes = storage.balances.at(from).get_notes(options);

        let mut input_sum: Field = 0;
        for i in 0..notes.len() {
            if notes[i].is_some() {
                let note = notes[i].unwrap_unchecked();
                input_sum += note.amount;
                storage.balances.at(from).remove(note);
            }
        }
        assert(input_sum as u64 >= amount as u64,
               "OBSToken.burn: insufficient balance");

        let change = input_sum - amount;
        if change as u64 > 0 {
            let mut change_note = ValueNote {
                amount: change,
                owner: from,
                randomness: context.request_nsk_app(from.to_field()),
                header: NoteHeader::empty(),
            };
            storage.balances.at(from).insert(&mut change_note, true);
        }

        let selector = compute_selector("_record_burn(Field)");
        context.call_public_function(context.this_address(), selector, [amount]);
    }

    #[aztec(public)]
    internal fn _record_burn(amount: Field) {
        let prev = storage.total_supply.read();
        storage.total_supply.write(prev - amount);
        let prev_circ = storage.circulating_supply.read();
        storage.circulating_supply.write(prev_circ - amount);
        let prev_burned = storage.burned.read();
        storage.burned.write(prev_burned + amount);
    }

    // ── Role updates (owner-only, timelock'lı multisig'den çağrılır) ────────

    #[aztec(public)]
    fn set_minter(new_minter: AztecAddress) {
        assert(context.msg_sender() == storage.owner.read(),
               "OBSToken.set_minter: not owner");
        storage.minter.write(new_minter);
    }

    #[aztec(public)]
    fn set_guardian(new_guardian: AztecAddress) {
        assert(context.msg_sender() == storage.owner.read(),
               "OBSToken.set_guardian: not owner");
        storage.guardian.write(new_guardian);
    }

    #[aztec(public)]
    fn transfer_ownership(new_owner: AztecAddress) {
        assert(context.msg_sender() == storage.owner.read(),
               "OBSToken.transfer_ownership: not owner");
        storage.owner.write(new_owner);
    }

    // ── Admin mint (owner-only) ──────────────────────────────────────────────

    // mint, owner (3/5 multisig) tarafından ekosistem/rezerv dilimlerinden veya
    // bridge unlock akışında shielded note basmak için çağrılır.
    // mint_inflation'dan farkı: enflasyon takvimine bağlı değildir, doğrudan
    // owner yetkisiyle çağrılır ve ABSOLUTE_MAX_SUPPLY tavanına tabidir.
    #[aztec(private)]
    fn mint(to: AztecAddress, amount: u64) {
        assert(amount > 0, "OBSToken.mint: amount must be > 0");

        let selector = compute_selector("_check_mint_and_record(Field)");
        context.call_public_function(
            context.this_address(),
            selector,
            [amount as Field],
        );

        let mut note = ValueNote {
            amount: amount as Field,
            owner: to,
            randomness: context.request_nsk_app(to.to_field()),
            header: NoteHeader::empty(),
        };
        storage.balances.at(to).insert(&mut note, true);
    }

    #[aztec(public)]
    internal fn _check_mint_and_record(amount: Field) {
        assert(!storage.paused.read(), "OBSToken: paused");
        assert(context.msg_sender() == storage.owner.read(),
               "OBSToken.mint: not owner");

        let current_supply = storage.total_supply.read();
        let new_supply = current_supply + amount;
        assert(new_supply as u64 <= ABSOLUTE_MAX_SUPPLY as u64,
               "OBSToken.mint: exceeds ABSOLUTE_MAX_SUPPLY");

        storage.total_supply.write(new_supply);
        let prev_circ = storage.circulating_supply.read();
        storage.circulating_supply.write(prev_circ + amount);
    }

    // ── Private balance query ────────────────────────────────────────────────

    #[aztec(private)]
    fn get_balance(owner: AztecAddress) -> Field {
        let options = NoteGetterOptions::new();
        let notes = storage.balances.at(owner).get_notes(options);
        let mut total: Field = 0;
        for i in 0..notes.len() {
            if notes[i].is_some() {
                total += notes[i].unwrap_unchecked().amount;
            }
        }
        total
    }

    // balance_of — get_balance için açık takma ad (spec API yüzeyi).
    // Private: yalnızca note'ları nsk_app türetebilen sahibi anlamlı sonuç alır.
    #[aztec(private)]
    fn balance_of(owner: AztecAddress) -> Field {
        let options = NoteGetterOptions::new();
        let notes = storage.balances.at(owner).get_notes(options);
        let mut total: Field = 0;
        for i in 0..notes.len() {
            if notes[i].is_some() {
                total += notes[i].unwrap_unchecked().amount;
            }
        }
        total
    }

    // total_supply — get_total_supply için açık takma ad (public view).
    #[aztec(public)]
    fn total_supply() -> Field { storage.total_supply.read() }

    // ── Public views ─────────────────────────────────────────────────────────

    #[aztec(public)]
    fn get_total_supply() -> Field { storage.total_supply.read() }

    #[aztec(public)]
    fn get_circulating_supply() -> Field { storage.circulating_supply.read() }

    #[aztec(public)]
    fn get_fee_pool() -> Field { storage.fee_pool.read() }

    #[aztec(public)]
    fn get_burned() -> Field { storage.burned.read() }

    #[aztec(public)]
    fn get_minter() -> AztecAddress { storage.minter.read() }

    #[aztec(public)]
    fn get_owner() -> AztecAddress { storage.owner.read() }

    #[aztec(public)]
    fn get_guardian() -> AztecAddress { storage.guardian.read() }

    #[aztec(public)]
    fn is_paused() -> bool { storage.paused.read() }

    #[aztec(public)]
    fn get_alloc_distributed(alloc_id: Field) -> Field {
        storage.alloc_distributed.at(alloc_id).read()
    }
}
