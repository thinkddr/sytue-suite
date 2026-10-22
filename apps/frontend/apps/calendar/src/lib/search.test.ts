// Las etiquetas se piden en un idioma CONCRETO: desde que las fechas salen de
// Intl, «3 de julio» solo es la respuesta correcta si se pregunta en español.
import { describe, expect, it } from 'bun:test';
import { isRecurring, whenLabel, snippet, startDate, fechaDelEnlace } from './search.js';
import type { EventSearchResult } from '../client/calendarApi.js';

function result(over: Partial<EventSearchResult>): EventSearchResult {
  return {
    id: 'e1',
    calendar_id: 'c1',
    uid: 'u1',
    summary: 'Evento',
    dtstart: '2026-07-01T09:00:00Z',
    dtend: '2026-07-01T10:00:00Z',
    all_day: false,
    ...over,
  };
}

describe('search helpers', () => {
  it('detecta recurrentes por rrule o rdate', () => {
    expect(isRecurring(result({}))).toBe(false);
    expect(isRecurring(result({ rrule: 'FREQ=WEEKLY' }))).toBe(true);
    expect(isRecurring(result({ rdate: ['2026-07-08T09:00:00Z'] }))).toBe(true);
    expect(isRecurring(result({ rdate: [] }))).toBe(false);
  });

  it('snippet prioriza lugar, luego descripción', () => {
    expect(snippet(result({ location: 'Sala 1', description: 'notas' }))).toBe('Sala 1');
    expect(snippet(result({ description: 'notas' }))).toBe('notas');
    expect(snippet(result({}))).toBe('');
  });

  it('todo el día de un solo día: sin rango', () => {
    const label = whenLabel(
      'es',
      result({ all_day: true, dtstart: '2026-07-01', dtend: '2026-07-02' }),
    );
    expect(label.startsWith('Todo el día')).toBe(true);
    expect(label.includes('–')).toBe(false);
  });

  it('todo el día de varios días: rango con guion (DTEND exclusivo)', () => {
    const label = whenLabel(
      'es',
      result({ all_day: true, dtstart: '2026-07-01', dtend: '2026-07-03' }),
    );
    expect(label.startsWith('Todo el día')).toBe(true);
    expect(label.includes('–')).toBe(true);
  });

  it('startDate de un todo-el-día cae en la fecha de calendario (sin desplazar el día)', () => {
    const d = startDate(result({ all_day: true, dtstart: '2026-07-01', dtend: '2026-07-02' }));
    expect(d.getFullYear()).toBe(2026);
    expect(d.getMonth()).toBe(6); // julio (0-based)
    expect(d.getDate()).toBe(1);
  });

  it('evento con hora produce una etiqueta no vacía', () => {
    expect(whenLabel('es', result({})).length).toBeGreaterThan(0);
  });
});

// El enlace de un evento que llega del buscador global abre la vista en su día.
describe('fechaDelEnlace', () => {
  it('lleva al instante de inicio, y un evento de todo el día a su fecha sin zona', () => {
    expect(fechaDelEnlace('?date=2026-07-01T09%3A00%3A00Z')?.toISOString()).toBe(
      '2026-07-01T09:00:00.000Z',
    );
    const todoElDia = fechaDelEnlace('?date=2026-07-01T00%3A00%3A00Z&all_day=1');
    expect(todoElDia?.getDate()).toBe(1);
    expect(todoElDia?.getMonth()).toBe(6);
  });

  it('sin fecha, o con una que no lo es, no navega', () => {
    expect(fechaDelEnlace('')).toBeNull();
    expect(fechaDelEnlace('?action=create')).toBeNull();
    expect(fechaDelEnlace('?date=ayer')).toBeNull();
    expect(fechaDelEnlace('?date=2026-99-99T00:00:00Z')).toBeNull();
  });
});
