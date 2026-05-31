// OBS Token — Obscura native privacy token
// Aztec Noir contract v1.0
//
// Token economics (ADR-0010):
//   - Total supply: 1_000_000_000 OBS (1e9 * 1e18 atomic units)
//   - Transfer fee: 0.01 OBS per transfer
//   - Fee split: 50% burn (reduces supply), 50% fee pool
//   - Shielded balances: Aztec UTXO note model (private by default)
//
// Companion circuit: circuits/token_balance.circom (off-chain Merkle bridge)

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

    global MAX_SUPPLY: Field = 1_000_000_000_000_000_000_000_000_000; // 1e9 * 1e18
    global TRANSFER_FEE: Field = 10_000_000_000_000_000;              // 0.01 OBS in atomic units
    global FEE_BURN_RATIO: Field = 2;                                  // 50% burn (fee / 2)

    // ── Note struct ──────────────────────────────────────────────────────────

    // ValueNote: a single shielded balance note owned by one address.
    // Stored in the owner's private note set; nullified on spend.
    struct ValueNote {
        amount: Field,
        owner: AztecAddress,
        randomness: Field,  // blinding factor — prevents note linkability
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

        fn get_note_type_id() -> Field { 0x4f4253 } // "OBS" in hex

        fn compute_nullifier(self, context: &mut PrivateContext) -> Field {
            let note_hash = self.compute_note_hash();
            let secret = context.request_nsk_app(self.owner.to_field());
            pedersen_hash([note_hash, secret], 0)
        }

        fn compute_note_hash(self) -> Field {
            pedersen_hash(self.serialize_content(), 0)
        }
    }

    // ── Storage ───────────────────────────────────────────────────────────────

    struct Storage {
        // Public: auditable supply invariant
        total_supply: PublicMutable<Field>,
        fee_pool:     PublicMutable<Field>,
        burned:       PublicMutable<Field>,
        minter:       PublicMutable<AztecAddress>,
        initialized:  PublicMutable<bool>,

        // Private: per-owner shielded note sets
        balances: Map<AztecAddress, PrivateSet<ValueNote, 3>>,
    }

    // ── Constructor ───────────────────────────────────────────────────────────

    #[aztec(private)]
    fn constructor(minter: AztecAddress) {
        let selector = compute_selector("_initialize((Field))");
        context.call_public_function(
            context.this_address(),
            selector,
            [minter.to_field()],
        );
    }

    #[aztec(public)]
    internal fn _initialize(minter: AztecAddress) {
        assert(!storage.initialized.read(), "OBSToken: already initialized");
        storage.minter.write(minter);
        storage.total_supply.write(0);
        storage.fee_pool.write(0);
        storage.burned.write(0);
        storage.initialized.write(true);
    }

    // ── Mint (public entrypoint, minter-only) ─────────────────────────────────

    // mint creates a shielded note crediting `to` with `amount` OBS.
    // Only the designated minter may call this. Increments total_supply.
    // Reverts if total_supply would exceed MAX_SUPPLY.
    #[aztec(private)]
    fn mint(amount: Field, to: AztecAddress) {
        assert(amount as u64 > 0, "OBSToken.mint: amount must be > 0");

        // Minter check happens in the public tail call
        let selector = compute_selector("_check_and_increment_supply(Field)");
        context.call_public_function(
            context.this_address(),
            selector,
            [amount],
        );

        // Emit private note to recipient
        let mut note = ValueNote {
            amount,
            owner: to,
            randomness: context.request_nsk_app(to.to_field()),
            header: NoteHeader::empty(),
        };
        storage.balances.at(to).insert(&mut note, true);
    }

    #[aztec(public)]
    internal fn _check_and_increment_supply(amount: Field) {
        let minter = storage.minter.read();
        assert(
            context.msg_sender() == minter,
            "OBSToken._check_and_increment_supply: not minter"
        );
        let prev = storage.total_supply.read();
        let next = prev + amount;
        assert(next as u64 <= MAX_SUPPLY as u64, "OBSToken.mint: exceeds MAX_SUPPLY");
        storage.total_supply.write(next);
    }

    // ── Transfer (private) ────────────────────────────────────────────────────

    // transfer spends a note owned by `from` for `amount + TRANSFER_FEE`,
    // creates a note for `to` with `amount`, returns change note to `from`.
    // Fee split: burn 50%, fee_pool 50% (via public tail call).
    #[aztec(private)]
    fn transfer(from: AztecAddress, to: AztecAddress, amount: Field) {
        assert(amount as u64 > 0, "OBSToken.transfer: amount must be > 0");

        let total_needed = amount + TRANSFER_FEE;

        // Select and nullify input note(s) from sender's note set.
        // Aztec PXE will pick notes summing to >= total_needed.
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

        assert(
            input_sum as u64 >= total_needed as u64,
            "OBSToken.transfer: insufficient balance"
        );

        // Recipient note
        let mut to_note = ValueNote {
            amount,
            owner: to,
            randomness: context.request_nsk_app(to.to_field()),
            header: NoteHeader::empty(),
        };
        storage.balances.at(to).insert(&mut to_note, true);

        // Change note back to sender (if any)
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

        // Public tail: burn half fee, credit half to pool
        let selector = compute_selector("_apply_fee(Field)");
        context.call_public_function(
            context.this_address(),
            selector,
            [TRANSFER_FEE],
        );
    }

    #[aztec(public)]
    internal fn _apply_fee(fee: Field) {
        let burn_amount = fee / FEE_BURN_RATIO;
        let pool_amount = fee - burn_amount;

        // Reduce supply by burned amount
        let prev_supply = storage.total_supply.read();
        storage.total_supply.write(prev_supply - burn_amount);

        // Accumulate fee pool
        let prev_pool = storage.fee_pool.read();
        storage.fee_pool.write(prev_pool + pool_amount);

        // Track total burned
        let prev_burned = storage.burned.read();
        storage.burned.write(prev_burned + burn_amount);
    }

    // ── Shielded balance query (private) ──────────────────────────────────────

    // get_balance returns the sum of all private notes owned by `owner`.
    // Only the owner can call this (PXE enforces note access).
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

    // ── Public views ──────────────────────────────────────────────────────────

    #[aztec(public)]
    fn get_total_supply() -> Field {
        storage.total_supply.read()
    }

    #[aztec(public)]
    fn get_fee_pool() -> Field {
        storage.fee_pool.read()
    }

    #[aztec(public)]
    fn get_burned() -> Field {
        storage.burned.read()
    }

    #[aztec(public)]
    fn get_minter() -> AztecAddress {
        storage.minter.read()
    }
}
