import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { expect, test } from 'bun:test';
import { ORIGENES_DE_AVISOS } from '@sytue/shell';
import {
  filaSilenciada,
  ORIGENES_FIJOS,
  silenciable,
  silenciado,
  TIPOS_PARTIDOS,
  tipoSilenciado,
} from './reglasAvisos.js';

test('los avisos de seguridad de Sytue ID van fijos y no se ofrecen para callar', () => {
  expect(ORIGENES_FIJOS).toEqual(['id']);
  const ofrecidos = ORIGENES_DE_AVISOS.flatMap((g) => g.origenes);
  for (const f of ORIGENES_FIJOS) expect(ofrecidos).not.toContain(f);
});

test('Sytue ID no se puede silenciar; el resto sí', () => {
  expect(silenciable('id')).toBe(false);
  expect(silenciable('mail')).toBe(true);
});

test('una regla antigua que silenciaba Sytue ID no lo deja en silencio', () => {
  const prefs = [
    { source: 'id', muted: true },
    { source: 'mail', muted: true },
  ];
  expect(silenciado(prefs, 'id')).toBe(false);
  expect(silenciado(prefs, 'mail')).toBe(true);
  // Las de un tipo concreto no cuentan para el interruptor del origen entero.
  expect(silenciado([{ source: 'drive', kind: 'share', muted: true }], 'drive')).toBe(false);
});

test('un tipo sigue la regla más específica, como el Hub al entregar', () => {
  const prefs = [
    { source: 'mail', muted: true },
    { source: 'mail', kind: 'mail.access.requested', muted: false },
    { source: 'rooms', kind: 'rooms.message', muted: true },
  ];
  // Todo el correo callado salvo las peticiones de acceso.
  expect(tipoSilenciado(prefs, 'mail', 'mail.message.received')).toBe(true);
  expect(tipoSilenciado(prefs, 'mail', 'mail.access.requested')).toBe(false);
  // Solo los directos de Rooms: las menciones siguen llegando.
  expect(tipoSilenciado(prefs, 'rooms', 'rooms.message')).toBe(true);
  expect(tipoSilenciado(prefs, 'rooms', 'rooms.mention')).toBe(false);
});

test('un tipo crítico nunca sale callado, aunque se calle su origen o quede una regla', () => {
  const prefs = [
    { source: 'pages', muted: true },
    { source: 'rooms', kind: 'rooms.report', muted: true },
  ];
  expect(tipoSilenciado(prefs, 'pages', 'pages.payment.duplicated')).toBe(false);
  expect(tipoSilenciado(prefs, 'rooms', 'rooms.report')).toBe(false);
  expect(tipoSilenciado(prefs, 'pages', 'pages.form.received')).toBe(true);
});

test('una fila de varios tipos sale encendida mientras llegue alguno', () => {
  const tipos = ['rooms.invitation', 'rooms.invitation.direct'];
  const una = [{ source: 'rooms', kind: 'rooms.invitation', muted: true }];
  expect(filaSilenciada(una, 'rooms', tipos)).toBe(false);
  const las2 = [...una, { source: 'rooms', kind: 'rooms.invitation.direct', muted: true }];
  expect(filaSilenciada(las2, 'rooms', tipos)).toBe(true);
});

test('un tipo partido hereda la regla del de antes, como el Hub', () => {
  const prefs = [{ source: 'cloud', kind: 'cloud.deploy', muted: true }];
  expect(tipoSilenciado(prefs, 'cloud', 'cloud.deploy.ready')).toBe(true);
  expect(filaSilenciada(prefs, 'cloud', ['cloud.deploy.ready', 'cloud.deploy.failed'])).toBe(true);
  // La regla propia manda sobre la heredada.
  const propia = [...prefs, { source: 'cloud', kind: 'cloud.deploy.failed', muted: false }];
  expect(tipoSilenciado(propia, 'cloud', 'cloud.deploy.failed')).toBe(false);
  // Solo los de la tabla: las invitaciones directas no salieron de las de canal.
  const salas = [{ source: 'rooms', kind: 'rooms.invitation', muted: true }];
  expect(tipoSilenciado(salas, 'rooms', 'rooms.invitation.direct')).toBe(false);
});

test('la tabla de tipos partidos es la del Hub', () => {
  const go = readFileSync(
    fileURLToPath(
      new URL('../../../../../../services/hub/internal/domain/domain.go', import.meta.url),
    ),
    'utf8',
  );
  const bloque = go.slice(go.indexOf('var tiposPartidos'), go.indexOf('// TipoAnterior'));
  const delHub = Object.fromEntries(
    [...bloque.matchAll(/"([a-z.]+)":\s+"([a-z.]+)"/g)].map((m) => [m[1], m[2]]),
  );
  expect(delHub).toEqual({ ...TIPOS_PARTIDOS });
});
