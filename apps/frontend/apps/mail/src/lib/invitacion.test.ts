/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Qué invitación ve el lector en un correo (D-MAIL-04). Ejecutar: `bun test`.
 */
import { expect, test } from 'bun:test';
import { leerInvitacion } from './invitacion.js';

const GOOGLE = [
  'BEGIN:VCALENDAR',
  'PRODID:-//Google Inc//Google Calendar 70.9054//EN',
  'VERSION:2.0',
  'METHOD:REQUEST',
  'BEGIN:VTIMEZONE',
  'TZID:Europe/Madrid',
  'BEGIN:STANDARD',
  'DTSTART:19701025T030000',
  'END:STANDARD',
  'END:VTIMEZONE',
  'BEGIN:VEVENT',
  'DTSTART:20261112T090000Z',
  'DTEND:20261112T100000Z',
  'UID:g-1@google.com',
  'ORGANIZER;CN="Luis Pérez":mailto:luis@otro.example',
  'ATTENDEE;PARTSTAT=NEEDS-ACTION;RSVP=TRUE:mailto:ana@sytue.eu',
  'SUMMARY:Revisión del\\, presupuesto',
  'LOCATION:Sala 3\\, planta',
  ' 2',
  'BEGIN:VALARM',
  'ACTION:DISPLAY',
  'DESCRIPTION:Aviso',
  'TRIGGER:-PT10M',
  'END:VALARM',
  'END:VEVENT',
  'END:VCALENDAR',
].join('\r\n');

test('resume el REQUEST: qué, cuándo, dónde y quién convoca', () => {
  const inv = leerInvitacion(GOOGLE);
  expect(inv).not.toBeNull();
  expect(inv!.uid).toBe('g-1@google.com');
  expect(inv!.summary).toBe('Revisión del, presupuesto');
  expect(inv!.location).toBe('Sala 3, planta2');
  expect(inv!.organizer).toBe('luis@otro.example');
  expect(inv!.organizerName).toBe('Luis Pérez');
  // El DTSTART es el del VEVENT, no el del VTIMEZONE que va antes.
  expect(inv!.start?.toISOString()).toBe('2026-11-12T09:00:00.000Z');
  expect(inv!.end?.toISOString()).toBe('2026-11-12T10:00:00.000Z');
  expect(inv!.allDay).toBe(false);
});

test('un día entero se marca como tal', () => {
  const inv = leerInvitacion(
    GOOGLE.replace('DTSTART:20261112T090000Z', 'DTSTART;VALUE=DATE:20261112').replace(
      'DTEND:20261112T100000Z',
      'DTEND;VALUE=DATE:20261113',
    ),
  );
  expect(inv!.allDay).toBe(true);
  expect(inv!.start?.getDate()).toBe(12);
});

test('solo un REQUEST con organizador se ofrece contestar', () => {
  expect(leerInvitacion(GOOGLE.replace('METHOD:REQUEST', 'METHOD:CANCEL'))).toBeNull();
  expect(leerInvitacion(GOOGLE.replace('METHOD:REQUEST\r\n', ''))).toBeNull();
  expect(
    leerInvitacion(GOOGLE.replace('ORGANIZER;CN="Luis Pérez":mailto:luis@otro.example\r\n', '')),
  ).toBeNull();
  expect(leerInvitacion(undefined)).toBeNull();
});

test('sin SUMMARY no se cuela el relleno sin traducir del SDK', () => {
  expect(leerInvitacion(GOOGLE.replace(/SUMMARY:[^\r]*\r\n/, ''))!.summary).toBe('');
});
