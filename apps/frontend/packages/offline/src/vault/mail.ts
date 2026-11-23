/* SPDX-License-Identifier: Apache-2.0 */
/** Sytue Mail sobre la bóveda (ADR 0087). Aparte de `./vault` para que las demás
 * apps de la bóveda no carguen los puertos del módulo de correo. */
export {
  checkVaultAuthority,
  guardMailDestruction,
  openVaultMailPrivate,
  type VaultMailOptions,
  type VaultMailPrivatePorts,
} from './mail-private.js';
export { VaultIngressControlPort, VaultIngressSecretsPort } from './mail-ports.js';
