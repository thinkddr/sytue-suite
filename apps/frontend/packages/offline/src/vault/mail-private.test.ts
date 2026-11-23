/* SPDX-License-Identifier: Apache-2.0 */
import { describe, expect, test } from 'bun:test';
import type { StorageRepository } from '../matrix/storage-repository.js';
import type { HostCallContext } from '../pqc/host-port.js';
import { VaultIngressControlPort, VaultIngressSecretsPort } from './mail-ports.js';
import {
  checkVaultAuthority,
  guardMailDestruction,
  openVaultMailEngine,
  openVaultMailPrivate,
} from './mail-private.js';
import { PqVaultError, type PqVault } from './pq-vault.js';

const encoder = new TextEncoder();
const decoder = new TextDecoder();
const scope = Object.freeze({ tenant: 'acme', subject: 'user-1', product: 'mail' as const });
const userID = '@sytue_storage_mail_acme_user-1:sytue-vault';
const nombre = 'org.sytue.ingress.key.v1.' + 'ab'.repeat(32);

function contexto(over: Partial<HostCallContext> = {}): HostCallContext {
  return { scope, signal: new AbortController().signal, assertOpen: () => {}, ...over };
}

const records = { assertOpen: () => {} } as unknown as StorageRepository;

function bovedaFalsa() {
  const guardados = new Map<string, Uint8Array>();
  const firmados: string[] = [];
  const vault = {
    isClosed: false,
    scope,
    readIngressSecret: async (name: string) => guardados.get(name)?.slice() ?? null,
    writeIngressSecret: async (name: string, value: Uint8Array) => {
      guardados.set(name, value.slice());
    },
    signIngressAction: async (raw: Uint8Array) => {
      firmados.push(decoder.decode(raw));
      return encoder.encode('{"firmada":true}');
    },
  };
  return { vault, guardados, firmados };
}

describe('Puertos de entrada del correo sobre la bóveda (ADR 0087)', () => {
  test('los secretos van y vuelven con el formato del puerto de Matrix', async () => {
    const { vault, guardados } = bovedaFalsa();
    const port = new VaultIngressSecretsPort(vault, records);
    const leer = () =>
      port.handlers['ingress.secret.read'](
        encoder.encode(JSON.stringify({ name: nombre })),
        contexto(),
      );
    expect([...(await leer())]).toEqual([0]);
    const valor = Buffer.from('{"v":1}').toString('base64');
    await port.handlers['ingress.secret.write'](
      encoder.encode(JSON.stringify({ name: nombre, value: valor })),
      contexto(),
    );
    expect(decoder.decode(guardados.get(nombre))).toBe('{"v":1}');
    const leido = await leer();
    expect(leido[0]).toBe(1);
    expect(decoder.decode(leido.slice(1))).toBe('{"v":1}');
    // Otro ámbito, un campo de más o un valor que no es JSON: no.
    await expect(
      port.handlers['ingress.secret.read'](
        encoder.encode(JSON.stringify({ name: nombre })),
        contexto({ scope: { ...scope, subject: 'otra' } }),
      ),
    ).rejects.toThrow();
    await expect(
      port.handlers['ingress.secret.read'](
        encoder.encode(JSON.stringify({ name: nombre, extra: 1 })),
        contexto(),
      ),
    ).rejects.toThrow();
    await expect(
      port.handlers['ingress.secret.write'](
        encoder.encode(JSON.stringify({ name: nombre, value: btoa('no json') })),
        contexto(),
      ),
    ).rejects.toThrow();
    await port.close();
    await expect(leer()).rejects.toThrow();
  });

  test('la firma pasa los bytes tal cual al núcleo, solo con la forma {expected, action}', async () => {
    const { vault, firmados } = bovedaFalsa();
    const port = new VaultIngressControlPort(vault, records);
    const pedido = '{"expected":{"kind":"task.ack"},"action":{"kind":"task.ack"}}';
    const prueba = await port.handlers['ingress.action.sign'](encoder.encode(pedido), contexto());
    expect(decoder.decode(prueba)).toBe('{"firmada":true}');
    expect(firmados).toEqual([pedido]);
    for (const malo of [
      '{"expected":{}}',
      '{"expected":[],"action":{}}',
      '{"expected":{},"action":{},"x":1}',
    ])
      await expect(
        port.handlers['ingress.action.sign'](encoder.encode(malo), contexto()),
      ).rejects.toThrow();
    expect(firmados).toHaveLength(1);
    vault.isClosed = true;
    await expect(
      port.handlers['ingress.action.sign'](encoder.encode(pedido), contexto()),
    ).rejects.toThrow();
  });
});

describe('openVaultMailPrivate (ADR 0087)', () => {
  const autoridad = (over: Record<string, unknown> = {}) =>
    encoder.encode(
      JSON.stringify({
        v: 1,
        scope,
        user_id: userID,
        root_key: 'RAIZ',
        generation: 1,
        status: 'active',
        ...over,
      }),
    );

  test('solo vale la autoridad de ESTA bóveda', () => {
    expect(checkVaultAuthority(autoridad(), scope, userID, 'RAIZ').generation).toBe(1);
    for (const over of [
      { user_id: '@sytue_storage_mail_acme_user-1:matrix.sytue.eu' },
      { root_key: 'OTRA' },
      { status: 'revoked' },
      { generation: 0 },
      { scope: { ...scope, subject: 'otra' } },
    ])
      expect(() => checkVaultAuthority(autoridad(over), scope, userID, 'RAIZ')).toThrow();
    expect(() => checkVaultAuthority(encoder.encode('x'), scope, userID, 'RAIZ')).toThrow();
  });

  test('sin la clave de firma activa en Sytue ID no se abre ni se pide sesión', async () => {
    let sesiones = 0;
    const abrir = (status: string, product = 'mail') =>
      openVaultMailPrivate(
        {
          isClosed: false,
          scope: { ...scope, product },
          signingKeyStatus: status,
        } as unknown as PqVault,
        {
          basePath: '/mail',
          openOwner: async () => {
            sesiones++;
            throw new Error('no debería pedirse');
          },
        },
      );
    const tras = await abrir('reauth').catch((e: unknown) => e);
    expect(tras).toBeInstanceOf(PqVaultError);
    expect((tras as PqVaultError).reason).toBe('signin');
    expect(((await abrir('missing').catch((e: unknown) => e)) as PqVaultError).reason).toBe(
      'unavailable',
    );
    await expect(abrir('registered', 'drive')).rejects.toThrow();
    expect(sesiones).toBe(0);
  });
});

describe('openVaultMailEngine (ADR 0087)', () => {
  const sesion = 'ab'.repeat(32);
  function motor(respuesta: unknown) {
    const pedidos: Array<{ operation: unknown; port: unknown; input: unknown }> = [];
    return {
      pedidos,
      domain: {
        invoke: async (request: Record<string, unknown>) => {
          pedidos.push({
            operation: request.operation,
            port: request.port,
            input: JSON.parse(decoder.decode(request.input as Uint8Array)),
          });
          return respuesta;
        },
      } as never,
    };
  }
  const signal = new AbortController().signal;

  test('el anfitrión pide el modo de bóveda, sin puertos de Matrix', async () => {
    const m = motor({ session: sesion, ready: true, scope });
    expect(await openVaultMailEngine(m.domain, 'p'.repeat(64), scope, signal)).toBe(sesion);
    expect(m.pedidos).toEqual([
      { operation: 'mail.open', port: 'p'.repeat(64), input: { store: 'vault' } },
    ]);
  });

  test('una respuesta que no es la de este ámbito no abre nada', async () => {
    for (const malo of [
      null,
      { session: 'corta', ready: true, scope },
      { session: sesion, ready: false, scope },
      { session: sesion, ready: true, scope: { ...scope, subject: 'otra' } },
      { session: sesion, ready: true },
    ])
      await expect(openVaultMailEngine(motor(malo).domain, 'p', scope, signal)).rejects.toThrow();
  });
});

describe('guardMailDestruction (ADR 0087)', () => {
  function motor(pendiente: unknown) {
    const llamadas: string[] = [];
    return {
      llamadas,
      domain: {
        invoke: async (request: Record<string, unknown>) => {
          llamadas.push(String(request.operation));
          expect(JSON.parse(decoder.decode(request.input as Uint8Array))).toEqual({ session: 's' });
          return request.operation === 'mail.pending' ? pendiente : { paused: true };
        },
      } as never,
    };
  }
  const signal = new AbortController().signal;

  test('pausa y solo deja borrar con cero pendientes', async () => {
    const vacio = motor({ scope, count: 0 });
    await guardMailDestruction(vacio.domain, 'p', scope, 's', signal);
    expect(vacio.llamadas).toEqual(['mail.pause', 'mail.pending']);
    for (const pendiente of [
      { scope, count: 1 },
      { scope: { ...scope, subject: 'otra' }, count: 0 },
      { scope, count: -1 },
      null,
    ])
      await expect(
        guardMailDestruction(motor(pendiente).domain, 'p', scope, 's', signal),
      ).rejects.toThrow();
  });
});
