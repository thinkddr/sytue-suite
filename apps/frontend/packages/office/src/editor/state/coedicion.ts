/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Co-edición por carácter: la POLÍTICA que une la secuencia RGA (./rga.ts) con
 * el transporte (./collab.ts) y el autosave (./useDocument.ts).
 *
 * El problema que resuelve: el contenido del documento vive en UN registro LWW
 * (`doc:html`), así que cuando dos personas escribían a la vez el último
 * guardado borraba lo del otro. La secuencia RGA da la fusión por carácter; lo
 * que hacía falta era decidir CUÁNDO se puede usar, porque el registro no se
 * puede abandonar: todo el backend (export, PDF, import, búsqueda) lee el
 * contenido de ahí, no de la secuencia.
 *
 * De ahí las dos reglas de las que depende todo:
 *
 *  1. CADA guardado escribe el registro LWW, como siempre. La secuencia viaja
 *     ADEMÁS, y solo manda en el cliente: al recibir, la fusión sustituye al
 *     valor del `set` ajeno.
 *
 *  2. Un `set` de contenido es AUTORITATIVO —«tira tu secuencia y quédate con
 *     mi texto»— cuando lo acompaña un registro `<clave>:lww` en el mismo lote.
 *     Sin esa marca no habría forma de distinguir «esta es mi vista, fusiónala»
 *     de «he reescrito el documento entero», y un cliente fusionando sobre un
 *     estado que el otro ya descartó es exactamente cómo se pierde texto sin
 *     que salte ningún error.
 *
 * Se manda la marca cuando el emisor NO tenía la secuencia en sincronía: en un
 * documento heredado (todo el contenido está en el registro y la secuencia está
 * vacía), al sembrar esa secuencia por primera vez, y cuando un cambio es tan
 * grande que sus ops no caben en el cuerpo que acepta el servicio. En régimen
 * normal —todos en sincronía— no se manda nada y la secuencia decide.
 *
 * Lo que NO se cubre a propósito:
 *  - Solo `kind === 'document'`. Hojas y presentaciones guardan
 *    `JSON.stringify(modelo)`: fusionar dos JSON carácter a carácter da un JSON
 *    que no parsea, y eso es peor que perder una edición. Se quedan en LWW.
 *  - La siembra de un documento heredado solo se hace A SOLAS (sin peers
 *    presentes). Si dos clientes sembraran a la vez, el documento entero se
 *    insertaría dos veces. La marca autoritativa deshace ese empate si la
 *    presencia llegara tarde, pero es red de seguridad, no el mecanismo.
 */
import { SecuenciaRGA, type OpRGA, type Punto } from './rga';
import { indiceHtmlDeTexto, longitudVisible, offsetDeIndiceHtml } from './textohtml';

/** Clave del registro que marca un `set` de contenido como autoritativo. */
export const claveLww = (claveContenido: string): string => `${claveContenido}:lww`;

/**
 * Tope del cuerpo de POST /api/ops. El servicio corta el JSON en 1 MiB
 * (`platform/pkg/httpx/respond.go`, `MaxBytesReader`); se deja margen porque el
 * mismo cuerpo lleva también el registro LWW con el documento entero.
 */
export const MAX_CUERPO_BYTES = 900_000;

/** true si el lote cabe en una sola petición (se mide en BYTES, no en chars). */
export function cabeEnUnLote(cuerpo: string): boolean {
  return new TextEncoder().encode(cuerpo).length <= MAX_CUERPO_BYTES;
}

/** Elemento de la secuencia tal y como lo serializa el servicio Go. */
interface ElementoEstado {
  dot: Punto;
  after?: Punto;
  value: string;
  deleted?: boolean;
}

function esPunto(v: unknown): v is Punto {
  const p = v as Punto | null;
  return typeof p === 'object' && p !== null && typeof p.a === 'string' && Number.isFinite(p.c);
}

/**
 * Frontera de confianza: `GET /api/state` devuelve `elements` como `unknown[]`
 * y sus valores son JSON libre (un elemento de presentación no es un carácter).
 * Se descarta todo lo que no sea un carácter con identidad válida en vez de
 * dejar que la secuencia reviente al recorrerlo.
 */
export function elementosDeEstado(raw: unknown): ElementoEstado[] {
  if (!Array.isArray(raw)) return [];
  const out: ElementoEstado[] = [];
  for (const item of raw) {
    if (typeof item !== 'object' || item === null) continue;
    const e = item as Record<string, unknown>;
    if (!esPunto(e.dot) || typeof e.value !== 'string' || e.value === '') continue;
    // `after` se omite (no se pone a undefined): exactOptionalPropertyTypes.
    out.push({
      dot: e.dot,
      ...(esPunto(e.after) ? { after: e.after } : {}),
      value: e.value,
      deleted: e.deleted === true,
    });
  }
  return out;
}

/** Op de secuencia ajena, ya validada (lo que llega del WS es JSON libre). */
export function opDeSecuencia(raw: unknown): OpRGA | null {
  if (typeof raw !== 'object' || raw === null) return null;
  const o = raw as Record<string, unknown>;
  if (!esPunto(o.dot)) return null;
  // UN carácter (un punto de código, lo que manda `insertarLocal`): un insert de
  // varios rompía el orden de la secuencia y los coautores divergían (FYE-05).
  if (o.kind === 'insert' && typeof o.value === 'string' && Array.from(o.value).length === 1) {
    return {
      kind: 'insert',
      dot: o.dot,
      ...(esPunto(o.after) ? { after: o.after } : {}),
      value: o.value,
      lamport: typeof o.lamport === 'number' ? o.lamport : 0,
    };
  }
  if (o.kind === 'delete' && esPunto(o.target)) {
    return {
      kind: 'delete',
      dot: o.dot,
      target: o.target,
      lamport: typeof o.lamport === 'number' ? o.lamport : 0,
    };
  }
  return null;
}

export class CoEdicion {
  private readonly seq: SecuenciaRGA;
  /** true si la secuencia representa fielmente el texto que ve el usuario. */
  private sincro: boolean;
  /** Ops ajenas recibidas y aún no volcadas al editor (llegan mientras se escribe). */
  private buffer: OpRGA[] = [];

  constructor(
    private readonly actor: string,
    elementos: unknown,
    textoInicial: string,
  ) {
    this.seq = SecuenciaRGA.desdeElementos(actor, elementosDeEstado(elementos));
    // Un documento anterior a esto tiene el texto en el registro y la secuencia
    // vacía: hasta que se siembre, el canal por carácter no puede usarse.
    this.sincro = this.seq.texto() === textoInicial;
  }

  /** true si el canal por carácter está operativo (si no: LWW de toda la vida). */
  get sincronizada(): boolean {
    return this.sincro;
  }

  /** Contador del actor local; se comparte con el dot de los `set` del registro. */
  get contador(): number {
    return this.seq.ultimoContador;
  }

  fijarContador(n: number): void {
    this.seq.fijarContador(n);
  }

  /**
   * Operaciones por carácter para llegar a `texto`. Devuelve [] cuando toca
   * modo LWW: documento heredado con alguien más presente (sembrarlo a dos
   * duplicaría el contenido) o secuencia anulada por un lote autoritativo.
   *
   * Sembrar es el mismo diff que editar: `opsDesdeTexto` compara prefijo y
   * sufijo, así que reengancharse tras un cambio grande cuesta solo el tramo
   * que cambió, no el documento entero.
   */
  opsLocales(texto: string, lamportBase: number, aSolas: boolean): OpRGA[] {
    if (!this.sincro && !aSolas) return [];
    const ops = this.seq.opsDesdeTexto(texto, lamportBase);
    this.sincro = true;
    return ops;
  }

  /**
   * Guarda ops ajenas de secuencia. No se aplican todavía: mientras el usuario
   * escribe, el editor y la secuencia tienen que seguir diciendo lo mismo o el
   * siguiente diff local borraría lo que acaba de llegar del otro.
   */
  recibir(ops: OpRGA[]): void {
    if (!this.sincro) return; // en modo LWW el contenido llega por el registro
    for (const op of ops) {
      if (op.dot.a === this.actor) continue; // eco de lo propio: ya está aplicado
      this.buffer.push(op);
    }
  }

  /**
   * Aplica lo recibido y devuelve el texto fusionado, o null si no hay nada que
   * fusionar. Llamar solo cuando no hay edición local en vuelo.
   */
  fusionar(): string | null {
    if (!this.sincro || this.buffer.length === 0) return null;
    for (const op of this.buffer) {
      // El contador local sube con lo ajeno (regla RGA: lo insertado después va
      // primero entre hermanos). No cambia el orden, mejora el entrelazado.
      if (op.kind === 'insert') this.seq.fijarContador(op.dot.c);
      this.seq.aplicarRemota(op);
    }
    this.buffer = [];
    return this.seq.texto();
  }

  /** El HTML de la secuencia, si dice lo mismo que el folio (misma cantidad de texto visible):
   *  solo entonces un offset del folio es un offset de la secuencia. */
  private htmlSiCoincide(longitudFolio: number): string | null {
    if (!this.sincro) return null;
    const html = this.seq.texto();
    return longitudVisible(html) === longitudFolio ? html : null;
  }

  /**
   * Ancla un cursor (offsets de texto del folio) a la IDENTIDAD de los caracteres que
   * tiene detrás, para que otra persona lo coloque bien aunque haya ediciones entre medias.
   * null si la secuencia no coincide con el folio (se acaba de teclear, o no está en
   * sincronía): quien llama manda solo los offsets.
   */
  puntosDeOffsets(
    longitudFolio: number,
    off: { anchor: number; focus: number },
  ): { anchorDot: Punto; focusDot: Punto } | null {
    const html = this.htmlSiCoincide(longitudFolio);
    if (html === null) return null;
    const ia = indiceHtmlDeTexto(html, off.anchor);
    const ifo = indiceHtmlDeTexto(html, off.focus);
    if (ia === null || ifo === null) return null;
    const anchorDot = this.seq.puntoEn(ia);
    const focusDot = this.seq.puntoEn(ifo);
    return anchorDot && focusDot ? { anchorDot, focusDot } : null;
  }

  /** Lo inverso: dónde caen en el folio de AHORA los caracteres a los que un par anclaba su
   *  cursor. null si aún no conocemos alguno (su inserción no ha llegado) o la secuencia no
   *  coincide con el folio: se usan entonces los offsets que acompañaban. */
  offsetsDePuntos(
    longitudFolio: number,
    dots: { anchorDot: Punto; focusDot: Punto },
  ): { anchor: number; focus: number } | null {
    const html = this.htmlSiCoincide(longitudFolio);
    if (html === null) return null;
    const ia = this.seq.indiceDe(dots.anchorDot);
    const ifo = this.seq.indiceDe(dots.focusDot);
    if (ia === null || ifo === null) return null;
    return { anchor: offsetDeIndiceHtml(html, ia), focus: offsetDeIndiceHtml(html, ifo) };
  }

  /**
   * Un par ha declarado su `set` autoritativo: nuestra secuencia describe un
   * estado que ya no existe. Se abandona el canal por carácter y manda el
   * registro LWW; al recargar, la secuencia se vuelve a evaluar desde cero.
   */
  anular(): void {
    this.sincro = false;
    this.buffer = [];
  }
}
