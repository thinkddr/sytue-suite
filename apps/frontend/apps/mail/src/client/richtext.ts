/**
 * Utilidades de rich-text del compositor de Sytue Mail (solo navegador). El
 * cuerpo se edita en un contenteditable y se envía como text/html; aquí se
 * SANEA (lista blanca de etiquetas/atributos, sin scripts/estilos/URLs
 * peligrosas) tanto al PEGAR (protege el DOM del propio compositor de un XSS por
 * pegado) como al ENVIAR (salida limpia). El lector ya aísla el HTML entrante en
 * un iframe sandbox; esto cubre la composición.
 */

/** Etiquetas permitidas en el cuerpo compuesto. */
const ALLOWED_TAGS = new Set([
  'p',
  'br',
  'div',
  'span',
  'b',
  'strong',
  'i',
  'em',
  'u',
  's',
  'strike',
  'a',
  'ul',
  'ol',
  'li',
  'blockquote',
  'h1',
  'h2',
  'h3',
  'h4',
  'h5',
  'h6',
  'pre',
  'code',
  'hr',
  // Tablas: son la mitad de los correos de trabajo —un presupuesto, un turno, un
  // resumen semanal— y hasta ahora el saneador las desenvolvía, así que quien
  // pegaba una tabla de Excel enviaba una lista de palabras sueltas sin saberlo.
  'table',
  'thead',
  'tbody',
  'tfoot',
  'tr',
  'td',
  'th',
  'caption',
  'colgroup',
  'col',
  // Imágenes: solo por URL https o `cid:` (adjunto embebido). Sin esto no hay
  // firma con logotipo ni captura pegada, que es de lo primero que se echa en
  // falta viniendo de Outlook.
  'img',
  'figure',
  'figcaption',
  // Sub/superíndice: fórmulas y notas al pie, y no los suple ningún otro.
  'sub',
  'sup',
]);

/**
 * Propiedades CSS admitidas en un atributo `style`.
 *
 * El correo se maqueta con estilos EN LÍNEA —no hay hoja de estilos que valga en
 * Gmail ni en Outlook—, así que sin `style` no hay color, ni alineación, ni
 * tipografía, ni bordes de tabla. Pero un `style` a pelo es una superficie de
 * ataque de verdad, y por eso esto es una lista blanca de PROPIEDADES y no un
 * «quita lo peligroso»:
 *
 *   - `url(...)` se rechaza SIEMPRE, en cualquier propiedad: es la vía de
 *     `background:url(http://…)` para confirmar que un correo se ha abierto —y de
 *     paso filtrar la IP— sin que haya ninguna imagen a la vista.
 *   - `position`, `z-index` y compañía no están: sirven para tapar el resto del
 *     mensaje con contenido superpuesto, que es un phishing bastante barato.
 *   - `expression(`, `behavior:` y `@import` mueren con la comprobación de `url(`
 *     y con la lista: ninguna propiedad que los admita está permitida.
 */
const ALLOWED_STYLE_PROPS = new Set([
  'color',
  'background-color',
  'font-family',
  'font-size',
  'font-style',
  'font-weight',
  'text-align',
  'text-decoration',
  'vertical-align',
  'line-height',
  'letter-spacing',
  'border',
  'border-top',
  'border-right',
  'border-bottom',
  'border-left',
  'border-color',
  'border-style',
  'border-width',
  'border-collapse',
  'border-radius',
  'padding',
  'padding-top',
  'padding-right',
  'padding-bottom',
  'padding-left',
  'margin',
  'margin-top',
  'margin-right',
  'margin-bottom',
  'margin-left',
  'width',
  'height',
  'max-width',
  'min-width',
  'white-space',
]);

/** Atributos de presentación admitidos por etiqueta (los clásicos del correo). */
const ALLOWED_ATTRS: Record<string, Set<string>> = {
  img: new Set(['src', 'alt', 'width', 'height', 'title']),
  table: new Set(['border', 'cellpadding', 'cellspacing', 'width', 'align']),
  td: new Set(['colspan', 'rowspan', 'align', 'valign', 'width', 'height']),
  th: new Set(['colspan', 'rowspan', 'align', 'valign', 'width', 'height', 'scope']),
  tr: new Set(['align', 'valign']),
  col: new Set(['span', 'width']),
  colgroup: new Set(['span', 'width']),
};

/**
 * Filtra un `style` a la lista blanca. Devuelve '' si no queda nada.
 *
 * Se parte por `;` y se compara la propiedad EXACTA: nada de comprobar si el
 * texto «contiene» algo peligroso, que es como se cuela lo que no se conoce.
 */
function cleanStyle(raw: string): string {
  const out: string[] = [];
  for (const decl of raw.split(';')) {
    const i = decl.indexOf(':');
    if (i < 0) continue;
    const prop = decl.slice(0, i).trim().toLowerCase();
    const value = decl.slice(i + 1).trim();
    if (!ALLOWED_STYLE_PROPS.has(prop) || value === '') continue;
    // Ni `url(`, ni cualquier cosa que huela a script o a expresión dinámica.
    const v = value.toLowerCase();
    if (v.includes('url(') || v.includes('expression') || v.includes('javascript:')) continue;
    if (v.includes('/*') || v.includes('\\')) continue; // comentarios y escapes: fuera
    out.push(`${prop}: ${value}`);
  }
  return out.join('; ');
}

/**
 * ¿Es admisible el `src` de una imagen? `https:` y `cid:` (adjunto embebido).
 *
 * `http:` queda fuera a propósito: una imagen en claro dentro de un correo
 * cifrado en tránsito delata al destinatario ante cualquiera que mire la red.
 * `data:` también: infla el mensaje y es el vector clásico para colar SVG con
 * script disfrazado de imagen.
 */
function safeImageSrc(raw: string): boolean {
  const v = raw.trim().toLowerCase();
  return v.startsWith('https://') || v.startsWith('cid:');
}

/** Etiquetas que se eliminan CON su contenido (nunca deben viajar en un correo). */
const DROP_WITH_CONTENT = new Set([
  'script',
  'style',
  'iframe',
  'object',
  'embed',
  'noscript',
  'template',
  'link',
  'meta',
  'base',
  'title',
  'head',
  'svg',
  'math',
  'form',
  'input',
  'button',
  'textarea',
  'select',
  'option',
]);

/** Solo se permiten enlaces http(s)/mailto; corta javascript:, data:, etc. */
function safeUrl(raw: string): boolean {
  const v = raw.trim().toLowerCase();
  if (v.startsWith('http://') || v.startsWith('https://') || v.startsWith('mailto:')) return true;
  // Rutas relativas / anclas también son inofensivas.
  return v.startsWith('/') || v.startsWith('#');
}

/** Reconstruye recursivamente un subárbol saneado dentro de `out`. */
function cleanInto(src: Node, out: Node, doc: Document): void {
  for (const child of Array.from(src.childNodes)) {
    if (child.nodeType === 3 /* text */) {
      out.appendChild(doc.createTextNode(child.textContent ?? ''));
      continue;
    }
    if (child.nodeType !== 1 /* element */) continue; // comentarios/otros: fuera
    const el = child as Element;
    const tag = el.tagName.toLowerCase();
    if (DROP_WITH_CONTENT.has(tag)) continue;
    if (!ALLOWED_TAGS.has(tag)) {
      // Etiqueta desconocida: se desenvuelve (se conservan sus hijos saneados).
      cleanInto(el, out, doc);
      continue;
    }
    const clean = doc.createElement(tag);
    if (tag === 'a') {
      const href = el.getAttribute('href') ?? '';
      if (safeUrl(href)) {
        clean.setAttribute('href', href);
        clean.setAttribute('rel', 'noopener noreferrer');
      }
    }
    if (tag === 'img') {
      const src = el.getAttribute('src') ?? '';
      // Una imagen sin `src` admisible no se deja vacía: se descarta entera, o
      // quedaría un icono de imagen rota en mitad del mensaje.
      if (!safeImageSrc(src)) continue;
      clean.setAttribute('src', src);
    }
    for (const attr of ALLOWED_ATTRS[tag] ?? []) {
      if (attr === 'src') continue; // ya tratado arriba
      const v = el.getAttribute(attr);
      if (v !== null && !v.toLowerCase().includes('javascript:')) clean.setAttribute(attr, v);
    }
    const estilo = cleanStyle(el.getAttribute('style') ?? '');
    if (estilo) clean.setAttribute('style', estilo);
    cleanInto(el, clean, doc);
    out.appendChild(clean);
  }
}

/**
 * Tipos de imagen `data:` que entran al PEGAR, con la extensión de su fichero: los de
 * mapa de bits que pinta cualquier cliente. Un SVG no, que es por donde se cuela un
 * script disfrazado de imagen.
 */
const TIPOS_EN_LINEA: Record<string, string> = {
  'image/png': 'png',
  'image/jpeg': 'jpg',
  'image/gif': 'gif',
  'image/webp': 'webp',
};

/** La imagen de una dirección `data:<tipo>;base64,…` de un tipo que entra, como fichero. */
export function ficheroDeDatos(src: string, numero: number): File | null {
  const m = /^data:([a-z]+\/[a-z0-9.+-]+);base64,(.*)$/is.exec(src.trim());
  const tipo = m?.[1]?.toLowerCase() ?? '';
  const ext = TIPOS_EN_LINEA[tipo];
  if (!m || !ext) return null;
  let binario: string;
  try {
    binario = atob((m[2] ?? '').replace(/\s+/g, ''));
  } catch {
    return null; // base64 roto: no es una imagen
  }
  if (binario === '') return null;
  const bytes = Uint8Array.from(binario, (c) => c.charCodeAt(0));
  return new File([bytes], `image${String(numero).padStart(3, '0')}.${ext}`, { type: tipo });
}

/**
 * Las imágenes `data:` de un HTML pegado pasan a ser partes del mensaje: `incrustar` es
 * el `onInsertarImagen` del compositor, que las adjunta y devuelve su `cid`, y su `src`
 * pasa a `cid:…`, que el saneado sí deja pasar. Es lo que copia un navegador de una
 * imagen suelta, o un editor de escritorio. La que no se incrusta se queda como estaba
 * (y el saneado la tira), igual que las de dentro de lo que se va con su contenido. Sin
 * ninguna, el HTML vuelve tal cual.
 */
export async function incrustarImagenesPegadas(
  html: string,
  incrustar: (fichero: File) => Promise<string | null>,
): Promise<string> {
  const doc = new DOMParser().parseFromString(html, 'text/html');
  const fuera = [...DROP_WITH_CONTENT].join(',');
  let n = 0;
  for (const img of Array.from(doc.querySelectorAll('img'))) {
    if (img.parentElement?.closest(fuera)) continue;
    const fichero = ficheroDeDatos(img.getAttribute('src') ?? '', n + 1);
    if (!fichero) continue;
    n++;
    const cid = await incrustar(fichero);
    if (cid) img.setAttribute('src', `cid:${cid}`);
  }
  return n > 0 ? doc.body.innerHTML : html;
}

/** Sanea un fragmento HTML a la lista blanca del compositor. */
export function sanitizeHtml(input: string): string {
  const doc = new DOMParser().parseFromString(input, 'text/html');
  const out = doc.createElement('div');
  cleanInto(doc.body, out, doc);
  return out.innerHTML;
}

/** Escapa texto para insertarlo como HTML seguro. */
function escapeHtml(text: string): string {
  return text.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
}

/** Texto plano → HTML del editor (saltos de línea como <br>). */
export function textToHtml(text: string): string {
  if (!text) return '';
  return escapeHtml(text).replace(/\r?\n/g, '<br>');
}

/**
 * HTML → texto plano aproximado (para guardar el borrador, que es text/plain).
 * Convierte <br> y cierres de bloque en saltos de línea antes de descartar el
 * marcado. ponytail: aproximación; el round-trip de formato del borrador queda
 * como texto (el envío sí va en rich-text).
 */
export function htmlToText(html: string): string {
  const withBreaks = html
    .replace(/<br\s*\/?>/gi, '\n')
    .replace(/<\/(p|div|li|h[1-6]|blockquote|pre)>/gi, '\n');
  const doc = new DOMParser().parseFromString(withBreaks, 'text/html');
  return (doc.body.textContent ?? '')
    .replace(/\n{3,}/g, '\n\n')
    .replace(/[ \t]+\n/g, '\n')
    .trim();
}

/** ¿El HTML compuesto está vacío (solo espacios/saltos)? */
export function isHtmlEmpty(html: string): boolean {
  return htmlToText(html).trim().length === 0;
}
