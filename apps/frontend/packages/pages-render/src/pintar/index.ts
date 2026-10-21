/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */

import type { DatosRender } from '../contexto';
import { TRAZOS } from './iconos';
import { cssNodos, cssTema } from './estilos';
import type {
  Enlace,
  Id,
  Idioma,
  ImagenRef,
  Nodo,
  Pagina,
  Seccion,
  Sitio,
  Textos,
  TextosRicos,
  Override,
  CampoFormulario,
} from '../ir/tipos';

export interface OpcionesLienzo {
  modo: 'lienzo';
  urlRecurso: (nombre: string) => string;
  puntoDeCorte: 'desktop' | 'tablet' | 'mobile';
}

interface ContextoPintura {
  ir: Sitio;
  datos: DatosRender;
  idioma: Idioma;
  recurso: (nombre: string) => string;
  vista: boolean;
  /** Prefijo de navegación de una vista previa; los recursos siguen siendo compartidos. */
  base?: string;
  /** Lienzo del editor y vistas previas de la galería: los rellenos de imagen se pintan en línea. */
  lienzo?: boolean;
}

const escapar = (s: string): string =>
  s
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#39;');
const texto = (v: Textos | undefined, idioma: Idioma, defecto = ''): string =>
  v?.[idioma] ?? v?.es ?? v?.en ?? defecto;
const rico = (v: TextosRicos, c: ContextoPintura): string =>
  (v[c.idioma] ?? v.es ?? v.en ?? [])
    .map((t) => {
      let salida = escapar(t.text);
      if (t.underline) salida = `<u>${salida}</u>`;
      if (t.italic) salida = `<em>${salida}</em>`;
      if (t.bold) salida = `<strong>${salida}</strong>`;
      return t.link === undefined
        ? salida
        : `<a href="${escapar(enlace(t.link, c))}">${salida}</a>`;
    })
    .join('');

function enlace(v: Enlace, c: ContextoPintura): string {
  const { idioma, ir, base = '' } = c;
  if ('url' in v) return v.url;
  if ('anchor' in v && !('page' in v)) return `#${v.anchor}`;
  if ('legal' in v) return `${base}/${idioma}/legal/${v.legal}/`;
  if ('cart' in v) return `${base}/${idioma}/cart/`;
  if ('file' in v) {
    const fichero = c.datos.files[`a:${v.file}`];
    return fichero === undefined ? '#' : c.recurso(`${fichero.file}.pdf`);
  }
  if ('product' in v) return `${base}/${idioma}/p/${v.product}/`;
  const pagina = ir?.pages.find((p) => p.id === v.page);
  const slug = pagina === undefined ? '' : texto(pagina.slug, idioma);
  return `${base}/${idioma}/${slug === '' ? '' : `${slug}/`}${v.anchor === undefined ? '' : `#${v.anchor}`}`;
}

const PROPORCION_RELLENO: Record<string, [number, number]> = {
  photo: [1200, 800],
  portrait: [960, 1200],
  product: [1000, 1000],
  landscape: [1600, 900],
  logo: [900, 300],
};

/** Una geometría para el lienzo y la publicación, sin ficheros de relleno ficticios. */
function rellenoEnLinea(tipo: string): { src: string; svg: string; width: number; height: number } {
  const [w, h] = PROPORCION_RELLENO[tipo] ?? [1200, 800];
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="${w}" height="${h}" viewBox="0 0 ${w} ${h}"><rect width="${w}" height="${h}" fill="#d9dbdd"/><path d="M0 ${h}L${w * 0.35} ${h * 0.45}L${w * 0.55} ${h * 0.7}L${w * 0.7} ${h * 0.55}L${w} ${h}z" fill="#c3c6c9"/><circle cx="${w * 0.78}" cy="${h * 0.25}" r="${h * 0.09}" fill="#c3c6c9"/></svg>`;
  return { src: `data:image/svg+xml;utf8,${encodeURIComponent(svg)}`, svg, width: w, height: h };
}

function imagen(
  ref: ImagenRef,
  c: ContextoPintura,
): { src: string; svg?: string; width: number; height: number } {
  if ('placeholder' in ref) return rellenoEnLinea(ref.placeholder);
  const clave = 'asset' in ref ? `a:${ref.asset}` : `l:${ref.library}`;
  const data = c.datos.images[clave];
  return data === undefined
    ? rellenoEnLinea('photo')
    : { src: c.recurso(data.file), width: data.width, height: data.height };
}

function attrsNodo(n: Nodo | Seccion | Sitio['header'], extra: string[] = []): string {
  if (n === null) return '';
  const clases = [`sp-${n.type}`, `n${n.id}`, ...extra];
  return ` class="${clases.join(' ')}" data-node="${n.id}"`;
}

/** Conserva la variante concreta al materializar los cambios comunes de una instancia. */
function conOverrideComun<T extends Nodo | Seccion>(nodo: T, override: Override | undefined): T {
  if (override === undefined) return nodo;
  const { hidden: _hidden, ...visible } = nodo;
  // El compilador no puede reconstruir el discriminante genérico T tras retirar
  // `hidden`; los campos se limitan a NodoBase y no cambian su variante.
  return {
    ...(override.hidden === false ? visible : nodo),
    ...(override.style ? { style: { ...nodo.style, ...override.style } } : {}),
    ...(override.hidden === true ? { hidden: true as const } : {}),
  } as T;
}

/** Materializa los overrides de una instancia sin mutar el componente maestro. */
function conOverrides(
  n: Nodo | Seccion,
  overrides: Record<Id, Override> | undefined,
): Nodo | Seccion {
  const override = overrides?.[n.id];
  switch (n.type) {
    case 'section': {
      const base = conOverrideComun(n, override);
      return {
        ...base,
        children: n.children.map((child) => conOverrides(child, overrides) as Nodo),
      };
    }
    case 'frame': {
      const base = conOverrideComun(n, override);
      return {
        ...base,
        children: n.children.map((child) => conOverrides(child, overrides) as Nodo),
      };
    }
    case 'text':
      return {
        ...conOverrideComun(n, override),
        ...(override?.text ? { content: override.text } : {}),
      };
    case 'image':
      return {
        ...conOverrideComun(n, override),
        ...(override?.image ? { src: override.image } : {}),
        ...(override?.alt ? { alt: override.alt } : {}),
      };
    case 'button':
    case 'cart_button':
    case 'menu':
      return {
        ...conOverrideComun(n, override),
        ...(override?.label ? { label: override.label } : {}),
      };
    case 'icon':
      return {
        ...conOverrideComun(n, override),
        ...(override?.label ? { label: override.label } : {}),
        ...(override?.icon ? { name: override.icon } : {}),
      };
    default:
      return conOverrideComun(n, override);
  }
}

const ETIQUETA_PRESET: Record<string, string> = {
  h1: 'h1',
  h2: 'h2',
  h3: 'h3',
  h4: 'h4',
  h5: 'h5',
  h6: 'h6',
  p: 'body',
  span: 'body',
  blockquote: 'body',
};

/** Icono del juego propio, en línea y con el color del texto. Decorativo salvo que lleve etiqueta. */
function iconoSvg(nombre: string, clase: string, etiqueta?: string): string {
  const d = (TRAZOS as Record<string, string | undefined>)[nombre] ?? TRAZOS.info;
  const acc =
    etiqueta === undefined
      ? 'aria-hidden="true" focusable="false"'
      : `role="img" aria-label="${escapar(etiqueta)}"`;
  return `<svg class="${clase}" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" ${acc}><path d="${d}"/></svg>`;
}

function pintarNodo(n: Nodo | Seccion, c: ContextoPintura): string {
  switch (n.type) {
    case 'section': {
      const etiqueta = n.kind === 'header' ? 'header' : n.kind === 'footer' ? 'footer' : 'section';
      const hijos = n.children.map((x) => pintarNodo(x, c)).join('');
      const dentro = n.full_bleed ? hijos : `<div class="sp-c">${hijos}</div>`;
      return `<${etiqueta}${attrsNodo(n)}${n.anchor === undefined ? '' : ` id="${escapar(n.anchor)}"`}>${dentro}</${etiqueta}>`;
    }
    case 'frame': {
      const contenido = n.children.map((x) => pintarNodo(x, c)).join('');
      const body = `<${n.tag}${attrsNodo(n)}>${contenido}</${n.tag}>`;
      return n.link === undefined
        ? body
        : `<a class="sp-enlace" href="${escapar(enlace(n.link, c))}">${body}</a>`;
    }
    case 'text': {
      const preset = n.style?.text?.preset ?? ETIQUETA_PRESET[n.tag] ?? 'body';
      return `<${n.tag}${attrsNodo(n, [`t-${preset}`])}>${rico(n.content, c)}</${n.tag}>`;
    }
    case 'image': {
      const im = imagen(n.src, c);
      const alt = n.decorative ? '' : texto(n.alt, c.idioma);
      const carga = n.priority
        ? ' fetchpriority="high" decoding="async"'
        : ' loading="lazy" decoding="async"';
      const contenido =
        im.svg !== undefined && c.lienzo !== true
          ? im.svg.replace(
              '<svg ',
              `<svg class="sp-image n${n.id}" ${n.decorative ? 'aria-hidden="true"' : `role="img" aria-label="${escapar(alt)}"`} `,
            )
          : `<img${attrsNodo(n)} src="${escapar(im.src)}" alt="${escapar(alt)}" width="${im.width}" height="${im.height}"${carga}>`;
      return n.link === undefined
        ? contenido
        : `<a class="sp-enlace" href="${escapar(enlace(n.link, c))}">${contenido}</a>`;
    }
    case 'button': {
      const ico = n.icon === undefined ? '' : iconoSvg(n.icon, 'sp-icono');
      const etiqueta = escapar(texto(n.label, c.idioma));
      const cuerpo = n.icon_position === 'end' ? `${etiqueta}${ico}` : `${ico}${etiqueta}`;
      return `<a${attrsNodo(n, [`sp-button-${n.variant}`, 't-button'])} href="${escapar(enlace(n.action, c))}">${cuerpo}</a>`;
    }
    case 'icon':
      return iconoSvg(n.name, `sp-icon n${n.id}`, texto(n.label, c.idioma) || undefined).replace(
        'class="',
        `data-node="${n.id}" class="`,
      );
    case 'menu': {
      const items = n.items
        .map(
          (i) =>
            `<li><a href="${escapar(enlace(i.link, c))}">${escapar(texto(i.label, c.idioma))}</a></li>`,
        )
        .join('');
      const etiqueta = escapar(texto(n.label, c.idioma, 'Navigation'));
      const vertical = n.orientation === 'vertical' ? ' sp-menu-v' : '';
      return `<div${attrsNodo(n, [`sp-col-${n.collapse_below}${vertical}`])}><nav aria-label="${etiqueta}"><ul>${items}</ul></nav><details class="sp-menu-m"><summary>${c.idioma === 'es' ? 'Menú' : 'Menu'}</summary><ul>${items}</ul></details></div>`;
    }
    case 'language_switcher':
      return `<nav${attrsNodo(n)} aria-label="Language"><ul>${[c.ir.languages.default, ...c.ir.languages.others].map((l) => `<li><a href="/${l}/">${n.display === 'code' ? l.toUpperCase() : l}</a></li>`).join('')}</ul></nav>`;
    case 'cart_button':
      return `<a${attrsNodo(n, ['sp-button-secondary', 't-button'])} href="/${c.idioma}/cart/">${iconoSvg('cart', 'sp-icono')}${escapar(texto(n.label, c.idioma))}</a>`;
    case 'form':
      return pintarFormulario(n, c);
    case 'table': {
      const idCaption = `c${n.id}`;
      return `<div${attrsNodo(n)} role="region" aria-labelledby="${idCaption}" tabindex="0"><table><caption id="${idCaption}">${escapar(texto(n.caption, c.idioma))}</caption><thead><tr>${n.header.map((x) => `<th scope="col">${escapar(texto(x, c.idioma))}</th>`).join('')}</tr></thead><tbody>${n.rows.map((r) => `<tr>${r.map((x) => `<td>${escapar(texto(x, c.idioma))}</td>`).join('')}</tr>`).join('')}</tbody></table></div>`;
    }
    case 'embed':
      return `<iframe${attrsNodo(n)} src="${escapar(embedUrl(n.provider, n.ref, c.datos.origins.cast))}" title="${escapar(texto(n.title, c.idioma))}" loading="lazy" referrerpolicy="no-referrer" width="800" height="450"></iframe>`;
    case 'product_grid':
      return `<section${attrsNodo(n, [`sp-cols-${Math.min(Math.max(Math.trunc(n.columns), 1), 6)}`])} aria-label="Products"></section>`;
    case 'instance': {
      const maestro = c.ir.components.find((componente) => componente.id === n.component);
      if (!maestro) return '';
      return `<div${attrsNodo(n)}>${pintarNodo(conOverrides(maestro.root, n.overrides), c)}</div>`;
    }
  }
}

function pintarCampo(f: CampoFormulario, c: ContextoPintura): string {
  const etiqueta = escapar(texto(f.label, c.idioma));
  const comunes = `name="${f.id}"${f.required ? ' required' : ''}${f.autocomplete === undefined ? '' : ` autocomplete="${f.autocomplete}"`}${c.vista ? ' disabled' : ''}`;
  const tope =
    f.max_length === undefined
      ? ''
      : ` maxlength="${Math.min(Math.max(Math.trunc(f.max_length), 1), 5000)}"`;
  switch (f.kind) {
    case 'textarea':
      return `<label class="sp-campo t-label"><span>${etiqueta}</span><textarea ${comunes}${tope} rows="4"></textarea></label>`;
    case 'select': {
      const opciones = (f.options ?? [])
        .map((o, i) => `<option value="${i}">${escapar(texto(o, c.idioma))}</option>`)
        .join('');
      return `<label class="sp-campo t-label"><span>${etiqueta}</span><select ${comunes}>${opciones}</select></label>`;
    }
    case 'checkbox':
      return `<label class="sp-campo sp-marca t-label"><input type="checkbox" ${comunes}><span>${etiqueta}</span></label>`;
    case 'number': {
      const lim = `${f.min === undefined ? '' : ` min="${Math.trunc(f.min)}"`}${f.max === undefined ? '' : ` max="${Math.trunc(f.max)}"`}`;
      return `<label class="sp-campo t-label"><span>${etiqueta}</span><input type="number" ${comunes}${lim}></label>`;
    }
    default:
      return `<label class="sp-campo t-label"><span>${etiqueta}</span><input type="${f.kind}" ${comunes}${tope}></label>`;
  }
}

function pintarFormulario(n: Extract<Nodo, { type: 'form' }>, c: ContextoPintura): string {
  const campos = n.fields.map((f) => pintarCampo(f, c)).join('');
  const consentimiento = `<label class="sp-campo sp-marca t-small"><input type="checkbox" name="consent" required${c.vista ? ' disabled' : ''}><span>${escapar(texto(n.consent, c.idioma))}</span></label>`;
  const boton = `<button type="submit" class="sp-button sp-button-primary t-button"${c.vista ? ' disabled' : ''}>${escapar(texto(n.submit, c.idioma))}</button>`;
  return c.vista
    ? `<section${attrsNodo(n, ['sp-form'])}>${campos}${consentimiento}${boton}<p>${c.idioma === 'es' ? 'Los formularios no envían en la vista previa' : 'Forms do not send in preview'}</p></section>`
    : `<form${attrsNodo(n, ['sp-form'])} action="/_pages/forms/${n.form}" method="post">${campos}${consentimiento}${boton}</form>`;
}

/**
 * Los orígenes de terceros que un bloque «embed» puede encuadrar, además de Cast. El
 * editor de Sytue Web los deja pasar en su `frame-src` (XSS-06) y a ningún otro: los
 * pinta dentro de su origen, con la sesión delante.
 */
export const ORIGENES_DE_INCRUSTADOS = {
  youtube: 'https://www.youtube-nocookie.com',
  vimeo: 'https://player.vimeo.com',
  mapa: 'https://www.openstreetmap.org',
} as const;

function embedUrl(provider: string, ref: string, cast: string): string {
  if (provider === 'cast') return `${cast}/cast/embed/${ref}`;
  if (provider === 'youtube') return `${ORIGENES_DE_INCRUSTADOS.youtube}/embed/${ref}`;
  if (provider === 'vimeo') return `${ORIGENES_DE_INCRUSTADOS.vimeo}/video/${ref}?dnt=1`;
  const [lat, lon, zoom] = ref.split(',');
  return `${ORIGENES_DE_INCRUSTADOS.mapa}/export/embed.html?marker=${lat},${lon}&zoom=${zoom}`;
}

const CSS_BASE = `*{box-sizing:border-box}body{margin:0;background:var(--c-background);color:var(--c-text);font-family:var(--f-body)}img,iframe,table{max-width:100%}a{color:inherit}ul,ol{list-style:none;margin:0;padding:0}h1,h2,h3,h4,h5,h6,p,blockquote{margin:0}.sp-enlace{display:contents;text-decoration:none}.sp-button{display:inline-flex;align-items:center;gap:.5rem;padding:.75rem 1.25rem;border-radius:var(--r-md);border:1.5px solid var(--c-primary);line-height:1.2;text-decoration:none}.sp-button-primary{background:var(--c-primary);color:var(--c-on_primary)}.sp-button-secondary{background:transparent;color:var(--c-primary)}.sp-button-ghost{background:transparent;color:var(--c-primary);border-color:transparent;padding:.5rem .6rem}.sp-icono{width:1.1em;height:1.1em;flex:none}.sp-menu ul{display:flex;gap:1.25rem;flex-wrap:wrap}.sp-menu-v ul{flex-direction:column;gap:.5rem}.sp-menu a{text-decoration:none}.sp-menu-m{display:none}.sp-menu-m summary{cursor:pointer}.sp-menu-m ul{display:flex;flex-direction:column;gap:.5rem;padding:.75rem 0}.sp-campo{display:flex;flex-direction:column;gap:.4rem;width:100%}.sp-marca{flex-direction:row;align-items:flex-start;gap:.6rem}.sp-campo input:not([type=checkbox]),.sp-campo select,.sp-campo textarea{font:inherit;padding:.65rem .75rem;border:1px solid var(--c-border);border-radius:var(--r-sm);background:var(--c-background);color:var(--c-text);width:100%}.sp-tabla{overflow-x:auto}.sp-tabla table{border-collapse:collapse;width:100%}.sp-tabla caption{text-align:start;font-weight:600;padding:0 0 .6rem;color:var(--c-muted)}.sp-tabla th,.sp-tabla td{padding:.75rem .85rem;text-align:start;border-bottom:1px solid var(--c-border)}.sp-tabla th{border-bottom:2px solid var(--c-text)}.sp-embed,iframe.sp-embed{border:0;aspect-ratio:16/9;width:100%;height:auto}blockquote{border-inline-start:3px solid var(--c-primary);padding-inline-start:.9rem}.sp-product_grid{display:grid;gap:1.25rem}.sp-cols-1{grid-template-columns:1fr}.sp-cols-2{grid-template-columns:repeat(2,1fr)}.sp-cols-3{grid-template-columns:repeat(3,1fr)}.sp-cols-4{grid-template-columns:repeat(4,1fr)}.sp-cols-5{grid-template-columns:repeat(5,1fr)}.sp-cols-6{grid-template-columns:repeat(6,1fr)}.sp-vista{position:sticky;top:0;z-index:10;background:var(--c-primary);color:var(--c-on_primary);padding:.5rem 1rem;text-align:center}.sp-hidden{display:none}@media (max-width:1023.98px){.sp-col-tablet>nav{display:none}.sp-col-tablet>.sp-menu-m{display:block}.sp-cols-4,.sp-cols-5,.sp-cols-6{grid-template-columns:repeat(2,1fr)}.sp-cols-3{grid-template-columns:repeat(2,1fr)}}@media (max-width:639.98px){.sp-col-mobile>nav,.sp-col-tablet>nav{display:none}.sp-col-mobile>.sp-menu-m,.sp-col-tablet>.sp-menu-m{display:block}.sp-cols-2,.sp-cols-3,.sp-cols-4,.sp-cols-5,.sp-cols-6{grid-template-columns:1fr 1fr}}`;

/**
 * CSS del sitio: tema (tokens, familias, estilos de texto), base común y, por nodo, su layout, tamaño y
 * estilo con los puntos de corte (§4.2). `datos` aporta las fuentes del catálogo; sin él se usa la pila
 * del sistema. Solo `@media` y `@font-face`: lo que admite la publicación.
 */
export function cssComun(
  ir: Sitio,
  datos?: DatosRender,
  recurso: (nombre: string) => string = (n) => `/_astro/${n}`,
): string {
  return `${cssTema(ir, datos, recurso)}:root{--sp-container:${Math.trunc(Number(ir.theme.container)) || 1200}px}${CSS_BASE}${cssNodos(ir)}`;
}

export function pintarPagina(
  ir: Sitio,
  datos: DatosRender,
  pagina: Id,
  idioma: Idioma,
  op: OpcionesLienzo,
): { html: string; css: string } {
  const p = ir.pages.find((x) => x.id === pagina);
  if (p === undefined) throw new Error(`página inexistente: ${pagina}`);
  const c: ContextoPintura = {
    ir,
    datos,
    idioma,
    recurso: op.urlRecurso,
    vista: true,
    lienzo: true,
  };
  return {
    html: `<a href="#contenido">Skip to content</a><main id="contenido">${p.show_header && ir.header !== null ? pintarNodo(ir.header, c) : ''}${p.sections.map((x) => pintarNodo(x, c)).join('')}${p.show_footer && ir.footer !== null ? pintarNodo(ir.footer, c) : ''}</main>`,
    css: cssComun(ir, datos, op.urlRecurso),
  };
}

export function paginaPorId(ir: Sitio, id: Id): Pagina | undefined {
  return ir.pages.find((p) => p.id === id);
}
export { escapar, texto, pintarNodo };
