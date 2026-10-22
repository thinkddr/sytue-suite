/**
 * La campana de punta a punta, con un Hub de mentira: lo que llega por el flujo se
 * ve, cuenta, deja de contar al leerlo y, tras un corte, vuelve con la lista al día.
 * Es la mitad web del guardián de «Todo al día con cosas pasando» (2026-09-24).
 */
import { describe, expect, it } from 'bun:test';
import type { Notification } from '@sytue/api-clients';
import {
  abrirAviso,
  alLlegarAviso,
  conectarAvisos,
  esConcesion,
  cuerpoAviso,
  destinoAviso,
  fotoConVersion,
  nombreDeOrganizacion,
  procedenciaAviso,
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
  const hub: FuenteAvisos = {
    notifications: async () => {
      cargas++;
      return [...lista];
    },
    contadores: async () => ({ unread: sinLeer, mailUnread: correo }),
    markRead: async (id: string) => {
      leidos.push(id);
    },
    markAllRead: async () => {
      todos++;
    },
    streamNotifications: async () => {
      const f = new FlujoDoble();
      flujos.push(f);
      return f;
    },
  };
  return { hub, flujos, leidos, todos: () => todos, cargas: () => cargas };
}

const tick = () => new Promise((r) => setTimeout(r, 0));

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
