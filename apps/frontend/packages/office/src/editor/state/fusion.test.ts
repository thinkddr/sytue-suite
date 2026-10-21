// Checks de la co-edición de hojas y presentaciones (./fusion.ts). Ejecuta: `bun test`.
//
// Lo que se vigila es la PÉRDIDA: dos personas que tocan celdas o diapositivas
// distintas a la vez tienen que acabar las dos con lo de las dos, lleguen las
// escrituras en el orden que lleguen. Con el registro LWW de antes, el último
// guardado borraba lo del otro; el test aleatorio de abajo lo detecta en cuanto
// `aEnviar` deja de fusionar o el servidor deja de rechazar la base obsoleta.
import { expect, test } from 'bun:test';
import {
  CoedicionModelo,
  diferencias,
  fusionar3,
  igual,
  type Cambio,
  type Registro,
} from './fusion.js';
import { puntoMenor, type Punto } from './rga.js';

/* ---------- fusionar3: rutas estables ---------- */

const libro = (cells: Record<string, string>, extra: object[] = []): object => ({
  active: 0,
  sheets: [{ name: 'Hoja1', cells }, ...extra],
});

test('celdas distintas de la misma hoja: quedan las dos', () => {
  const base = libro({ A1: '1' });
  const mio = libro({ A1: '1', B2: 'mío' });
  const suyo = libro({ A1: 'suyo' });
  expect(fusionar3(base, mio, suyo)).toEqual(libro({ A1: 'suyo', B2: 'mío' }));
});

test('la misma celda: gana lo propio, que llega después al servidor', () => {
  const r = fusionar3(libro({ A1: '1' }), libro({ A1: 'mío' }), libro({ A1: 'suyo' }));
  expect(r).toEqual(libro({ A1: 'mío' }));
});

test('una hoja nueva del otro y una celda mía en la primera', () => {
  const nueva = { name: 'Hoja2', cells: { C3: 'x' } };
  const r = fusionar3(libro({}), libro({ A1: 'mío' }), libro({}, [nueva]));
  expect(r).toEqual(libro({ A1: 'mío' }, [nueva]));
});

test('el primer estilo de la hoja, creado a la vez en los dos lados', () => {
  const hoja = (styles: object): object => ({
    active: 0,
    sheets: [{ name: 'H', cells: {}, styles }],
  });
  const base = { active: 0, sheets: [{ name: 'H', cells: {} }] };
  const r = fusionar3(base, hoja({ A1: { fill: '#ff0000' } }), hoja({ B1: { u: true } }));
  expect(r).toEqual(hoja({ A1: { fill: '#ff0000' }, B1: { u: true } }));
});

test('renombrar una hoja con id mientras otra persona escribe en ella: sigue siendo una', () => {
  const l = (name: string, cells: object): object => ({
    active: 0,
    sheets: [{ id: 'h1', name, cells }],
  });
  const r = fusionar3(l('Ventas', {}), l('Ventas 2026', {}), l('Ventas', { A1: 'x' }));
  expect(r).toEqual(l('Ventas 2026', { A1: 'x' }));
});

const diapo = (id: string, ...cajas: [string, string][]): object => ({
  id,
  boxes: cajas.map(([bid, html]) => ({ id: bid, x: 0, y: 0, w: 10, h: 10, html })),
});

test('diapositivas: cajas distintas, diapositivas añadidas por los dos y orden', () => {
  const base = { slides: [diapo('s1', ['b1', 'a']), diapo('s2', ['b2', 'b'])] };
  // Yo edito una caja de s1 y añado s3 al final; el otro edita s2 y la sube arriba.
  const mio = {
    slides: [diapo('s1', ['b1', 'A']), diapo('s2', ['b2', 'b']), diapo('s3', ['b3', 'c'])],
  };
  const suyo = { slides: [diapo('s2', ['b2', 'B']), diapo('s1', ['b1', 'a'])] };
  expect(fusionar3(base, mio, suyo)).toEqual({
    slides: [diapo('s2', ['b2', 'B']), diapo('s1', ['b1', 'A']), diapo('s3', ['b3', 'c'])],
  });
});

test('borrar frente a editar: gana lo propio, también si lo propio es borrar', () => {
  const base = { slides: [diapo('s1'), diapo('s2', ['b', 'x'])] };
  const editada = { slides: [diapo('s1'), diapo('s2', ['b', 'y'])] };
  const borrada = { slides: [diapo('s1')] };
  expect(fusionar3(base, borrada, editada)).toEqual(borrada);
  expect(fusionar3(base, editada, borrada)).toEqual(editada);
});

test('una lista sin identidad es atómica', () => {
  const r = fusionar3({ v: [{ r: 'A1' }] }, { v: [{ r: 'A2' }] }, { v: [{ r: 'A3' }] });
  expect(r).toEqual({ v: [{ r: 'A2' }] });
});

test('igual no depende del orden de las claves', () => {
  expect(igual({ a: 1, b: [1, { c: 2, d: 3 }] }, { b: [1, { d: 3, c: 2 }], a: 1 })).toBe(true);
  expect(igual({ a: 1 }, { a: 1, b: undefined })).toBe(false);
});

/* ---------- Convergencia: servidor simulado + clientes como useDocument ---------- */

/** POST /ops con base_seq y el registro LWW, como store/operations.go + crdt.go. */
class Servidor {
  log: { seq: number; actor: string; valor: string; lamport: number; dot: Punto }[] = [];
  reg: Registro = { valor: '', lamport: 0, dot: null };
  lamport = 0;

  get seq(): number {
    return this.log.length;
  }

  /** La seq asignada, o null si es un 409 (alguien escribió después de `base`). */
  escribir(actor: string, valor: string, lamport: number, dot: Punto, base: number): number | null {
    if (this.log.some((e) => e.seq > base)) return null;
    const seq = this.log.length + 1;
    this.log.push({ seq, actor, valor, lamport, dot });
    this.lamport = Math.max(this.lamport, lamport);
    const cur = this.reg;
    if (
      lamport > cur.lamport ||
      (lamport === cur.lamport && (cur.dot === null || puntoMenor(cur.dot, dot)))
    )
      this.reg = { valor, lamport, dot };
    return seq;
  }
}

interface Lote {
  local: string;
  enviado: string;
  lamport: number;
  dot: Punto;
  seq: number | null;
}

/** Lo que hace useDocument con una hoja: editor, autosave, 409 y adopción. */
class Cliente {
  editor: string;
  pendiente: string | null = null;
  enVuelo: Lote | null = null;
  lamport: number;
  contador = 0;
  vistas: number;
  modelo: CoedicionModelo;

  constructor(
    readonly actor: string,
    srv: Servidor,
  ) {
    this.editor = srv.reg.valor;
    this.lamport = srv.lamport;
    this.vistas = srv.seq;
    this.modelo = new CoedicionModelo({ ...srv.reg }, srv.seq, srv.reg.valor);
  }

  editar(fn: (m: { sheets: { name: string; cells: Record<string, string> }[] }) => void): void {
    const m = this.editor
      ? JSON.parse(this.editor)
      : { active: 0, sheets: [{ name: 'Hoja1', cells: {} }] };
    fn(m);
    this.editor = JSON.stringify(m);
    this.pendiente = this.editor;
  }

  /** doSave hasta el fetch: el servidor lo secuencia al llegar; la respuesta va aparte. */
  enviar(srv: Servidor): void {
    if (this.enVuelo !== null || this.pendiente === null) return;
    const local = this.pendiente;
    const enviado = this.modelo.aEnviar(local);
    this.lamport += 1;
    this.contador += 1;
    const dot = { a: this.actor, c: this.contador };
    const seq = srv.escribir(this.actor, enviado, this.lamport, dot, this.modelo.baseSeq);
    this.enVuelo = { local, enviado, lamport: this.lamport, dot, seq };
  }

  /** La respuesta del POST. Un 409 relee el estado (GET /api/state) para el reintento. */
  responder(srv: Servidor): void {
    const l = this.enVuelo;
    if (l === null) return;
    this.enVuelo = null;
    if (l.seq === null) {
      if (srv.reg.dot !== null)
        this.modelo.recibir(srv.reg.valor, srv.seq, srv.reg.lamport, srv.reg.dot);
      this.lamport = Math.max(this.lamport, srv.lamport);
      return;
    }
    this.modelo.confirmar(l.local, l.enviado, l.seq, l.lamport, l.dot);
    if (this.pendiente === l.local) {
      this.pendiente = null;
      this.adoptar();
    }
  }

  /** Push del WS hasta `hasta`; el eco propio se descarta, como en collab.ts. */
  entregar(srv: Servidor, hasta = srv.seq): void {
    for (const e of srv.log.slice(this.vistas, hasta)) {
      this.lamport = Math.max(this.lamport, e.lamport);
      if (e.actor !== this.actor) this.modelo.recibir(e.valor, e.seq, e.lamport, e.dot);
    }
    this.vistas = Math.max(this.vistas, hasta);
    if (this.pendiente === null && this.enVuelo === null) this.adoptar();
  }

  private adoptar(): void {
    const v = this.modelo.adoptar();
    if (v !== null) this.editor = v;
  }
}

/** Todo el mundo deja de escribir: se responde, se reenvía y se entrega hasta que no queda nada. */
function calmar(srv: Servidor, clientes: Cliente[]): void {
  for (let vuelta = 0; vuelta < 50; vuelta++) {
    for (const c of clientes) {
      c.responder(srv);
      c.enviar(srv);
      c.responder(srv);
      c.entregar(srv);
    }
    if (clientes.every((c) => c.pendiente === null && c.enVuelo === null && c.vistas === srv.seq))
      return;
  }
  throw new Error('no se calma');
}

function celdas(valor: string): Record<string, string> {
  return (JSON.parse(valor) as { sheets: { cells: Record<string, string> }[] }).sheets[0]!.cells;
}

test('dos personas, celdas distintas, a la vez: el orden de llegada no cambia el resultado', () => {
  const finales: Record<string, string>[] = [];
  for (const primero of [0, 1]) {
    const srv = new Servidor();
    const [a, b] = [new Cliente('a', srv), new Cliente('b', srv)];
    a.editar((m) => (m.sheets[0]!.cells.A1 = 'de a'));
    b.editar((m) => (m.sheets[0]!.cells.B2 = 'de b'));
    // Los dos guardan antes de saber nada del otro.
    const orden = primero === 0 ? [a, b] : [b, a];
    for (const c of orden) c.enviar(srv);
    for (const c of orden) c.responder(srv);
    calmar(srv, [a, b]);
    expect(celdas(srv.reg.valor)).toEqual({ A1: 'de a', B2: 'de b' });
    expect(igual(JSON.parse(a.editor), JSON.parse(srv.reg.valor))).toBe(true);
    expect(igual(JSON.parse(b.editor), JSON.parse(srv.reg.valor))).toBe(true);
    finales.push(celdas(srv.reg.valor));
  }
  expect(finales[0]).toEqual(finales[1]!); // mismo contenido; el orden de las claves da igual
});

/** Generador determinista (mismas decisiones en cada ejecución). */
function azar(semilla: number): (n: number) => number {
  let s = semilla;
  return (n) => {
    s = (s * 1103515245 + 12345) & 0x7fffffff;
    return s % n;
  };
}

test('tres personas, ediciones y entregas entrelazadas al azar: nadie pierde lo suyo', () => {
  for (let semilla = 1; semilla <= 200; semilla++) {
    const r = azar(semilla);
    const srv = new Servidor();
    const clientes = ['a', 'b', 'c'].map((id) => new Cliente(id, srv));
    /** Último valor que cada cliente escribió en SUS celdas (nadie más las toca). */
    const esperado: Record<string, string> = {};
    for (let paso = 0; paso < 60; paso++) {
      const c = clientes[r(3)]!;
      switch (r(5)) {
        case 0:
        case 1: {
          const celda = `${c.actor.toUpperCase()}${r(4) + 1}`;
          const v = `${c.actor}${paso}`;
          esperado[celda] = v;
          c.editar((m) => {
            m.sheets[0]!.cells[celda] = v;
            m.sheets[0]!.cells.Z1 = v; // y una celda compartida, que solo tiene que converger
          });
          break;
        }
        case 2:
          c.enviar(srv);
          break;
        case 3:
          c.responder(srv);
          break;
        default:
          c.entregar(srv, c.vistas + r(srv.seq - c.vistas + 1));
      }
    }
    calmar(srv, clientes);
    if (srv.reg.valor === '') {
      expect(esperado).toEqual({}); // nadie llegó a escribir
      continue;
    }
    const final = celdas(srv.reg.valor);
    const propias = Object.fromEntries(Object.entries(final).filter(([k]) => k !== 'Z1'));
    expect({ semilla, celdas: propias }).toEqual({ semilla, celdas: esperado });
    for (const c of clientes) {
      expect({
        semilla,
        actor: c.actor,
        iguales: igual(JSON.parse(c.editor), JSON.parse(srv.reg.valor)),
      }).toEqual({ semilla, actor: c.actor, iguales: true });
    }
  }
});

/* ---------- protección: un set rechazado no corrompe el modelo ---------- */

test('tras un rechazo por protección, el modelo parte del servidor y el siguiente guardado fusiona bien', () => {
  const p = (a: string, c: number): Punto => ({ a, c });
  const inicial = JSON.stringify(libro({ A1: '1' }));
  const modelo = new CoedicionModelo({ valor: inicial, lamport: 1, dot: p('x', 1) }, 1, inicial);
  // Quien edita intenta cambiar una celda bloqueada: aEnviar es puro, el servidor
  // rechaza con 403 y NO se llama a confirmar. Mientras, otra persona escribió B2.
  const rechazado = JSON.stringify(libro({ A1: 'hackeada' }));
  expect(modelo.aEnviar(rechazado)).toBe(rechazado);
  const ajeno = JSON.stringify(libro({ A1: '1', B2: 'ajeno' }));
  modelo.recibir(ajeno, 2, 2, p('y', 1));
  // adoptarServidor: el editor pasa a enseñar lo del servidor y el modelo lo adopta.
  expect(modelo.adoptar()).toBe(ajeno);
  expect(modelo.baseSeq).toBe(2);
  // Un cambio legítimo posterior se manda sobre lo del servidor, sin rastro de lo rechazado.
  const nuevo = JSON.stringify(libro({ A1: '1', B2: 'ajeno', C3: 'mío' }));
  expect(JSON.parse(modelo.aEnviar(nuevo))).toEqual(JSON.parse(nuevo));
});

/* ---------- diferencias: parches de celda y de caja ---------- */

type Json = Record<string, unknown>;

/** Lo que hace el servidor con un parche (domain.AplicarCambios), para comprobar el diff. */
function aplicar(raiz: Json, cambios: Cambio[]): Json {
  const m = structuredClone(raiz);
  const buscar = (l: Json[], id: string): number => l.findIndex((e) => e.id === id);
  for (const c of cambios) {
    let nodo = m;
    let i = 0;
    for (; i < c.path.length - 1; i++) {
      const t = c.path[i]!;
      const sig = c.path[i + 1]!;
      if (typeof t !== 'string') throw new Error('ruta mal formada');
      if (typeof sig !== 'string') {
        const lista = (nodo[t] as Json[] | undefined) ?? [];
        const j = buscar(lista, sig.id);
        if (i + 1 === c.path.length - 1) {
          if (c.del) {
            if (j >= 0) lista.splice(j, 1);
          } else if (j >= 0) lista[j] = { ...(c.value as Json), id: sig.id };
          else lista.push({ ...(c.value as Json), id: sig.id });
          nodo[t] = lista;
          nodo = {};
          break;
        }
        if (j < 0) {
          nodo = {};
          break;
        }
        nodo = lista[j]!;
        i++;
      } else {
        nodo = (nodo[t] ??= {}) as Json;
      }
    }
    const ultimo = c.path[c.path.length - 1]!;
    if (typeof ultimo === 'string' && i === c.path.length - 1) {
      if (c.del) delete nodo[ultimo];
      else nodo[ultimo] = c.value;
    }
  }
  return m;
}

const slide = (id: string, ...cajas: [string, string][]): Json => ({
  id,
  boxes: cajas.map(([bid, html]) => ({ id: bid, html })),
});

test('diferencias: una celda y una caja son un cambio cada una', () => {
  const base = { sheets: [{ id: 'h1', name: 'H', cells: { A1: '1' } }] };
  const mio = { sheets: [{ id: 'h1', name: 'H', cells: { A1: '1', B2: 'x' } }] };
  expect(diferencias(base, mio)).toEqual([
    { path: ['sheets', { id: 'h1' }, 'cells', 'B2'], value: 'x' },
  ]);
  const d0 = { slides: [slide('s1', ['b1', 'a'], ['b2', 'b'])] };
  const d1 = { slides: [slide('s1', ['b1', 'A'], ['b2', 'b'])] };
  expect(diferencias(d0, d1)).toEqual([
    { path: ['slides', { id: 's1' }, 'boxes', { id: 'b1' }, 'html'], value: 'A' },
  ]);
  expect(diferencias(d0, d0)).toEqual([]);
});

test('diferencias: lo que no se deja decir como cambio da null (se manda el registro)', () => {
  const hoja = (name: string): Json => ({ name, cells: {} });
  // hojas sin id: la fusión las trata por nombre
  expect(diferencias({ sheets: [hoja('A')] }, { sheets: [hoja('A'), hoja('B')] })).toBeNull();
  // diapositiva reordenada, o nueva en medio
  const s = (...ids: string[]): Json => ({ slides: ids.map((i) => slide(i)) });
  expect(diferencias(s('a', 'b'), s('b', 'a'))).toBeNull();
  expect(diferencias(s('a', 'b'), s('a', 'n', 'b'))).toBeNull();
  // nueva al final y borrada: sí
  expect(diferencias(s('a', 'b'), s('a', 'b', 'n'))).not.toBeNull();
  expect(diferencias(s('a', 'b'), s('b'))).not.toBeNull();
});

test('diferencias: aplicar el diff a la base da el modelo propio (ediciones al azar)', () => {
  let semilla = 7;
  const azar = (n: number): number => {
    semilla = (semilla * 1103515245 + 12345) % 2147483648;
    return semilla % n;
  };
  let comprobadas = 0;
  for (let ronda = 0; ronda < 200; ronda++) {
    const base: { sheets: Json[]; slides: Json[] } = {
      sheets: [
        { id: 'h1', name: 'A', cells: { A1: '1', B1: '2' }, freeze: { cols: 1, rows: 0 } },
        { id: 'h2', name: 'B', cells: {} },
      ],
      slides: [slide('s1', ['b1', 'a'], ['b2', 'b']), slide('s2', ['b3', 'c'])],
    };
    const mio = structuredClone(base);
    for (let k = azar(6); k >= 0; k--) {
      switch (azar(7)) {
        case 0:
          (mio.sheets[azar(2)]!.cells as Json)[`C${azar(4)}`] = `v${azar(9)}`;
          break;
        case 1:
          delete (mio.sheets[0]!.cells as Json).A1;
          break;
        case 2:
          mio.sheets[1]!.freeze = { cols: azar(3), rows: 1 };
          break;
        case 3:
          {
            const caja = ((mio.slides[0]?.boxes as Json[] | undefined) ?? [])[azar(2)];
            if (caja) caja.html = `h${azar(9)}`;
          }
          break;
        case 4:
          (mio.slides[1]?.boxes as Json[] | undefined)?.push({ id: `n${azar(3)}`, html: 'nueva' });
          break;
        case 5:
          mio.slides.push(slide(`s${3 + azar(2)}`, ['x', 'y']));
          break;
        default:
          mio.slides.splice(azar(2), 1);
      }
    }
    // ids repetidos por el azar: no es un modelo válido
    const unicos = (xs: unknown[]): boolean => new Set(xs).size === xs.length;
    if (
      !unicos(mio.slides.map((x) => x.id)) ||
      !mio.slides.every((x) => unicos((x.boxes as Json[]).map((bx) => bx.id)))
    )
      continue;
    const cambios = diferencias(base, mio);
    if (cambios === null) continue;
    expect(aplicar(base, cambios)).toEqual(mio);
    comprobadas++;
  }
  expect(comprobadas).toBeGreaterThan(60);
});

test('parche confirmado: el servidor devuelve las claves en otro orden y el editor no se recarga', () => {
  const inicial = JSON.stringify({ sheets: [{ id: 'h1', name: 'H', cells: { A1: '1' } }] });
  const reg: Registro = { valor: inicial, lamport: 1, dot: { a: 'x', c: 1 } };
  const modelo = new CoedicionModelo(reg, 3, inicial);
  const local = JSON.stringify({ sheets: [{ id: 'h1', name: 'H', cells: { A1: '1', B1: '2' } }] });
  expect(modelo.cambios(local)).toEqual([
    { path: ['sheets', { id: 'h1' }, 'cells', 'B1'], value: '2' },
  ]);
  // lo que devuelve el servidor (Go ordena las claves)
  const servidor = '{"sheets":[{"cells":{"A1":"1","B1":"2"},"id":"h1","name":"H"}]}';
  modelo.confirmarParche(local, servidor, 4, 2, { a: 'srv', c: 1 });
  expect(modelo.baseSeq).toBe(4);
  expect(modelo.adoptar()).toBeNull();
  // si en el servidor hay además lo de otro, sí se adopta
  modelo.confirmarParche(local, servidor.replace('"A1":"1"', '"A1":"9"'), 5, 3, { a: 'srv', c: 2 });
  expect(modelo.adoptar()).toContain('"A1":"9"');
});
