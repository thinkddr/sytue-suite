/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Posponer y silenciar una conversación (D-MAIL-07). Las fechas fijas del menú
 * dependen de la hora y del día (no se ofrece «Más tarde hoy» por la noche ni «Este
 * fin de semana» un viernes); el lector enseña el estado del hilo con su salida a
 * mano; y en «Pospuestos» la fila dice cuándo VUELVE. Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, describe, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import type { EmailSummary } from '@sytue/api-clients';

if (typeof document === 'undefined') GlobalRegistrator.register();
process.env.BASE_URL ??= '/mail/';

globalThis.fetch = (async () =>
  new Response(JSON.stringify({}), {
    status: 200,
    headers: { 'content-type': 'application/json' },
  })) as unknown as typeof fetch;

const { ThemeProvider } = await import('@sytue/ui');
const { opcionesDePosponer, PosponerMenu } = await import('./PosponerMenu.js');
const { MessageReader } = await import('./MessageReader.js');
const { groupByThread, sortMailboxes, mailboxLabel, idsDeLaEntrada } = await import('./utils.js');
const { destinosDeRegla } = await import('./FiltersModal.js');
const { useT } = await import('../i18n/index.js');

afterEach(() => {
  document.body.innerHTML = '';
});

/** Fecha local (no UTC): las opciones se calculan en la hora de quien pospone. */
const local = (y: number, m: number, d: number, h: number, min = 0): Date =>
  new Date(y, m - 1, d, h, min);

describe('opcionesDePosponer', () => {
  test('un lunes por la mañana ofrece las cuatro, a las horas de siempre', () => {
    // 2026-11-09 es lunes.
    const ops = opcionesDePosponer(local(2026, 11, 9, 10, 30));
    expect(ops.map((o) => o.opcion)).toEqual(['laterToday', 'tomorrow', 'weekend', 'nextWeek']);
    expect(ops.map((o) => o.fecha.getTime())).toEqual([
      local(2026, 11, 9, 18).getTime(),
      local(2026, 11, 10, 8).getTime(),
      local(2026, 11, 14, 8).getTime(), // sábado
      local(2026, 11, 16, 8).getTime(), // lunes siguiente
    ]);
  });

  test('un viernes por la noche no ofrece «Más tarde hoy» ni «Este fin de semana»', () => {
    const ops = opcionesDePosponer(local(2026, 11, 13, 20));
    expect(ops.map((o) => o.opcion)).toEqual(['tomorrow', 'nextWeek']);
    expect(ops[0]?.fecha.getTime()).toBe(local(2026, 11, 14, 8).getTime());
    expect(ops[1]?.fecha.getTime()).toBe(local(2026, 11, 16, 8).getTime());
  });

  test('un domingo, «Mañana» ya es el lunes y no se repite como «La semana que viene»', () => {
    const ops = opcionesDePosponer(local(2026, 11, 15, 9));
    expect(ops.map((o) => o.opcion)).toEqual(['laterToday', 'tomorrow']);
    expect(ops[1]?.fecha.getTime()).toBe(local(2026, 11, 16, 8).getTime());
  });
});

describe('«Pospuestos» en el lateral', () => {
  test('va justo debajo de Recibidos y se rotula por su rol', () => {
    const cajas = sortMailboxes('es', [
      { id: 'a', name: 'Archive', role: 'archive' },
      { id: 'z', name: 'Proyectos' },
      { id: 's', name: 'Snoozed', role: 'snoozed' },
      { id: 'i', name: 'Inbox', role: 'inbox' },
      { id: 'd', name: 'Drafts', role: 'drafts' },
    ]);
    expect(cajas.map((c) => c.id)).toEqual(['i', 's', 'd', 'a', 'z']);
    function Probe() {
      return <span>{mailboxLabel(useT('es'), 'Snoozed', 'snoozed')}</span>;
    }
    const host = document.body.appendChild(document.createElement('div'));
    render(<Probe />, host);
    expect(host.textContent).toBe('Pospuestos');
  });
});

function boton(host: ParentNode, texto: string): HTMLElement {
  const b = Array.from(host.querySelectorAll<HTMLElement>('button, [role="menuitem"]')).find((el) =>
    el.textContent?.trim().startsWith(texto),
  );
  if (!b) throw new Error(`no está «${texto}»`);
  return b;
}

describe('PosponerMenu', () => {
  test('elegir una fecha fija la pasa tal cual; la personalizada se valida junto al campo', async () => {
    const elegidas: Date[] = [];
    const host = document.body.appendChild(document.createElement('div'));
    await act(async () => {
      render(
        <ThemeProvider>
          <PosponerMenu locale="es" onPosponer={(d) => elegidas.push(d)} />
        </ThemeProvider>,
        host,
      );
    });
    await act(async () => boton(host, '').click()); // el disparador (solo icono)
    await act(async () => boton(document.body, 'Mañana').click());
    expect(elegidas).toHaveLength(1);
    const manana = new Date();
    manana.setDate(manana.getDate() + 1);
    manana.setHours(8, 0, 0, 0);
    expect(elegidas[0]?.getTime()).toBe(manana.getTime());

    // «Elegir fecha y hora…» con una fecha pasada: se dice en el campo y no se pospone.
    await act(async () => boton(host, '').click());
    await act(async () => boton(document.body, 'Elegir fecha y hora').click());
    const campo = document.getElementById('posponer-fecha') as HTMLInputElement;
    expect(campo).toBeTruthy();
    await act(async () => {
      campo.value = '2020-01-01T08:00';
      campo.dispatchEvent(new Event('input', { bubbles: true }));
    });
    await act(async () => boton(document.body, 'Posponer').click());
    expect(elegidas).toHaveLength(1);
    expect(document.body.textContent).toContain('Elige una fecha y hora entre');
    expect(campo.getAttribute('aria-invalid')).toBe('true');
  });
});

const correo: EmailSummary = {
  id: 'm1',
  threadId: 'h1',
  subject: 'Cadena de todos',
  from: [{ email: 'ana@ejemplo.eu' }],
  to: [{ email: 'yo@ejemplo.eu' }],
  cc: [],
  preview: 'Hola',
  receivedAt: '2026-11-05T10:00:00Z',
  keywords: { $seen: true },
  hasAttachment: false,
};

interface Extra {
  snoozedUntil?: string | null;
  muted?: boolean;
  onSnooze?: ((c: unknown, d: Date) => void) | null;
  onUnsnooze?: ((c: unknown) => void) | null;
  onMute?: ((c: unknown, m: boolean) => void) | null;
}

async function montarLector(extra: Extra): Promise<HTMLElement> {
  const host = document.body.appendChild(document.createElement('div'));
  const nada = (): void => {};
  await act(async () => {
    render(
      <ThemeProvider>
        <MessageReader
          locale="es"
          conversation={groupByThread([correo])[0] ?? null}
          onReply={nada}
          onCompose={nada}
          onToggleFlag={nada}
          onMarkUnread={nada}
          onArchive={null}
          onTrash={null}
          onSpam={null}
          onNotSpam={null}
          onPurge={null}
          onMove={nada}
          moveTargets={[]}
          labels={[]}
          onToggleLabel={nada}
          {...(extra as object)}
        />
      </ThemeProvider>,
      host,
    );
  });
  return host;
}

describe('el lector', () => {
  test('un hilo pospuesto lo dice, con la fecha y «Volver a Recibidos»', async () => {
    const vueltas: unknown[] = [];
    const host = await montarLector({
      snoozedUntil: '2026-12-01T08:00:00Z',
      onUnsnooze: (c) => vueltas.push(c),
      onSnooze: () => {},
    });
    expect(host.textContent).toContain('Pospuesto hasta el');
    expect(host.textContent).toContain('2026');
    // Ya pospuesto, el botón de posponer cambia la fecha.
    expect(host.querySelector('[aria-label="Cambiar la fecha"]')).toBeTruthy();
    await act(async () => boton(host, 'Volver a Recibidos').click());
    expect(vueltas).toHaveLength(1);
  });

  test('sin posponer ni silenciar no hay franja, y sin onSnooze no hay botón', async () => {
    const host = await montarLector({});
    expect(host.textContent).not.toContain('Pospuesto hasta');
    expect(host.textContent).not.toContain('silenciado');
    expect(host.querySelector('[aria-label="Posponer"]')).toBeNull();
  });

  test('silenciar está en el menú de más acciones, y un hilo silenciado ofrece deshacerlo', async () => {
    const cambios: boolean[] = [];
    let host = await montarLector({ onMute: (_c, m) => cambios.push(m) });
    const mas = host.querySelector<HTMLElement>('[aria-label="Más acciones"]');
    expect(mas).toBeTruthy();
    await act(async () => mas!.click());
    await act(async () => boton(document.body, 'Silenciar conversación').click());
    expect(cambios).toEqual([true]);

    document.body.innerHTML = '';
    host = await montarLector({ muted: true, onMute: (_c, m) => cambios.push(m) });
    expect(host.textContent).toContain('Has silenciado esta conversación');
    await act(async () => boton(host, 'Dejar de silenciar').click());
    expect(cambios).toEqual([true, false]);
  });
});

describe('lo que no debe acabar en «Pospuestos» ni salir de ella sin querer', () => {
  test('una regla no puede mandar el correo a «Pospuestos»', () => {
    const roles = destinosDeRegla([
      { id: 'in', role: 'inbox' },
      { id: 'sn', role: 'snoozed' },
      { id: 'ar', role: 'archive' },
      { id: 'pr', role: '' },
    ]).map((m) => m.id);
    expect(roles).toEqual(['in', 'ar', 'pr']);
  });

  test('«Deshacer» silenciar devuelve a Recibidos solo lo que estaba en Recibidos', () => {
    const conv = {
      messages: [
        { id: 'a', mailboxId: 'in' },
        { id: 'b', mailboxId: 'sent' },
        { id: 'c', mailboxId: 'arch' },
        { id: 'd' },
      ] as unknown as EmailSummary[],
    };
    // Desde una búsqueda (fuera de Recibidos), lo que no dice su carpeta no se mueve.
    expect(idsDeLaEntrada(conv, 'in', false)).toEqual(['a']);
    // En Recibidos, lo que no dice su carpeta es de la carpeta vista.
    expect(idsDeLaEntrada(conv, 'in', true)).toEqual(['a', 'd']);
  });
});
