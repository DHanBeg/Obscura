// Client-side ZK proof generation for Obscura ZK circuits.
// Only called in browser context (dynamic import in component).

export interface ProofResult {
  proof_json: string;
  public_inputs: string[];
}

// Alias for backward compat
export type VoteProofResult = ProofResult;

// buildIdentityProof generates a Groth16 identity_proof for airdrop claims.
//
// identity_secret: 252-bit BigInt derived from BIP39 mnemonic (HKDF-SHA512 output)
// phone_hash:     Poseidon(phone_number_as_field) — must match server's snapshot
//
// The caller is responsible for persisting identity_secret securely (localStorage
// encrypted with the user's PIN, or fetched from the mnemonic via HKDF).
export async function buildIdentityProof(
  identitySecretHex: string,
  phoneNumberStr: string
): Promise<ProofResult> {
  const snarkjs = await import("snarkjs");
  const { buildPoseidon } = await import("circomlibjs");

  const poseidon = await buildPoseidon();
  const F = poseidon.F;

  const identitySecret = BigInt("0x" + identitySecretHex);
  // phone as ASCII digit field element
  const phoneField = BigInt(phoneNumberStr.replace(/\D/g, "").slice(0, 15));
  const epochBig = BigInt(Math.floor(Date.now() / 86400000)); // day number

  const phoneHash = BigInt(F.toString(poseidon([phoneField])));
  const didCommitment = BigInt(F.toString(poseidon([identitySecret, phoneField])));
  const nullifierHash = BigInt(F.toString(poseidon([identitySecret, epochBig])));

  const input = {
    identity_secret: identitySecret.toString(),
    phone_hash:      phoneHash.toString(),
    did_commitment:  didCommitment.toString(),
    nullifier_hash:  nullifierHash.toString(),
    epoch:           epochBig.toString(),
  };

  const { proof, publicSignals } = await snarkjs.groth16.fullProve(
    input,
    "/zk/identity_proof/identity_proof.wasm",
    "/zk/identity_proof/identity_proof_final.zkey"
  );

  return {
    proof_json:    JSON.stringify(proof),
    public_inputs: publicSignals as string[],
  };
}

// buildVoteProof generates a real Groth16 proof for an anonymous vote.
//
// poll_id and voter_root come from GET /v1/governance/proposals/{id}/voter-root.
// vote_choice is 0=yes, 1=no, 2=abstain, 3=veto.
//
// The voter_secret is generated fresh each call (anonymous — no identity linkage).
export async function buildVoteProof(
  pollIdStr: string,
  voterRoot: string,
  voteChoice: number
): Promise<VoteProofResult> {
  // Dynamic imports — snarkjs and circomlibjs are large; load only when needed.
  const snarkjs = await import("snarkjs");
  const { buildPoseidon } = await import("circomlibjs");

  const poseidon = await buildPoseidon();
  const F = poseidon.F;

  // Represent as BigInt for circuit arithmetic (BN254 scalar field)
  const pollId = BigInt(pollIdStr);
  const voterRootBig = BigInt(voterRoot);
  const choiceBig = BigInt(voteChoice);
  const timestamp = BigInt(Date.now());

  // Generate random 252-bit voter secret (fits in BN254 scalar field)
  const secretBytes = crypto.getRandomValues(new Uint8Array(31));
  let voterSecret = 0n;
  for (const b of secretBytes) voterSecret = (voterSecret << 8n) | BigInt(b);

  // vote_commitment = Poseidon(vote_choice, voter_secret)
  const commitmentHash = poseidon([choiceBig, voterSecret]);
  const voteCommitment = BigInt(F.toString(commitmentHash));

  // nullifier = Poseidon(voter_secret, poll_id)
  const nullifierHash = poseidon([voterSecret, pollId]);
  const nullifier = BigInt(F.toString(nullifierHash));

  const input = {
    voter_secret: voterSecret.toString(),
    vote_choice:  voteChoice.toString(),
    voter_index:  "0",
    poll_id:          pollId.toString(),
    vote_commitment:  voteCommitment.toString(),
    voter_root:       voterRootBig.toString(),
    nullifier:        nullifier.toString(),
    timestamp:        timestamp.toString(),
  };

  const { proof, publicSignals } = await snarkjs.groth16.fullProve(
    input,
    "/zk/vote_proof/vote_proof.wasm",
    "/zk/vote_proof/vote_proof_final.zkey"
  );

  return {
    proof_json:    JSON.stringify(proof),
    public_inputs: publicSignals as string[],
  };
}
