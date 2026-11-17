/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * LOS AJUSTES DE CALENDAR, de punta a punta: la página cambia cada valor sin botón de
 * guardar, y lo elegido se nota donde toca —la semana sin sábado ni domingo, el número
 * de semana en el mes y en la semana, la jornada atenuada fuera de horas y el
 * recordatorio que trae un evento nuevo—. Sin ajustes, todo como siempre.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, describe, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import { useT } from '../i18n/index.js';
import { AJUSTES_POR_DEFECTO, type AjustesCalendario } from '../lib/preferencias.js';
import { CalendarGrid, diasDeSemana } from './CalendarGrid.js';
import { CalendarSettings, etiquetaDuracion } from './CalendarSettings.js';
import { recordatoriosIniciales } from './EventModal.js';

const registrado = typeof document === 'undefined';
if (registrado) GlobalRegistrator.register();
afterEach(() => {
  render(null, document.body);
});
afterAll(async () => {
  if (registrado) await GlobalRegistrator.unregister();
});

function Pagina({
  ajustes,
  onChange,
}: {
  ajustes: AjustesCalendario;
  onChange: (a: AjustesCalendario) => void;
}) {
  const t = useT('es');
  return <CalendarSettings t={t} ajustes={ajustes} onChange={onChange} onClose={() => {}} />;
}

function control<E extends HTMLElement>(nombre: string): E {
  const el = document.querySelector(`[data-ajuste="${nombre}"]`);
  if (!el) throw new Error(`sin control ${nombre}`);
  return el as E;
}

async function rejilla(mode: 'month' | 'week', vista?: AjustesCalendario): Promise<void> {
  await act(() => {
    render(
      <CalendarGrid
        locale="es"
        mode={mode}
        cursor={new Date(2026, 10, 11)}
        events={[]}
        onSelectEvent={() => {}}
        onCreateAt={() => {}}
        onCreateRange={() => {}}
        onMoveResize={async () => {}}
        vista={vista}
      />,
      document.body,
    );
  });
}

describe('página de ajustes de Calendar', () => {
  test('cada control cambia su ajuste al momento', async () => {
    const cambios: AjustesCalendario[] = [];
    await act(() => {
      render(
        <Pagina ajustes={{ ...AJUSTES_POR_DEFECTO }} onChange={(a) => cambios.push(a)} />,
        document.body,
      );
    });
    expect(document.querySelector('h2')?.textContent).toBe('Ajustes del calendario');

    const elegir = async (nombre: string, valor: string): Promise<void> => {
      const s = control<HTMLSelectElement>(nombre);
      s.value = valor;
      await act(() => {
        s.dispatchEvent(new Event('change', { bubbles: true }));
      });
    };
    await elegir('duracion', '30');
    await elegir('recordatorio', '10');
    await elegir('recordatorio', '');
    await elegir('inicioLaboral', '7');
    const finde = control<HTMLInputElement>('finesDeSemana');
    finde.checked = false;
    await act(() => {
      finde.dispatchEvent(new Event('change', { bubbles: true }));
    });

    expect(cambios.map((c) => c.duracion)).toEqual([30, 60, 60, 60, 60]);
    expect(cambios.map((c) => c.recordatorio)).toEqual([null, 10, null, null, null]);
    expect(cambios[3]?.inicioLaboral).toBe(7);
    expect(cambios[4]?.finesDeSemana).toBe(false);
  });

  test('la jornada no se puede elegir al revés', async () => {
    await act(() => {
      render(<Pagina ajustes={{ ...AJUSTES_POR_DEFECTO }} onChange={() => {}} />, document.body);
    });
    const inicios = [...control<HTMLSelectElement>('inicioLaboral').options].map((o) => o.value);
    const fines = [...control<HTMLSelectElement>('finLaboral').options].map((o) => o.value);
    expect(inicios.at(-1)).toBe('17');
    expect(fines[0]).toBe('10');
  });

  test('las duraciones se dicen en su unidad', () => {
    const t = (k: string, p?: Record<string, unknown>) => `${k}:${String(p?.['count'])}`;
    expect(etiquetaDuracion(t as never, 45)).toBe('settings.durationMinutes:45');
    expect(etiquetaDuracion(t as never, 120)).toBe('settings.durationHours:2');
  });
});

describe('los ajustes en la rejilla', () => {
  test('sin fines de semana, la semana va de lunes a viernes', async () => {
    expect(diasDeSemana(new Date(2026, 10, 11), false).map((d) => d.getDay())).toEqual([
      1, 2, 3, 4, 5,
    ]);
    await rejilla('week', { ...AJUSTES_POR_DEFECTO, finesDeSemana: false });
    const columnas = document.querySelector<HTMLElement>('[style*="grid-template-columns"]');
    expect(columnas?.style.gridTemplateColumns).toContain('repeat(5');
    await rejilla('week');
    const siete = document.querySelector<HTMLElement>('[style*="grid-template-columns"]');
    expect(siete?.style.gridTemplateColumns).toContain('repeat(7');
  });

  test('el número de semana sale en el mes y en la semana solo si se pide', async () => {
    await rejilla('month');
    expect(document.querySelectorAll('[data-numero-semana]')).toHaveLength(0);
    await rejilla('month', { ...AJUSTES_POR_DEFECTO, numeroSemana: true });
    const filas = document.querySelectorAll('[data-numero-semana]');
    expect(filas.length).toBeGreaterThanOrEqual(5);
    // Noviembre de 2026 empieza en la semana 44 (lunes 26 de octubre).
    expect(filas[0]?.textContent).toContain('Semana 44');
    await rejilla('week', { ...AJUSTES_POR_DEFECTO, numeroSemana: true });
    expect(document.querySelector('[data-numero-semana]')?.textContent).toContain('Semana 46');
  });

  test('las horas fuera de la jornada se atenúan', async () => {
    await rejilla('week', { ...AJUSTES_POR_DEFECTO, inicioLaboral: 8, finLaboral: 17 });
    const fuera = [...document.querySelectorAll<HTMLElement>('[data-fuera-de-jornada]')];
    // Dos franjas por columna: antes de las 8 y desde las 17.
    expect(fuera).toHaveLength(14);
    expect(fuera[0]?.style.height).toBe(`${8 * 48}px`);
    expect(fuera[1]?.style.top).toBe(`${17 * 48}px`);
  });
});

describe('recordatorio por defecto', () => {
  const nuevo = { href: null, summary: '', start: new Date(2026, 10, 11, 9) };
  test('un evento nuevo trae el de los ajustes', () => {
    expect(recordatoriosIniciales(nuevo, 15)).toEqual([15]);
    expect(recordatoriosIniciales(nuevo, null)).toEqual([]);
  });
  test('uno que ya existe, o un duplicado con alarmas, conserva las suyas', () => {
    expect(recordatoriosIniciales({ ...nuevo, href: '/c/a.ics' }, 15)).toEqual([]);
    const alarma = 'BEGIN:VALARM\r\nACTION:DISPLAY\r\nTRIGGER:-PT30M\r\nEND:VALARM';
    expect(recordatoriosIniciales({ ...nuevo, alarms: [alarma] }, 15)).toEqual([30]);
  });
});
