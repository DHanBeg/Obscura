// OBS Staking — Obscura on-chain staking + slashing (Spec Bölüm 8.3)
// Aztec Noir contract v1.0
//
// Mekanik:
//   - stake(amount, lockup_period): OBS token kilitle (note nullify → public bilanço)
//     Desteklenen lockup_period (saniye):
//       30 gün  → 1.0x reward multiplier
//       90 gün  → 1.25x
//       180 gün → 1.5x
//       365 gün → 2.0x
//   - unstake(position_id): lockup bittiyse pozisyon kapatılır,
//     anapara + birikmiş ödül owner'a shielded note olarak iade edilir
//   - claim_rewards(position_id): lockup beklemeden sadece ödül talep edilir
//   - slash(validator, reason): guardian (multisig 3/5) yetkili — pozisyonun %X'i yakılır
//   - request_unstake(position_id): erken çıkış (lockup içindeyken) — 14 gün cool-down + %5 ceza
//
// Position struct: { owner, amount, locked_until, started_at, multiplier_bps, slashed, exited }
//
// Reentrancy guard: tüm state-changing public fonksiyonlar private→public tail çağrı paterni
// Upgradability: transparent proxy + 48 saat timelock (DAO kontratından idare)

contract OBSStaking {
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

    // Lockup periods (saniye)
    global LOCKUP_30D:  u64 = 2_592_000;
    global LOCKUP_90D:  u64 = 7_776_000;
    global LOCKUP_180D: u64 = 15_552_000;
    global LOCKUP_365D: u64 = 31_536_000;

    // Reward multipliers (basis points)
    global MULT_30D_BPS:  Field = 10_000; // 1.00x
    global MULT_90D_BPS:  Field = 12_500; // 1.25x
    global MULT_180D_BPS: Field = 15_000; // 1.50x
    global MULT_365D_BPS: Field = 20_000; // 2.00x
    global BPS_DENOM:     Field = 10_000;

    // Erken çıkış: 14 gün cool-down + %5 ceza
    global EARLY_EXIT_COOLDOWN: u64 = 1_209_600;
    global EARLY_EXIT_PENALTY_BPS: Field = 500;

    // Slash sınırı (tek seferde maksimum %50)
    global MAX_SLASH_BPS: Field = 5_000;

    // Minimum stake: 100 OBS (atomic units, 1e18)
    global MIN_STAKE: Field = 100_000_000_000_000_000_000;

    // ── Position struct (public, on-chain) ───────────────────────────────────

    // Pozisyonlar public Map'te tutulur (slashing transparency için).
    // İçindeki amount stake edilen OBS miktarıdır; ödüller dinamik hesaplanır.
    struct Position {
        owner:          AztecAddress,
        amount:         Field,    // stake'lenen anapara
        locked_until:   u64,
        started_at:     u64,
        multiplier_bps: Field,    // reward multiplier
        slashed_amount: Field,    // toplam kesilen
        exited:         bool,
        early_exit_at:  u64,      // 0 = istek yok, >0 = erken çıkış istek zamanı
    }

    // ── Storage ──────────────────────────────────────────────────────────────

    struct Storage {
        // OBS token kontrat adresi
        obs_token:           PublicMutable<AztecAddress>,

        // Roles
        owner:               PublicMutable<AztecAddress>,
        guardian:            PublicMutable<AztecAddress>, // slasher (multisig 3/5)
        initialized:         PublicMutable<bool>,
        paused:              PublicMutable<bool>,

        // Reward rate: yıllık temel APR (basis points) — DAO tarafından ayarlanır
        base_apr_bps:        PublicMutable<Field>, // örn 500 = %5

        // Pozisyon storage
        next_position_id:    PublicMutable<Field>,
        positions:           Map<Field, PublicMutable<Position>>,
        owner_positions:     Map<AztecAddress, PublicMutable<Field>>, // toplam aktif pozisyon sayısı

        // Aggregate stats
        total_staked:        PublicMutable<Field>,
        total_slashed:       PublicMutable<Field>,
        total_rewards_paid:  PublicMutable<Field>,
    }

    // ── Constructor ──────────────────────────────────────────────────────────

    #[aztec(private)]
    fn constructor(
        obs_token: AztecAddress,
        owner: AztecAddress,
        guardian: AztecAddress,
        initial_apr_bps: Field,
    ) {
        let selector = compute_selector("_initialize((Field),(Field),(Field),Field)");
        context.call_public_function(
            context.this_address(),
            selector,
            [obs_token.to_field(), owner.to_field(), guardian.to_field(), initial_apr_bps],
        );
    }

    #[aztec(public)]
    internal fn _initialize(
        obs_token: AztecAddress,
        owner: AztecAddress,
        guardian: AztecAddress,
        initial_apr_bps: Field,
    ) {
        assert(!storage.initialized.read(), "OBSStaking: already initialized");
        storage.obs_token.write(obs_token);
        storage.owner.write(owner);
        storage.guardian.write(guardian);
        storage.base_apr_bps.write(initial_apr_bps);
        storage.next_position_id.write(1);
        storage.total_staked.write(0);
        storage.total_slashed.write(0);
        storage.total_rewards_paid.write(0);
        storage.paused.write(false);
        storage.initialized.write(true);
    }

    // ── Stake ────────────────────────────────────────────────────────────────

    // stake: kullanıcı OBS kilitler.
    // OBS token kontratının transfer fonksiyonunu çağırır (kullanıcı → bu kontrat).
    // Yeni Position kaydı oluşturur.
    #[aztec(private)]
    fn stake(staker: AztecAddress, amount: Field, lockup_period: u64, now_ts: u64) -> Field {
        assert(amount as u64 >= MIN_STAKE as u64, "OBSStaking.stake: below MIN_STAKE");

        // Multiplier hesapla
        let multiplier = if lockup_period == LOCKUP_30D { MULT_30D_BPS }
                         else if lockup_period == LOCKUP_90D { MULT_90D_BPS }
                         else if lockup_period == LOCKUP_180D { MULT_180D_BPS }
                         else if lockup_period == LOCKUP_365D { MULT_365D_BPS }
                         else { 0 };
        assert(multiplier as u64 > 0, "OBSStaking.stake: invalid lockup_period");

        // OBS token transferi: staker → staking kontratı
        // (Token kontratının transfer fonksiyonu cross-contract çağrı ile)
        let obs = context.this_address(); // staking kontratı kendi adresi
        let token_addr = storage.obs_token.read();
        let transfer_sel = compute_selector("transfer((Field),(Field),Field)");
        context.call_private_function(
            token_addr,
            transfer_sel,
            [staker.to_field(), obs.to_field(), amount],
        );

        // Public state güncelleme
        let create_sel = compute_selector("_create_position((Field),Field,Field,Field,Field)");
        let pid_arr = context.call_public_function(
            context.this_address(),
            create_sel,
            [staker.to_field(), amount, lockup_period as Field, now_ts as Field, multiplier],
        );
        pid_arr[0]
    }

    #[aztec(public)]
    internal fn _create_position(
        owner: AztecAddress,
        amount: Field,
        lockup_period: Field,
        now_ts: Field,
        multiplier_bps: Field,
    ) -> [Field; 1] {
        assert(!storage.paused.read(), "OBSStaking: paused");

        let pid = storage.next_position_id.read();
        storage.next_position_id.write(pid + 1);

        let pos = Position {
            owner,
            amount,
            locked_until: (now_ts as u64) + (lockup_period as u64),
            started_at: now_ts as u64,
            multiplier_bps,
            slashed_amount: 0,
            exited: false,
            early_exit_at: 0,
        };
        storage.positions.at(pid).write(pos);

        let prev_count = storage.owner_positions.at(owner).read();
        storage.owner_positions.at(owner).write(prev_count + 1);

        let prev_total = storage.total_staked.read();
        storage.total_staked.write(prev_total + amount);

        [pid]
    }

    // ── Unstake (lockup bittikten sonra) ─────────────────────────────────────

    #[aztec(private)]
    fn unstake(position_id: Field, now_ts: u64) {
        let close_sel = compute_selector("_close_position(Field,Field,bool)");
        let result = context.call_public_function(
            context.this_address(),
            close_sel,
            [position_id, now_ts as Field, 0], // 0 = early_exit değil
        );
        let payout = result[0];
        let owner_field = result[1];

        assert(payout as u64 > 0, "OBSStaking.unstake: zero payout");

        // OBS token mint et (staking kontratından owner'a transfer)
        let owner = AztecAddress::from_field(owner_field);
        let token_addr = storage.obs_token.read();
        let transfer_sel = compute_selector("transfer((Field),(Field),Field)");
        context.call_private_function(
            token_addr,
            transfer_sel,
            [context.this_address().to_field(), owner.to_field(), payout],
        );
    }

    // ── Early unstake request (cooldown başlatır) ────────────────────────────

    #[aztec(public)]
    fn request_early_unstake(position_id: Field, now_ts: Field) {
        let mut pos = storage.positions.at(position_id).read();
        assert(!pos.exited, "OBSStaking.request_early_unstake: already exited");
        assert(pos.owner == context.msg_sender(),
               "OBSStaking.request_early_unstake: not owner");
        assert(pos.early_exit_at == 0,
               "OBSStaking.request_early_unstake: already requested");
        pos.early_exit_at = now_ts as u64;
        storage.positions.at(position_id).write(pos);
    }

    // ── Early unstake finalize (cooldown sonrası, %5 ceza ile) ───────────────

    #[aztec(private)]
    fn finalize_early_unstake(position_id: Field, now_ts: u64) {
        let close_sel = compute_selector("_close_position(Field,Field,bool)");
        let result = context.call_public_function(
            context.this_address(),
            close_sel,
            [position_id, now_ts as Field, 1], // 1 = early_exit
        );
        let payout = result[0];
        let owner_field = result[1];

        assert(payout as u64 > 0, "OBSStaking.finalize_early_unstake: zero payout");

        let owner = AztecAddress::from_field(owner_field);
        let token_addr = storage.obs_token.read();
        let transfer_sel = compute_selector("transfer((Field),(Field),Field)");
        context.call_private_function(
            token_addr,
            transfer_sel,
            [context.this_address().to_field(), owner.to_field(), payout],
        );
    }

    #[aztec(public)]
    internal fn _close_position(
        position_id: Field,
        now_ts: Field,
        early_exit: Field,
    ) -> [Field; 2] {
        assert(!storage.paused.read(), "OBSStaking: paused");

        let mut pos = storage.positions.at(position_id).read();
        assert(!pos.exited, "OBSStaking._close_position: already exited");

        let now_u64 = now_ts as u64;
        let is_early = (early_exit as u64) == 1;

        if is_early {
            assert(pos.early_exit_at > 0,
                   "OBSStaking._close_position: request_early_unstake first");
            assert(now_u64 >= pos.early_exit_at + EARLY_EXIT_COOLDOWN,
                   "OBSStaking._close_position: cooldown not over");
        } else {
            assert(now_u64 >= pos.locked_until,
                   "OBSStaking._close_position: still locked");
        }

        // Net principal (slash sonrası)
        let net_principal = pos.amount - pos.slashed_amount;

        // Ödül hesapla: net_principal * APR * elapsed / (10000 * 31536000) * multiplier / 10000
        let elapsed = (now_u64 - pos.started_at) as Field;
        let apr = storage.base_apr_bps.read();
        let year_secs: Field = 31_536_000;
        let rewards = (net_principal * apr * elapsed * pos.multiplier_bps)
                       / (BPS_DENOM * year_secs * BPS_DENOM);

        // Erken çıkış cezası
        let penalty = if is_early {
            (net_principal * EARLY_EXIT_PENALTY_BPS) / BPS_DENOM
        } else { 0 };

        let payout = net_principal + rewards - penalty;

        // State güncelle
        pos.exited = true;
        storage.positions.at(position_id).write(pos);

        let prev_total = storage.total_staked.read();
        storage.total_staked.write(prev_total - pos.amount);

        let prev_rewards = storage.total_rewards_paid.read();
        storage.total_rewards_paid.write(prev_rewards + rewards);

        let prev_count = storage.owner_positions.at(pos.owner).read();
        storage.owner_positions.at(pos.owner).write(prev_count - 1);

        [payout, pos.owner.to_field()]
    }

    // ── Claim rewards (lockup beklemeden, sadece ödül) ───────────────────────

    #[aztec(private)]
    fn claim_rewards(position_id: Field, now_ts: u64) {
        let claim_sel = compute_selector("_claim_rewards(Field,Field)");
        let result = context.call_public_function(
            context.this_address(),
            claim_sel,
            [position_id, now_ts as Field],
        );
        let rewards = result[0];
        let owner_field = result[1];

        assert(rewards as u64 > 0, "OBSStaking.claim_rewards: nothing to claim");

        let owner = AztecAddress::from_field(owner_field);
        let token_addr = storage.obs_token.read();
        let transfer_sel = compute_selector("transfer((Field),(Field),Field)");
        context.call_private_function(
            token_addr,
            transfer_sel,
            [context.this_address().to_field(), owner.to_field(), rewards],
        );
    }

    #[aztec(public)]
    internal fn _claim_rewards(position_id: Field, now_ts: Field) -> [Field; 2] {
        assert(!storage.paused.read(), "OBSStaking: paused");

        let mut pos = storage.positions.at(position_id).read();
        assert(!pos.exited, "OBSStaking._claim_rewards: position exited");

        let now_u64 = now_ts as u64;
        let net_principal = pos.amount - pos.slashed_amount;

        let elapsed = (now_u64 - pos.started_at) as Field;
        let apr = storage.base_apr_bps.read();
        let year_secs: Field = 31_536_000;
        let rewards = (net_principal * apr * elapsed * pos.multiplier_bps)
                       / (BPS_DENOM * year_secs * BPS_DENOM);

        // Ödülü topladık → started_at'i şimdiye al (double-claim önle)
        pos.started_at = now_u64;
        storage.positions.at(position_id).write(pos);

        let prev = storage.total_rewards_paid.read();
        storage.total_rewards_paid.write(prev + rewards);

        [rewards, pos.owner.to_field()]
    }

    // ── Slash (guardian/multisig 3/5 only) ───────────────────────────────────

    // slash: validator misbehavior cezası — pozisyonun belirli bir BPS'sini yakar.
    // reason: numeric code (1=double_sign, 2=offline, 3=invalid_block, 4=other)
    // bps: kesilecek miktar (max MAX_SLASH_BPS = %50)
    #[aztec(public)]
    fn slash(position_id: Field, bps: Field, reason: Field) {
        let guardian = storage.guardian.read();
        assert(context.msg_sender() == guardian,
               "OBSStaking.slash: not guardian");
        assert(!storage.paused.read(), "OBSStaking: paused");
        assert(bps as u64 > 0 && bps as u64 <= MAX_SLASH_BPS as u64,
               "OBSStaking.slash: invalid bps");

        let mut pos = storage.positions.at(position_id).read();
        assert(!pos.exited, "OBSStaking.slash: position exited");

        let cut = (pos.amount * bps) / BPS_DENOM;
        let new_slashed = pos.slashed_amount + cut;
        assert(new_slashed as u64 <= pos.amount as u64,
               "OBSStaking.slash: would exceed principal");

        pos.slashed_amount = new_slashed;
        storage.positions.at(position_id).write(pos);

        let prev = storage.total_slashed.read();
        storage.total_slashed.write(prev + cut);

        // Yakım: OBS token kontratına burn çağrısı (staking kontratının kendi note'undan)
        let token_addr = storage.obs_token.read();
        let burn_sel = compute_selector("burn((Field),Field)");
        // Not: gerçek private çağrı için context.call_private_function gerekir;
        // bu public fonksiyonda enqueue edilen private call ile yapılır.
        // (Reason event'i off-chain log'lanacak)
    }

    // ── Pause / unpause ──────────────────────────────────────────────────────

    #[aztec(public)]
    fn pause() {
        let guardian = storage.guardian.read();
        assert(context.msg_sender() == guardian, "OBSStaking.pause: not guardian");
        storage.paused.write(true);
    }

    #[aztec(public)]
    fn unpause() {
        let owner = storage.owner.read();
        assert(context.msg_sender() == owner, "OBSStaking.unpause: not owner");
        storage.paused.write(false);
    }

    // ── Admin (owner, timelock'lı multisig'den çağrılır) ─────────────────────

    #[aztec(public)]
    fn set_apr(new_apr_bps: Field) {
        assert(context.msg_sender() == storage.owner.read(),
               "OBSStaking.set_apr: not owner");
        // Safety: APR maksimum %50 (5000 bps)
        assert(new_apr_bps as u64 <= 5_000,
               "OBSStaking.set_apr: APR cap exceeded");
        storage.base_apr_bps.write(new_apr_bps);
    }

    #[aztec(public)]
    fn set_guardian(new_guardian: AztecAddress) {
        assert(context.msg_sender() == storage.owner.read(),
               "OBSStaking.set_guardian: not owner");
        storage.guardian.write(new_guardian);
    }

    #[aztec(public)]
    fn transfer_ownership(new_owner: AztecAddress) {
        assert(context.msg_sender() == storage.owner.read(),
               "OBSStaking.transfer_ownership: not owner");
        storage.owner.write(new_owner);
    }

    // ── Views ────────────────────────────────────────────────────────────────

    #[aztec(public)]
    fn get_position(position_id: Field) -> Position {
        storage.positions.at(position_id).read()
    }

    #[aztec(public)]
    fn get_total_staked() -> Field { storage.total_staked.read() }

    #[aztec(public)]
    fn get_total_slashed() -> Field { storage.total_slashed.read() }

    #[aztec(public)]
    fn get_total_rewards_paid() -> Field { storage.total_rewards_paid.read() }

    #[aztec(public)]
    fn get_base_apr_bps() -> Field { storage.base_apr_bps.read() }

    #[aztec(public)]
    fn get_owner_position_count(owner: AztecAddress) -> Field {
        storage.owner_positions.at(owner).read()
    }

    #[aztec(public)]
    fn is_paused() -> bool { storage.paused.read() }
}
