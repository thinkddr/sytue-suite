/* SPDX-License-Identifier: Apache-2.0 */
/** Executable modules are compiler-owned artifacts; document URLs never select code. */
export const clientCoreModuleBounds = Object.freeze({
  /** El núcleo completo (`cmd/pqc-wasm`): bóveda + Matrix, recursos y búsqueda de Rooms. */
  core: 16 << 20,
  /** El núcleo de la bóveda (`cmd/vault-wasm`, ADR 0086): ~7,7 MB, solo lo de la bóveda. */
  vault: 16 << 20,
  office: 64 << 20,
  mail: 64 << 20,
  /** El motor de PDF para los PDF de la bóveda (`cmd/pdf-wasm`, ADR 0086): ~17 MiB. */
  pdf: 32 << 20,
});
export type ClientCoreModule = keyof typeof clientCoreModuleBounds;
export function isClientCoreModule(value: unknown): value is ClientCoreModule {
  return typeof value === 'string' && Object.hasOwn(clientCoreModuleBounds, value);
}
/**
 * El módulo con el que arranca un `PqcRuntime` sin módulo explícito: el núcleo ligero
 * de la bóveda, que es lo que toda app con bóveda descarga antes de pintar. Quien
 * precargue el núcleo (`<link rel=preload>`) pide el manifiesto de ESTE módulo
 * (`manifest.json?module=vault`) y sus URL con `?module=vault&sha256=…`.
 */
export const vaultCoreModule: ClientCoreModule = 'vault';
