/* SPDX-License-Identifier: Apache-2.0 */
/**
 * Los puertos de entrada del módulo de correo cuando Mail vive en la bóveda
 * (ADR 0087, entrega 4). Son los mismos métodos que sirven `PqcIngressSecretsPort`
 * y `PqcIngressControlPort` sobre Matrix; cambia quién los sirve:
 *
 * - `ingress.secret.read`/`.write`: los secretos de entrada se guardan cifrados en
 *   la propia bóveda (`PqVault.readIngressSecret`/`writeIngressSecret`), no en el
 *   almacén de secretos (SSSS) de Matrix. El formato de los bytes no cambia.
 * - `ingress.action.sign`: firma la clave de la bóveda, en el núcleo
 *   (`PqVault.signIngressAction`), y el servidor la verifica contra Sytue ID.
 */
import { storageJSONBytes } from '../matrix/storage.js';
import type { StorageRepository } from '../matrix/storage-repository.js';
import { decodeHostObject } from '../pqc/host-codec.js';
import type { HostCallContext, HostHandler } from '../pqc/host-port.js';
import type { PqVault } from './pq-vault.js';

const maxIngressSecretBytes = 2048;

export type VaultIngressSigner = Pick<
  PqVault,
  'isClosed' | 'scope' | 'signIngressAction' | 'readIngressSecret' | 'writeIngressSecret'
>;

function sameScope(vault: VaultIngressSigner, context: HostCallContext): boolean {
  return (
    context.scope.tenant === vault.scope.tenant &&
    context.scope.subject === vault.scope.subject &&
    context.scope.product === vault.scope.product
  );
}

export class VaultIngressSecretsPort {
  private closed = false;
  readonly handlers: {
    'ingress.secret.read': HostHandler;
    'ingress.secret.write': HostHandler;
  };
  constructor(vault: VaultIngressSigner, records: StorageRepository) {
    const fail = (): never => {
      throw new Error('ingress_secret_rejected');
    };
    const check = (context: HostCallContext): void => {
      context.assertOpen();
      records.assertOpen(context.signal);
      if (this.closed || vault.isClosed || !sameScope(vault, context)) fail();
    };
    const name = (value: unknown): string => (typeof value === 'string' ? value : fail());
    this.handlers = {
      'ingress.secret.read': async (raw, context) => {
        check(context);
        const value = decodeHostObject(raw, fail);
        if (Object.keys(value).length !== 1) return fail();
        const secret = await vault.readIngressSecret(name(value.name), context.signal);
        try {
          check(context);
          if (!secret) return new Uint8Array([0]);
          if (secret.byteLength > maxIngressSecretBytes) return fail();
          const out = new Uint8Array(1 + secret.byteLength);
          out[0] = 1;
          out.set(secret, 1);
          return out;
        } finally {
          secret?.fill(0);
        }
      },
      'ingress.secret.write': async (raw, context) => {
        check(context);
        const value = decodeHostObject(raw, fail);
        if (Object.keys(value).length !== 2) return fail();
        const secret = name(value.name),
          bytes = storageJSONBytes(value.value, maxIngressSecretBytes);
        try {
          await vault.writeIngressSecret(secret, bytes, context.signal);
          check(context);
          return new Uint8Array();
        } finally {
          bytes.fill(0);
        }
      },
    };
  }
  close(): Promise<void> {
    this.closed = true;
    return Promise.resolve();
  }
}

export class VaultIngressControlPort {
  private closed = false;
  readonly handlers: { 'ingress.action.sign': HostHandler };
  constructor(vault: VaultIngressSigner, records: StorageRepository) {
    const fail = (): never => {
      throw new Error('ingress_control_rejected');
    };
    const check = (context: HostCallContext): void => {
      context.assertOpen();
      records.assertOpen(context.signal);
      if (this.closed || vault.isClosed || !sameScope(vault, context)) fail();
    };
    this.handlers = {
      'ingress.action.sign': async (raw, context) => {
        check(context);
        // La forma la comprueba aquí; el contenido (intención, acción, raíz,
        // vigencia) lo comprueba el núcleo antes de firmar.
        const value = decodeHostObject(raw, fail);
        if (
          Object.keys(value).length !== 2 ||
          !value.expected ||
          typeof value.expected !== 'object' ||
          Array.isArray(value.expected) ||
          !value.action ||
          typeof value.action !== 'object' ||
          Array.isArray(value.action)
        )
          return fail();
        const proof = await vault.signIngressAction(raw, context.signal);
        try {
          check(context);
        } catch (error) {
          proof.fill(0);
          throw error;
        }
        return proof;
      },
    };
  }
  close(): Promise<void> {
    this.closed = true;
    return Promise.resolve();
  }
}
