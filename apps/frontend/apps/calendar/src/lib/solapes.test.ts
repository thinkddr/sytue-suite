/**
 * «Coincide con…» del formulario de un evento y «Mostrar como» (TRANSP): qué cuenta
 * como choque y que el guardado escribe, conserva y quita el TRANSP como toca.
 *
 * Ejecutar: `bun test`.
 */
import { describe, expect, test } from 'bun:test';
import type { GridEvent } from '../components/CalendarGrid.js';
import { buildVevent, fundirVevent, parseTransp } from './ical.js';
import { solapes } from './solapes.js';

function ev(uid: string, start: string, end: string, extra: string[] = []): GridEvent {
  const ical = [
    'BEGIN:VCALENDAR',
    'BEGIN:VEVENT',
    `UID:${uid}`,
    `DTSTART:${start}`,
    `DTEND:${end}`,
    `SUMMARY:${uid}`,
    ...extra,
    'END:VEVENT',
    'END:VCALENDAR',
  ].join('\r\n');
  return { href: `/dav/${uid}.ics`, uid, summary: uid, start, end, ical };
}

const DIEZ = new Date(Date.UTC(2026, 10, 30, 10, 0));
const ONCE = new Date(Date.UTC(2026, 10, 30, 11, 0));

describe('solapes', () => {
  test('cuenta lo que se pisa y no lo que solo toca el borde', () => {
    const eventos = [
      ev('antes', '20261130T090000Z', '20261130T100000Z'), // acaba justo al empezar
      ev('pisa', '20261130T103000Z', '20261130T113000Z'),
      ev('dentro', '20261130T101500Z', '20261130T104500Z'),
      ev('despues', '20261130T110000Z', '20261130T120000Z'), // empieza justo al acabar
    ];
    expect(solapes(eventos, DIEZ, ONCE).map((s) => s.evento.uid)).toEqual(['dentro', 'pisa']);
  });

  test('no cuentan el propio evento, el día completo, lo disponible ni lo rechazado', () => {
    const eventos = [
      ev('yo', '20261130T100000Z', '20261130T110000Z'),
      ev('cumple', '20261130', '20261201'),
      ev('libre', '20261130T100000Z', '20261130T110000Z', ['TRANSP:TRANSPARENT']),
      ev('rechazado', '20261130T100000Z', '20261130T110000Z', [
        'ATTENDEE;PARTSTAT=DECLINED:mailto:ana@sytue.eu',
      ]),
      ev('aceptado', '20261130T100000Z', '20261130T110000Z', [
        'ATTENDEE;PARTSTAT=ACCEPTED:mailto:ana@sytue.eu',
      ]),
    ];
    const r = solapes(eventos, DIEZ, ONCE, { uid: 'yo', correo: 'Ana@sytue.eu' });
    expect(r.map((s) => s.evento.uid)).toEqual(['aceptado']);
  });

  test('el mismo evento visto desde dos calendarios cuenta una vez', () => {
    const a = ev('reunion', '20261130T100000Z', '20261130T110000Z');
    expect(solapes([a, { ...a, href: '/dav/otro/reunion.ics' }], DIEZ, ONCE)).toHaveLength(1);
  });

  test('un tramo vacío no se pisa con nada', () => {
    expect(solapes([ev('x', '20261130T100000Z', '20261130T110000Z')], ONCE, DIEZ)).toEqual([]);
  });
});

describe('Mostrar como (TRANSP)', () => {
  const base = {
    uid: 'u1',
    summary: 'Recoger el coche',
    start: '2026-11-30T10:00:00.000Z',
    end: '2026-11-30T11:00:00.000Z',
  };

  test('«disponible» se escribe y «ocupado» es no escribir nada', () => {
    expect(buildVevent({ ...base, transp: 'TRANSPARENT' })).toContain('TRANSP:TRANSPARENT\r\n');
    expect(buildVevent({ ...base, transp: 'OPAQUE' })).not.toContain('TRANSP');
    expect(parseTransp(buildVevent({ ...base, transp: 'TRANSPARENT' }))).toBe('TRANSPARENT');
    expect(parseTransp(buildVevent(base))).toBe('OPAQUE');
  });

  test('el guardado sin «Mostrar como» (un arrastre) conserva el TRANSP de antes', () => {
    const guardado = buildVevent({ ...base, transp: 'TRANSPARENT' });
    expect(parseTransp(fundirVevent(guardado, base))).toBe('TRANSPARENT');
  });

  test('volver a «ocupado» desde el formulario quita la línea', () => {
    const guardado = buildVevent({ ...base, transp: 'TRANSPARENT' });
    const nuevo = fundirVevent(guardado, { ...base, transp: 'OPAQUE' });
    expect(nuevo).not.toContain('TRANSP');
    expect(parseTransp(nuevo)).toBe('OPAQUE');
  });

  test('el TRANSP de un VALARM o de una excepción no es el del maestro', () => {
    const ical = [
      'BEGIN:VCALENDAR',
      'BEGIN:VEVENT',
      'UID:u1',
      'DTSTART:20261130T100000Z',
      'END:VEVENT',
      'BEGIN:VEVENT',
      'UID:u1',
      'RECURRENCE-ID:20261207T100000Z',
      'TRANSP:TRANSPARENT',
      'END:VEVENT',
      'END:VCALENDAR',
    ].join('\r\n');
    expect(parseTransp(ical)).toBe('OPAQUE');
  });
});
