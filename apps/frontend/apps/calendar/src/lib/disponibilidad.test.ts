/**
 * «Compartir mi disponibilidad»: los huecos se agrupan por día EN LA ZONA de quien
 * comparte (no en UTC) y el texto dice la zona. Las mismas entradas y salidas que
 * `apps/fyne/calendario/disponibilidad_test.go`.
 *
 * Ejecutar: `bun test`.
 */
import { describe, expect, test } from 'bun:test';
// `useT` no es un hook: devuelve el traductor ya atado a un idioma.
import { useT as traductor } from '../i18n/index.js';
import { diasDisponibles, textoDisponibilidad } from './disponibilidad.js';

const t = traductor('en');

describe('disponibilidad', () => {
  test('agrupa por día en la zona y ordena los tramos', () => {
    const huecos = [
      { start: '2026-11-30T14:00:00Z', end: '2026-11-30T17:00:00Z' },
      { start: '2026-11-30T08:00:00Z', end: '2026-11-30T09:30:00Z' },
      // 23:30 en UTC del domingo es el lunes 00:30 en Madrid: cuenta como lunes.
      { start: '2026-11-29T23:30:00Z', end: '2026-11-30T00:00:00Z' },
      { start: '2026-12-01T09:00:00Z', end: '2026-12-01T10:00:00Z' },
      // Un tramo al revés no se escribe.
      { start: '2026-12-01T12:00:00Z', end: '2026-12-01T11:00:00Z' },
    ];
    const dias = diasDisponibles(huecos, 'Europe/Madrid', 'en');
    expect(dias.map((d) => d.tramos)).toEqual([
      ['00:30–01:00', '09:00–10:30', '15:00–18:00'],
      ['10:00–11:00'],
    ]);
    expect(dias[0]!.dia).toContain('30');
    expect(dias[1]!.dia).toContain('1');
  });

  test('el texto lleva la zona y una línea por día; sin huecos, nada que copiar', () => {
    const dias = [
      { dia: 'Mon 30 Nov', tramos: ['09:00–10:30', '15:00–18:00'] },
      { dia: 'Tue 1 Dec', tramos: ['10:00–11:00'] },
    ];
    expect(textoDisponibilidad(t, dias, 'Europe/Madrid')).toBe(
      [
        'My availability (Europe/Madrid):',
        'Mon 30 Nov: 09:00–10:30, 15:00–18:00',
        'Tue 1 Dec: 10:00–11:00',
      ].join('\n'),
    );
    expect(textoDisponibilidad(t, [], 'Europe/Madrid')).toBe('');
  });
});
