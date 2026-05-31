// Type stubs for @tauri-apps/api — only available in Tauri desktop runtime.
// In web/Docker builds these modules are aliased to stubs at runtime.
declare module "@tauri-apps/api/core" {
  export function invoke<T = void>(cmd: string, args?: Record<string, unknown>): Promise<T>;
}
declare module "@tauri-apps/api/event" {
  export type UnlistenFn = () => void;
  export function listen<T = unknown>(event: string, handler: (e: { payload: T }) => void): Promise<UnlistenFn>;
}
