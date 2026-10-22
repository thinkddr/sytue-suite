/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { describe, expect, test } from 'bun:test';
import { detalleDominio, progresoDominio, type DominioSitio } from './dominios';

const dominio = (datos: Partial<DominioSitio>): DominioSitio => ({
  id: 'dom-1',
  host: 'www.aurora.es',
  status: 'pending',
  detail: '',
  primary: false,
  ...datos,
});

describe('hitos de un dominio basados en la comprobación del servicio', () => {
  test('añadir un dominio no completa el DNS ni HTTPS', () => {
    expect(progresoDominio(dominio({}))).toEqual([true, false, false, false]);
  });
  test('DNS verificado con certificado pendiente no permite hacerlo principal', () => {
    expect(progresoDominio(dominio({ status: 'verified', detail: 'certificate' }))).toEqual([
      true,
      true,
      false,
      false,
    ]);
    expect(detalleDominio(dominio({ status: 'verified', detail: 'certificate' }))).toBe(
      'domains.detail.certificate',
    );
  });
  test('solo un activo sin incidencias completa la conexión y la dirección principal', () => {
    expect(progresoDominio(dominio({ status: 'active' }))).toEqual([true, true, true, false]);
    expect(progresoDominio(dominio({ status: 'active', primary: true }))).toEqual([
      true,
      true,
      true,
      true,
    ]);
  });
  test('un activo histórico con DNS roto vuelve a pendiente de revisión', () => {
    expect(
      progresoDominio(dominio({ status: 'active', detail: 'routing', primary: true })),
    ).toEqual([true, false, false, false]);
    expect(detalleDominio(dominio({ status: 'active', detail: 'routing' }))).toBe(
      'domains.detail.routing',
    );
  });
  test('motivos nuevos se explican sin marcar avances que no constan', () => {
    expect(progresoDominio(dominio({ status: 'active', detail: 'nuevo_motivo' }))).toEqual([
      true,
      false,
      false,
      false,
    ]);
    expect(detalleDominio(dominio({ detail: 'nuevo_motivo' }))).toBe('domains.detail.unknown');
  });
});
