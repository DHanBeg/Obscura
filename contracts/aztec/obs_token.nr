// OBS Token — Obscura native privacy token
// Aztec Noir contract (STUB for FAZ 2)
//
// Full implementation: FAZ 2 GA per ADR-0005.
// Token economics: see ADR-0010 (supply 1e9, 18 decimals, transfer fee 0.01 OBS,
// 50% burn / 50% fee pool).
// Companion Circom design (in-house Merkle UTXO bridge): circuits/token_balance.circom
//
// Status: signatures and storage layout only; no proving logic.
// Do NOT deploy.

contract OBSToken {
    use dep::aztec::{
        context::{PrivateContext, PublicContext},
        state_vars::{Map, PublicMutable},
        protocol_types::address::AztecAddress,
    };

    // ── Storage ─────────────────────────────────────────────────────────────
    //
    // Shielded balances live as private notes per owner (Aztec UTXO model).
    // Total supply and the fee-pool sink are public so the protocol can audit
    // the invariant (sum-of-notes + burned == 1e9 * 1e18).
    //
    // The `ShieldedBalance` note struct mirrors the `Balance` signal in
    // circuits/token_balance.circom so the off-chain Merkle bridge and the
    // on-chain Aztec notes can interop during the FAZ 2 → FAZ 3 migration.
    struct ShieldedBalance {
        owner: AztecAddress,   // who can spend this note
        amount: Field,         // OBS in atomic units (1e-18 OBS)
        nonce: Field,          // anti-replay; checked against nullifier tree
    }

    struct Storage {
        total_supply: PublicMutable<Field>,
        fee_pool:     PublicMutable<Field>,
        // Per-owner note set is implicit in Aztec's PXE; no explicit Map here.
    }

    // ── Public entrypoints ──────────────────────────────────────────────────

    // mint creates a new shielded note crediting `to` with `amount` OBS.
    // Only the protocol minter (set at deploy) may call this. Increments
    // total_supply. Reverts if total_supply would exceed 1e9 * 1e18.
    //
    // FAZ 2 GA: gated by a multisig minter address; FAZ 3: governance-gated
    // per ADR-0012.
    #[aztec(public)]
    fn mint(amount: Field, to: AztecAddress) {
        // STUB. Real implementation:
        //   1. assert(msg_sender == minter)
        //   2. assert(storage.total_supply.read() + amount <= MAX_SUPPLY)
        //   3. emit_encrypted_note(ShieldedBalance { owner: to, amount, nonce })
        //   4. storage.total_supply.write(prev + amount)
        assert(false, "OBSToken.mint: not yet implemented (FAZ 2 GA)");
    }

    // ── Private entrypoints ─────────────────────────────────────────────────

    // transfer spends one or more notes owned by `from`, creates a new note
    // for `to` with `amount`, applies the 0.01 OBS fee (50% burn, 50% to
    // fee_pool), and emits the change note back to `from`. All amounts hidden;
    // only the fee accounting touches public state.
    //
    // Companion circuit: circuits/token_balance.circom proves the same
    // invariant for the in-house Merkle path used in Phase 0.
    #[aztec(private)]
    fn transfer(from: AztecAddress, to: AztecAddress, amount: Field) {
        // STUB. Real implementation:
        //   1. select input notes covering amount + fee
        //   2. nullify input notes
        //   3. create output note(s): {to, amount}, {from, change}
        //   4. public-call burn(fee/2) and fee_pool += fee/2
        assert(false, "OBSToken.transfer: not yet implemented (FAZ 2 GA)");
    }

    // ── Views ───────────────────────────────────────────────────────────────

    #[aztec(public)]
    fn get_total_supply() -> Field {
        // STUB
        0
    }
}
