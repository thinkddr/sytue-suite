/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Atajos de Markdown al escribir: «- », «1. », «# », «> » y «``` » al principio
 * de una línea se convierten en la lista, el título, la cita o el bloque de
 * código que nombran, y «---» seguido de Intro en una regla. Lo que hacen
 * Notion o Google Docs, con las MISMAS órdenes que pulsa la barra del editor:
 * nada nuevo que deshacer ni que sanear.
 *
 * Aquí solo vive la decisión (qué marcador dispara qué orden); el DOM lo toca
 * RichText, que además comprueba que el marcador abre la línea.
 */
import { htmlATexto, pegadoConFormato, textoPegado } from '@sytue/ui/browser/portapapeles';
import { escapar, esURLWeb } from './captura.js';

export interface AtajoMarkdown {
  /** Orden de edición, la misma que aplica la barra. */
  cmd: string;
  valor?: string;
  /** Caracteres del marcador que hay que borrar antes de aplicar la orden. */
  borrar: number;
}

const POR_ESPACIO: [RegExp, (m: RegExpMatchArray) => AtajoMarkdown][] = [
  [/^[-*+]$/, () => ({ cmd: 'insertUnorderedList', borrar: 1 })],
  [/^1[.)]$/, () => ({ cmd: 'insertOrderedList', borrar: 2 })],
  [
    /^(#{1,5})$/,
    (m) => ({ cmd: 'formatBlock', valor: `<h${m[1]!.length}>`, borrar: m[1]!.length }),
  ],
  [/^>$/, () => ({ cmd: 'formatBlock', valor: '<blockquote>', borrar: 1 })],
  [/^```$/, () => ({ cmd: 'formatBlock', valor: '<pre>', borrar: 3 })],
  [
    /^(\[\]|\[\s?\]|[-*]\s*\[\s?\])$/,
    (m) => ({
      cmd: 'insertHTML',
      valor: '<div class="task-row"><input type="checkbox" /> <span>&nbsp;</span></div>',
      borrar: m[0].length,
    }),
  ],
];

/**
 * Qué dispara `tecla` cuando `linea` es lo que hay antes del cursor en su
 * línea. `null` = nada: la tecla sigue su curso.
 */
export function atajoMarkdown(linea: string, tecla: ' ' | 'Enter'): AtajoMarkdown | null {
  if (tecla === 'Enter') {
    return /^(-{3,}|\*{3,})$/.test(linea)
      ? { cmd: 'insertHorizontalRule', borrar: linea.length }
      : null;
  }
  for (const [re, hacer] of POR_ESPACIO) {
    const m = linea.match(re);
    if (m) return hacer(m);
  }
  return null;
}

/*
 * PEGAR MARKDOWN. Lo que se pega de un `.md`, de Obsidian o de la respuesta de un
 * asistente llega como texto plano lleno de marcas; Bear y Notion lo convierten y
 * aquí también, pero solo un subconjunto que el editor sabe pintar: títulos, listas,
 * tareas, citas, código, regla, negrita, cursiva, tachado y enlaces http(s). Nada de
 * HTML crudo dentro del Markdown: todo el texto sale escapado, y el resultado pasa
 * además por el saneado del pegado (`pegadoConFormato`) como cualquier HTML ajeno.
 *
 * Es el MISMO algoritmo que `markdownAHTML` en `apps/fyne/apps/notes/markdown.go`, y
 * los dos leen los mismos casos de `apps/fyne/apps/notes/testdata/markdown.json`: si
 * uno cambia, el otro también.
 */

const MD_TITULO = /^(#{1,6})\s+(.*)$/;
const MD_TAREA = /^(?:[-*+]\s+)?\[([ xX]?)\](?:\s+(.*))?$/;
const MD_VINETA = /^[-*+]\s+(.*)$/;
const MD_NUMERO = /^\d{1,9}[.)]\s+(.*)$/;
const MD_CITA = /^>\s?(.*)$/;
const MD_REGLA = /^(-{3,}|\*{3,})$/;
const MD_VALLA = /^```/;
const MD_BLOQUE = [MD_TITULO, MD_TAREA, MD_VINETA, MD_NUMERO, MD_CITA, MD_REGLA, MD_VALLA];

/** Cuántas marcas de bloque hacen falta para tratar un pegado como Markdown. */
const MARCAS_MINIMAS = 2;

function lineas(texto: string): string[] {
  return texto.replace(/\r\n?/g, '\n').split('\n');
}

/**
 * Si el texto plano «parece Markdown»: al menos dos líneas con marca de bloque. Una
 * frase con un asterisco o un «#etiqueta» suelto no lo es, y entra tal cual.
 */
export function pareceMarkdown(texto: string): boolean {
  let marcas = 0;
  for (const l of lineas(texto)) {
    const t = l.trim();
    if (t && MD_BLOQUE.some((re) => re.test(t)) && ++marcas >= MARCAS_MINIMAS) return true;
  }
  return false;
}

function escaparMd(s: string): string {
  return s
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

const ES_PALABRA = /[\p{L}\p{N}]/u;
const ES_BLANCO = /\s/;

/** Dónde cierra la marca `d` abierta en `i`, o -1 si no es una marca. */
function cierre(s: string, i: number, d: string): number {
  const tras = s[i + d.length];
  if (tras === undefined || ES_BLANCO.test(tras)) return -1;
  // «snake_case» no es cursiva: el guion bajo solo marca fuera de una palabra.
  const bajo = d[0] === '_';
  if (bajo && i > 0 && ES_PALABRA.test(s[i - 1]!)) return -1;
  for (let j = s.indexOf(d, i + d.length + 1); j !== -1; j = s.indexOf(d, j + 1)) {
    if (ES_BLANCO.test(s[j - 1]!)) continue;
    if (d.length === 1 && (s[j - 1] === d || s[j + 1] === d)) continue;
    if (bajo && s[j + d.length] !== undefined && ES_PALABRA.test(s[j + d.length]!)) continue;
    return j;
  }
  return -1;
}

const MARCAS_EN_LINEA: [string, string][] = [
  ['**', 'b'],
  ['__', 'b'],
  ['~~', 's'],
  ['*', 'i'],
  ['_', 'i'],
];
const MD_ENLACE = /^\[([^\]]+)\]\((https?:\/\/[^\s)]+)\)/;

function enLinea(s: string): string {
  let out = '';
  let i = 0;
  sigue: while (i < s.length) {
    const c = s[i]!;
    if (c === '`') {
      const j = s.indexOf('`', i + 1);
      if (j > i + 1) {
        out += `<code>${escaparMd(s.slice(i + 1, j))}</code>`;
        i = j + 1;
        continue;
      }
    }
    if (c === '[') {
      const m = MD_ENLACE.exec(s.slice(i));
      if (m) {
        out += `<a href="${escaparMd(m[2]!)}">${enLinea(m[1]!)}</a>`;
        i += m[0].length;
        continue;
      }
    }
    for (const [d, etiqueta] of MARCAS_EN_LINEA) {
      if (!s.startsWith(d, i)) continue;
      const j = cierre(s, i, d);
      if (j === -1) continue;
      out += `<${etiqueta}>${enLinea(s.slice(i + d.length, j))}</${etiqueta}>`;
      i = j + d.length;
      continue sigue;
    }
    out += escaparMd(c);
    i++;
  }
  return out;
}

/**
 * Markdown → el HTML del editor de notas (el mismo vocabulario que guarda el
 * servicio). Una línea suelta es un párrafo; las líneas en blanco solo separan.
 */
export function markdownAHtml(texto: string): string {
  const out: string[] = [];
  let lista: { tipo: 'ul' | 'ol'; items: string[] } | null = null;
  let cita: string[] | null = null;
  let codigo: string[] | null = null;
  const cerrar = () => {
    if (lista) out.push(`<${lista.tipo}>${lista.items.join('')}</${lista.tipo}>`);
    if (cita) out.push(`<blockquote>${cita.join('<br>')}</blockquote>`);
    lista = null;
    cita = null;
  };
  const enLista = (tipo: 'ul' | 'ol', contenido: string) => {
    if (lista?.tipo !== tipo) {
      cerrar();
      lista = { tipo, items: [] };
    }
    lista.items.push(`<li>${enLinea(contenido)}</li>`);
  };
  const cerrarCodigo = (c: string[]) => out.push(`<pre>${c.map(escaparMd).join('<br>')}</pre>`);
  for (const linea of lineas(texto)) {
    const t = linea.trim();
    if (codigo) {
      if (MD_VALLA.test(t)) {
        cerrarCodigo(codigo);
        codigo = null;
      } else codigo.push(linea);
      continue;
    }
    let m: RegExpMatchArray | null;
    if (MD_VALLA.test(t)) {
      cerrar();
      codigo = [];
    } else if (!t) {
      cerrar();
    } else if (MD_REGLA.test(t)) {
      cerrar();
      out.push('<hr>');
    } else if ((m = t.match(MD_TITULO))) {
      cerrar();
      const n = Math.min(m[1]!.length, 5);
      out.push(`<h${n}>${enLinea(m[2]!)}</h${n}>`);
    } else if ((m = t.match(MD_TAREA))) {
      cerrar();
      const marcada = m[1] === 'x' || m[1] === 'X' ? ' checked' : '';
      out.push(
        `<div class="task-row"><input type="checkbox"${marcada}> <span>${enLinea(m[2] ?? '')}</span></div>`,
      );
    } else if ((m = t.match(MD_VINETA))) {
      enLista('ul', m[1]!);
    } else if ((m = t.match(MD_NUMERO))) {
      enLista('ol', m[1]!);
    } else if ((m = t.match(MD_CITA))) {
      if (!cita) {
        cerrar();
        cita = [];
      }
      cita.push(enLinea(m[1]!));
    } else {
      cerrar();
      out.push(`<p>${enLinea(t)}</p>`);
    }
  }
  if (codigo) cerrarCodigo(codigo);
  cerrar();
  return out.join('');
}

/** Lo que entra en el editor al pegar: HTML ya saneado, o texto plano. */
export type Pegado = { html: string } | { texto: string };

/** Lo que el editor de notas sabe pintar además de la lista común: sus tareas y sus bloques. */
const DE_NOTAS = { tareas: true, bloques: true } as const;

/**
 * Una URL sola, la que se pega desde la barra de direcciones: entra como enlace, como en
 * Confluence o en Notion. Con texto seleccionado, ese texto pasa a ser el enlace
 * (`seleccion`). El enlace sale de `pegadoConFormato`, como todo lo demás.
 */
function enlacePegado(texto: string, seleccion: string): string | null {
  const url = texto.trim();
  if (/\s/.test(url) || !esURLWeb(url)) return null;
  const rotulo = seleccion.trim() || url;
  return pegadoConFormato(`<a href="${escapar(url)}">${escapar(rotulo)}</a>`) || null;
}

/**
 * Qué se pega. Con `sinFormato` (Ctrl/⌘+Mayús+V), el texto plano tal cual. Si no:
 * el Markdown, si el texto lo parece y el HTML que lo acompaña (si lo hay) es solo
 * ese mismo fuente —el de un editor de código, que colorea el `.md` pero no lo
 * interpreta—; después, el HTML del origen; y si no hay, el texto. Todo HTML sale de
 * `pegadoConFormato`: lo generado aquí tampoco se mete sin sanear.
 *
 * Con `conMarkdown` a falso (el cursor está en un bloque de código) el Markdown no se
 * interpreta: un trozo de `.md` pegado en un `<pre>` es código, como al teclear.
 */
export function queSePega(
  datos: Pick<DataTransfer, 'getData'> | null | undefined,
  sinFormato = false,
  conMarkdown = true,
  seleccion = '',
): Pegado {
  const texto = textoPegado(datos);
  if (sinFormato) return { texto };
  const html = datos?.getData('text/html') ?? '';
  // La URL sola (o el `<a>` que la envuelve, que es lo que copian algunos navegadores).
  if (conMarkdown && (!html || htmlATexto(html).trim() === texto.trim())) {
    const enlace = enlacePegado(texto, seleccion);
    if (enlace) return { html: enlace };
  }
  const esMarkdown = conMarkdown && pareceMarkdown(texto);
  if (esMarkdown && (!html || pareceMarkdown(htmlATexto(html)))) {
    const md = pegadoConFormato(markdownAHtml(texto), DE_NOTAS);
    if (md) return { html: md };
  }
  // El `.md` coloreado de un editor de código, dentro de un bloque de código, es texto.
  if (!conMarkdown && html && pareceMarkdown(texto) && pareceMarkdown(htmlATexto(html)))
    return { texto };
  // Lo copiado de otra nota conserva sus tareas y sus bloques, con sus listas cerradas.
  const rico = pegadoConFormato(html, DE_NOTAS);
  return rico ? { html: rico } : { texto };
}
