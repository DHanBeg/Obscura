declare module 'snarkjs' {
  const groth16: {
    fullProve(input: Record<string, any>, wasmFile: string | Uint8Array, zkeyFile: string | Uint8Array): Promise<{ proof: object; publicSignals: string[] }>;
    verify(vkey: object, publicSignals: string[], proof: object): Promise<boolean>;
    exportSolidityCallData(proof: object, publicSignals: string[]): Promise<string>;
  };
  export { groth16 };
}

declare module 'circomlibjs' {
  type PoseidonFn = {
    (inputs: (bigint | number | string)[]): Uint8Array;
    F: { toString(el: Uint8Array): string; toObject(el: Uint8Array): bigint };
  };
  export function buildPoseidon(): Promise<PoseidonFn>;
  export function buildBabyjub(): Promise<any>;
  export function buildEddsa(): Promise<any>;
}
