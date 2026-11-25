/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */

/*
 * La lista de los bloques del editor de Notes, aparte del saneador del pegado
 * (`portapapeles.ts`): ese va sobre parse5 (~225 KB con entities) y Notes lo baja
 * bajo demanda, pero la lista la necesita al cargar (los colores, los paneles y los
 * lenguajes de la barra de bloque). `portapapeles.ts` la reexporta.
 */

const COLORES_DE_BLOQUE = ['gris', 'azul', 'verde', 'amarillo', 'rojo', 'morado'] as const;

/**
 * LOS BLOQUES DEL EDITOR DE NOTAS: el HTML que guardan la web y Fyne. Es la misma lista
 * que la del saneador del servicio (`apps/nucleo/notes/model/html.go`), y la prueba
 * `bloques-saneado.test.ts` de Notes compara las dos.
 *
 *   panel       <div data-panel="info|nota|exito|aviso|error">…</div>
 *   expandir    <details data-expandir [open]><summary>Título</summary>…</details>
 *   estado      <span data-estado="gris|azul|verde|amarillo|rojo|morado">TEXTO</span>
 *   decisiones  <ul data-tipo="decisiones"><li>…</li></ul>
 *   fecha       <time datetime="AAAA-MM-DD">texto</time>
 *   código      <pre [data-lineas] [data-ajuste]><code data-lenguaje="go|javascript|…">…</code></pre>
 *   tabla       <table [data-numerada]>, <thead> de <th>, <th scope="row">, data-fondo en td/th,
 *               colspan="2…20" y rowspan="2…100" en td/th (celdas combinadas)
 *   columnas    <div data-columnas="2|3"><div data-columna>…</div>…</div>
 *   imagen      <img src="adjunto:ID" alt="…">, y fichero <a href="adjunto:ID">nombre</a>
 *
 * Las imágenes y los ficheros del texto son ADJUNTOS de la nota (`data.attachments`) y
 * el cuerpo los nombra por su id (`adjunto:`): el saneador del servicio no deja en un
 * `src` ninguna dirección, ni de fuera ni propia (ver `apps/nucleo/notes/model/html.go`).
 */
export const BLOQUES_DE_NOTAS = {
  /** Etiqueta → atributo → valores que valen. */
  enumerados: {
    div: { 'data-panel': ['info', 'nota', 'exito', 'aviso', 'error'], 'data-columnas': ['2', '3'] },
    span: { 'data-estado': COLORES_DE_BLOQUE },
    ul: { 'data-tipo': ['decisiones'] },
    code: {
      // La lista del editor de Fyne, con su misma grafía.
      'data-lenguaje': [
        'texto',
        'bash',
        'c',
        'cpp',
        'csharp',
        'css',
        'go',
        'html',
        'java',
        'javascript',
        'json',
        'kotlin',
        'markdown',
        'php',
        'python',
        'ruby',
        'rust',
        'sql',
        'swift',
        'typescript',
        'xml',
        'yaml',
      ],
    },
    th: { scope: ['row'], 'data-fondo': COLORES_DE_BLOQUE },
    td: { 'data-fondo': COLORES_DE_BLOQUE },
  } as Readonly<Record<string, Readonly<Record<string, readonly string[]>>>>,
  /** Etiqueta → atributos que van sin valor. */
  sinValor: {
    details: ['data-expandir', 'open'],
    div: ['data-columna'],
    pre: ['data-lineas', 'data-ajuste'],
    table: ['data-numerada'],
  } as Readonly<Record<string, readonly string[]>>,
  /**
   * Etiqueta → atributo → el número más alto que vale: de 2 al tope, en decimal y sin
   * ceros delante (un 1 es no ponerlo). Las celdas combinadas.
   */
  numericos: {
    td: { colspan: 20, rowspan: 100 },
    th: { colspan: 20, rowspan: 100 },
  } as Readonly<Record<string, Readonly<Record<string, number>>>>,
  /** Etiquetas que solo entran con los bloques (las demás ya están en la lista común). */
  etiquetas: ['details', 'summary', 'time', 'sub', 'sup'] as readonly string[],
  coloresDeBloque: COLORES_DE_BLOQUE,
} as const;

/** El prefijo con que el cuerpo de una nota nombra uno de sus adjuntos. */
export const ESQUEMA_ADJUNTO = 'adjunto:';

/** Un `src` o un `href` que nombra un adjunto de la nota: `adjunto:` y su id. */
export function idDeAdjunto(ref: string | null | undefined): string | null {
  const m = /^adjunto:([A-Za-z0-9_-]{1,64})$/.exec(ref ?? '');
  return m ? m[1]! : null;
}

/** Un número de bloque (`colspan`, `rowspan`) que vale en esa etiqueta, o null. */
export function numeroDeBloque(etiqueta: string, nombre: string, valor: string): string | null {
  const tope = BLOQUES_DE_NOTAS.numericos[etiqueta]?.[nombre];
  if (tope === undefined || !/^[1-9]\d*$/.test(valor)) return null;
  const n = Number(valor);
  return n >= 2 && n <= tope ? valor : null;
}
