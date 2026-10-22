/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * SANEADOR DE HTML DE DOCUMENTO, para el navegador.
 *
 * El contenido de un documento de Sytue Docs ES HTML y se pinta con `innerHTML` o
 * `dangerouslySetInnerHTML`. Lo escribe una persona y lo ve OTRA —un coautor, o
 * quien canjea un enlace de edición, también de otra organización (ADR 0052)—, así
 * que pintarlo tal cual es XSS almacenado: basta un `<img onerror>` para ejecutar
 * código en la sesión de quien abre el documento.
 *
 * LAS CUATRO REGLAS, que son las mismas que aplica el servidor al guardar y al
 * exportar (`services/docs`). Si cambias una aquí, cámbiala allí:
 *
 *   1. Se ELIMINAN con su contenido los elementos que ejecutan, cargan código o
 *      cambian de significado entre dos análisis del mismo texto (`ETIQUETAS_ELIMINADAS`), y
 *      los comentarios.
 *   2. Se CONSERVAN las etiquetas de `ETIQUETAS_PERMITIDAS`: lo que produce el editor, lo que
 *      produce el navegador al editar y el HTML corriente que llega pegado.
 *      Cualquier otra se DESENVUELVE: se quita la etiqueta y se quedan sus hijos.
 *      Nunca se tira contenido por una etiqueta desconocida.
 *   3. Atributos: fuera los manejadores de evento (`on*`) y `srcdoc`. El resto se
 *      queda, porque el editor usa más de cien `data-*` y los de presentación de SVG.
 *   4. URL: lista blanca de esquemas por prefijo, distinta para lo que NAVEGA y para
 *      lo que CARGA un recurso.
 *
 * TRES COSAS QUE PARECEN PELIGROSAS Y SON DEL EDITOR. Quitarlas rompe documentos, y
 * como el folio se vuelve a guardar desde el DOM, los rompe para siempre:
 *
 *   - `<foreignObject>`: lleva el texto de las formas (`docblocks.ts`) y de SmartArt
 *     (`smartart.ts`). Se conserva, y sus hijos se sanean como los demás.
 *   - `style` con `url(data:image/…)`: es la marca de agua de imagen (`pagedecor.ts`).
 *     `style` solo se quita si lleva `javascript:`, `expression(` y compañía.
 *   - `<image href="data:image/…">`: la misma marca de agua, en SVG. Ahí `href` es un
 *     recurso, no un enlace.
 *
 * POR QUÉ SE QUITAN `xmp`, `noembed`, `plaintext`, `math` Y LAS ANIMACIONES, que no
 * ejecutan nada por sí solas. Aquí se analiza el texto, se limpia el árbol y se
 * vuelve a escribir como texto, que el navegador analiza OTRA vez al pintarlo. Hay
 * elementos cuyo contenido es marcado en un contexto y texto crudo en otro; si entre
 * los dos análisis cambian de contexto, un atributo inofensivo en el primero pasa a
 * ser un elemento en el segundo. Sin esos elementos en la salida, no hay con qué.
 * `title` se queda —es la accesibilidad de un SVG— pero aplanado a texto.
 *
 * El paso rápido (sin `<`, se devuelve igual) hace que una celda con un número o un
 * texto, que es casi toda una hoja, no llegue al analizador.
 */

/*
 * LAS LISTAS. Son las mismas que las de `services/docs/internal/domain/saneado_html.go`,
 * y `apps/docs/test/saneado-lienzos.test.ts` lo comprueba leyendo aquel fichero: si
 * cambias una aquí sin cambiarla allí, o al revés, esa prueba falla.
 */

/** Regla 1: se eliminan ENTEROS, con su contenido. */
export const ETIQUETAS_ELIMINADAS: ReadonlySet<string> = new Set(
  (
    'script style iframe object embed base meta link noscript template frame frameset ' +
    'applet portal fencedframe xmp noembed noframes plaintext math ' +
    'animate animatemotion animatetransform set'
  ).split(' '),
);

/** Regla 2: se conservan. En minúsculas: los nombres de SVG llegan con mayúsculas intercaladas. */
export const ETIQUETAS_PERMITIDAS: ReadonlySet<string> = new Set(
  // Lo que produce el editor.
  (
    'a b blockquote br del div em figure font h1 h2 h3 h4 h5 h6 hr i img ins li mark nav ol p ' +
    's section span strong sup table tbody td th tr u ul ' +
    // Lo que produce el navegador al editar, o llega pegado o importado.
    'strike sub pre code small big center thead tfoot caption colgroup col figcaption dl dt dd ' +
    'abbr cite q kbd samp var time wbr bdi bdo address article aside header footer main hgroup ' +
    'details summary label audio video source track picture ' +
    // SVG de formas, gráficos, SmartArt y dibujos.
    'svg g path rect circle ellipse line polygon polyline text tspan image defs title desc ' +
    'foreignobject clippath lineargradient radialgradient stop marker pattern mask symbol use'
  ).split(' '),
);

/** Regla 4: atributos que navegan o envían. */
export const ATRIBUTOS_DE_ENLACE: ReadonlySet<string> = new Set([
  'href',
  'xlink:href',
  'action',
  'formaction',
  'ping',
]);
/** Regla 4: atributos que cargan un recurso. */
export const ATRIBUTOS_DE_RECURSO: ReadonlySet<string> = new Set([
  'src',
  'poster',
  'background',
  'srcset',
]);
/** Elementos en los que `href` carga una imagen en vez de navegar. */
export const ELEMENTOS_DE_RECURSO: ReadonlySet<string> = new Set(['image', 'feimage']);

/** Regla 4: comienzos admitidos de una URL que navega (y, por tanto, también de un recurso). */
export const ESQUEMAS_DE_ENLACE: readonly string[] = [
  'http:',
  'https:',
  'mailto:',
  'tel:',
  'cid:',
  '/',
  './',
  '../',
  '#',
];
/** Regla 4: lo que además admite una URL que carga un recurso. */
export const ESQUEMAS_DE_RECURSO: readonly string[] = [
  'blob:',
  'data:image/',
  'data:video/',
  'data:audio/',
  'data:text/vtt',
];
/** Regla 3: un `style` se conserva entero salvo que lleve una de estas. */
export const ESTILO_VETADO: readonly string[] = [
  'javascript:',
  'expression(',
  '-moz-binding',
  'behavior:',
];

/** Lista blanca por prefijo sobre el valor recortado y en minúsculas: `javascript:`,
 *  `vbscript:`, `data:text/html` y sus variantes disfrazadas caen solas. */
function urlAdmitida(valor: string, recurso: boolean): boolean {
  const v = valor.trim().toLowerCase();
  if (v === '') return true;
  return (
    ESQUEMAS_DE_ENLACE.some((p) => v.startsWith(p)) ||
    (recurso && ESQUEMAS_DE_RECURSO.some((p) => v.startsWith(p)))
  );
}

function estiloPeligroso(valor: string): boolean {
  const v = valor.toLowerCase();
  return ESTILO_VETADO.some((x) => v.includes(x));
}

function limpiaAtributos(el: Element, etiqueta: string): void {
  for (const attr of Array.from(el.attributes)) {
    const nombre = attr.name.toLowerCase();
    let fuera = false;
    if (nombre.startsWith('on') || nombre === 'srcdoc') fuera = true;
    else if (nombre === 'style') fuera = estiloPeligroso(attr.value);
    else if (ATRIBUTOS_DE_RECURSO.has(nombre)) fuera = !urlAdmitida(attr.value, true);
    else if (ATRIBUTOS_DE_ENLACE.has(nombre))
      fuera = !urlAdmitida(attr.value, ELEMENTOS_DE_RECURSO.has(etiqueta));
    if (fuera) el.removeAttribute(attr.name);
  }
}

/** Comentarios (8) e instrucciones de proceso (7), hijos directos de `nodo`. */
function quitaComentarios(nodo: Element): void {
  for (const hijo of Array.from(nodo.childNodes)) {
    if (hijo.nodeType === 8 || hijo.nodeType === 7) hijo.remove();
  }
}

/**
 * El HTML de un documento, sin nada que ejecute al pintarlo. No cambia el formato
 * que produce el editor (lo fija `apps/docs/test/saneado-lienzos.test.ts`, que lo
 * pasa por las plantillas y por lo que hornea cada bloque).
 *
 * Sin `DOMParser` devuelve cadena vacía, que es fallar cerrado: estas superficies
 * cargan su contenido en el navegador, así que en el servidor no pasa nada real por
 * aquí, y de pasar es mejor no emitirlo que emitirlo sin sanear.
 */
export function saneaHtmlDeDocumento(
  html: string,
  opciones: {
    /** Etiquetas que esta superficie conserva además de las del contrato: las casillas
     *  (`input`) de las tareas de Notepads. Sus atributos se limpian igual. */
    conservar?: readonly string[];
  } = {},
): string {
  if (!html || !html.includes('<')) return html;
  const conservar = new Set(opciones.conservar ?? []);
  if (typeof DOMParser === 'undefined') return '';
  const doc = new DOMParser().parseFromString(html, 'text/html');
  quitaComentarios(doc.body);
  // En orden de documento: un padre se decide antes que sus hijos, y lo que cuelga
  // de un elemento eliminado ya no está conectado cuando le llega el turno.
  for (const el of Array.from(doc.body.querySelectorAll('*'))) {
    if (!el.isConnected) continue;
    const etiqueta = el.tagName.toLowerCase();
    if (ETIQUETAS_ELIMINADAS.has(etiqueta)) {
      el.remove();
      continue;
    }
    quitaComentarios(el);
    if (!ETIQUETAS_PERMITIDAS.has(etiqueta) && !conservar.has(etiqueta)) {
      el.replaceWith(...Array.from(el.childNodes));
      continue;
    }
    if (etiqueta === 'title' && el.children.length > 0) {
      const texto = el.textContent ?? '';
      el.textContent = texto;
    }
    limpiaAtributos(el, etiqueta);
  }
  return doc.body.innerHTML;
}
