/**
 * La campana de punta a punta, con un Hub de mentira: lo que llega por el flujo se
 * ve, cuenta, deja de contar al leerlo y, tras un corte, vuelve con la lista al día.
 * Es la mitad web del guardián de «Todo al día con cosas pasando» (2026-09-24).
 */
import { describe, expect, it } from 'bun:test';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import type { Notification } from '@sytue/api-clients';
import { catalogs } from '@sytue/i18n';
import {
  abrirAviso,
  alLlegarAviso,
  conectarAvisos,
  esConcesion,
  cuerpoAviso,
  destinoAviso,
  fotoConVersion,
  nombreDeOrganizacion,
  ORIGENES_DE_AVISOS,
  procedenciaAviso,
  TIPOS_CRITICOS,
  TIPOS_DE_AVISOS,
  tituloAviso,
  type EstadoAvisos,
  type FlujoAvisos,
  type FuenteAvisos,
} from './avisos.js';

class FlujoDoble implements FlujoAvisos {
  oyentes = new Map<string, ((e: { data?: unknown }) => void)[]>();
  cerrado = false;
  addEventListener(tipo: string, fn: (e: { data?: unknown }) => void): void {
    this.oyentes.set(tipo, [...(this.oyentes.get(tipo) ?? []), fn]);
  }
  close(): void {
    this.cerrado = true;
  }
  emitir(tipo: string, data?: unknown): void {
    for (const fn of this.oyentes.get(tipo) ?? []) fn({ data });
  }
}

const aviso = (id: string, extra: Partial<Notification> = {}): Notification => ({
  id,
  source: 'docs',
  type: 'docs.comment',
  title: 'Comentario en Presupuesto',
  params: { actor: 'Ana', name: 'Presupuesto' },
  url: 'https://docus.sytue.eu/docus/edit?id=d1',
  created_at: '2026-09-24T10:00:00Z',
  read: false,
  ...extra,
});

function hubDoble(lista: Notification[], sinLeer: number, correo: number | null = null) {
  const flujos: FlujoDoble[] = [];
  const leidos: string[] = [];
  let todos = 0;
  let cargas = 0;
  let pendientes = sinLeer;
  let correoPendiente = correo;
  let filas = [...lista];
  const hub: FuenteAvisos = {
    notifications: async () => {
      cargas++;
      return [...filas];
    },
    contadores: async () => ({ unread: pendientes, mailUnread: correoPendiente }),
    markRead: async (id: string) => {
      leidos.push(id);
      if (filas.some((n) => n.id === id && !n.read)) pendientes = Math.max(0, pendientes - 1);
      filas = filas.map((n) => (n.id === id ? { ...n, read: true } : n));
    },
    markAllRead: async () => {
      todos++;
      pendientes = 0;
      filas = filas.map((n) => ({ ...n, read: true }));
    },
    streamNotifications: async () => {
      const f = new FlujoDoble();
      f.addEventListener('notification', (e) => {
        try {
          const n = JSON.parse(String(e.data)) as Notification;
          if (!n.id) return;
          const anterior = filas.find((fila) => fila.id === n.id);
          if (!anterior && !n.read) pendientes++;
          if (!anterior && n.type === 'mail.message.received' && correoPendiente !== null)
            correoPendiente++;
          filas = [
            { ...n, read: n.read || Boolean(anterior?.read) },
            ...filas.filter((fila) => fila.id !== n.id),
          ];
        } catch {
          // El doble conserva solo avisos válidos.
        }
      });
      flujos.push(f);
      return f;
    },
  };
  // Un aviso que aparece en el Hub sin pasar por el flujo (la entrada privada de One).
  const agregar = (n: Notification) => {
    filas = [n, ...filas];
    if (!n.read) pendientes++;
  };
  return { hub, flujos, leidos, todos: () => todos, cargas: () => cargas, agregar };
}

const tick = () => new Promise((r) => setTimeout(r, 0));

function aplazado<T>() {
  let resolver!: (value: T) => void;
  let rechazar!: (error: Error) => void;
  const promesa = new Promise<T>((resolve, reject) => {
    resolver = resolve;
    rechazar = reject;
  });
  return { promesa, resolver, rechazar };
}

async function hasta(comprobar: () => boolean): Promise<void> {
  for (let i = 0; i < 100; i++) {
    if (comprobar()) return;
    await Promise.resolve();
  }
  throw new Error('no se alcanzó el estado esperado');
}

describe('cargas y gestos intercalados', () => {
  it('descarta la lista y el contador de una carga anterior', async () => {
    const listas = [aplazado<Notification[]>(), aplazado<Notification[]>()];
    const cuentas = [
      aplazado<{ unread: number; mailUnread: number | null }>(),
      aplazado<{ unread: number; mailUnread: number | null }>(),
    ];
    let li = 0;
    let ci = 0;
    const hub: FuenteAvisos = {
      notifications: () => listas[li++]!.promesa,
      contadores: () => cuentas[ci++]!.promesa,
      markRead: async () => undefined,
      markAllRead: async () => undefined,
      streamNotifications: async () => new FlujoDoble(),
    };
    let estado: EstadoAvisos | undefined;
    const avisos = conectarAvisos(hub, (e) => (estado = e), { limite: 30 });
    avisos.reintentar();
    listas[1]!.resolver([aviso('nuevo')]);
    cuentas[1]!.resolver({ unread: 2, mailUnread: 1 });
    await hasta(() => estado?.items[0]?.id === 'nuevo' && estado.sinLeer === 2);
    listas[0]!.resolver([aviso('viejo')]);
    cuentas[0]!.resolver({ unread: 99, mailUnread: 99 });
    await listas[0]!.promesa;
    await cuentas[0]!.promesa;
    await Promise.resolve();
    expect(estado?.items.map((n) => n.id)).toEqual(['nuevo']);
    expect(estado?.sinLeer).toBe(2);
    expect(estado?.correoSinLeer).toBe(1);
    avisos.cerrar();
  });

  it('conserva un evento llegado durante la carga y vuelve a contar sin tormenta', async () => {
    const lista = aplazado<Notification[]>();
    const cuentas = [
      aplazado<{ unread: number; mailUnread: number | null }>(),
      aplazado<{ unread: number; mailUnread: number | null }>(),
    ];
    const flujo = new FlujoDoble();
    const timers: { fn: () => void; ms: number; cancelado: boolean }[] = [];
    let ci = 0;
    let estado: EstadoAvisos | undefined;
    const hub: FuenteAvisos = {
      notifications: () => lista.promesa,
      contadores: () => cuentas[ci++]!.promesa,
      markRead: async () => undefined,
      markAllRead: async () => undefined,
      streamNotifications: async () => flujo,
    };
    const avisos = conectarAvisos(hub, (e) => (estado = e), {
      limite: 30,
      programar: (fn, ms) => {
        const timer = { fn, ms, cancelado: false };
        timers.push(timer);
        return () => {
          timer.cancelado = true;
        };
      },
    });
    await hasta(() => flujo.oyentes.has('notification'));
    flujo.emitir('notification', JSON.stringify(aviso('nuevo')));
    lista.resolver([aviso('anterior')]);
    cuentas[0]!.resolver({ unread: 0, mailUnread: null });
    await hasta(() => estado?.items.length === 2 && timers.length === 1);
    expect(estado?.items.map((n) => n.id)).toEqual(['nuevo', 'anterior']);
    expect(estado?.sinLeer).toBe(1);
    expect(timers[0]?.ms).toBe(500);
    timers[0]!.fn();
    cuentas[1]!.resolver({ unread: 2, mailUnread: null });
    await hasta(() => estado?.sinLeer === 2);
    expect(ci).toBe(2);
    avisos.cerrar();
  });

  it('al fallar leer todos conserva el aviso nuevo y no restaura toda la lista', async () => {
    const escritura = aplazado<void>();
    const cargaSiguiente = aplazado<Notification[]>();
    const cuentaSiguiente = aplazado<{ unread: number; mailUnread: number | null }>();
    const flujo = new FlujoDoble();
    let cargas = 0;
    let llamadas = 0;
    let cuentas = 0;
    let estado: EstadoAvisos | undefined;
    const hub: FuenteAvisos = {
      notifications: () =>
        ++cargas === 1 ? Promise.resolve([aviso('a')]) : cargaSiguiente.promesa,
      contadores: () =>
        ++cuentas === 1
          ? Promise.resolve({ unread: 1, mailUnread: null })
          : cuentaSiguiente.promesa,
      markRead: async () => undefined,
      markAllRead: () => {
        llamadas++;
        return escritura.promesa;
      },
      streamNotifications: async () => flujo,
    };
    const avisos = conectarAvisos(hub, (e) => (estado = e), { limite: 30 });
    await hasta(() => estado?.items.length === 1 && flujo.oyentes.has('notification'));
    const accion = avisos.leerTodos();
    await hasta(() => llamadas === 1);
    flujo.emitir('notification', JSON.stringify(aviso('b')));
    escritura.rechazar(new Error('fallo'));
    await accion;
    expect(estado?.items.map((n) => n.id)).toEqual(['b', 'a']);
    expect(estado?.items.every((n) => !n.read)).toBe(true);
    expect(estado?.sinLeer).toBe(2);
    const final = estado;
    avisos.cerrar();
    cargaSiguiente.resolver([]);
    await cargaSiguiente.promesa;
    await Promise.resolve();
    expect(estado).toBe(final);
  });

  it('serializa leer y leer todos, conservando avisos que llegaron entre gestos', async () => {
    const uno = aplazado<void>();
    const todos = aplazado<void>();
    const posteriores = aplazado<Notification[]>();
    const flujo = new FlujoDoble();
    let cargas = 0;
    let llamadasUno = 0;
    let llamadasTodos = 0;
    let estado: EstadoAvisos | undefined;
    const hub: FuenteAvisos = {
      notifications: () =>
        ++cargas === 1 ? Promise.resolve([aviso('a'), aviso('b')]) : posteriores.promesa,
      contadores: async () => ({ unread: 2, mailUnread: null }),
      markRead: () => {
        llamadasUno++;
        return uno.promesa;
      },
      markAllRead: () => {
        llamadasTodos++;
        return todos.promesa;
      },
      streamNotifications: async () => flujo,
    };
    const avisos = conectarAvisos(hub, (e) => (estado = e), { limite: 30 });
    await hasta(() => estado?.items.length === 2 && flujo.oyentes.has('notification'));
    const primera = avisos.leer('a');
    const segunda = avisos.leerTodos();
    await hasta(() => llamadasUno === 1);
    expect(llamadasTodos).toBe(0);
    flujo.emitir('notification', JSON.stringify(aviso('c')));
    uno.resolver(undefined);
    await primera;
    await hasta(() => llamadasTodos === 1);
    todos.rechazar(new Error('fallo'));
    await segunda;
    expect(estado?.items.map((n) => [n.id, n.read])).toEqual([
      ['c', false],
      ['b', false],
      ['a', true],
    ]);
    expect(estado?.sinLeer).toBe(2);
    avisos.cerrar();
  });

  it('no permite que el duplicado del flujo ni una página vieja deshagan una lectura', async () => {
    const escritura = aplazado<void>();
    const posteriores = aplazado<Notification[]>();
    const cuentaSiguiente = aplazado<{ unread: number; mailUnread: number | null }>();
    const flujo = new FlujoDoble();
    let cargas = 0;
    let llamada = false;
    let cuentas = 0;
    let estado: EstadoAvisos | undefined;
    const hub: FuenteAvisos = {
      notifications: () => (++cargas === 1 ? Promise.resolve([aviso('a')]) : posteriores.promesa),
      contadores: () =>
        ++cuentas === 1
          ? Promise.resolve({ unread: 1, mailUnread: null })
          : cuentaSiguiente.promesa,
      markRead: () => {
        llamada = true;
        return escritura.promesa;
      },
      markAllRead: async () => undefined,
      streamNotifications: async () => flujo,
    };
    const avisos = conectarAvisos(hub, (e) => (estado = e), { limite: 1 });
    await hasta(() => estado?.items.length === 1 && flujo.oyentes.has('notification'));
    const accion = avisos.leer('a');
    await hasta(() => llamada);
    flujo.emitir('notification', JSON.stringify(aviso('a')));
    expect(estado?.items[0]?.read).toBe(true);
    await avisos.masAntiguos();
    expect(cargas).toBe(1);
    escritura.resolver(undefined);
    await accion;
    flujo.emitir('notification', JSON.stringify(aviso('a')));
    expect(estado?.items[0]?.read).toBe(true);
    expect(estado?.sinLeer).toBe(0);
    avisos.cerrar();
  });

  it('concilia tras ready el aviso creado entre la primera carga y la suscripción', async () => {
    const primera = aplazado<Notification[]>();
    const segunda = aplazado<Notification[]>();
    const flujo = new FlujoDoble();
    const llegados: string[] = [];
    let cargas = 0;
    let estado: EstadoAvisos | undefined;
    const a = aviso('a', { created_at: '2026-09-24T10:00:00Z' });
    const b = aviso('b', { created_at: '2026-09-24T10:00:01Z' });
    const hub: FuenteAvisos = {
      notifications: () => (++cargas === 1 ? primera.promesa : segunda.promesa),
      contadores: async () => ({ unread: 2, mailUnread: null }),
      markRead: async () => undefined,
      markAllRead: async () => undefined,
      streamNotifications: async () => flujo,
    };
    const avisos = conectarAvisos(hub, (e) => (estado = e), {
      limite: 30,
      alLlegar: (n) => llegados.push(n.id),
    });
    await hasta(() => flujo.oyentes.has('ready'));
    flujo.emitir('ready');
    primera.resolver([a]);
    await hasta(() => cargas === 2);
    segunda.resolver([b, a]);
    await hasta(() => estado?.items[0]?.id === 'b');
    expect(llegados).toEqual(['b']);
    expect(estado?.items.map((n) => n.id)).toEqual(['b', 'a']);
    avisos.cerrar();
  });

  it('concilia ready durante una escritura incluso si la primera carga quedó obsoleta', async () => {
    const inicial = aplazado<Notification[]>();
    const base = aplazado<Notification[]>();
    const reconciliada = aplazado<Notification[]>();
    const escritura = aplazado<void>();
    const flujo = new FlujoDoble();
    const llegados: string[] = [];
    let cargas = 0;
    let llamada = false;
    let estado: EstadoAvisos | undefined;
    const a = aviso('a', { created_at: '2026-09-24T10:00:00Z' });
    const b = aviso('b', { created_at: '2026-09-24T10:00:01Z' });
    const hub: FuenteAvisos = {
      notifications: () => [inicial, base, reconciliada][cargas++]!.promesa,
      contadores: async () => ({ unread: 0, mailUnread: null }),
      markRead: () => {
        llamada = true;
        return escritura.promesa;
      },
      markAllRead: async () => undefined,
      streamNotifications: async () => flujo,
    };
    const avisos = conectarAvisos(hub, (e) => (estado = e), {
      limite: 30,
      alLlegar: (n) => llegados.push(n.id),
    });
    await hasta(() => flujo.oyentes.has('ready'));
    flujo.emitir('notification', JSON.stringify(a));
    const accion = avisos.leer('a');
    await hasta(() => llamada);
    flujo.emitir('ready');
    inicial.resolver([]);
    escritura.resolver(undefined);
    await accion;
    await hasta(() => cargas === 2);
    base.resolver([{ ...a, read: true }]);
    await hasta(() => cargas === 3);
    reconciliada.resolver([b, { ...a, read: true }]);
    await hasta(() => estado?.items[0]?.id === 'b');
    expect(llegados).toEqual(['a', 'b']);
    expect(estado?.items.map((n) => [n.id, n.read])).toEqual([
      ['b', false],
      ['a', true],
    ]);
    avisos.cerrar();
  });

  it('ordena eventos fuera de orden y rechaza eventos sin fecha válida', async () => {
    const flujo = new FlujoDoble();
    let estado: EstadoAvisos | undefined;
    const hub = hubDoble([], 0).hub;
    hub.streamNotifications = async () => flujo;
    const avisos = conectarAvisos(hub, (e) => (estado = e), { limite: 30 });
    await hasta(() => estado?.cargando === false && flujo.oyentes.has('notification'));
    flujo.emitir(
      'notification',
      JSON.stringify(aviso('nuevo', { created_at: '2026-09-24T10:00:02Z' })),
    );
    flujo.emitir(
      'notification',
      JSON.stringify(aviso('viejo', { created_at: '2026-09-24T10:00:01Z' })),
    );
    flujo.emitir('notification', JSON.stringify({ id: 'invalido', read: false }));
    flujo.emitir(
      'notification',
      JSON.stringify({ id: 'parcial', read: false, created_at: '2026-09-24T10:00:03Z' }),
    );
    expect(estado?.items.map((n) => n.id)).toEqual(['nuevo', 'viejo']);
    expect(estado?.sinLeer).toBe(2);
    avisos.cerrar();
  });

  it('reemite una concesión recuperada ya leída a la página sin avisar al sistema', async () => {
    const g = globalThis as { window?: EventTarget };
    const habia = g.window;
    g.window ??= new EventTarget();
    const oidos: string[] = [];
    const quitar = alLlegarAviso((n) => oidos.push(n.id));
    const flujo = new FlujoDoble();
    const base = aviso('a', { created_at: '2026-09-24T10:00:00Z' });
    const concesion = aviso('b', {
      type: 'docs.access.granted',
      read: true,
      created_at: '2026-09-24T10:00:01Z',
    });
    const notificados: string[] = [];
    let cargas = 0;
    let estado: EstadoAvisos | undefined;
    const hub: FuenteAvisos = {
      notifications: async () => (++cargas === 1 ? [base] : [concesion, base]),
      contadores: async () => ({ unread: 1, mailUnread: null }),
      markRead: async () => undefined,
      markAllRead: async () => undefined,
      streamNotifications: async () => flujo,
    };
    const avisos = conectarAvisos(hub, (e) => (estado = e), {
      limite: 30,
      alLlegar: (n) => notificados.push(n.id),
    });
    try {
      await hasta(() => estado?.items[0]?.id === 'a');
      avisos.reintentar();
      await hasta(() => estado?.items[0]?.id === 'b');
      expect(oidos).toEqual(['b']);
      expect(notificados).toEqual([]);
    } finally {
      avisos.cerrar();
      quitar();
      if (!habia) delete g.window;
    }
  });

  it('al cerrar ignora el fallo tardío y no inicia gestos pendientes de la cuenta anterior', async () => {
    const escritura = aplazado<void>();
    const flujo = new FlujoDoble();
    let llamadasUno = 0;
    let llamadasTodos = 0;
    let cambios = 0;
    let estado: EstadoAvisos | undefined;
    const hub = hubDoble([aviso('a')], 1).hub;
    hub.streamNotifications = async () => flujo;
    hub.markRead = () => {
      llamadasUno++;
      return escritura.promesa;
    };
    hub.markAllRead = async () => {
      llamadasTodos++;
    };
    const avisos = conectarAvisos(
      hub,
      (e) => {
        estado = e;
        cambios++;
      },
      { limite: 30 },
    );
    await hasta(() => estado?.items.length === 1);
    const primera = avisos.leer('a');
    const segunda = avisos.leerTodos();
    await hasta(() => llamadasUno === 1);
    avisos.cerrar();
    const alCerrar = cambios;
    escritura.rechazar(new Error('sesión retirada'));
    await primera;
    await segunda;
    flujo.emitir('notification', JSON.stringify(aviso('tardio')));
    expect(cambios).toBe(alCerrar);
    expect(llamadasTodos).toBe(0);
    expect(flujo.cerrado).toBe(true);
  });

  it('no emite más cambios si el callback de llegada cierra la sesión de forma reentrante', async () => {
    const flujo = new FlujoDoble();
    let cambios = 0;
    let cambiosAlCerrar = -1;
    let estado: EstadoAvisos | undefined;
    const hub = hubDoble([], 0).hub;
    hub.streamNotifications = async () => flujo;
    const avisos = conectarAvisos(
      hub,
      (e) => {
        estado = e;
        cambios++;
      },
      {
        limite: 30,
        alLlegar: () => {
          avisos.cerrar();
          cambiosAlCerrar = cambios;
        },
      },
    );
    await hasta(() => estado?.cargando === false && flujo.oyentes.has('notification'));
    flujo.emitir('notification', JSON.stringify(aviso('nuevo')));
    expect(cambiosAlCerrar).toBeGreaterThanOrEqual(0);
    expect(cambios).toBe(cambiosAlCerrar);
    expect(flujo.cerrado).toBe(true);
  });

  it('un error del callback externo no impide actualizar página ni campana', async () => {
    const g = globalThis as { window?: EventTarget };
    const habia = g.window;
    g.window ??= new EventTarget();
    const oidos: string[] = [];
    const quitar = alLlegarAviso((n) => oidos.push(n.id));
    const d = hubDoble([], 0);
    let estado: EstadoAvisos | undefined;
    const avisos = conectarAvisos(d.hub, (e) => (estado = e), {
      limite: 30,
      alLlegar: () => {
        throw new Error('fallo del sistema');
      },
    });
    try {
      await hasta(() => d.flujos.length === 1 && !estado?.cargando);
      d.flujos[0]!.emitir('notification', JSON.stringify(aviso('nuevo')));
      expect(oidos).toEqual(['nuevo']);
      expect(estado?.items.map((n) => n.id)).toEqual(['nuevo']);
      expect(estado?.sinLeer).toBe(1);
    } finally {
      avisos.cerrar();
      quitar();
      if (!habia) delete g.window;
    }
  });
});

describe('conectarAvisos', () => {
  it('un aviso nuevo llega, cuenta, y deja de contar al leerlo', async () => {
    const d = hubDoble([aviso('a', { read: true })], 0);
    let e: EstadoAvisos | undefined;
    const llegados: string[] = [];
    const avisos = conectarAvisos(d.hub, (x) => (e = x), {
      limite: 30,
      alLlegar: (n) => llegados.push(n.id),
    });
    await tick();
    expect(e?.items.map((n) => n.id)).toEqual(['a']);
    expect(e?.sinLeer).toBe(0);

    d.flujos[0]?.emitir('notification', JSON.stringify(aviso('b')));
    expect(e?.items.map((n) => n.id)).toEqual(['b', 'a']);
    expect(e?.sinLeer).toBe(1);
    // El mismo otra vez (el flujo puede repetir): ni se duplica ni vuelve a contar.
    d.flujos[0]?.emitir('notification', JSON.stringify(aviso('b')));
    expect(e?.items).toHaveLength(2);
    expect(e?.sinLeer).toBe(1);
    expect(llegados).toEqual(['b']);

    await avisos.leer('b');
    expect(d.leidos).toEqual(['b']);
    expect(e?.sinLeer).toBe(0);
    expect(e?.items[0]?.read).toBe(true);
    avisos.cerrar();
  });

  it('la señal sin datos de la entrada privada recarga la campana', async () => {
    const d = hubDoble([aviso('a', { read: true })], 0);
    let e: EstadoAvisos | undefined;
    const llegados: string[] = [];
    const avisos = conectarAvisos(d.hub, (x) => (e = x), {
      limite: 30,
      alLlegar: (n) => llegados.push(n.id),
    });
    await tick();
    const cargas = d.cargas();
    d.agregar(aviso('p', { private: true, type: 'security.storage.released' }));
    d.flujos[0]?.emitir('private_ingress', '{}');
    await hasta(() => e?.items.length === 2);
    expect(d.cargas()).toBe(cargas + 1);
    expect(e?.items.map((n) => n.id)).toEqual(['p', 'a']);
    expect(e?.items[0]?.private).toBe(true);
    await hasta(() => e?.sinLeer === 1);
    expect(llegados).toEqual(['p']);
    avisos.cerrar();
  });

  it('«cambió la foto» llega por el flujo sin entrar en la campana', async () => {
    const d = hubDoble([], 0);
    let e: EstadoAvisos | undefined;
    const fotos: string[] = [];
    const avisos = conectarAvisos(d.hub, (x) => (e = x), {
      limite: 30,
      alCambiarFoto: (v) => fotos.push(v),
    });
    await tick();
    d.flujos[0]?.emitir('avatar', JSON.stringify({ version: 'lq3x9' }));
    d.flujos[0]?.emitir('avatar', JSON.stringify({ version: '' }));
    d.flujos[0]?.emitir('avatar', 'no es JSON');
    expect(fotos).toEqual(['lq3x9', '']);
    expect(e?.items).toEqual([]);
    expect(e?.sinLeer).toBe(0);
    avisos.cerrar();
  });

  it('trae el correo sin leer de Mail y suma el que llega', async () => {
    const d = hubDoble([], 0, 4);
    let e: EstadoAvisos | undefined;
    const avisos = conectarAvisos(d.hub, (x) => (e = x), { limite: 30 });
    await tick();
    expect(e?.correoSinLeer).toBe(4);
    d.flujos[0]?.emitir(
      'notification',
      JSON.stringify(aviso('m', { source: 'mail', type: 'mail.message.received' })),
    );
    expect(e?.correoSinLeer).toBe(5);
    d.flujos[0]?.emitir('notification', JSON.stringify(aviso('c')));
    expect(e?.correoSinLeer).toBe(5);
    avisos.cerrar();
  });

  it('«marcar todo» deja el contador a cero', async () => {
    const d = hubDoble([aviso('a'), aviso('b')], 7);
    let e: EstadoAvisos | undefined;
    const avisos = conectarAvisos(d.hub, (x) => (e = x), { limite: 30 });
    await tick();
    expect(e?.sinLeer).toBe(7);
    await avisos.leerTodos();
    expect(d.todos()).toBe(1);
    expect(e?.sinLeer).toBe(0);
    expect(e?.items.every((n) => n.read)).toBe(true);
    avisos.cerrar();
  });

  it('tras un corte, reconecta y recarga lo que llegó con el flujo caído', async () => {
    const d = hubDoble([], 0);
    const esperas: number[] = [];
    let pendiente: (() => void) | undefined;
    const avisos = conectarAvisos(d.hub, () => undefined, {
      limite: 30,
      programar: (fn, ms) => {
        esperas.push(ms);
        pendiente = fn;
        return () => undefined;
      },
    });
    await tick();
    expect(d.cargas()).toBe(1);
    d.flujos[0]?.emitir('error');
    expect(d.flujos[0]?.cerrado).toBe(true);
    pendiente?.();
    await tick();
    expect(d.flujos).toHaveLength(2);
    expect(d.cargas()).toBe(2);
    // Sin `ready` de por medio, la espera crece; con él, vuelve a empezar.
    d.flujos[1]?.emitir('error');
    d.flujos[1]?.emitir('ready');
    expect(esperas).toEqual([1000, 2000]);
    avisos.cerrar();
  });
});

describe('tituloAviso', () => {
  it('compone el título en el idioma de quien lo mira', () => {
    expect(tituloAviso('es', aviso('a'))).toBe('Ana ha comentado en «Presupuesto»');
    expect(tituloAviso('en', aviso('a'))).toBe('Ana commented on “Presupuesto”');
  });
  it('cae al título del emisor con un tipo desconocido o sin variables', () => {
    expect(tituloAviso('es', aviso('a', { type: 'docs.nuevo' }))).toBe('Comentario en Presupuesto');
    expect(tituloAviso('es', aviso('a', { params: { actor: 'Ana' } }))).toBe(
      'Comentario en Presupuesto',
    );
  });
});

describe('masAntiguos', () => {
  it('pide lo de detrás del último por (fecha, id), lo añade sin repetir y sabe cuándo acaba', async () => {
    const pedidas: unknown[] = [];
    const paginas = [[aviso('a'), aviso('b')], [aviso('b'), aviso('c')], [aviso('d')]];
    const hub: FuenteAvisos = {
      notifications: async (o) => {
        pedidas.push(o?.antes ?? null);
        return paginas.shift() ?? [];
      },
      contadores: async () => ({ unread: 0, mailUnread: null }),
      markRead: async () => undefined,
      markAllRead: async () => undefined,
      streamNotifications: async () => new FlujoDoble(),
    };
    let e: EstadoAvisos | undefined;
    const avisos = conectarAvisos(hub, (x) => (e = x), { limite: 2 });
    await tick();
    expect(e?.hayMas).toBe(true);
    await avisos.masAntiguos();
    expect(pedidas[1]).toEqual(aviso('b'));
    expect(e?.items.map((n) => n.id)).toEqual(['a', 'b', 'c']);
    expect(e?.hayMas).toBe(true);
    await avisos.masAntiguos();
    expect(e?.items.map((n) => n.id)).toEqual(['a', 'b', 'c', 'd']);
    expect(e?.hayMas).toBe(false);
    await avisos.masAntiguos(); // ya no hay más: no pregunta
    expect(pedidas).toHaveLength(3);
    avisos.cerrar();
  });
});

describe('masAntiguos con la lista viva', () => {
  const fuente = (paginas: Notification[][], pedidas: unknown[] = []): FuenteAvisos => ({
    notifications: async (o) => {
      pedidas.push(o?.antes ?? null);
      const p = paginas.shift();
      if (!p) throw new Error('sin red');
      return p;
    },
    contadores: async () => ({ unread: 0, mailUnread: null }),
    markRead: async () => undefined,
    markAllRead: async () => undefined,
    streamNotifications: async () => new FlujoDoble(),
  });
  const en = (id: string, t: string) => aviso(id, { created_at: t });

  it('recargar (al reconectar el flujo) conserva lo ya traído de detrás', async () => {
    const a = en('a', '2026-09-24T10:00:03Z');
    const b = en('b', '2026-09-24T10:00:02.000002Z');
    const c = en('c', '2026-09-24T10:00:02.000001Z'); // mismo milisegundo que b
    const d = en('d', '2026-09-24T10:00:01Z');
    const nuevo = en('n', '2026-09-24T10:00:04Z');
    // La recarga acaba en b: c es del mismo milisegundo pero anterior (microsegundos).
    const paginas = [
      [a, b],
      [c, d],
      [nuevo, b],
    ];
    let e: EstadoAvisos | undefined;
    const avisos = conectarAvisos(fuente(paginas), (x) => (e = x), { limite: 2 });
    await tick();
    await avisos.masAntiguos();
    expect(e?.items.map((n) => n.id)).toEqual(['a', 'b', 'c', 'd']);
    // Recarga: la primera página ya es otra, y lo de detrás no se pierde.
    avisos.reintentar();
    await tick();
    // `a` ya no sale en la primera página (se borró): no se resucita.
    expect(e?.items.map((n) => n.id)).toEqual(['n', 'b', 'c', 'd']);
    avisos.cerrar();
  });

  it('sin «más antiguos», recargar no hace crecer la lista (la campana)', async () => {
    const a = en('a', '2026-09-24T10:00:03Z');
    const b = en('b', '2026-09-24T10:00:02Z');
    const nuevo = en('n', '2026-09-24T10:00:04Z');
    let e: EstadoAvisos | undefined;
    const avisos = conectarAvisos(
      fuente([
        [a, b],
        [nuevo, a],
      ]),
      (x) => (e = x),
      { limite: 2 },
    );
    await tick();
    avisos.reintentar();
    await tick();
    expect(e?.items.map((n) => n.id)).toEqual(['n', 'a']);
    avisos.cerrar();
  });

  it('si la recarga no enlaza con lo que había, no se pega lo viejo con un hueco', async () => {
    const a = en('a', '2026-09-24T10:00:03Z');
    const b = en('b', '2026-09-24T10:00:02Z');
    const c = en('c', '2026-09-24T10:00:01Z');
    const x = en('x', '2026-09-24T10:00:09Z');
    const y = en('y', '2026-09-24T10:00:08Z');
    let e: EstadoAvisos | undefined;
    const avisos = conectarAvisos(fuente([[a, b], [c], [x, y]]), (v) => (e = v), { limite: 2 });
    await tick();
    await avisos.masAntiguos();
    avisos.reintentar();
    await tick();
    expect(e?.items.map((n) => n.id)).toEqual(['x', 'y']);
    expect(e?.hayMas).toBe(true);
    avisos.cerrar();
  });

  it('una página pedida antes de recargar se tira, y un fallo de red no rompe nada', async () => {
    let soltar: (v: Notification[]) => void = () => undefined;
    const a = en('a', '2026-09-24T10:00:03Z');
    const b = en('b', '2026-09-24T10:00:02Z');
    let llamada = 0;
    const hub: FuenteAvisos = {
      ...fuente([]),
      notifications: async () => {
        llamada++;
        if (llamada === 1) return [a, b];
        if (llamada === 2) return new Promise<Notification[]>((r) => (soltar = r));
        if (llamada === 3)
          return [en('n', '2026-09-24T10:00:09Z'), en('m', '2026-09-24T10:00:08Z')];
        throw new Error('sin red');
      },
    };
    let e: EstadoAvisos | undefined;
    const avisos = conectarAvisos(hub, (x) => (e = x), { limite: 2 });
    await tick();
    const vieja = avisos.masAntiguos();
    avisos.reintentar(); // recarga mientras la página vuelve
    await tick();
    soltar([en('z', '2026-09-24T10:00:01Z')]);
    await vieja;
    // La recarga trae [n, m], que no enlaza con [a, b] (con el flujo caído llegó más de
    // una página): lo de antes no se pega detrás, que dejaría un hueco. Y la página vieja
    // (z), pedida para la lista de antes, se tira.
    expect(e?.items.map((n) => n.id)).toEqual(['n', 'm']);
    expect(e?.hayMas).toBe(true);
    await avisos.masAntiguos(); // sin red: ni revienta ni cambia nada
    expect(e?.items.map((n) => n.id)).toEqual(['n', 'm']);
    avisos.cerrar();
  });
});

describe('procedenciaAviso', () => {
  it('marca lo reconstruido como anterior al registro de avisos, y lo de otra organización', () => {
    expect(procedenciaAviso('es', aviso('a'))).toEqual([]);
    expect(procedenciaAviso('es', aviso('a', { historic: true }))).toEqual([
      'Anterior al registro de avisos',
    ]);
    expect(procedenciaAviso('en', aviso('a', { organization: 'globex', historic: true }))).toEqual([
      'globex',
      'Before the notification log',
    ]);
    expect(
      procedenciaAviso(
        'es',
        aviso('a', { organization: 'globex', organization_name: 'Globex Ingeniería S.L.' }),
      ),
    ).toEqual(['Globex Ingeniería S.L.']);
  });
});

describe('nombreDeOrganizacion', () => {
  it('enseña el nombre visible y, sin él, el identificador', () => {
    expect(
      nombreDeOrganizacion({ organization: 'acme', organization_name: 'Acme Ingeniería S.L.' }),
    ).toBe('Acme Ingeniería S.L.');
    expect(nombreDeOrganizacion({ organization: 'acme' })).toBe('acme');
    expect(nombreDeOrganizacion({})).toBe('');
  });
});

describe('abrirAviso', () => {
  it('lo da por leído y abre su elemento', async () => {
    const leidos: string[] = [];
    const idas: string[] = [];
    await abrirAviso({ leer: async (id) => void leidos.push(id) }, aviso('a'), (u) => idas.push(u));
    expect(leidos).toEqual(['a']);
    expect(idas).toEqual(['https://docus.sytue.eu/docus/edit?id=d1']);
  });
});

describe('cuerpoAviso', () => {
  it('sin cuerpo, la hora de lo que avisa en el idioma de quien lo mira', () => {
    const n = aviso('a', { body: '', params: { title: 'x', start: '2026-10-01T08:00:00Z' } });
    expect(cuerpoAviso('es', n)).toMatch(/2026|oct/);
    expect(cuerpoAviso('es', aviso('a', { body: 'hola' }))).toBe('hola');
  });
});

describe('destinoAviso', () => {
  it('lleva al elemento y nunca a un esquema ajeno', () => {
    expect(destinoAviso(aviso('a'))).toBe('https://docus.sytue.eu/docus/edit?id=d1');
    expect(destinoAviso(aviso('a', { url: 'javascript:alert(1)' }))).toBeNull();
    expect(destinoAviso(aviso('a', { url: '' }))).toBeNull();
  });
});

describe('fotoConVersion', () => {
  it('cambia la versión de la URL para que el navegador no pinte la de su caché', () => {
    const r = '/drive/auth/avatar';
    expect(fotoConVersion('/drive/auth/avatar?v=vieja', 'nueva', r)).toBe(
      '/drive/auth/avatar?v=nueva',
    );
    // Quien no tenía foto al cargar la página no trae URL: sirve la de su app.
    expect(fotoConVersion(undefined, 'nueva', r)).toBe('/drive/auth/avatar?v=nueva');
    // La quitó: las iniciales.
    expect(fotoConVersion('/drive/auth/avatar?v=vieja', '', r)).toBeUndefined();
  });
});

describe('el flujo del Hub, para la página (desbloqueo en vivo de la franja)', () => {
  it('cada aviso NUEVO se reemite como sytue:aviso y le llega a quien escucha', async () => {
    const g = globalThis as { window?: EventTarget };
    const habia = g.window;
    g.window ??= new EventTarget();
    const oidos: string[] = [];
    const quitar = alLlegarAviso((n) => oidos.push(n.id));
    const d = hubDoble([], 0);
    const avisos = conectarAvisos(d.hub, () => undefined, { limite: 30 });
    await tick();
    d.flujos[0]?.emitir('notification', JSON.stringify(aviso('c')));
    d.flujos[0]?.emitir('notification', JSON.stringify(aviso('c'))); // repetido: no
    quitar();
    d.flujos[0]?.emitir('notification', JSON.stringify(aviso('d'))); // ya sin oyente
    expect(oidos).toEqual(['c']);
    avisos.cerrar();
    g.window = habia;
  });

  it('la concesión se reconoce por su tipo y su recurso, no por cualquier aviso', () => {
    const concedido = aviso('e', {
      type: 'calendar.access.granted',
      params: { resource: 'cal-1' },
    });
    expect(esConcesion(concedido, 'calendar.access.granted', 'cal-1')).toBe(true);
    expect(esConcesion(concedido, 'calendar.access.granted', 'cal-2')).toBe(false);
    expect(esConcesion(concedido, 'drive.access.granted', 'cal-1')).toBe(false);
  });
});

/**
 * Los orígenes que el Hub acepta para un aviso, leídos de su `domain.go`: los de
 * `AllSources` más los que `NotificationSources` añade. Se lee el Go y no una copia
 * para que un origen nuevo allí —como lo fue Cast, que avisaba sin poder callarse—
 * ponga esto en rojo el mismo día.
 */
function origenesDelHub(): Set<string> {
  const raiz = fileURLToPath(new URL('../../../../../', import.meta.url));
  const go = readFileSync(`${raiz}services/hub/internal/domain/domain.go`, 'utf8');
  const valor = new Map<string, string>();
  for (const [, nombre, v] of go.matchAll(/\b(Source\w+)\s+Source\s*=\s*"([a-z]+)"/g)) {
    valor.set(nombre ?? '', v ?? '');
  }
  const cuerpo = (fn: string): string =>
    new RegExp(`func ${fn}\\(\\) \\[\\]Source \\{([^}]*)\\}`).exec(go)?.[1] ?? '';
  const nombres = [
    ...cuerpo('AllSources').matchAll(/\bSource[A-Z]\w*/g),
    ...cuerpo('NotificationSources').matchAll(/\bSource[A-Z]\w*/g),
  ].map(([n]) => n);
  return new Set(nombres.map((n) => valor.get(n) ?? `¿${n}?`));
}

describe('ORIGENES_DE_AVISOS', () => {
  const ofrecidos = ORIGENES_DE_AVISOS.flatMap((g) => g.origenes);

  it('ofrece todos los orígenes que pueden avisar, salvo seguridad y el alias de búsqueda', () => {
    const delHub = origenesDelHub();
    // Si la lectura del Go fallara, la comparación pasaría con dos conjuntos vacíos.
    expect(delHub.size).toBeGreaterThan(15);
    for (const fuera of ['id', 'chat']) delHub.delete(fuera);
    expect([...ofrecidos].sort()).toEqual([...delHub].sort());
  });

  it('nunca ofrece callar los avisos de seguridad de Sytue ID', () => {
    expect(ofrecidos).not.toContain('id');
  });

  it('cada origen sale una sola vez y ningún grupo queda vacío', () => {
    expect(new Set(ofrecidos).size).toBe(ofrecidos.length);
    for (const g of ORIGENES_DE_AVISOS) expect(g.origenes.length).toBeGreaterThan(0);
  });
});

/** Los `notifications.kind.*` del catálogo común (los títulos de los avisos del Hub). */
function tiposDelCatalogo(): string[] {
  return Object.keys(catalogs.en)
    .filter((k) => k.startsWith('notifications.kind.'))
    .map((k) => k.slice('notifications.kind.'.length));
}

/** `tiposCriticos` del `domain.go` del Hub, leído del Go y no de una copia. */
function criticosDelHub(): string[] {
  const raiz = fileURLToPath(new URL('../../../../../', import.meta.url));
  const go = readFileSync(`${raiz}services/hub/internal/domain/domain.go`, 'utf8');
  const cuerpo = /var tiposCriticos = \[\]string\{([^}]*)\}/.exec(go)?.[1] ?? '';
  return [...cuerpo.matchAll(/"([a-z._]+)"/g)].map(([, t]) => t ?? '');
}

describe('TIPOS_DE_AVISOS', () => {
  const filas = Object.values(TIPOS_DE_AVISOS).flatMap((f) => f ?? []);
  const tipos = new Set(filas.flatMap((f) => f.tipos));

  it('cubre todos los tipos de aviso del catálogo salvo los de seguridad, que van fijos', () => {
    const delCatalogo = tiposDelCatalogo();
    expect(delCatalogo.length).toBeGreaterThan(40);
    const faltan = delCatalogo.filter((t) => !t.startsWith('security.') && !tipos.has(t));
    expect(faltan).toEqual([]);
    for (const t of tipos) if (t.startsWith('security.')) throw new Error(`${t} no se calla`);
  });

  it('cada tipo tiene su título en el catálogo', () => {
    const delCatalogo = new Set(tiposDelCatalogo());
    const sinTitulo = [...tipos].filter((t) => !delCatalogo.has(t));
    expect(sinTitulo).toEqual([]);
  });

  it('solo da tipos a orígenes que se ofrecen, y sin repetir fila en un origen', () => {
    const ofrecidos = new Set<string>(ORIGENES_DE_AVISOS.flatMap((g) => g.origenes));
    for (const [origen, f] of Object.entries(TIPOS_DE_AVISOS)) {
      expect(ofrecidos.has(origen)).toBe(true);
      const ids = (f ?? []).map((x) => x.id);
      expect(new Set(ids).size).toBe(ids.length);
    }
  });

  it('los críticos son los mismos que el Hub no deja callar', () => {
    const delHub = criticosDelHub();
    expect(delHub.length).toBeGreaterThan(0);
    expect([...TIPOS_CRITICOS].sort()).toEqual([...delHub].sort());
    // Una fila crítica solo reúne críticos: si no, callaría lo que no se puede.
    for (const f of filas) {
      if (!f.critico) for (const t of f.tipos) expect(TIPOS_CRITICOS.has(t)).toBe(false);
    }
  });
});
