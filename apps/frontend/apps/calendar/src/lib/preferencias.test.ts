/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Calendar recuerda la vista y los calendarios ocultos de CADA persona: ocultar
 * «Trabajo» y pasar a semana, recargar, y está igual; otra cuenta en el mismo navegador
 * no lo hereda; un calendario nuevo aparece encendido; y sin almacenamiento todo
 * funciona como siempre (mes y todos visibles).
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, beforeEach, describe, expect, test } from 'bun:test';
import {
  AJUSTES_POR_DEFECTO,
  guardarAjustes,
  guardarOcultos,
  leerAjustes,
  guardarVista,
  leerOcultos,
  leerVista,
  normalizarAjustes,
  ocultoPorRechazo,
  visiblesTrasCarga,
} from './preferencias.js';

const registrado = typeof document === 'undefined';
if (registrado) GlobalRegistrator.register();
afterAll(async () => {
  if (registrado) await GlobalRegistrator.unregister();
});

const PERSONAL = '/dav/ana/personal/';
const TRABAJO = '/dav/ana/trabajo/';
const NUEVO = '/dav/luis/equipo/';

/** Una carga «en frío», como al recargar la página: nada conocido todavía. */
function primeraCarga(userId: string, urls: string[]): Set<string> {
  return visiblesTrasCarga(urls, new Set(), new Set(), leerOcultos(userId));
}

describe('preferencias de Calendar por persona', () => {
  beforeEach(() => localStorage.clear());

  test('sin nada guardado: mes y todos visibles', () => {
    expect(leerVista('ana')).toBeNull();
    expect([...primeraCarga('ana', [PERSONAL, TRABAJO])]).toEqual([PERSONAL, TRABAJO]);
  });

  test('ocultar «Trabajo» y pasar a semana sobrevive a recargar', () => {
    guardarVista('ana', 'week');
    guardarOcultos('ana', [PERSONAL, TRABAJO], new Set([PERSONAL]));

    expect(leerVista('ana')).toBe('week');
    expect([...primeraCarga('ana', [PERSONAL, TRABAJO])]).toEqual([PERSONAL]);
    expect(localStorage.getItem('sytue.calendar.ana.vista')).toBe('week');
  });

  test('otra cuenta en el mismo navegador no lo hereda', () => {
    guardarVista('ana', 'agenda');
    guardarOcultos('ana', [PERSONAL, TRABAJO], new Set([PERSONAL]));

    expect(leerVista('luis')).toBeNull();
    expect([...primeraCarga('luis', [PERSONAL, TRABAJO])]).toEqual([PERSONAL, TRABAJO]);
  });

  test('se guardan los OCULTOS: un calendario nuevo aparece encendido', () => {
    guardarOcultos('ana', [PERSONAL, TRABAJO], new Set([PERSONAL]));
    expect([...primeraCarga('ana', [PERSONAL, TRABAJO, NUEVO])]).toEqual([PERSONAL, NUEVO]);
  });

  test('volver a encenderlo lo olvida', () => {
    guardarOcultos('ana', [PERSONAL, TRABAJO], new Set([PERSONAL]));
    guardarOcultos('ana', [PERSONAL, TRABAJO], new Set([PERSONAL, TRABAJO]));
    expect(localStorage.getItem('sytue.calendar.ana.ocultos')).toBeNull();
  });

  test('un oculto que esta carga no trajo no se olvida', () => {
    guardarOcultos('ana', [PERSONAL, TRABAJO, NUEVO], new Set([PERSONAL]));
    // Los compartidos fallaron al listarse: NUEVO no está entre los conocidos.
    guardarOcultos('ana', [PERSONAL, TRABAJO], new Set([PERSONAL, TRABAJO]));
    expect([...leerOcultos('ana')]).toEqual([NUEVO]);
  });

  test('tras la primera carga se respeta la selección y se encienden los recién llegados', () => {
    guardarOcultos('ana', [PERSONAL, TRABAJO], new Set([PERSONAL]));
    const conocidos = new Set([PERSONAL, TRABAJO]);
    const antes = new Set<string>([TRABAJO]); // la persona encendió Trabajo y apagó Personal
    const ahora = visiblesTrasCarga(
      [PERSONAL, TRABAJO, NUEVO],
      antes,
      conocidos,
      leerOcultos('ana'),
    );
    expect([...ahora]).toEqual([TRABAJO, NUEVO]);
  });

  test('lo guardado que no es una vista, o ilegible, cuenta como nada', () => {
    localStorage.setItem('sytue.calendar.ana.vista', 'year');
    localStorage.setItem('sytue.calendar.ana.ocultos', '{roto');
    expect(leerVista('ana')).toBeNull();
    expect(leerOcultos('ana').size).toBe(0);
  });

  test('sin almacenamiento todo funciona como hoy', () => {
    const original = Object.getOwnPropertyDescriptor(globalThis, 'localStorage');
    Object.defineProperty(globalThis, 'localStorage', {
      configurable: true,
      get() {
        throw new Error('SecurityError');
      },
    });
    try {
      expect(() => guardarVista('ana', 'week')).not.toThrow();
      expect(() => guardarOcultos('ana', [TRABAJO], new Set())).not.toThrow();
      expect(leerVista('ana')).toBeNull();
      expect([...primeraCarga('ana', [PERSONAL, TRABAJO])]).toEqual([PERSONAL, TRABAJO]);
    } finally {
      if (original) Object.defineProperty(globalThis, 'localStorage', original);
    }
  });
});

describe('ajustes de Calendar', () => {
  beforeEach(() => localStorage.clear());

  test('sin nada guardado, los de siempre', () => {
    expect(leerAjustes('ana')).toEqual({ ...AJUSTES_POR_DEFECTO });
  });

  test('se guardan por persona y sobreviven a recargar', () => {
    const propios = {
      duracion: 30,
      recordatorio: 10,
      inicioLaboral: 7,
      finLaboral: 15,
      finesDeSemana: false,
      numeroSemana: true,
      rechazados: false,
    };
    guardarAjustes('ana', propios);
    expect(leerAjustes('ana')).toEqual(propios);
    expect(leerAjustes('luis')).toEqual({ ...AJUSTES_POR_DEFECTO });
    expect(JSON.parse(localStorage.getItem('sytue.calendar.ana.ajustes') ?? '{}')).toEqual(propios);
  });

  test('lo roto vuelve al valor de siempre campo a campo', () => {
    expect(normalizarAjustes({ duracion: 7, recordatorio: 'x', numeroSemana: true })).toEqual({
      ...AJUSTES_POR_DEFECTO,
      numeroSemana: true,
    });
    // Una franja al revés no se arregla a medias: vuelve entera.
    const alReves = normalizarAjustes({ inicioLaboral: 18, finLaboral: 9 });
    expect([alReves.inicioLaboral, alReves.finLaboral]).toEqual([9, 18]);
    localStorage.setItem('sytue.calendar.ana.ajustes', '{no es json');
    expect(leerAjustes('ana')).toEqual({ ...AJUSTES_POR_DEFECTO });
  });

  test('los rechazados se ven salvo que se diga lo contrario (también en lo de antes)', () => {
    // Un JSON guardado antes de que existiera el ajuste no esconde nada.
    expect(normalizarAjustes({ duracion: 30 }).rechazados).toBe(true);
    expect(normalizarAjustes({ rechazados: 'no' }).rechazados).toBe(true);
    expect(normalizarAjustes({ rechazados: false }).rechazados).toBe(false);
  });

  test('solo se esconde lo que YO he rechazado, y solo con el ajuste apagado', () => {
    const ical = [
      'BEGIN:VEVENT',
      'ORGANIZER:mailto:luis@sytue.eu',
      'ATTENDEE;PARTSTAT=DECLINED:mailto:ana@sytue.eu',
      'ATTENDEE;PARTSTAT=DECLINED:mailto:eva@sytue.eu',
      'END:VEVENT',
    ].join('\r\n');
    expect(ocultoPorRechazo({ rechazados: false }, ical, 'Ana@sytue.eu')).toBe(true);
    expect(ocultoPorRechazo({ rechazados: true }, ical, 'ana@sytue.eu')).toBe(false);
    expect(ocultoPorRechazo({ rechazados: false }, ical, 'luis@sytue.eu')).toBe(false);
    expect(ocultoPorRechazo({ rechazados: false }, undefined, 'ana@sytue.eu')).toBe(false);
  });
});
