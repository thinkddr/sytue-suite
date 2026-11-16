/**
 * LAS REGLAS DE SILENCIO DE LA BANDEJA, CON UN HUB DE MENTIRA.
 *
 * Se vigila:
 * - que salen TODOS los orígenes que pueden avisar (no seis), agrupados y con nombre;
 * - que mientras cargan no hay nada que pulsar;
 * - que un fallo de carga no enseña interruptores —salían todos encendidos aunque
 *   hubiera cosas calladas— y trae «Reintentar»;
 * - que un fallo de guardado devuelve el interruptor a su sitio y lo dice en su fila;
 * - que la pantalla dice de qué organización son las reglas;
 * - que «Por tipo» despliega los tipos de cada origen, guarda la regla del tipo, enseña
 *   lo que decide el Hub (la regla más específica gana) y deja fijos los críticos.
 *
 * Vistos en ROJO con la sección de antes, que vivía dentro de NotificationsInbox.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, describe, expect, test } from 'bun:test';
import { render, type ComponentChild } from 'preact';
import { act } from 'preact/test-utils';
import type { NotificationPref } from '@sytue/api-clients';
import { ORIGENES_DE_AVISOS } from '@sytue/shell';
import { en } from '../i18n/messages/en.js';
import type { FuentePreferencias } from './PreferenciasAvisos.js';

const registrado = typeof document === 'undefined';
if (registrado) GlobalRegistrator.register({ url: 'https://one.sytue.test/notificaciones' });

const { PreferenciasAvisos } = await import('./PreferenciasAvisos.js');

afterEach(() => {
  act(() => render(null, document.body));
  document.body.innerHTML = '';
});

afterAll(async () => {
  if (registrado) await GlobalRegistrator.unregister();
});

async function esperar(cond: () => boolean): Promise<void> {
  for (let i = 0; i < 50 && !cond(); i++) {
    await act(async () => {
      await new Promise((r) => setTimeout(r, 5));
    });
  }
  expect(cond()).toBe(true);
}

async function montar(arbol: ComponentChild): Promise<void> {
  await act(async () => {
    render(arbol, document.body);
  });
}

/** Una promesa que el test resuelve o rechaza cuando quiere. */
function diferida<T>(): {
  promesa: Promise<T>;
  resolver: (v: T) => void;
  rechazar: (e: unknown) => void;
} {
  let resolver!: (v: T) => void;
  let rechazar!: (e: unknown) => void;
  const promesa = new Promise<T>((res, rej) => {
    resolver = res;
    rechazar = rej;
  });
  return { promesa, resolver, rechazar };
}

/** Un Hub que contesta lo que se le diga y apunta lo que se le pide guardar. */
function hubFalso(opciones: {
  cargar?: () => Promise<NotificationPref[]>;
  guardar?: (p: NotificationPref) => Promise<NotificationPref>;
}): FuentePreferencias & { guardadas: NotificationPref[]; cargas: number } {
  const hub = {
    guardadas: [] as NotificationPref[],
    cargas: 0,
    notificationPrefs: async () => {
      hub.cargas++;
      return (opciones.cargar ?? (async () => []))();
    },
    setNotificationPref: async (p: NotificationPref) => {
      hub.guardadas.push(p);
      return (opciones.guardar ?? (async (x) => x))(p);
    },
  };
  return hub;
}

/** Los interruptores que se pueden tocar: los fijos (Sytue ID) van aparte. */
const interruptores = (): HTMLInputElement[] => [
  ...document.querySelectorAll<HTMLInputElement>('input[role="switch"][data-origen]'),
];
const fijo = (origen: string): HTMLInputElement | null =>
  document.querySelector<HTMLInputElement>(`input[data-fijo="${origen}"]`);
const interruptor = (origen: string): HTMLInputElement | null =>
  document.querySelector<HTMLInputElement>(`input[data-origen="${origen}"]`);
const alertas = (): string[] =>
  [...document.querySelectorAll('[role="alert"]')].map((a) => a.textContent ?? '');

const tipo = (clave: string): HTMLInputElement | null =>
  document.querySelector<HTMLInputElement>(`input[data-tipo="${clave}"]`);
const tipoFijo = (clave: string): HTMLInputElement | null =>
  document.querySelector<HTMLInputElement>(`input[data-tipo-fijo="${clave}"]`);
const porTipo = (origen: string): HTMLButtonElement | null =>
  document.querySelector<HTMLButtonElement>(`button[data-tipos="${origen}"]`);

async function desplegar(origen: string): Promise<void> {
  await act(async () => {
    porTipo(origen)!.click();
  });
}

async function pulsar(el: HTMLInputElement): Promise<void> {
  await act(async () => {
    el.click();
  });
}

describe('PreferenciasAvisos', () => {
  test('ofrece todos los orígenes de la lista compartida, agrupados y con su nombre', async () => {
    const hub = hubFalso({ cargar: async () => [{ source: 'erp', muted: true }] });
    await montar(<PreferenciasAvisos locale="en" hub={hub} organizacion="acme" />);
    await esperar(() => interruptores().length > 0);

    const todos = ORIGENES_DE_AVISOS.flatMap((g) => g.origenes);
    expect(interruptores().map((i) => i.dataset['origen'])).toEqual(todos);
    // Los que la lista de seis se dejaba fuera: ERP, Web, Cast…
    for (const o of ['erp', 'pages', 'cast', 'sheets', 'integrations']) {
      expect(interruptor(o)).not.toBeNull();
    }
    expect(interruptor('id')).toBeNull();

    const leyendas = [...document.querySelectorAll('legend')].map((l) => l.textContent ?? '');
    expect(leyendas).toEqual(
      ORIGENES_DE_AVISOS.map((g) => en[`inbox.prefsGroup.${g.grupo}` as keyof typeof en] as string),
    );
    const etiquetas = [...document.querySelectorAll('label')].map((l) => l.textContent?.trim());
    expect(etiquetas).toContain('Sytue Mail');
    expect(etiquetas).toContain(en['inbox.prefsSource.integrations']);

    // Lo callado sale apagado; lo demás, encendido.
    expect(interruptor('erp')?.checked).toBe(false);
    expect(interruptor('mail')?.checked).toBe(true);
  });

  test('los avisos de seguridad de Sytue ID salen al final, encendidos, inertes y explicados', async () => {
    // Una regla antigua que los callaba no los enseña apagados: el Hub los entrega igual.
    const hub = hubFalso({ cargar: async () => [{ source: 'id', muted: true }] });
    await montar(<PreferenciasAvisos locale="en" hub={hub} organizacion="acme" />);
    await esperar(() => fijo('id') !== null);

    const id = fijo('id')!;
    expect(id.checked).toBe(true);
    expect(id.disabled).toBe(true);
    const nota = document.getElementById(id.getAttribute('aria-describedby')!);
    expect(nota?.textContent).toBe(en['inbox.prefsAlwaysOn']);
    const todos = [...document.querySelectorAll<HTMLInputElement>('input[role="switch"]')];
    expect(todos.at(-1)).toBe(id);

    await pulsar(id);
    expect(id.checked).toBe(true);
    expect(hub.guardadas).toHaveLength(0);
  });

  test('mientras cargan no hay interruptores que pulsar', async () => {
    const carga = diferida<NotificationPref[]>();
    const hub = hubFalso({ cargar: () => carga.promesa });
    await montar(<PreferenciasAvisos locale="en" hub={hub} organizacion="acme" />);

    expect(interruptores()).toHaveLength(0);
    expect(document.querySelectorAll('[data-fantasma]').length).toBeGreaterThan(0);
    expect(document.querySelector('[aria-busy="true"]')).not.toBeNull();

    await act(async () => carga.resolver([]));
    await esperar(() => interruptores().length > 0);
    expect(document.querySelector('[data-fantasma]')).toBeNull();
  });

  test('si la carga falla no enseña interruptores, lo dice y deja reintentar', async () => {
    let falla = true;
    const hub = hubFalso({
      cargar: async () => {
        if (falla) throw new Error('503');
        return [{ source: 'mail', muted: true }];
      },
    });
    await montar(<PreferenciasAvisos locale="en" hub={hub} organizacion="acme" />);
    await esperar(() => alertas().length > 0);

    expect(interruptores()).toHaveLength(0);
    expect(alertas()[0]).toContain(en['inbox.prefsLoadError']);

    falla = false;
    const reintentar = [...document.querySelectorAll('button')].find(
      (b) => b.textContent?.trim() === en['inbox.retry'],
    );
    expect(reintentar).toBeDefined();
    await act(async () => {
      reintentar!.click();
    });
    await esperar(() => interruptores().length > 0);
    expect(hub.cargas).toBe(2);
    expect(interruptor('mail')?.checked).toBe(false);
    expect(alertas()).toHaveLength(0);
  });

  test('si el guardado falla, el interruptor vuelve a su sitio y la fila lo dice', async () => {
    const hub = hubFalso({
      guardar: async () => {
        throw new Error('500');
      },
    });
    await montar(<PreferenciasAvisos locale="en" hub={hub} organizacion="acme" />);
    await esperar(() => interruptor('cast') !== null);

    await pulsar(interruptor('cast')!);
    await esperar(() => alertas().length > 0);

    expect(hub.guardadas).toEqual([{ source: 'cast', muted: true }]);
    expect(interruptor('cast')?.checked).toBe(true);
    expect(interruptor('cast')?.disabled).toBe(false);
    expect(alertas()).toEqual([en['inbox.prefsSaveError']]);
    // El aviso va ligado a SU interruptor, no suelto en la página.
    const hint = document.getElementById(interruptor('cast')!.getAttribute('aria-describedby')!);
    expect(hint?.textContent).toBe(en['inbox.prefsSaveError']);
  });

  test('un guardado que sale bien deja el interruptor en su nueva posición', async () => {
    const hub = hubFalso({});
    await montar(<PreferenciasAvisos locale="en" hub={hub} organizacion="acme" />);
    await esperar(() => interruptor('pages') !== null);

    await pulsar(interruptor('pages')!);
    await esperar(() => hub.guardadas.length === 1);

    expect(hub.guardadas).toEqual([{ source: 'pages', muted: true }]);
    expect(interruptor('pages')?.checked).toBe(false);
    expect(alertas()).toHaveLength(0);
  });

  test('dice de qué organización son las reglas; una cuenta personal no ve ERP, Bases ni Web', async () => {
    await montar(<PreferenciasAvisos locale="en" hub={hubFalso({})} organizacion="acme" />);
    await esperar(() => interruptores().length > 0);
    expect(document.body.textContent).toContain(en['inbox.prefsScopeOrg'].replace('{org}', 'acme'));

    await montar(
      <PreferenciasAvisos locale="en" hub={hubFalso({})} organizacion="p-ana" personal />,
    );
    await esperar(
      () => document.body.textContent?.includes(en['inbox.prefsScopePersonal']) ?? false,
    );
    await esperar(() => interruptores().length > 0);
    for (const o of ['erp', 'bases', 'pages']) expect(interruptor(o)).toBeNull();
    expect(interruptor('mail')).not.toBeNull();
  });

  test('«Por tipo» despliega los tipos de un origen con su rótulo, y se vuelve a plegar', async () => {
    await montar(<PreferenciasAvisos locale="en" hub={hubFalso({})} organizacion="acme" />);
    await esperar(() => porTipo('mail') !== null);

    // Plegado de entrada: solo los interruptores de cada producto.
    const boton = porTipo('mail')!;
    expect(boton.getAttribute('aria-expanded')).toBe('false');
    expect(tipo('mail:messages')).toBeNull();
    // Un origen sin tipos (Contacts) no ofrece desplegar nada.
    expect(porTipo('contacts')).toBeNull();

    await desplegar('mail');
    expect(boton.getAttribute('aria-expanded')).toBe('true');
    const grupo = document.getElementById(boton.getAttribute('aria-controls')!);
    expect(grupo?.getAttribute('role')).toBe('group');
    expect(grupo?.getAttribute('aria-label')).toBe(
      en['inbox.prefsKindsOf'].replace('{app}', 'Sytue Mail'),
    );
    expect(
      [...grupo!.querySelectorAll<HTMLInputElement>('input[data-tipo]')].map(
        (i) => i.dataset['tipo'],
      ),
    ).toEqual(['mail:messages', 'mail:accessRequests', 'mail:accessGranted']);
    expect(grupo?.textContent).toContain(en['inbox.prefsKind.messages']);
    // Los tipos no se cuelan entre los interruptores de producto.
    expect(interruptores().map((i) => i.dataset['origen'])).not.toContain(undefined);

    await desplegar('mail');
    expect(tipo('mail:messages')).toBeNull();
  });

  test('callar un tipo guarda la regla de ese tipo y deja el producto encendido', async () => {
    const hub = hubFalso({});
    await montar(<PreferenciasAvisos locale="en" hub={hub} organizacion="acme" />);
    await esperar(() => porTipo('rooms') !== null);
    await desplegar('rooms');

    await pulsar(tipo('rooms:directMessages')!);
    await esperar(() => hub.guardadas.length === 1);
    expect(hub.guardadas).toEqual([{ source: 'rooms', kind: 'rooms.message', muted: true }]);
    expect(tipo('rooms:directMessages')?.checked).toBe(false);
    expect(tipo('rooms:mentions')?.checked).toBe(true);
    expect(interruptor('rooms')?.checked).toBe(true);

    // Una fila que reúne dos tipos del Hub los guarda los dos.
    await pulsar(tipo('rooms:invitations')!);
    await esperar(() => hub.guardadas.length === 3);
    expect(hub.guardadas.slice(1)).toEqual([
      { source: 'rooms', kind: 'rooms.invitation', muted: true },
      { source: 'rooms', kind: 'rooms.invitation.direct', muted: true },
    ]);
    expect(tipo('rooms:invitations')?.checked).toBe(false);
  });

  test('enseña lo que decide el Hub: con el producto callado, solo llega lo que se dejó', async () => {
    const hub = hubFalso({
      cargar: async () => [
        { source: 'mail', muted: true },
        { source: 'mail', kind: 'mail.access.requested', muted: false },
      ],
    });
    await montar(<PreferenciasAvisos locale="en" hub={hub} organizacion="acme" />);
    await esperar(() => porTipo('mail') !== null);
    await desplegar('mail');

    expect(interruptor('mail')?.checked).toBe(false);
    expect(tipo('mail:messages')?.checked).toBe(false);
    expect(tipo('mail:accessRequests')?.checked).toBe(true);
    expect(tipo('mail:accessGranted')?.checked).toBe(false);
  });

  test('callar el producto después calla también los tipos que se apagaron y encendieron', async () => {
    const hub = hubFalso({});
    await montar(<PreferenciasAvisos locale="en" hub={hub} organizacion="acme" />);
    await esperar(() => porTipo('mail') !== null);
    await desplegar('mail');

    // Apagar y encender un tipo con el producto encendido no deja contra-regla...
    await pulsar(tipo('mail:messages')!);
    await esperar(() => hub.guardadas.length === 1 && !tipo('mail:messages')!.disabled);
    await pulsar(tipo('mail:messages')!);
    await esperar(() => hub.guardadas.length === 2 && !tipo('mail:messages')!.disabled);
    expect(tipo('mail:messages')?.checked).toBe(true);

    // ...así que callar el producto entero lo calla, como hará el Hub.
    await pulsar(interruptor('mail')!);
    await esperar(() => hub.guardadas.length === 3);
    expect(interruptor('mail')?.checked).toBe(false);
    expect(tipo('mail:messages')?.checked).toBe(false);
  });

  test('volver a permitir el producto se lleva las excepciones que tenía', async () => {
    const hub = hubFalso({
      cargar: async () => [
        { source: 'mail', muted: true },
        { source: 'mail', kind: 'mail.access.requested', muted: false },
      ],
    });
    await montar(<PreferenciasAvisos locale="en" hub={hub} organizacion="acme" />);
    await esperar(() => porTipo('mail') !== null);
    await desplegar('mail');

    await pulsar(interruptor('mail')!);
    await esperar(() => hub.guardadas.length === 1 && !interruptor('mail')!.disabled);
    await pulsar(interruptor('mail')!);
    await esperar(() => hub.guardadas.length === 2 && !interruptor('mail')!.disabled);
    expect(tipo('mail:accessRequests')?.checked).toBe(false);
  });

  test('los tipos críticos salen fijos y encendidos aunque se calle el producto', async () => {
    const hub = hubFalso({
      cargar: async () => [
        { source: 'pages', muted: true },
        // Una regla de antes de que el Hub la rechazara no los enseña apagados.
        { source: 'pages', kind: 'pages.withdrawal.requested', muted: true },
      ],
    });
    await montar(<PreferenciasAvisos locale="en" hub={hub} organizacion="acme" />);
    await esperar(() => porTipo('pages') !== null);
    await desplegar('pages');

    expect(tipo('pages:orders')?.checked).toBe(false);
    for (const clave of ['pages:storeIncidents', 'pages:withdrawals']) {
      const f = tipoFijo(clave)!;
      expect(f.checked).toBe(true);
      expect(f.disabled).toBe(true);
      expect(tipo(clave)).toBeNull();
      const nota = document.getElementById(f.getAttribute('aria-describedby')!);
      expect(nota?.textContent).toBe(en['inbox.prefsKindCritical']);
      await pulsar(f);
      expect(f.checked).toBe(true);
    }
    expect(hub.guardadas).toHaveLength(0);
  });

  test('si falla el guardado de un tipo, vuelve a su sitio y su fila lo dice', async () => {
    const hub = hubFalso({
      guardar: async (p) => {
        // De las dos invitaciones, solo la directa falla.
        if (p.kind === 'rooms.invitation.direct') throw new Error('500');
        return p;
      },
    });
    await montar(<PreferenciasAvisos locale="en" hub={hub} organizacion="acme" />);
    await esperar(() => porTipo('rooms') !== null);
    await desplegar('rooms');

    await pulsar(tipo('rooms:invitations')!);
    await esperar(() => alertas().length > 0);

    // La que no se guardó sigue llegando, así que la fila sigue encendida, y lo dice.
    expect(tipo('rooms:invitations')?.checked).toBe(true);
    expect(tipo('rooms:invitations')?.disabled).toBe(false);
    const hint = document.getElementById(
      tipo('rooms:invitations')!.getAttribute('aria-describedby')!,
    );
    expect(hint?.textContent).toBe(en['inbox.prefsSaveError']);
    expect(interruptor('rooms')?.getAttribute('aria-describedby')).toBeNull();
  });
});
