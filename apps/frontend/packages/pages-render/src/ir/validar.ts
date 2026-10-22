/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Validador del IR, espejo de `services/pages/internal/ir` (el de Go es la autoridad; los dos
 * recorren las mismas fixtures y tienen que dar el mismo conjunto de problemas). Dos fases:
 *
 * 1. ESTRUCTURAL sobre el valor crudo, guiada por `campos.ts`: `json`, `limite` (bytes),
 *    `version`, `tipo`, `valor`, `campo_desconocido`, `campo_no_aplica` y `obligatorio`. Si hay
 *    alguno, se devuelven solo esos.
 * 2. SEMÁNTICA sobre el IR ya tipado (contrato §4.3). Dos modos: en `borrador`, `texto_vacio`,
 *    `alt_falta` y `etiqueta_falta` son avisos (guardar nunca se bloquea por ellos); en `publicar`,
 *    errores.
 *
 * El resultado va sin repetidos y ordenado por ruta y código, igual que en Go.
 */

import { CAMPOS_NODO, CAMPOS_OBJETO, ENUMERADOS, UNIONES } from './campos';
import {
  ANCLAS_RESERVADAS,
  COLORES_OBLIGATORIOS,
  ICONOS,
  IDIOMAS,
  IDIOMAS_VISITANTE,
  LIMITES,
  SLUGS_RESERVADOS,
  VERSION_IR,
} from './tipos';
import type {
  CampoFormulario,
  Componente,
  ElementoMenu,
  Enlace,
  Estilo,
  EstiloTexto,
  ImagenRef,
  Layout,
  Nodo,
  Override,
  Pagina,
  Seccion,
  Sitio,
  Tamano,
  Textos,
  TextosRicos,
} from './tipos';

export type CodigoProblema =
  | 'json'
  | 'limite'
  | 'version'
  | 'tipo'
  | 'valor'
  | 'campo_desconocido'
  | 'campo_no_aplica'
  | 'obligatorio'
  | 'id_formato'
  | 'id_duplicado'
  | 'idioma'
  | 'portada'
  | 'slug_falta'
  | 'slug_formato'
  | 'slug_duplicado'
  | 'slug_reservado'
  | 'texto_vacio'
  | 'traduccion_falta'
  | 'enlace_roto'
  | 'ancla_rota'
  | 'ancla_duplicada'
  | 'ancla_reservada'
  | 'componente_falta'
  | 'componente_anidado'
  | 'override_invalido'
  | 'anidamiento'
  | 'color'
  | 'token'
  | 'rango'
  | 'url'
  | 'incrustado_ref'
  | 'icono'
  | 'alt_falta'
  | 'etiqueta_falta'
  | 'texto_control'
  | 'fuente'
  | 'biblioteca'
  | 'formato'
  | 'h1_varios'
  | 'salto_encabezado'
  | 'relleno_sin_minimo';

export interface Problema {
  /** JSON Pointer (RFC 6901) al miembro con el problema; '' es el documento. */
  path: string;
  code: CodigoProblema;
  severity: 'error' | 'warning';
  detail?: string;
}

export type ModoValidacion = 'borrador' | 'publicar';

export interface CatalogoValidacion {
  fuentes: string[];
  biblioteca: string[];
}

const AVISOS: ReadonlySet<CodigoProblema> = new Set([
  'traduccion_falta',
  'enlace_roto',
  'ancla_rota',
  'h1_varios',
  'salto_encabezado',
  'relleno_sin_minimo',
]);
const SEGUN_MODO: ReadonlySet<CodigoProblema> = new Set([
  'texto_vacio',
  'alt_falta',
  'etiqueta_falta',
]);

/** Valida un IR. `cat` activa `fuente` y `biblioteca` (el servicio lo pasa al guardar). */
export function validar(
  ir: unknown,
  op: { cat?: CatalogoValidacion; modo: ModoValidacion },
): Problema[] {
  const e = new Registro(op.modo);
  estructural(ir, e);
  if (e.lista.length === 0) new Semantica(ir as Sitio, op.cat, e).todo();
  return e.resultado();
}

class Registro {
  readonly lista: Problema[] = [];
  constructor(private readonly modo: ModoValidacion) {}
  add(path: string, code: CodigoProblema, detail?: string): void {
    const severity = AVISOS.has(code)
      ? 'warning'
      : SEGUN_MODO.has(code) && this.modo === 'borrador'
        ? 'warning'
        : 'error';
    this.lista.push(
      detail === undefined ? { path, code, severity } : { path, code, severity, detail },
    );
  }
  resultado(): Problema[] {
    const vistos = new Set<string>();
    const out: Problema[] = [];
    for (const p of this.lista) {
      const k = `${p.path}\u0000${p.code}\u0000${p.severity}`;
      if (!vistos.has(k)) {
        vistos.add(k);
        out.push(p);
      }
    }
    return out.sort(
      (a, b) => cmp(a.path, b.path) || cmp(a.code, b.code) || cmp(a.severity, b.severity),
    );
  }
}

function cmp(a: string, b: string): number {
  return a < b ? -1 : a > b ? 1 : 0;
}

/** Segmento de JSON Pointer. */
function seg(k: string | number): string {
  return '/' + String(k).replaceAll('~', '~0').replaceAll('/', '~1');
}

function esObjeto(v: unknown): v is Record<string, unknown> {
  return typeof v === 'object' && v !== null && !Array.isArray(v);
}

// =============================================================================================
// Fase estructural.

type Tabla = Readonly<Record<string, string>>;
const OBJETOS = CAMPOS_OBJETO as unknown as Readonly<Record<string, Tabla>>;
const NODOS = CAMPOS_NODO as unknown as Readonly<Record<string, Tabla>>;
const ENUMS = ENUMERADOS as unknown as Readonly<Record<string, readonly (string | number)[]>>;
const UNIS = UNIONES as unknown as Readonly<Record<string, readonly string[]>>;
const CLAVES_DE_NODO = new Set(Object.values(NODOS).flatMap((t) => Object.keys(t)));

/** Tipos de nodo cuyo `size` admite `aspect_ratio` (una instancia pinta su maestro, un marco). */
const CON_PROPORCION = new Set(['image', 'embed', 'frame', 'instance']);
/** Tipos de nodo cuyos deltas admiten `layout`. */
const CON_LAYOUT = new Set(['section', 'frame', 'form', 'instance']);
const CON_LONGITUD = new Set(['text', 'email', 'tel', 'textarea']);

/** Sustituto UTF-16 sin pareja: Go lo busca en los bytes, aquí en las cadenas. */
const SUSTITUTO_SUELTO = /[\ud800-\udbff](?![\udc00-\udfff])|(?<![\ud800-\udbff])[\udc00-\udfff]/;

function haySustitutoSuelto(v: unknown): boolean {
  if (typeof v === 'string') return SUSTITUTO_SUELTO.test(v);
  if (Array.isArray(v)) return v.some(haySustitutoSuelto);
  if (esObjeto(v)) {
    return Object.entries(v).some(([k, x]) => SUSTITUTO_SUELTO.test(k) || haySustitutoSuelto(x));
  }
  return false;
}

/** Bytes UTF-8 de la serialización compacta: lo que mide `LIMITES.irBytes`. */
export function bytesIR(ir: unknown): number {
  return new TextEncoder().encode(JSON.stringify(ir)).length;
}

function estructural(ir: unknown, e: Registro): void {
  if (haySustitutoSuelto(ir)) return e.add('', 'json');
  if (!esObjeto(ir)) return e.add('', 'tipo');
  if (bytesIR(ir) > LIMITES.irBytes) return e.add('', 'limite');
  const v = ir['version'];
  if (typeof v === 'number' && Number.isInteger(v) && v !== VERSION_IR) {
    return e.add('/version', 'version');
  }
  objeto(ir, 'sitio', '', '', e);
}

/** Comprueba `v` contra un descriptor de `campos.ts`. `nodo` es el tipo del nodo que lo contiene. */
function comprobar(v: unknown, desc: string, path: string, nodo: string, e: Registro): void {
  let d = desc.endsWith('!') ? desc.slice(0, -1) : desc;
  if (d.endsWith('|null')) {
    if (v === null) return;
    d = d.slice(0, -5);
  }
  const tuple = /^([in])\[(\d)\]$/.exec(d);
  if (tuple) {
    if (!Array.isArray(v) || v.length !== Number(tuple[2])) return e.add(path, 'tipo');
    v.forEach((x: unknown, i) => comprobar(x, tuple[1] ?? '', path + seg(i), nodo, e));
    return;
  }
  const [cab, resto] = partir(d);
  switch (cab) {
    case 's':
      if (typeof v !== 'string') e.add(path, 'tipo');
      return;
    case 'b':
      if (typeof v !== 'boolean') e.add(path, 'tipo');
      return;
    case 't':
      if (typeof v !== 'boolean') e.add(path, 'tipo');
      else if (!v) e.add(path, 'valor');
      return;
    case 'i':
      if (typeof v !== 'number' || !Number.isInteger(v)) e.add(path, 'tipo');
      return;
    case 'n':
      if (typeof v !== 'number') e.add(path, 'tipo');
      return;
    case 'e':
      return enumerado(v, resto, path, e);
    case 'o':
      return objeto(v, resto, path, nodo, e);
    case 'u':
      return union(v, resto, path, nodo, e);
    case 'a':
      if (!Array.isArray(v)) return e.add(path, 'tipo');
      v.forEach((x: unknown, i) => comprobar(x, resto, path + seg(i), nodo, e));
      return;
    case 'm':
      if (!esObjeto(v)) return e.add(path, 'tipo');
      for (const [k, x] of Object.entries(v)) comprobar(x, resto, path + seg(k), nodo, e);
      return;
    case 'textos':
      return comprobar(v, 'm:s', path, nodo, e);
    case 'ricos':
      return comprobar(v, 'm:a:o:tramo', path, nodo, e);
    case 'nodo':
      return nodoCrudo(v, path, e);
    case 'medida':
      if (typeof v === 'string') {
        if (v !== 'fill' && v !== 'hug') e.add(path, 'valor');
      } else if (typeof v !== 'number' || !Number.isInteger(v)) e.add(path, 'tipo');
      return;
    case 'radio':
      if (typeof v === 'string') {
        if (v !== '$sm' && v !== '$md' && v !== '$lg') e.add(path, 'valor');
      } else if (typeof v !== 'number' || !Number.isInteger(v)) e.add(path, 'tipo');
      return;
    case 'fondo':
      if (typeof v !== 'string') objeto(v, 'fondo_imagen', path, nodo, e);
      return;
    default:
      throw new Error(`descriptor desconocido: ${desc}`);
  }
}

function partir(d: string): [string, string] {
  const i = d.indexOf(':');
  return i < 0 ? [d, ''] : [d.slice(0, i), d.slice(i + 1)];
}

function enumerado(v: unknown, nombre: string, path: string, e: Registro): void {
  const valores = ENUMS[nombre] ?? [];
  if (typeof valores[0] === 'number') {
    if (typeof v !== 'number' || !Number.isInteger(v)) return e.add(path, 'tipo');
  } else if (typeof v !== 'string') return e.add(path, 'tipo');
  if (!valores.includes(v)) e.add(path, 'valor');
}

/** Claves de un objeto contra su tabla; `ajena` decide entre `campo_no_aplica` y `campo_desconocido`. */
function claves(
  v: Record<string, unknown>,
  tabla: Tabla,
  path: string,
  nodo: string,
  e: Registro,
  ajena: (k: string) => boolean,
  noAplica: (k: string) => boolean = () => false,
): void {
  for (const [k, x] of Object.entries(v)) {
    const d = tabla[k];
    if (d === undefined || noAplica(k)) {
      e.add(path + seg(k), d !== undefined || ajena(k) ? 'campo_no_aplica' : 'campo_desconocido');
    } else comprobar(x, d, path + seg(k), nodo, e);
  }
  for (const [k, d] of Object.entries(tabla)) {
    if (d.endsWith('!') && !(k in v)) e.add(path + seg(k), 'obligatorio');
  }
}

function objeto(v: unknown, nombre: string, path: string, nodo: string, e: Registro): void {
  if (!esObjeto(v)) return e.add(path, 'tipo');
  const tabla = OBJETOS[nombre] ?? {};
  claves(
    v,
    tabla,
    path,
    nodo,
    e,
    () => false,
    (k) => noAplica(nombre, k, v, nodo),
  );
  if (nombre === 'campo' && v['kind'] === 'select' && !('options' in v)) {
    e.add(path + '/options', 'obligatorio');
  }
  if (nodo === 'section' && (nombre === 'tamano' || nombre === 'tamano_delta')) {
    if ((typeof v['width'] === 'number' && Number.isInteger(v['width'])) || v['width'] === 'hug') {
      e.add(path + '/width', 'valor');
    }
    if (v['height'] === 'fill') e.add(path + '/height', 'valor');
  }
}

/** Campos que existen en la tabla pero no valen en este contexto (contrato §4.1). */
function noAplica(objeto: string, k: string, v: Record<string, unknown>, nodo: string): boolean {
  switch (objeto) {
    case 'tamano':
    case 'tamano_delta':
      return k === 'aspect_ratio' && !CON_PROPORCION.has(nodo);
    case 'delta':
      return (
        (k === 'layout' && !CON_LAYOUT.has(nodo)) || (k === 'columns' && nodo !== 'product_grid')
      );
    case 'campo': {
      const kind = v['kind'];
      if (typeof kind !== 'string' || !ENUMS['tipo_campo']?.includes(kind)) return false;
      if (k === 'options') return kind !== 'select';
      if (k === 'max_length') return !CON_LONGITUD.has(kind);
      if (k === 'min' || k === 'max') return kind !== 'number';
      return false;
    }
    default:
      return false;
  }
}

function union(v: unknown, nombre: string, path: string, nodo: string, e: Registro): void {
  if (!esObjeto(v)) return e.add(path, 'tipo');
  const discs = (UNIS[nombre] ?? []).filter((d) => d in v);
  const variante = (d: string): Tabla => OBJETOS[`${nombre}_${d}`] ?? {};
  const quedan = discs.filter((d) => !discs.some((o) => o !== d && d in variante(o)));
  const [elegida] = quedan;
  if (quedan.length !== 1 || elegida === undefined) return e.add(path, 'valor');
  const todas = new Set((UNIS[nombre] ?? []).flatMap((d) => Object.keys(variante(d))));
  claves(v, variante(elegida), path, nodo, e, (k) => todas.has(k));
}

function nodoCrudo(v: unknown, path: string, e: Registro): void {
  if (!esObjeto(v)) return e.add(path, 'tipo');
  const t = v['type'];
  if (t === undefined) return e.add(path + '/type', 'obligatorio');
  if (typeof t !== 'string') return e.add(path + '/type', 'tipo');
  const tabla = NODOS[t];
  if (!tabla) return e.add(path + '/type', 'valor');
  claves(v, tabla, path, t, e, (k) => CLAVES_DE_NODO.has(k));
}

// =============================================================================================
// Fase semántica.

const RE_ID = /^[a-z0-9]{10}$/;
const RE_SLUG = /^[a-z0-9]+(-[a-z0-9]+)*$/;
const RE_ANCLA = /^[a-z][a-z0-9-]{0,39}$/;
const RE_CLAVE_COLOR = /^[a-z][a-z0-9_]{0,31}$/;
const RE_COLOR = /^#[0-9a-f]{6}([0-9a-f]{2})?$/;
const RE_HORA = /^([01][0-9]|2[0-3]):[0-5][0-9]$/;
const RE_PAIS = /^[A-Z]{2}$/;
const RE_UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;
const RE_YOUTUBE = /^[A-Za-z0-9_-]{11}$/;
const RE_VIMEO = /^[0-9]{6,12}$/;
const RE_OSM = /^(-?[0-9]{1,2}(\.[0-9]{1,7})?),(-?[0-9]{1,3}(\.[0-9]{1,7})?),([0-9]{1,2})$/;
/** Categoría Cc salvo `\n`, más U+2028 y U+2029. */
// eslint-disable-next-line no-control-regex
const RE_CONTROL = /[\u0000-\u0009\u000b-\u001f\u007f-\u009f\u2028\u2029]/;

const INTERACTIVOS = new Set([
  'button',
  'form',
  'menu',
  'embed',
  'product_grid',
  'cart_button',
  'language_switcher',
]);
const CON_ETIQUETA = new Set(['button', 'icon', 'cart_button', 'menu']);
const CON_ENLACE = new Set(['frame', 'image', 'button']);
const ICONO = new Set<string>(ICONOS);

/** Lo que comparten `Estilo`, el estilo de un delta y el de un override. */
interface EstiloLaxo {
  background?: Estilo['background'] | null;
  color?: string;
  border?: Estilo['border'] | null;
  radius?: Estilo['radius'];
  opacity?: number;
  text?: EstiloTexto;
}

type ClaseTexto = 'obligatorio' | 'opcional' | 'alt' | 'etiqueta' | 'slug';

/** Vacío = sin el idioma, o solo espacios y saltos de línea (mismo criterio que Go). */
function vacio(s: string | undefined): boolean {
  return s === undefined || /^[ \n]*$/.test(s);
}

function textoDeRicos(t: TextosRicos, l: string): string | undefined {
  const tramos = t[l as keyof TextosRicos];
  return tramos === undefined ? undefined : tramos.map((x) => x.text).join('');
}

function runas(s: string): number {
  let n = 0;
  for (const _ of s) n++;
  return n;
}

/** Contexto de un recorrido de nodos. */
interface Donde {
  /** Página en la que se pinta (-1 = cabecera, pie o componente: en todas). */
  pagina: number;
  enMaestro: boolean;
  dentroDeEnlace: boolean;
  profundidad: number;
}

class Semantica {
  private readonly idiomas: string[];
  private readonly principal: string;
  private readonly componentes = new Map<string, Componente>();
  private readonly paginas = new Map<string, number>();
  private readonly anclas: Set<string>[] = [];
  private readonly anclasGlobales = new Set<string>();

  constructor(
    private readonly s: Sitio,
    private readonly cat: CatalogoValidacion | undefined,
    private readonly e: Registro,
  ) {
    this.principal = s.languages.default;
    this.idiomas = [...new Set([s.languages.default, ...s.languages.others])];
    for (const c of s.components) if (!this.componentes.has(c.id)) this.componentes.set(c.id, c);
    s.pages.forEach((p, i) => {
      if (!this.paginas.has(p.id)) this.paginas.set(p.id, i);
    });
    for (const g of [s.header, s.footer])
      if (g?.anchor !== undefined) this.anclasGlobales.add(g.anchor);
    for (const p of s.pages) {
      const a = new Set(this.anclasGlobales);
      for (const sec of p.sections)
        if (sec.type === 'section' && sec.anchor !== undefined) a.add(sec.anchor);
      this.anclas.push(a);
    }
  }

  todo(): void {
    const s = this.s;
    this.ids();
    this.lenguas();
    this.tema();
    this.global(s.header, '/header', 'header');
    this.global(s.footer, '/footer', 'footer');
    this.anclasDeCabeceraYPie();
    if (s.pages.length === 0) this.e.add('/pages', 'portada');
    if (s.pages.length > LIMITES.paginas) this.e.add('/pages' + seg(LIMITES.paginas), 'limite');
    const slugs = new Map<string, Set<string>>();
    s.pages.forEach((p, i) => this.pagina(p, i, slugs));
    if (s.components.length > LIMITES.componentes) {
      this.e.add('/components' + seg(LIMITES.componentes), 'limite');
    }
    s.components.forEach((c, i) => this.componente(c, i));
    this.seo();
  }

  // ---- ids ------------------------------------------------------------------------------------

  private ids(): void {
    const vistos = new Set<string>();
    const id = (v: string, path: string): void => {
      if (!RE_ID.test(v)) this.e.add(path, 'id_formato');
      else if (vistos.has(v)) this.e.add(path, 'id_duplicado');
      else vistos.add(v);
    };
    const menu = (items: ElementoMenu[], path: string): void =>
      items.forEach((it, i) => {
        id(it.id, path + seg(i) + '/id');
        if (it.children) menu(it.children, path + seg(i) + '/children');
      });
    const nodo = (n: Seccion | Nodo, path: string): void => {
      id(n.id, path + '/id');
      if (n.type === 'form') {
        id(n.form, path + '/form');
        n.fields.forEach((f, i) => id(f.id, path + '/fields' + seg(i) + '/id'));
      }
      if (n.type === 'menu') menu(n.items, path + '/items');
      if (n.type === 'section' || n.type === 'frame') {
        n.children.forEach((c, i) => nodo(c, path + '/children' + seg(i)));
      }
    };
    const s = this.s;
    if (s.header) nodo(s.header, '/header');
    if (s.footer) nodo(s.footer, '/footer');
    s.pages.forEach((p, i) => {
      id(p.id, `/pages${seg(i)}/id`);
      p.sections.forEach((n, j) => nodo(n, `/pages${seg(i)}/sections${seg(j)}`));
    });
    s.components.forEach((c, i) => {
      id(c.id, `/components${seg(i)}/id`);
      nodo(c.root, `/components${seg(i)}/root`);
    });
  }

  // ---- idiomas y textos -----------------------------------------------------------------------

  private lengua(l: string, path: string): void {
    if ((IDIOMAS_VISITANTE as readonly string[]).includes(l)) return;
    if ((IDIOMAS as readonly string[]).includes(l)) this.e.add(path, 'idioma', 'no_disponible');
    else this.e.add(path, 'idioma');
  }

  private lenguas(): void {
    const { default: d, others } = this.s.languages;
    this.lengua(d, '/languages/default');
    const vistos = new Set([d]);
    others.forEach((l, i) => {
      const path = '/languages/others' + seg(i);
      if (vistos.has(l)) this.e.add(path, 'idioma');
      else this.lengua(l, path);
      vistos.add(l);
    });
  }

  private control(s: string, path: string): void {
    if (RE_CONTROL.test(s)) this.e.add(path, 'texto_control');
  }

  private textos(t: Textos | undefined, path: string, clase: ClaseTexto): void {
    if (t === undefined) {
      if (clase !== 'opcional' && clase !== 'slug') this.vacio(path, clase);
      return;
    }
    for (const [l, v] of Object.entries(t)) {
      if (!this.idiomas.includes(l)) this.e.add(path + seg(l), 'idioma');
      this.control(v, path + seg(l));
      if (runas(v) > LIMITES.caracteresPorTexto) this.e.add(path + seg(l), 'limite');
    }
    if (clase !== 'slug') this.traducciones((l) => t[l as keyof Textos], path, clase);
  }

  private ricos(t: TextosRicos | undefined, path: string, clase: ClaseTexto, donde: Donde): void {
    if (t === undefined) {
      if (clase !== 'opcional') this.vacio(path, clase);
      return;
    }
    for (const [l, tramos] of Object.entries(t)) {
      const pl = path + seg(l);
      if (!this.idiomas.includes(l)) this.e.add(pl, 'idioma');
      let n = 0;
      tramos.forEach((tr, j) => {
        this.control(tr.text, pl + seg(j) + '/text');
        n += runas(tr.text);
        if (tr.link) this.enlace(tr.link, pl + seg(j) + '/link', donde);
      });
      if (n > LIMITES.caracteresPorTexto) this.e.add(pl, 'limite');
    }
    this.traducciones((l) => textoDeRicos(t, l), path, clase);
  }

  private vacio(path: string, clase: ClaseTexto): void {
    const p = path + seg(this.principal);
    if (clase === 'obligatorio') this.e.add(p, 'texto_vacio');
    else if (clase === 'alt') this.e.add(p, 'alt_falta');
    else if (clase === 'etiqueta') this.e.add(p, 'etiqueta_falta');
  }

  private traducciones(
    de: (l: string) => string | undefined,
    path: string,
    clase: ClaseTexto,
  ): void {
    if (vacio(de(this.principal))) {
      this.vacio(path, clase);
      return;
    }
    for (const l of this.idiomas) {
      if (l !== this.principal && vacio(de(l))) this.e.add(path + seg(l), 'traduccion_falta');
    }
  }

  // ---- tema, colores y medidas ----------------------------------------------------------------

  private tema(): void {
    const t = this.s.theme;
    const claves = Object.keys(t.colors);
    if (claves.length > LIMITES.coloresTema) this.e.add('/theme/colors', 'limite');
    for (const [k, v] of Object.entries(t.colors)) {
      const path = '/theme/colors' + seg(k);
      if (!RE_CLAVE_COLOR.test(k)) this.e.add(path, 'token');
      if (!RE_COLOR.test(v)) this.e.add(path, 'color');
    }
    for (const k of COLORES_OBLIGATORIOS) {
      if (!(k in t.colors)) this.e.add('/theme/colors' + seg(k), 'token');
    }
    for (const k of ['heading', 'body'] as const) {
      this.control(t.fonts[k], '/theme/fonts/' + k);
      if (this.cat && !this.cat.fuentes.includes(t.fonts[k]))
        this.e.add('/theme/fonts/' + k, 'fuente');
    }
    for (const [k, st] of Object.entries(t.text_styles)) {
      const path = '/theme/text_styles' + seg(k);
      st.size.forEach((n, i) => this.rango(n, 8, 160, path + '/size' + seg(i)));
      this.rango(st.line_height, 0.8, 3, path + '/line_height');
      this.rango(st.letter_spacing, -0.1, 0.5, path + '/letter_spacing');
    }
    for (const k of ['sm', 'md', 'lg'] as const)
      this.rango(t.radius[k], 0, 999, '/theme/radius/' + k);
    this.rango(t.container, 640, 1600, '/theme/container');
  }

  private rango(n: number | null | undefined, min: number, max: number, path: string): void {
    if (typeof n === 'number' && (n < min || n > max)) this.e.add(path, 'rango');
  }

  private color(v: string | undefined, path: string): void {
    if (v === undefined) return;
    if (v.startsWith('$')) {
      if (!Object.hasOwn(this.s.theme.colors, v.slice(1))) this.e.add(path, 'token');
    } else if (!RE_COLOR.test(v)) this.e.add(path, 'color');
  }

  private imagen(r: ImagenRef | undefined, path: string): void {
    if (r && 'library' in r && this.cat && !this.cat.biblioteca.includes(r.library)) {
      this.e.add(path + '/library', 'biblioteca');
    }
  }

  private tamano(t: Partial<Record<keyof Tamano, unknown>> | undefined, path: string): void {
    if (!t) return;
    for (const k of ['width', 'height'] as const) {
      const v = t[k];
      if (typeof v === 'number') this.rango(v, 1, 4000, path + '/' + k);
    }
    for (const k of ['min_width', 'max_width', 'min_height', 'max_height'] as const) {
      const v = t[k];
      if (typeof v === 'number') this.rango(v, 0, 4000, path + '/' + k);
    }
    if (typeof t.aspect_ratio === 'number')
      this.rango(t.aspect_ratio, 0.1, 10, path + '/aspect_ratio');
  }

  private layout(l: Partial<Layout> | undefined, path: string): void {
    if (!l) return;
    this.rango(l.gap, 0, 400, path + '/gap');
    this.rango(l.row_gap, 0, 400, path + '/row_gap');
    l.padding?.forEach((n, i) => this.rango(n, 0, 400, path + '/padding' + seg(i)));
    this.rango(l.columns, 1, 12, path + '/columns');
  }

  private estilo(st: EstiloLaxo | undefined, path: string): void {
    if (!st) return;
    const bg = st.background;
    if (typeof bg === 'string') this.color(bg, path + '/background');
    else if (bg) {
      this.imagen(bg.image, path + '/background/image');
      this.color(bg.overlay, path + '/background/overlay');
    }
    this.color(st.color, path + '/color');
    if (st.border) {
      this.rango(st.border.width, 0, 20, path + '/border/width');
      this.color(st.border.color, path + '/border/color');
    }
    if (typeof st.radius === 'number') this.rango(st.radius, 0, 999, path + '/radius');
    this.rango(st.opacity, 0, 1, path + '/opacity');
    this.estiloTexto(st.text, path + '/text');
  }

  private estiloTexto(t: EstiloTexto | undefined, path: string): void {
    if (!t) return;
    this.rango(t.size, 8, 160, path + '/size');
    this.rango(t.line_height, 0.8, 3, path + '/line_height');
    this.rango(t.letter_spacing, -0.1, 0.5, path + '/letter_spacing');
  }

  // ---- enlaces --------------------------------------------------------------------------------

  private enlace(en: Enlace, path: string, donde: Donde): void {
    if ('page' in en) {
      const i = this.paginas.get(en.page);
      if (i === undefined) this.e.add(path + '/page', 'enlace_roto');
      else if (en.anchor !== undefined && !this.anclas[i]?.has(en.anchor)) {
        this.e.add(path + '/anchor', 'ancla_rota');
      }
    } else if ('anchor' in en) {
      const ok =
        donde.pagina >= 0
          ? this.anclas[donde.pagina]?.has(en.anchor)
          : this.anclas.some((a) => a.has(en.anchor)) || this.anclasGlobales.has(en.anchor);
      if (!ok) this.e.add(path + '/anchor', 'ancla_rota');
    } else if ('url' in en && !urlValida(en.url)) this.e.add(path + '/url', 'url');
  }

  // ---- páginas, secciones globales y componentes ----------------------------------------------

  private global(n: Seccion | null, path: string, kind: 'header' | 'footer'): void {
    if (!n) return;
    const donde: Donde = { pagina: -1, enMaestro: false, dentroDeEnlace: false, profundidad: 1 };
    if (n.type !== 'section') {
      this.e.add(path, 'anidamiento');
      this.nodo(n, path, donde, null);
      return;
    }
    if (n.kind !== kind) this.e.add(path + '/kind', 'anidamiento');
    this.nodo(n, path, donde, null);
  }

  private anclasDeCabeceraYPie(): void {
    const vistas = new Set<string>();
    for (const [n, path] of [
      [this.s.header, '/header'],
      [this.s.footer, '/footer'],
    ] as const) {
      if (n?.type === 'section') this.ancla(n, path, vistas);
    }
  }

  private ancla(n: Seccion, path: string, vistas: Set<string>): void {
    if (n.anchor === undefined) return;
    const p = path + '/anchor';
    if (!RE_ANCLA.test(n.anchor)) this.e.add(p, 'formato');
    else if ((ANCLAS_RESERVADAS as readonly string[]).includes(n.anchor))
      this.e.add(p, 'ancla_reservada');
    else if (vistas.has(n.anchor)) this.e.add(p, 'ancla_duplicada');
    vistas.add(n.anchor);
  }

  private pagina(p: Pagina, i: number, slugs: Map<string, Set<string>>): void {
    const path = '/pages' + seg(i);
    this.textos(p.slug, path + '/slug', 'slug');
    for (const l of this.idiomas) {
      const v = p.slug[l as keyof Textos];
      const sp = path + '/slug' + seg(l);
      if (v === undefined) this.e.add(sp, 'slug_falta');
      else if (i === 0 || v === '') {
        if ((i === 0) !== (v === '')) this.e.add(sp, 'portada');
      } else if (!RE_SLUG.test(v) || v.length > LIMITES.slug) this.e.add(sp, 'slug_formato');
      else if ((SLUGS_RESERVADOS as readonly string[]).includes(v))
        this.e.add(sp, 'slug_reservado');
      else {
        const vistos = slugs.get(l) ?? new Set<string>();
        if (vistos.has(v)) this.e.add(sp, 'slug_duplicado');
        vistos.add(v);
        slugs.set(l, vistos);
      }
    }
    this.textos(p.title, path + '/title', 'obligatorio');
    this.textos(p.description, path + '/description', 'obligatorio');
    this.imagen(p.og_image, path + '/og_image');
    if (p.sections.length > LIMITES.seccionesPorPagina) {
      this.e.add(path + '/sections' + seg(LIMITES.seccionesPorPagina), 'limite');
    }
    const anclas = new Set<string>();
    for (const g of [this.s.header, this.s.footer])
      if (g?.anchor !== undefined) anclas.add(g.anchor);
    p.sections.forEach((n, j) => {
      const np = path + '/sections' + seg(j);
      const donde: Donde = { pagina: i, enMaestro: false, dentroDeEnlace: false, profundidad: 1 };
      if (n.type !== 'section') this.e.add(np, 'anidamiento');
      else this.ancla(n, np, anclas);
      this.nodo(n, np, donde, null);
    });
    this.encabezados(p, path);
    if (this.nodosDePagina(p) > LIMITES.nodosPorPagina) this.e.add(path, 'limite');
  }

  /** Encabezados en orden del DOM: cabecera, secciones y pie. `ponytail:` sin expandir instancias. */
  private encabezados(p: Pagina, path: string): void {
    const lista: [number, string][] = [];
    const visitar = (n: Seccion | Nodo, np: string): void => {
      if (n.type === 'text' && /^h[1-6]$/.test(n.tag)) lista.push([Number(n.tag.slice(1)), np]);
      if (n.type === 'section' || n.type === 'frame') {
        n.children.forEach((c, i) => visitar(c, np + '/children' + seg(i)));
      }
    };
    if (p.show_header && this.s.header) visitar(this.s.header, '/header');
    p.sections.forEach((n, j) => visitar(n, path + '/sections' + seg(j)));
    if (p.show_footer && this.s.footer) visitar(this.s.footer, '/footer');
    let h1 = false;
    let previo = 1;
    for (const [nivel, np] of lista) {
      if (nivel === 1 && h1) this.e.add(np, 'h1_varios');
      if (nivel === 1) h1 = true;
      if (nivel > previo + 1) this.e.add(np, 'salto_encabezado');
      previo = nivel;
    }
  }

  private nodosDePagina(p: Pagina): number {
    const usados = new Set<string>();
    const contar = (n: Seccion | Nodo): number => {
      if (n.type === 'instance') usados.add(n.component);
      return (
        1 +
        (n.type === 'section' || n.type === 'frame'
          ? n.children.reduce((a, c) => a + contar(c), 0)
          : 0)
      );
    };
    let total = p.sections.reduce((a, n) => a + contar(n), 0);
    if (p.show_header && this.s.header) total += contar(this.s.header);
    if (p.show_footer && this.s.footer) total += contar(this.s.footer);
    for (const c of usados) {
      const m = this.componentes.get(c);
      if (m) total += contarNodos(m.root);
    }
    return total;
  }

  private componente(c: Componente, i: number): void {
    const path = '/components' + seg(i);
    this.control(c.name, path + '/name');
    if (c.root.type !== 'frame') this.e.add(path + '/root', 'anidamiento');
    if (contarNodos(c.root) > LIMITES.nodosPorComponente) this.e.add(path, 'limite');
    const donde: Donde = { pagina: -1, enMaestro: true, dentroDeEnlace: false, profundidad: 1 };
    this.nodo(c.root, path + '/root', donde, null);
  }

  private seo(): void {
    const s = this.s.seo;
    this.textos(s.site_name, '/seo/site_name', 'obligatorio');
    this.imagen(s.og_image, '/seo/og_image');
    this.imagen(s.favicon, '/seo/favicon');
    const b = s.business;
    if (!b) return;
    for (const k of ['phone', 'email'] as const) {
      const v = b[k];
      if (v !== undefined) this.control(v, '/seo/business/' + k);
    }
    if (b.address) {
      for (const [k, v] of Object.entries(b.address))
        this.control(v, '/seo/business/address' + seg(k));
      if (!RE_PAIS.test(b.address.country)) this.e.add('/seo/business/address/country', 'formato');
    }
    b.opening_hours?.forEach((h, i) => {
      const path = '/seo/business/opening_hours' + seg(i);
      if (!RE_HORA.test(h.opens)) this.e.add(path + '/opens', 'formato');
      if (!RE_HORA.test(h.closes)) this.e.add(path + '/closes', 'formato');
    });
  }

  // ---- nodos ----------------------------------------------------------------------------------

  private maestro(n: Seccion | Nodo): Nodo | undefined {
    return n.type === 'instance' ? this.componentes.get(n.component)?.root : undefined;
  }

  private esLi(n: Seccion | Nodo): boolean {
    const r = n.type === 'instance' ? this.maestro(n) : n;
    return r?.type === 'frame' && r.tag === 'li';
  }

  private interactivo(n: Seccion | Nodo): boolean {
    switch (n.type) {
      case 'image':
      case 'frame':
        return n.link !== undefined;
      case 'text':
        return Object.values(n.content).some((ts) => ts.some((t) => t.link !== undefined));
      case 'instance': {
        if (Object.values(n.overrides ?? {}).some((o) => o.link !== undefined && o.link !== null)) {
          return true;
        }
        const m = this.maestro(n);
        // Un maestro no contiene instancias (`componente_anidado`); no se siguen: podrían ser un ciclo.
        return (
          m !== undefined &&
          algunoInteractivo(m, (x) => x.type !== 'instance' && this.interactivo(x))
        );
      }
      default:
        return INTERACTIVOS.has(n.type);
    }
  }

  private nodo(n: Seccion | Nodo, path: string, donde: Donde, padre: Seccion | Nodo | null): void {
    const e = this.e;
    if (donde.profundidad > LIMITES.profundidad) return e.add(path, 'limite');
    if (n.name !== undefined && n.type !== 'icon') {
      this.control(n.name, path + '/name');
      if (runas(n.name) > 60) e.add(path + '/name', 'limite');
    }
    if (donde.dentroDeEnlace && this.interactivo(n)) e.add(path, 'anidamiento');
    this.tamano(n.size, path + '/size');
    this.estilo(n.style, path + '/style');
    for (const bp of ['tablet', 'mobile'] as const) {
      const d = n.breakpoints?.[bp];
      if (!d) continue;
      const dp = path + '/breakpoints/' + bp;
      this.tamano(d.size, dp + '/size');
      this.layout(d.layout, dp + '/layout');
      this.estilo(d.style, dp + '/style');
      this.rango(d.columns, 1, 6, dp + '/columns');
    }
    switch (n.type) {
      case 'section':
      case 'frame':
        this.contenedor(n, path, donde);
        break;
      case 'text':
        this.ricos(n.content, path + '/content', 'obligatorio', donde);
        break;
      case 'image':
        this.imagen(n.src, path + '/src');
        this.textos(n.alt, path + '/alt', n.decorative ? 'opcional' : 'alt');
        if (n.link) this.enlace(n.link, path + '/link', donde);
        break;
      case 'button':
        this.textos(n.label, path + '/label', 'etiqueta');
        this.enlace(n.action, path + '/action', donde);
        if (n.icon !== undefined && !ICONO.has(n.icon)) e.add(path + '/icon', 'icono');
        break;
      case 'icon':
        if (!ICONO.has(n.name)) e.add(path + '/name', 'icono');
        this.textos(n.label, path + '/label', 'opcional');
        if (
          donde.dentroDeEnlace &&
          vacio(n.label?.[this.principal as keyof Textos]) &&
          !(padre && 'children' in padre && padre.children.some((c) => c.type === 'text'))
        ) {
          e.add(path + '/label' + seg(this.principal), 'etiqueta_falta');
        }
        break;
      case 'embed':
        this.textos(n.title, path + '/title', 'obligatorio');
        if (!refValida(n.provider, n.ref)) e.add(path + '/ref', 'incrustado_ref');
        break;
      case 'form':
        this.formulario(n, path, donde);
        break;
      case 'table':
        this.textos(n.caption, path + '/caption', 'obligatorio');
        n.header.forEach((t, i) => this.textos(t, path + '/header' + seg(i), 'opcional'));
        if (n.header.length > LIMITES.columnasTabla)
          e.add(path + '/header' + seg(LIMITES.columnasTabla), 'limite');
        if (n.rows.length > LIMITES.filasTabla)
          e.add(path + '/rows' + seg(LIMITES.filasTabla), 'limite');
        n.rows.forEach((fila, f) => {
          const fp = path + '/rows' + seg(f);
          if (fila.length > LIMITES.columnasTabla) e.add(fp + seg(LIMITES.columnasTabla), 'limite');
          fila.forEach((t, c) => this.textos(t, fp + seg(c), 'opcional'));
        });
        break;
      case 'product_grid':
        this.rango(n.limit, 1, LIMITES.productosEnRejilla, path + '/limit');
        this.rango(n.columns, 1, 6, path + '/columns');
        if ('category' in n.source) this.control(n.source.category, path + '/source/category');
        if ('products' in n.source && n.source.products.length > LIMITES.productosEnRejilla) {
          e.add(path + '/source/products' + seg(LIMITES.productosEnRejilla), 'limite');
        }
        break;
      case 'cart_button':
        this.textos(n.label, path + '/label', 'etiqueta');
        break;
      case 'language_switcher':
        break;
      case 'menu':
        this.textos(n.label, path + '/label', 'opcional');
        if (n.items.length > LIMITES.elementosMenu)
          e.add(path + '/items' + seg(LIMITES.elementosMenu), 'limite');
        this.elementos(n.items, path + '/items', donde);
        break;
      case 'instance':
        this.instancia(n, path, donde);
        break;
    }
  }

  private contenedor(
    n: Seccion | Extract<Nodo, { type: 'frame' }>,
    path: string,
    donde: Donde,
  ): void {
    const e = this.e;
    this.layout(n.layout, path + '/layout');
    let dentro = donde.dentroDeEnlace;
    if (n.type === 'frame' && n.link) {
      if (n.tag !== 'div') e.add(path + '/link', 'anidamiento');
      this.enlace(n.link, path + '/link', donde);
      dentro = true;
    }
    const lista = n.type === 'frame' && (n.tag === 'ul' || n.tag === 'ol');
    const flujo = n.layout.direction === 'row' && n.layout.wrap && n.layout.columns === undefined;
    n.children.forEach((c, i) => {
      const cp = path + '/children' + seg(i);
      if ((c.type as string) === 'section') e.add(cp, 'anidamiento');
      if (lista !== this.esLi(c)) e.add(cp, 'anidamiento');
      if (flujo && c.size?.width === 'fill' && c.size.min_width === undefined) {
        e.add(cp + '/size/width', 'relleno_sin_minimo');
      }
      this.nodo(c, cp, { ...donde, dentroDeEnlace: dentro, profundidad: donde.profundidad + 1 }, n);
    });
  }

  private formulario(n: Extract<Nodo, { type: 'form' }>, path: string, donde: Donde): void {
    if (donde.enMaestro) this.e.add(path, 'anidamiento');
    this.layout(n.layout, path + '/layout');
    if (n.fields.length > LIMITES.camposFormulario) {
      this.e.add(path + '/fields' + seg(LIMITES.camposFormulario), 'limite');
    }
    n.fields.forEach((f, i) => this.campo(f, path + '/fields' + seg(i)));
    for (const k of ['submit', 'success', 'consent'] as const)
      this.textos(n[k], path + '/' + k, 'obligatorio');
  }

  private campo(f: CampoFormulario, path: string): void {
    this.textos(f.label, path + '/label', 'obligatorio');
    f.options?.forEach((o, i) => this.textos(o, path + '/options' + seg(i), 'obligatorio'));
    this.rango(f.max_length, 1, 5000, path + '/max_length');
    if (f.min !== undefined && f.max !== undefined && f.min > f.max)
      this.e.add(path + '/max', 'rango');
  }

  private elementos(items: ElementoMenu[], path: string, donde: Donde): void {
    items.forEach((it, i) => {
      const ip = path + seg(i);
      this.textos(it.label, ip + '/label', 'obligatorio');
      this.enlace(it.link, ip + '/link', donde);
      if (it.children) {
        if (it.children.length > LIMITES.hijosPorElementoMenu) {
          this.e.add(ip + '/children' + seg(LIMITES.hijosPorElementoMenu), 'limite');
        }
        this.elementos(it.children, ip + '/children', donde);
      }
    });
  }

  private instancia(n: Extract<Nodo, { type: 'instance' }>, path: string, donde: Donde): void {
    if (donde.enMaestro) this.e.add(path, 'componente_anidado');
    const c = this.componentes.get(n.component);
    if (!c) this.e.add(path + '/component', 'componente_falta');
    const nodos = new Map<string, Nodo>();
    if (c)
      recorrer(c.root, (x) => {
        if (x.type !== 'section' && !nodos.has(x.id)) nodos.set(x.id, x);
      });
    for (const [k, o] of Object.entries(n.overrides ?? {})) {
      const op = path + '/overrides' + seg(k);
      const destino = nodos.get(k);
      if (c && !destino) this.e.add(op, 'override_invalido');
      else if (destino) this.override(o, destino, op);
      this.contenidoOverride(o, op, donde);
    }
  }

  private override(o: Override, destino: Nodo, path: string): void {
    const t = destino.type;
    const casa: Record<keyof Override, boolean> = {
      text: t === 'text',
      label: CON_ETIQUETA.has(t),
      image: t === 'image',
      alt: t === 'image',
      link: CON_ENLACE.has(t) && !(destino.type === 'frame' && destino.tag !== 'div'),
      icon: t === 'icon' || t === 'button',
      hidden: true,
      style: true,
    };
    for (const k of Object.keys(o) as (keyof Override)[]) {
      if (!casa[k]) this.e.add(path + seg(k), 'override_invalido');
    }
  }

  private contenidoOverride(o: Override, path: string, donde: Donde): void {
    this.ricos(o.text, path + '/text', 'opcional', donde);
    this.textos(o.label, path + '/label', 'opcional');
    this.textos(o.alt, path + '/alt', 'opcional');
    this.imagen(o.image, path + '/image');
    if (o.link) this.enlace(o.link, path + '/link', donde);
    if (o.icon !== undefined && !ICONO.has(o.icon)) this.e.add(path + '/icon', 'icono');
    this.estilo(o.style, path + '/style');
  }
}

function recorrer(n: Seccion | Nodo, fn: (n: Seccion | Nodo) => void): void {
  fn(n);
  if (n.type === 'section' || n.type === 'frame') for (const c of n.children) recorrer(c, fn);
}

function contarNodos(n: Seccion | Nodo): number {
  let total = 0;
  recorrer(n, () => total++);
  return total;
}

function algunoInteractivo(n: Seccion | Nodo, es: (n: Seccion | Nodo) => boolean): boolean {
  let si = false;
  recorrer(n, (x) => {
    if (!si && es(x)) si = true;
  });
  return si;
}

/** `https://` con host, `mailto:` o `tel:`; sin espacios ni control; ≤ `LIMITES.url`. */
export function urlValida(u: string): boolean {
  if (u.length > LIMITES.url) return false;
  for (const ch of u) {
    const c = ch.codePointAt(0) ?? 0;
    if (c <= 0x20 || (c >= 0x7f && c <= 0x9f) || c === 0x2028 || c === 0x2029) return false;
  }
  return /^https:\/\/[^/?#]+/.test(u) || /^mailto:./.test(u) || /^tel:./.test(u);
}

function refValida(p: string, ref: string): boolean {
  switch (p) {
    case 'osm': {
      const m = RE_OSM.exec(ref);
      if (!m) return false;
      const [lat, lon, zoom] = [Number(m[1]), Number(m[3]), Number(m[5])];
      return Math.abs(lat) <= 90 && Math.abs(lon) <= 180 && zoom >= 1 && zoom <= 19;
    }
    case 'cast':
      return RE_UUID.test(ref);
    case 'youtube':
      return RE_YOUTUBE.test(ref);
    case 'vimeo':
      return RE_VIMEO.test(ref);
    default:
      return false;
  }
}
