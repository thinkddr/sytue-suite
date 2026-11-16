/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { describe, expect, test } from 'bun:test';
import { nombreDePeticion, pedidasDe, pendienteDe, quienFirma, type PeticionFirma } from './api';

const base: PeticionFirma = {
  id: '1',
  requester: 'a',
  requester_name: 'Ana',
  recipient: 'b',
  file_id: 'f',
  path: '/PDF/Contrato.pdf',
  status: 'pendiente',
  created_at: '2026-09-26T10:00:00Z',
};

describe('peticiones de firma', () => {
  test('la franja sale solo con una PENDIENTE de este mismo fichero', () => {
    const ps = {
      received: [
        { ...base, id: '2', status: 'firmada' as const },
        { ...base, id: '3', path: '/PDF/Otro.pdf' },
        base,
      ],
      sent: [],
    };
    expect(pendienteDe(ps, '/PDF/Contrato.pdf')?.id).toBe('1');
    expect(pendienteDe(ps, '/PDF/Nada.pdf')).toBeUndefined();
    expect(pendienteDe(null, '/PDF/Contrato.pdf')).toBeUndefined();
  });

  test('a la otra persona se la nombra por el directorio, y si no por su token', () => {
    expect(
      quienFirma({ ...base, persona: { email: 'ana@acme.eu', name: 'Ana López' } }, 'received'),
    ).toBe('Ana López');
    expect(quienFirma({ ...base, persona: { email: 'ana@acme.eu' } }, 'received')).toBe(
      'ana@acme.eu',
    );
    expect(quienFirma(base, 'received')).toBe('Ana');
    expect(quienFirma(base, 'sent')).toBe('');
    expect(quienFirma({ ...base, recipient_email: 'irati@acme.eu' }, 'sent')).toBe('irati@acme.eu');
  });

  test('de otra organización, sin copia aún no hay franja, y el nombre sale igual', () => {
    const fuera = { ...base, external: true, name: 'Contrato.pdf', path: '' };
    expect(pendienteDe({ received: [fuera], sent: [] }, '')).toBeUndefined();
    expect(nombreDePeticion(fuera)).toBe('Contrato.pdf');
    expect(nombreDePeticion(base)).toBe('Contrato.pdf');
    const traida = { ...fuera, path: '/Sytue Docs/Contrato.pdf' };
    expect(pendienteDe({ received: [traida], sent: [] }, '/Sytue Docs/Contrato.pdf')?.id).toBe('1');
  });

  test('una cadena sale entera bajo el PDF original, aunque las siguientes firmen otra ruta', () => {
    const firmado = '/PDF/Contrato (firmado).pdf';
    const sent = [
      { ...base, id: 's', created_at: '2026-09-27T10:00:00Z' },
      { ...base, id: 'c1', chain: 'c', status: 'firmada' as const },
      { ...base, id: 'c3', chain: 'c', step: 2, status: 'en_espera' as const },
      { ...base, id: 'c2', chain: 'c', step: 1, path: firmado },
      { ...base, id: 'otro', chain: 'd', path: '/PDF/Otro.pdf' },
    ];
    const grupos = pedidasDe(sent, '/PDF/Contrato.pdf');
    expect(grupos.map((g) => g.map((p) => p.id))).toEqual([['s'], ['c1', 'c2', 'c3']]);
    // Desde el firmado intermedio no se lista la cadena: su primera persona no es de ahí.
    expect(pedidasDe(sent, firmado)).toEqual([]);
  });
});
