/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Las cuentas del registro RGPD que la consola hace sin red: el plazo que se pinta
 * (por días de calendario, como lo cuenta quien contesta), cuándo se ofrece la
 * prórroga y la validación del alta. El plazo lo calcula Sytue ID; aquí solo se
 * comprueba que la pantalla no diga otra cosa.
 */
import { expect, test } from 'bun:test';
import {
  diasHastaPlazo,
  prorrogable,
  urgencia,
  validarAlta,
  type SolicitudRGPD,
} from './privacidad.js';

const ahora = new Date('2026-11-25T10:00:00Z');

function solicitud(extra: Partial<SolicitudRGPD> = {}): SolicitudRGPD {
  return {
    id: 's-1',
    subject_email: 'rosa@acme.test',
    subject_name: 'Rosa',
    kind: 'access',
    channel: 'email',
    received_at: '2026-11-20T09:00:00Z',
    due_at: '2026-12-20T09:00:00Z',
    extended: false,
    identity_verified: false,
    status: 'received',
    created_at: '2026-11-20T09:00:00Z',
    updated_at: '2026-11-20T09:00:00Z',
    ...extra,
  };
}

test('los días hasta el plazo son de calendario, no de 24 horas', () => {
  expect(diasHastaPlazo('2026-11-25T23:59:00Z', ahora)).toBe(0);
  expect(diasHastaPlazo('2026-11-26T00:01:00Z', ahora)).toBe(1);
  expect(diasHastaPlazo('2026-11-24T23:00:00Z', ahora)).toBe(-1);
});

test('la urgencia: cerrada, vencida, a una semana o con tiempo', () => {
  expect(urgencia(solicitud(), ahora)).toBe('ok');
  expect(urgencia(solicitud({ due_at: '2026-12-01T09:00:00Z' }), ahora)).toBe('soon');
  expect(urgencia(solicitud({ due_at: '2026-11-24T09:00:00Z' }), ahora)).toBe('overdue');
  // Cerrada manda sobre vencida: una contestada tarde ya no es urgente.
  expect(
    urgencia(
      solicitud({ due_at: '2026-11-24T09:00:00Z', closed_at: '2026-11-23T09:00:00Z' }),
      ahora,
    ),
  ).toBe('closed');
});

test('la prórroga se ofrece una vez, abierta y antes de vencer', () => {
  expect(prorrogable(solicitud(), ahora)).toBe(true);
  expect(prorrogable(solicitud({ extended: true }), ahora)).toBe(false);
  expect(prorrogable(solicitud({ closed_at: '2026-11-24T00:00:00Z' }), ahora)).toBe(false);
  expect(prorrogable(solicitud({ due_at: '2026-11-25T09:00:00Z' }), ahora)).toBe(false);
});

test('el alta exige a la persona (cuenta o correo válido) y una fecha razonable', () => {
  const base = { kind: 'erasure', channel: 'letter', identity_verified: false } as const;
  expect(validarAlta({ ...base })).toBe('subject');
  expect(validarAlta({ ...base, subject_email: 'no-es-correo' })).toBe('email');
  expect(validarAlta({ ...base, subject_email: 'ana@fuera.test' })).toBeNull();
  expect(validarAlta({ ...base, user_id: 'u-1' })).toBeNull();
  const manana = new Date(Date.now() + 3 * 86_400_000).toISOString().slice(0, 10);
  expect(validarAlta({ ...base, user_id: 'u-1', received_at: manana })).toBe('date');
  expect(validarAlta({ ...base, user_id: 'u-1', received_at: '2020-01-01' })).toBe('date');
  expect(validarAlta({ ...base, user_id: 'u-1', received_at: 'ayer' })).toBe('date');
  // Tipo o canal que no existen: los rechaza aunque la isla nunca los mande.
  expect(validarAlta({ ...base, user_id: 'u-1', kind: 'olvido' as never })).toBe('kind');
  expect(validarAlta({ ...base, user_id: 'u-1', channel: 'paloma' as never })).toBe('channel');
});
