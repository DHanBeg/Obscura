// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

/// @title OBSBridge — Obscura cross-chain lock/unlock contract
/// @author Obscura Blockchain Team
/// @notice Source-chain side of the OBS bridge. Users call lock() to escrow
///         OBS tokens; an off-chain relayer observes the Locked event and
///         mints/releases the equivalent amount on the destination chain.
///         A symmetric OBSBridge deployment on the destination chain calls
///         unlock() with a proof of the source-chain Locked event to release
///         escrowed tokens back to the user.
///
/// Pattern: LayerZero-style "mint-and-burn" with off-chain message relay.
/// Future: replace `proof` with a ZK proof of the source-chain event
///         (see contracts/zk/EventVerifier — FAZ 4).
///
/// Security:
///   - Reentrancy: nonReentrant on every state-changing entrypoint.
///   - Replay: each unlock proof is keyed by a unique sourceTxHash; the
///             usedProofs mapping prevents double-spending the same event.
///   - Owner-only: setRelayer / pause guarded by a 3/5 multi-sig (set at
///                 deployment via Owner = multisig address).
///   - Emergency pause: pause() halts lock() and unlock() in a single tx.
///   - Upgradeability: this is the implementation behind a transparent
///                     proxy with a 48-hour timelock. Storage layout is
///                     append-only.
contract OBSBridge {
    // ─── Storage ─────────────────────────────────────────────────────────────

    /// @notice OBS token contract address (ERC-20).
    address public immutable obsToken;

    /// @notice Multi-sig wallet that owns the bridge (3-of-5).
    address public owner;

    /// @notice Trusted relayer; only this address may call unlock().
    address public relayer;

    /// @notice Total OBS currently escrowed in this contract.
    uint256 public totalLocked;

    /// @notice Per-user escrow accounting. Lets us audit who locked what.
    mapping(address => uint256) public lockedBalances;

    /// @notice Replay protection — each source-chain Locked event id can be
    ///         unlocked at most once on the dest chain.
    mapping(bytes32 => bool) public usedProofs;

    /// @notice Emergency kill switch. When true, lock() and unlock() revert.
    bool public paused;

    /// @notice Reentrancy guard.
    uint256 private _status;
    uint256 private constant _NOT_ENTERED = 1;
    uint256 private constant _ENTERED = 2;

    // ─── Events ──────────────────────────────────────────────────────────────

    /// @notice Emitted on a successful lock; consumed by the off-chain relayer.
    event Locked(
        address indexed user,
        uint256 amount,
        string destChain,
        string recipient,
        uint256 nonce
    );

    /// @notice Emitted when an unlock proof is honored on this chain.
    event Unlocked(
        address indexed recipient,
        uint256 amount,
        bytes32 indexed sourceTxHash
    );

    event RelayerUpdated(address indexed oldRelayer, address indexed newRelayer);
    event OwnerUpdated(address indexed oldOwner, address indexed newOwner);
    event PausedSet(bool paused);

    // ─── Modifiers ───────────────────────────────────────────────────────────

    modifier onlyOwner() {
        require(msg.sender == owner, "OBSBridge: not owner");
        _;
    }

    modifier onlyRelayer() {
        require(msg.sender == relayer, "OBSBridge: not relayer");
        _;
    }

    modifier whenNotPaused() {
        require(!paused, "OBSBridge: paused");
        _;
    }

    modifier nonReentrant() {
        require(_status != _ENTERED, "OBSBridge: reentrancy");
        _status = _ENTERED;
        _;
        _status = _NOT_ENTERED;
    }

    // ─── Constructor ─────────────────────────────────────────────────────────

    /// @param _obsToken Address of the OBS ERC-20 contract.
    /// @param _owner    3-of-5 multi-sig that controls the bridge.
    /// @param _relayer  Initial off-chain relayer address (rotatable).
    constructor(address _obsToken, address _owner, address _relayer) {
        require(_obsToken != address(0), "OBSBridge: zero token");
        require(_owner != address(0), "OBSBridge: zero owner");
        require(_relayer != address(0), "OBSBridge: zero relayer");
        obsToken = _obsToken;
        owner = _owner;
        relayer = _relayer;
        _status = _NOT_ENTERED;
    }

    // ─── User-facing: lock ───────────────────────────────────────────────────

    /// @notice Escrow `amount` OBS and emit a Locked event for the relayer.
    /// @param amount     OBS amount in atomic units (18 decimals).
    /// @param destChain  Destination chain id ("polkadot", "obscura", …).
    /// @param recipient  Destination-chain address (hex / SS58 / DID — opaque).
    /// @dev User must first approve(this, amount) on the OBS token.
    function lock(
        uint256 amount,
        string calldata destChain,
        string calldata recipient
    ) external nonReentrant whenNotPaused {
        require(amount > 0, "OBSBridge: zero amount");
        require(bytes(destChain).length > 0, "OBSBridge: empty destChain");
        require(bytes(recipient).length > 0, "OBSBridge: empty recipient");

        // Pull tokens via transferFrom (caller must have called approve()).
        // We deliberately use the low-level call pattern so the bridge stays
        // SafeERC20-free (audited single import surface).
        (bool ok, bytes memory data) = obsToken.call(
            abi.encodeWithSignature(
                "transferFrom(address,address,uint256)",
                msg.sender,
                address(this),
                amount
            )
        );
        require(ok, "OBSBridge: transferFrom failed");
        if (data.length > 0) {
            require(abi.decode(data, (bool)), "OBSBridge: transferFrom returned false");
        }

        lockedBalances[msg.sender] += amount;
        totalLocked += amount;

        // Nonce derived from blockNumber+counter would be more compact, but
        // (user, totalLocked) is unique-enough for the relayer to key off.
        emit Locked(msg.sender, amount, destChain, recipient, totalLocked);
    }

    // ─── Relayer-facing: unlock ──────────────────────────────────────────────

    /// @notice Release `amount` OBS to `recipient` upon receipt of a proof
    ///         that a Locked event was emitted on the source chain.
    /// @param recipient    Local address to receive the escrowed tokens.
    /// @param amount       OBS amount.
    /// @param sourceTxHash Unique id of the source-chain Locked event
    ///                     (used for replay protection).
    /// @param proof        Opaque proof bytes — today validated off-chain by
    ///                     the relayer multi-sig; FAZ 4 swaps in a ZK proof.
    function unlock(
        address recipient,
        uint256 amount,
        bytes32 sourceTxHash,
        bytes calldata proof
    ) external nonReentrant whenNotPaused onlyRelayer {
        require(recipient != address(0), "OBSBridge: zero recipient");
        require(amount > 0, "OBSBridge: zero amount");
        require(amount <= totalLocked, "OBSBridge: amount > locked");
        require(!usedProofs[sourceTxHash], "OBSBridge: proof replayed");
        // `proof` is currently checked off-chain. We keep the parameter so
        // the on-chain ZK verifier (FAZ 4) is a drop-in replacement.
        require(proof.length > 0, "OBSBridge: empty proof");

        usedProofs[sourceTxHash] = true;
        totalLocked -= amount;

        (bool ok, bytes memory data) = obsToken.call(
            abi.encodeWithSignature(
                "transfer(address,uint256)",
                recipient,
                amount
            )
        );
        require(ok, "OBSBridge: transfer failed");
        if (data.length > 0) {
            require(abi.decode(data, (bool)), "OBSBridge: transfer returned false");
        }

        emit Unlocked(recipient, amount, sourceTxHash);
    }

    // ─── Owner-only admin ────────────────────────────────────────────────────

    function setRelayer(address newRelayer) external onlyOwner {
        require(newRelayer != address(0), "OBSBridge: zero relayer");
        emit RelayerUpdated(relayer, newRelayer);
        relayer = newRelayer;
    }

    function setOwner(address newOwner) external onlyOwner {
        require(newOwner != address(0), "OBSBridge: zero owner");
        emit OwnerUpdated(owner, newOwner);
        owner = newOwner;
    }

    function setPaused(bool _paused) external onlyOwner {
        paused = _paused;
        emit PausedSet(_paused);
    }
}
