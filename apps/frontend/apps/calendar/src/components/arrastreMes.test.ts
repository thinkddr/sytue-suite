/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La aritmética del arrastre en el mes: qué celda hay bajo el puntero y dónde cae un
 * evento movido N días (hora de pared, duración y fin exclusivo de los días completos).
 */
import { describe, expect, test } from 'bun:test';
import { celdaEnPunto, desplazarDias, eventoDesplazado } from './arrastreMes.js';
import { eventInstant, parseTzid } from './dates.js';

const RECT = { left: 10, top: 20, width: 700, height: 600 };

describe('celdaEnPunto', () => {
  test('columna y semana del punto', () => {
    expect(celdaEnPunto(RECT, 6, 15, 25)).toBe(0);
    expect(celdaEnPunto(RECT, 6, 10 + 350, 20 + 250)).toBe(2 * 7 + 3);
  });

  test('fuera de la rejilla se pega al borde', () => {
    expect(celdaEnPunto(RECT, 6, -500, -500)).toBe(0);
    expect(celdaEnPunto(RECT, 6, 5000, 5000)).toBe(41);
  });

  test('sin tamaño no hay celda', () => {
    expect(celdaEnPunto({ left: 0, top: 0, width: 0, height: 0 }, 6, 1, 1)).toBeNull();
  });
});

describe('desplazarDias', () => {
  test('conserva la hora de pared aunque haya cambio de hora por medio', () => {
    // Del sábado 24 al lunes 26 de octubre de 2026 (en Europa cambia la hora el 25).
    const r = desplazarDias({ start: '20261024T090000', end: '20261024T100000' }, 2);
    expect(r?.start).toEqual(new Date(2026, 9, 26, 9));
    expect(r?.end).toEqual(new Date(2026, 9, 26, 10));
  });

  test('un día completo sin DTEND dura un día y su fin sigue siendo exclusivo', () => {
    const r = desplazarDias({ start: '20261110' }, -1);
    expect(r?.start).toEqual(new Date(2026, 10, 9));
    expect(r?.end).toEqual(new Date(2026, 10, 10));
  });

  test('uno con hora y sin DTEND no gana duración', () => {
    const r = desplazarDias({ start: '20261110T090000' }, 3);
    expect(r?.end).toEqual(r?.start);
  });

  test('sin inicio legible no hay movimiento', () => {
    expect(desplazarDias({ start: '' }, 1)).toBeNull();
  });
});

describe('eventoDesplazado', () => {
  test('un día completo sigue siendo de fecha', () => {
    const e = eventoDesplazado({ start: '20261110', end: '20261113' }, 2);
    expect(e.start).toBe('20261112');
    expect(e.end).toBe('20261115');
  });

  test('uno con zona se pinta en el mismo instante que se va a guardar', () => {
    const ical = 'BEGIN:VEVENT\r\nDTSTART;TZID=America/New_York:20261110T090000\r\nEND:VEVENT';
    const original = { start: '20261110T090000', end: '20261110T100000', ical };
    const movido = eventoDesplazado(original, 1);
    const esperado = desplazarDias(original, 1);
    expect(eventInstant(movido.start, parseTzid(movido.ical))).toEqual(esperado!.start);
    expect(eventInstant(movido.end!, parseTzid(movido.ical))).toEqual(esperado!.end);
  });
});
