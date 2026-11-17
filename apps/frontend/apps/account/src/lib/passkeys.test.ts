/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La passkey nueva desbloquea desde ya (ADR 0085): si el autenticador admite PRF,
 * tras el alta se va a la página de Sytue ID que le hace su sobre y vuelve aquí. La
 * salida de la PRF nunca se pide aquí (solo si la admite), y una passkey sin PRF no
 * viaja a ninguna parte.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, expect, spyOn, test } from 'bun:test';

const registrado = typeof document === 'undefined';
if (registrado) GlobalRegistrator.register({ url: 'https://id.sytue.eu/account/seguridad' });
const { altaPasskey } = await import('./passkeys.js');
const { appDomains } = await import('@sytue/config');

const fetchOriginal = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = fetchOriginal;
});
afterAll(async () => {
  if (registrado) await GlobalRegistrator.unregister();
});

function servidor() {
  globalThis.fetch = (async (input: RequestInfo | URL) => {
    if (String(input) === '/api/passkeys/begin')
      return Response.json({
        challenge: 'AAAA',
        rp: { id: 'sytue.eu', name: 'Sytue' },
        user: { id: 'AAAA', name: 'ana', displayName: 'Ana' },
        pubKeyCredParams: [{ type: 'public-key', alg: -7 }],
      });
    return Response.json({ credential_id: 'oKGio6Sl', name: 'Portátil' }, { status: 201 });
  }) as unknown as typeof fetch;
}

function autenticador(prf: { enabled?: boolean } | undefined) {
  const pedidos: CredentialCreationOptions[] = [];
  const create = async (opciones: CredentialCreationOptions) => {
    pedidos.push(opciones);
    return {
      id: 'oKGio6Sl',
      rawId: new Uint8Array([0xa0, 0xa1, 0xa2, 0xa3, 0xa4, 0xa5]).buffer,
      type: 'public-key',
      response: { attestationObject: new ArrayBuffer(4), clientDataJSON: new ArrayBuffer(4) },
      getClientExtensionResults: () => (prf ? { prf } : {}),
    };
  };
  Object.defineProperty(navigator, 'credentials', {
    configurable: true,
    value: { create },
  });
  (window as unknown as { PublicKeyCredential: unknown }).PublicKeyCredential = function () {};
  return pedidos;
}

test('con PRF, tras el alta va a Sytue ID a hacerle el sobre y vuelve aquí', async () => {
  servidor();
  const pedidos = autenticador({ enabled: true });
  const ir = spyOn(window.location, 'assign').mockImplementation(() => undefined);
  try {
    expect(await altaPasskey('Portátil')).toBeNull();
    // Solo se pregunta si admite PRF: ninguna salida de la PRF se pide aquí.
    expect(pedidos[0]!.publicKey!.extensions).toEqual({ prf: {} });
    expect(ir).toHaveBeenCalledTimes(1);
    const destino = new URL(String(ir.mock.calls[0]![0]));
    expect(destino.origin).toBe(new URL(appDomains.id).origin);
    expect(destino.pathname).toBe('/e2e/passkey');
    expect(destino.searchParams.get('cred')).toBe('oKGio6Sl');
    expect(destino.searchParams.get('return_to')).toBe(window.location.href);
  } finally {
    ir.mockRestore();
  }
});

test('sin PRF no va a ninguna parte: desbloqueará en su primer uso si puede', async () => {
  for (const prf of [{ enabled: false }, undefined]) {
    servidor();
    autenticador(prf);
    const ir = spyOn(window.location, 'assign').mockImplementation(() => undefined);
    try {
      expect(await altaPasskey('Llave')).toBeNull();
      expect(ir).not.toHaveBeenCalled();
    } finally {
      ir.mockRestore();
    }
  }
});
