/**
 * overlapsDay decide en qué día(s) se pinta un evento en las vistas mes/semana/
 * día. La regresión que cubre: los eventos que cruzan medianoche desde el día
 * anterior deben aparecer también en el día siguiente (antes solo se pintaban el
 * día de su DTSTART). El fin es EXCLUSIVO: terminar justo a medianoche NO cuenta
 * para el día siguiente. Hora local (constructor Date sin Z).
 */
import { describe, expect, test } from 'bun:test';
import type { CalendarEvent } from '@sytue/api-clients';
import {
  addMonthsClamped,
  compactTitle,
  fullDateLabel,
  isoWeek,
  titleFor,
  overlapsDay,
  expandOccurrences,
  shiftEnd,
  withLocalDate,
  weekdayLabels,
  weekdayLongLabels,
  weekdayNarrowLabels,
} from './dates.js';

// 10 y 11 de julio de 2026 a mediodía local (referencia estable de cada día).
const D10 = new Date(2026, 6, 10, 12, 0, 0);
const D11 = new Date(2026, 6, 11, 12, 0, 0);
const D12 = new Date(2026, 6, 12, 12, 0, 0);

describe('overlapsDay', () => {
  test('evento dentro del día se pinta ese día y no en los vecinos', () => {
    const start = new Date(2026, 6, 10, 9, 0);
    const end = new Date(2026, 6, 10, 10, 0);
    expect(overlapsDay(start, end, D10)).toBe(true);
    expect(overlapsDay(start, end, D11)).toBe(false);
  });

  test('evento que cruza medianoche aparece en AMBOS días', () => {
    const start = new Date(2026, 6, 10, 23, 0); // 10 jul 23:00
    const end = new Date(2026, 6, 11, 1, 0); //    11 jul 01:00
    expect(overlapsDay(start, end, D10)).toBe(true);
    expect(overlapsDay(start, end, D11)).toBe(true); // <- la regresión arreglada
    expect(overlapsDay(start, end, D12)).toBe(false);
  });

  test('fin exclusivo: terminar justo a medianoche no cuenta para el día siguiente', () => {
    const start = new Date(2026, 6, 10, 23, 0);
    const end = new Date(2026, 6, 11, 0, 0); // medianoche exacta
    expect(overlapsDay(start, end, D10)).toBe(true);
    expect(overlapsDay(start, end, D11)).toBe(false);
  });

  test('evento sin fin (instante) pertenece solo al día de su inicio, incl. medianoche', () => {
    const midnight = new Date(2026, 6, 11, 0, 0);
    expect(overlapsDay(midnight, null, D11)).toBe(true);
    expect(overlapsDay(midnight, null, D10)).toBe(false);
  });

  test('evento de todo el día (fin = medianoche siguiente, exclusivo) solo en su día', () => {
    const start = new Date(2026, 6, 11, 0, 0);
    const end = new Date(2026, 6, 12, 0, 0);
    expect(overlapsDay(start, end, D11)).toBe(true);
    expect(overlapsDay(start, end, D12)).toBe(false);
  });

  test('evento multi-día se pinta en cada jornada que abarca', () => {
    const start = new Date(2026, 6, 10, 8, 0);
    const end = new Date(2026, 6, 12, 17, 0);
    expect(overlapsDay(start, end, D10)).toBe(true);
    expect(overlapsDay(start, end, D11)).toBe(true);
    expect(overlapsDay(start, end, D12)).toBe(true);
  });
});

/** Serie semanal (COUNT=4) desde el 1-jul, con un EXDATE opcional. */
function weekly(exdate?: string): CalendarEvent {
  const lines = [
    'BEGIN:VCALENDAR',
    'VERSION:2.0',
    'BEGIN:VEVENT',
    'UID:w1@sytue.eu',
    'DTSTART:20260701T090000Z',
    'DTEND:20260701T100000Z',
    'RRULE:FREQ=WEEKLY;COUNT=4',
    'SUMMARY:Semanal',
    ...(exdate ? [`EXDATE:${exdate}`] : []),
    'END:VEVENT',
    'END:VCALENDAR',
  ];
  return {
    href: '/w1.ics',
    uid: 'w1@sytue.eu',
    summary: 'Semanal',
    start: '20260701T090000Z',
    end: '20260701T100000Z',
    ical: lines.join('\r\n') + '\r\n',
  };
}

describe('expandOccurrences + EXDATE', () => {
  const from = new Date('2026-06-30T00:00:00Z');
  const to = new Date('2026-08-01T00:00:00Z');

  test('sin EXDATE expande las 4 ocurrencias semanales', () => {
    expect(expandOccurrences([weekly()], from, to)).toHaveLength(4);
  });

  test('EXDATE excluye esa instancia (edición «solo este» / borrado)', () => {
    const out = expandOccurrences([weekly('20260708T090000Z')], from, to);
    expect(out).toHaveLength(3);
    expect(out.some((e) => e.start === '20260708T090000Z')).toBe(false);
    // Las demás siguen presentes.
    expect(out.some((e) => e.start === '20260701T090000Z')).toBe(true);
    expect(out.some((e) => e.start === '20260715T090000Z')).toBe(true);
  });
});

/**
 * Los dos campos de fecha del EventModal. La regresión que cubren: mover el
 * inicio dejaba el fin donde estaba, así que reprogramar una reunión de las 10 a
 * las 12 la guardaba terminando ANTES de empezar; y en «todo el día» el estado se
 * quedaba con una fecha sin hora, que el <input type="datetime-local"> no sabe
 * leer y mostraba en blanco al desactivar el interruptor.
 */
describe('withLocalDate', () => {
  test('cambia la fecha y conserva la hora', () => {
    expect(withLocalDate('2026-07-10T09:30', '2026-07-14')).toBe('2026-07-14T09:30');
  });

  test('un valor previo sin hora asume medianoche (no deja el campo a medias)', () => {
    expect(withLocalDate('2026-07-10', '2026-07-14')).toBe('2026-07-14T00:00');
  });

  test('fecha vacía (el usuario borra el campo) no destruye el valor', () => {
    expect(withLocalDate('2026-07-10T09:30', '')).toBe('2026-07-10T09:30');
  });
});

describe('shiftEnd', () => {
  test('mover el inicio arrastra el fin conservando la duración', () => {
    expect(shiftEnd('2026-07-10T10:00', '2026-07-10T12:00', '2026-07-10T11:00')).toBe(
      '2026-07-10T13:00',
    );
  });

  test('atrasar el inicio también arrastra el fin', () => {
    expect(shiftEnd('2026-07-10T10:00', '2026-07-09T10:00', '2026-07-10T11:30')).toBe(
      '2026-07-09T11:30',
    );
  });

  test('un valor ilegible deja el fin intacto', () => {
    expect(shiftEnd('', '2026-07-10T12:00', '2026-07-10T11:00')).toBe('2026-07-10T11:00');
    expect(shiftEnd('2026-07-10T10:00', '2026-07-10T12:00', '')).toBe('');
  });
});

/**
 * Los tres anchos del día de la semana salen del mismo sitio —`Intl`, de lunes a domingo—
 * y cada uno es lo que dice ser: la cabecera de una columna estrecha enseña la inicial y
 * un lector de pantalla oye el nombre entero (`DiaDeSemana`). Solo se fija el inglés, que
 * es igual en cualquier ICU; del resto, la forma.
 */
describe('los días de la semana, en sus tres anchos', () => {
  test('en inglés: Monday→Sunday, Mon→Sun y sus iniciales', () => {
    expect(weekdayLongLabels('en')).toEqual([
      'Monday',
      'Tuesday',
      'Wednesday',
      'Thursday',
      'Friday',
      'Saturday',
      'Sunday',
    ]);
    expect(weekdayLabels('en')).toEqual(['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun']);
    expect(weekdayNarrowLabels('en')).toEqual(['M', 'T', 'W', 'T', 'F', 'S', 'S']);
  });

  test('en cualquier idioma son siete, y la inicial nunca es más larga que la abreviatura', () => {
    for (const locale of ['es', 'pt', 'lv', 'pl', 'da', 'sv', 'de', 'hu'] as const) {
      const corto = weekdayLabels(locale);
      const inicial = weekdayNarrowLabels(locale);
      const entero = weekdayLongLabels(locale);
      expect([corto.length, inicial.length, entero.length]).toEqual([7, 7, 7]);
      for (let i = 0; i < 7; i++) {
        expect(inicial[i]!.length, `${locale}: ${inicial[i]} / ${corto[i]}`).toBeLessThanOrEqual(
          corto[i]!.length,
        );
        expect(entero[i]!.length).toBeGreaterThanOrEqual(corto[i]!.length);
      }
    }
  });

  test('un ancho no pisa la caché de otro', () => {
    expect(weekdayNarrowLabels('en')[0]).toBe('M');
    expect(weekdayLabels('en')[0]).toBe('Mon');
    expect(weekdayLongLabels('en')[0]).toBe('Monday');
  });
});

/**
 * V12 mover-mes-recorta — vector espejo de `TestV12MoverMesRecorta` (Fyne, `MoverMes`).
 * Cambiar de mes conserva el día, recortado al último del mes de destino: con `addMonths`
 * el 31 de enero + 1 era el 3 de MARZO. Visto en ROJO volviendo `navigate` a `addMonths`
 * (aquí, devolviendo `addMonths(d, n)` desde `addMonthsClamped`).
 */
describe('V12 mover-mes-recorta', () => {
  const dia = (d: Date) => [d.getFullYear(), d.getMonth() + 1, d.getDate()];
  test('el día se recorta al último del mes de destino', () => {
    expect(dia(addMonthsClamped(new Date(2027, 0, 31), 1))).toEqual([2027, 2, 28]);
    expect(dia(addMonthsClamped(new Date(2026, 2, 31), -1))).toEqual([2026, 2, 28]);
    expect(dia(addMonthsClamped(new Date(2026, 11, 31), 2))).toEqual([2027, 2, 28]);
    expect(dia(addMonthsClamped(new Date(2024, 0, 31), 1))).toEqual([2024, 2, 29]);
  });
  test('si el día cabe no cambia, y la hora se conserva', () => {
    const x = addMonthsClamped(new Date(2026, 10, 6, 19, 36), 1);
    expect(dia(x)).toEqual([2026, 12, 6]);
    expect([x.getHours(), x.getMinutes()]).toEqual([19, 36]);
  });
});

/**
 * V13 titulo — vector espejo de `TestV13Titulo` (Fyne, `TituloCompacto`). El título del
 * mes del teléfono: el mes suelto en el año en curso, con el año fuera de él, y sin
 * capitalizar, como lo escribe el idioma. Visto en ROJO comparando con el año del CURSOR
 * en vez del de hoy (siempre «noviembre» a secas), y con `{ month: 'long' }` sin año
 * (en griego, «Νοεμβρίου»: el genitivo).
 */
describe('V13 titulo', () => {
  const hoy = new Date(2026, 10, 6);
  test('es y en', () => {
    expect(compactTitle('es', new Date(2026, 10, 6), hoy)).toBe('noviembre');
    expect(compactTitle('es', new Date(2027, 0, 15), hoy)).toBe('enero de 2027');
    expect(compactTitle('en', new Date(2026, 10, 6), hoy)).toBe('November');
    expect(compactTitle('en', new Date(2027, 0, 15), hoy)).toBe('January 2027');
  });
  test('el griego, en nominativo como el mini-mes y no en genitivo', () => {
    expect(compactTitle('el', new Date(2026, 10, 6), hoy)).toBe('Νοέμβριος');
    expect(compactTitle('el', new Date(2027, 0, 15), hoy)).toBe('Ιανουάριος 2027');
  });
  test('la fecha entera de la lista sale sin capitalizar; el título de la vista día, capitalizado', () => {
    expect(fullDateLabel('es', hoy)).toBe('viernes, 6 de noviembre de 2026');
    expect(titleFor('day', hoy, 'es')).toBe('Viernes, 6 de noviembre de 2026');
    expect(titleFor('month', hoy, 'es')).toBe('Noviembre de 2026');
  });
});

/**
 * El número de semana ISO 8601: la de su jueves; la 1 es la del 4 de enero. Visto en ROJO
 * contando la semana 1 desde el 1 de enero.
 */
describe('isoWeek', () => {
  test('los bordes de año', () => {
    expect(isoWeek(new Date(2024, 11, 30))).toBe(1); // lunes de la 1 de 2025
    expect(isoWeek(new Date(2024, 11, 31))).toBe(1);
    expect(isoWeek(new Date(2027, 0, 1))).toBe(53); // viernes de la 53 de 2026
    expect(isoWeek(new Date(2027, 0, 3))).toBe(53);
    expect(isoWeek(new Date(2027, 0, 4))).toBe(1);
    expect(isoWeek(new Date(2021, 0, 3))).toBe(53);
  });
  test('dentro del año, lunes y domingo de la misma semana', () => {
    expect(isoWeek(new Date(2026, 10, 2))).toBe(45);
    expect(isoWeek(new Date(2026, 10, 8))).toBe(45);
    expect(isoWeek(new Date(2026, 10, 9))).toBe(46);
  });
});
