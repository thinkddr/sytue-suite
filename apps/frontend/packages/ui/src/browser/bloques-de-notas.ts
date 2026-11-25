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
 *   tabla       <table [data-numerada]>, <thead> de <th>, <th scope="row">, data-fondo en td/th
 *   columnas    <div data-columnas="2|3"><div data-columna>…</div>…</div>
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
  /** Etiquetas que solo entran con los bloques (las demás ya están en la lista común). */
  etiquetas: ['details', 'summary', 'time', 'sub', 'sup'] as readonly string[],
  coloresDeBloque: COLORES_DE_BLOQUE,
} as const;
