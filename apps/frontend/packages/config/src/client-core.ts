/* SPDX-License-Identifier: Apache-2.0 */
/** Executable modules are compiler-owned artifacts; document URLs never select code. */
export const clientCoreModuleBounds = Object.freeze({
  core: 16 << 20,
  office: 64 << 20,
  mail: 64 << 20,
  /** El motor de PDF para los PDF de la bóveda (`cmd/pdf-wasm`, ADR 0086): ~17 MiB. */
  pdf: 32 << 20,
});
export type ClientCoreModule = keyof typeof clientCoreModuleBounds;
export function isClientCoreModule(value: unknown): value is ClientCoreModule {
  return typeof value === 'string' && Object.hasOwn(clientCoreModuleBounds, value);
}
