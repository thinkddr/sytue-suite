/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Co-edición de hojas y presentaciones: fusión a tres bandas del modelo JSON.
 *
 * El problema: Calcus y Diapos guardan el modelo ENTERO en un registro LWW
 * (`doc:sheet`, `doc:slides`), así que dos personas que tocaban celdas o
 * diapositivas distintas a la vez perdían lo del que guardaba primero. La
 * fusión por carácter de ./coedicion.ts no sirve aquí: fusionar dos JSON letra
 * a letra da un JSON que no parsea.
 *
 * Lo que hace esto, en dos piezas:
 *
 *  1. El servidor serializa. Cada guardado lleva `base_seq`, la seq hasta la que
 *     este cliente conoce el registro; si otro lo escribió después, el servicio
 *     contesta 409 sin escribir nada (store/operations.go, `comprobarBase`). Así
 *     el historial del registro es LINEAL: cada versión parte de la anterior, y
 *     todos los clientes acaban viendo la última. La convergencia sale de ahí,
 *     no de que la fusión sea conmutativa.
 *
 *  2. El cliente fusiona antes de mandar. Guarda el ANCESTRO común de lo que
 *     enseña el editor y de lo último que sabe del servidor, y manda
 *     `fusionar3(ancestro, local, servidor)`: lo suyo encima de lo ajeno. Tras un
 *     409 relee el registro y vuelve a fusionar.
 *
 * La fusión va por rutas estables: objetos por clave (las celdas son claves de
 * `cells`), listas por `id` (diapositivas, cajas, conectores, gráficos) o, si no
 * lo tienen, por `name` cuando es único (las tablas). Lo
 * que cambió solo un lado se queda con ese lado; si los dos cambiaron la misma
 * hoja del árbol, gana lo propio, que es lo que llega después al servidor.
 *
 * Además de fusionar, el cliente puede mandar solo lo que cambió: `cambios` (celda, caja,
 * diapositiva) como operación `patch`, que el servidor aplica sobre lo último que tiene, en
 * orden de seq y sin 409 (services/docs/internal/domain/parche.go). Lo que no se deja decir así
 * (`diferencias` da null) sigue yendo como registro entero con la fusión de arriba.
 *
 * Las hojas van por `id` (SheetData.id, sheets/idhoja.ts): renombrar una hoja mientras
 * otra persona escribe en ella no la parte en dos. Un libro anterior a los ids las
 * recibe al cargar, deducidas del nombre (iguales en todos los clientes).
 * ponytail: una lista sin identidad (reglas de formato condicional,
 * validaciones, minigráficos) es atómica: si los dos la cambian, gana la propia.
 * ponytail: el texto de una celda o de una caja es una hoja del árbol: dos
 * personas en la MISMA caja a la vez se pisan; la secuencia RGA del documento
 * sería el camino si eso se vuelve habitual.
 */
import { puntoMenor, type Punto } from './rga';

type Objeto = Record<string, unknown>;

function esObjeto(v: unknown): v is Objeto {
  return typeof v === 'object' && v !== null && !Array.isArray(v);
}

/** Igualdad estructural de dos valores JSON (sin depender del orden de claves). */
export function igual(a: unknown, b: unknown): boolean {
  if (a === b) return true;
  if (Array.isArray(a)) {
    if (!Array.isArray(b) || a.length !== b.length) return false;
    return a.every((x, i) => igual(x, b[i]));
  }
  if (!esObjeto(a) || !esObjeto(b)) return false;
  const ka = Object.keys(a);
  if (ka.length !== Object.keys(b).length) return false;
  return ka.every((k) => Object.hasOwn(b, k) && igual(a[k], b[k]));
}

/** Claves de una lista por `campo`, o null si algún elemento no tiene una única. */
function clavesPor(lista: unknown[], campo: string): string[] | null {
  const out: string[] = [];
  for (const e of lista) {
    const k = esObjeto(e) ? e[campo] : undefined;
    if (typeof k !== 'string' || out.includes(k)) return null;
    out.push(k);
  }
  return out;
}

/**
 * Fusión a tres bandas de valores JSON. `undefined` es «no está» (clave borrada
 * o nunca creada). Devuelve el valor que combina lo que `mio` y `suyo` cambiaron
 * respecto de `base`.
 */
export function fusionar3(base: unknown, mio: unknown, suyo: unknown): unknown {
  if (igual(mio, suyo) || igual(suyo, base)) return mio;
  if (igual(mio, base)) return suyo;
  // Los dos lo cambiaron. Un contenedor que nace a la vez en los dos lados (el
  // primer estilo de una hoja, el primer gráfico) se fusiona como si partiera vacío.
  if (esObjeto(mio) && esObjeto(suyo)) {
    return fusionarObjetos(esObjeto(base) ? base : {}, mio, suyo);
  }
  if (Array.isArray(mio) && Array.isArray(suyo)) {
    const b = Array.isArray(base) ? base : [];
    for (const campo of ['id', 'name']) {
      const kb = clavesPor(b, campo);
      const km = clavesPor(mio, campo);
      const ks = clavesPor(suyo, campo);
      if (kb && km && ks) return fusionarListas(b, mio, suyo, kb, km, ks);
    }
  }
  return mio; // conflicto en una hoja del árbol: gana lo propio (ver cabecera)
}

function fusionarObjetos(base: Objeto, mio: Objeto, suyo: Objeto): Objeto {
  const out: Objeto = {};
  for (const k of new Set([...Object.keys(mio), ...Object.keys(suyo), ...Object.keys(base)])) {
    const v = fusionar3(base[k], mio[k], suyo[k]);
    if (v !== undefined) out[k] = v;
  }
  return out;
}

function fusionarListas(
  base: unknown[],
  mio: unknown[],
  suyo: unknown[],
  kb: string[],
  km: string[],
  ks: string[],
): unknown[] {
  const porClave = (lista: unknown[], claves: string[]): Map<string, unknown> =>
    new Map(claves.map((k, i) => [k, lista[i]]));
  const b = porClave(base, kb);
  const m = porClave(mio, km);
  const s = porClave(suyo, ks);
  const valor = new Map<string, unknown>();
  for (const k of new Set([...km, ...ks, ...kb])) {
    const v = fusionar3(b.get(k), m.get(k), s.get(k));
    if (v !== undefined) valor.set(k, v);
  }
  // Orden: el de quien reordenó (si fueron los dos, el propio); lo que añadió el
  // otro entra delante de su vecino siguiente en la lista de quien lo añadió, y
  // al final si no lo tiene (añadir una diapositiva al final sigue al final).
  const enAmbos = (xs: string[], ys: string[]): string[] => xs.filter((k) => ys.includes(k));
  const mioReordena = !igual(enAmbos(km, kb), enAmbos(kb, km));
  const [guia, otra] = mioReordena ? [km, ks] : [ks, km];
  const orden = guia.filter((k) => valor.has(k));
  otra.forEach((k, i) => {
    if (!valor.has(k) || orden.includes(k)) return;
    const siguiente = otra.slice(i + 1).find((x) => orden.includes(x));
    orden.splice(siguiente === undefined ? orden.length : orden.indexOf(siguiente), 0, k);
  });
  return orden.map((k) => valor.get(k));
}

/* ---------- Parches: lo que cambió, no el modelo entero ---------- */

/** Un tramo de la ruta de un cambio: clave de objeto, o elemento de lista por `id`. */
export type Tramo = string | { id: string };

/** Mismo contrato que `domain.Cambio` del servicio (services/docs/internal/domain/parche.go). */
export interface Cambio {
  path: Tramo[];
  value?: unknown;
  del?: boolean;
}

/** Los topes del servidor (maxCambios, maxRuta, maxClaveLen): pasados, se manda el registro. */
const MAX_CAMBIOS = 2000;
const MAX_RUTA = 10;
const MAX_CLAVE = 256;

/** Lo que el servidor no sabe expresar como cambio: se cae al registro completo. */
class NoExpresable extends Error {}

/**
 * Los cambios que llevan `base` hasta `mio`: poner o quitar una clave, una celda, una caja,
 * una diapositiva. `null` si el cambio no se deja decir así —una lista sin `id` que la
 * fusión trata por nombre (hojas de libros antiguos, tablas), un elemento nuevo que no va
 * al final, una lista reordenada, demasiados cambios— y entonces se manda el registro
 * entero, con la fusión de siempre. Dos valores iguales dan `[]`.
 */
export function diferencias(base: unknown, mio: unknown): Cambio[] | null {
  if (!esObjeto(base) || !esObjeto(mio)) return null;
  const out: Cambio[] = [];
  try {
    difObjetos([], base, mio, out);
  } catch (e) {
    if (e instanceof NoExpresable) return null;
    throw e;
  }
  return out;
}

function poner(out: Cambio[], c: Cambio): void {
  if (c.path.length > MAX_RUTA || out.length >= MAX_CAMBIOS) throw new NoExpresable();
  out.push(c);
}

function difObjetos(ruta: Tramo[], base: Objeto, mio: Objeto, out: Cambio[]): void {
  for (const k of new Set([...Object.keys(base), ...Object.keys(mio)])) {
    if (k === '' || k.length > MAX_CLAVE) throw new NoExpresable();
    difValor([...ruta, k], base[k], mio[k], out);
  }
}

function difValor(ruta: Tramo[], base: unknown, mio: unknown, out: Cambio[]): void {
  if (igual(base, mio)) return;
  if (mio === undefined) return poner(out, { path: ruta, del: true });
  if (esObjeto(base) && esObjeto(mio)) return difObjetos(ruta, base, mio, out);
  if (Array.isArray(base) && Array.isArray(mio)) {
    const kb = clavesPor(base, 'id');
    const km = clavesPor(mio, 'id');
    if (kb && km) return difLista(ruta, base, mio, kb, km, out);
    // Sin identidad: si la fusión la trataría por nombre, no se puede decir como cambio.
    if (clavesPor(base, 'name') && clavesPor(mio, 'name')) throw new NoExpresable();
  }
  poner(out, { path: ruta, value: mio }); // valor suelto, o lista atómica entera
}

function difLista(
  ruta: Tramo[],
  base: unknown[],
  mio: unknown[],
  kb: string[],
  km: string[],
  out: Cambio[],
): void {
  // El servidor añade al final, así que el orden de mío tiene que ser el de base sin lo
  // quitado y lo nuevo detrás; cualquier otra cosa (reordenar, insertar en medio) no es un cambio.
  const quedan = kb.filter((k) => km.includes(k));
  if (!igual(km.slice(0, quedan.length), quedan)) throw new NoExpresable();
  for (const k of kb) if (!km.includes(k)) poner(out, { path: [...ruta, { id: k }], del: true });
  km.forEach((k, i) => {
    const antes = kb.indexOf(k);
    const tramo = [...ruta, { id: k }];
    if (antes < 0) return poner(out, { path: tramo, value: mio[i] });
    const b = base[antes];
    const m = mio[i];
    if (esObjeto(b) && esObjeto(m)) difObjetos(tramo, b, m, out);
    else if (!igual(b, m)) throw new NoExpresable();
  });
}

/** Parsea un contenido guardado; '' es «documento vacío». `null` si no es JSON. */
function parsear(texto: string): { v: unknown } | null {
  if (texto === '') return { v: undefined };
  try {
    return { v: JSON.parse(texto) as unknown };
  } catch {
    return null;
  }
}

/** fusionar3 sobre los textos guardados; si alguno no parsea, manda lo propio. */
export function fusionarTextos(base: string, mio: string, suyo: string): string {
  const b = parsear(base);
  const m = parsear(mio);
  const s = parsear(suyo);
  if (b === null || m === null || s === null) return mio;
  const v = fusionar3(b.v, m.v, s.v);
  return v === undefined ? '' : JSON.stringify(v);
}

/** El registro de contenido tal y como lo tiene el servidor. */
export interface Registro {
  valor: string;
  lamport: number;
  dot: Punto | null;
}

/**
 * Estado de la co-edición de UNA hoja o presentación abierta. No hace red: lo
 * usa useDocument al guardar y al recibir, y los tests con un servidor simulado.
 */
export class CoedicionModelo {
  private servidor: Registro;
  /** Seq hasta la que `servidor` está al día: el `base_seq` del próximo guardado. */
  private seq: number;
  /**
   * Ancestro común de lo que enseña el editor y de `servidor`. null = no se sabe
   * (borrador offline de antes de guardar la base): manda lo propio entero, que
   * es lo que pasaba siempre.
   */
  private ancestro: string | null;

  constructor(servidor: Registro, seq: number, ancestro: string | null) {
    this.servidor = servidor;
    this.seq = seq;
    this.ancestro = ancestro;
  }

  get baseSeq(): number {
    return this.seq;
  }

  /**
   * Una escritura del registro (ajena por WS o sondeo, o la de un 409 releído).
   * Replica el LWW del servidor —gana mayor lamport, desempate por punto—, así
   * que el resultado no depende del orden de llegada.
   */
  recibir(valor: string, seq: number, lamport: number, dot: Punto): void {
    this.seq = Math.max(this.seq, seq);
    const cur = this.servidor;
    const gana =
      lamport > cur.lamport ||
      (lamport === cur.lamport && (cur.dot === null || puntoMenor(cur.dot, dot)));
    if (gana) this.servidor = { valor, lamport, dot };
  }

  /** Lo que hay que mandar para guardar `local` sin pisar lo que ya escribió otro. */
  aEnviar(local: string): string {
    if (this.ancestro === null || this.ancestro === this.servidor.valor) return local;
    return fusionarTextos(this.ancestro, local, this.servidor.valor);
  }

  /**
   * Los cambios que llevan lo último que sabe el servidor de este editor (el ancestro)
   * hasta `local`, o null si hay que mandar el registro entero (sin ancestro, o un cambio
   * que no se deja decir así: ver `diferencias`).
   */
  cambios(local: string): Cambio[] | null {
    if (this.ancestro === null) return null;
    const b = parsear(this.ancestro);
    const m = parsear(local);
    if (b === null || m === null) return null;
    return diferencias(b.v, m.v);
  }

  /**
   * El servidor aplicó el parche y contestó con `materializado`, el registro que resultó
   * de ponerlo sobre lo último que había (lo de los demás incluido). Si es lo mismo que
   * `local` se queda con el texto de `local`: así `adoptar` no recarga el editor por una
   * diferencia de orden de claves.
   */
  confirmarParche(
    local: string,
    materializado: string,
    seq: number,
    lamport: number,
    dot: Punto,
  ): void {
    const a = parsear(local);
    const b = parsear(materializado);
    const mismo = a !== null && b !== null && igual(a.v, b.v);
    this.confirmar(local, mismo ? local : materializado, seq, lamport, dot);
  }

  /** Parche aplicado sin efecto nuevo (reintento ya aplicado, o no cambiaba nada). */
  confirmarSinOp(local: string, seq: number): void {
    this.ancestro = local;
    this.seq = Math.max(this.seq, seq);
  }

  /** El servidor aceptó `enviado` (= aEnviar(local)) con esa seq, lamport y punto. */
  confirmar(local: string, enviado: string, seq: number, lamport: number, dot: Punto): void {
    // Lo que el editor tenga ahora parte de `local`; el servidor, de `enviado`.
    this.ancestro = local;
    this.recibir(enviado, seq, lamport, dot);
  }

  /**
   * Con el editor SIN cambios propios: el contenido que tiene que pasar a
   * enseñar porque el servidor ya tiene otro, o null si no hay nada nuevo.
   */
  adoptar(): string | null {
    if (this.ancestro === this.servidor.valor) return null;
    this.ancestro = this.servidor.valor;
    return this.servidor.valor;
  }
}
