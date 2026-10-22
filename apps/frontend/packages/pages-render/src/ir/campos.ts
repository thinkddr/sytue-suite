/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Tabla de campos del IR: guía la fase ESTRUCTURAL del validador (aquí y en Go) y el JSON Schema de
 * la IA. Cada campo lleva un descriptor en una sola cadena, para que el guardián de Go
 * (`TestEspejoDeCampos`) la lea con una expresión regular y la compare con su tabla:
 *
 *   s texto · b booleano · t solo `true` · i entero · n decimal
 *   e:<x> valor de ENUMERADOS.x · o:<x> objeto de CAMPOS_OBJETO · u:<x> unión de UNIONES
 *   a:<d> array de <d> · m:<d> mapa de claves libres · i[4], i[3], n[2] tuplas
 *   textos Textos · ricos TextosRicos · nodo cualquier Nodo (lo decide `type`)
 *   medida 'fill' | 'hug' | entero · radio entero | '$sm' | '$md' | '$lg' · fondo color | o:fondo_imagen
 *   sufijo `|null` anulable · sufijo `!` obligatorio
 *
 * El compilador comprueba que cada tabla tiene EXACTAMENTE las claves de su tipo de `tipos.ts`, las
 * mismas obligatorias y los mismos valores de cada enumerado (`CamposExactos`, al final): si alguien
 * añade un campo al tipo y no aquí, o al revés, `tsc` falla con el nombre del objeto.
 */

import type {
  BotonCarrito,
  Boton,
  CampoFormulario,
  Componente,
  Delta,
  ElementoMenu,
  Enlace,
  Estilo,
  EstiloTexto,
  EstiloTipografico,
  FondoImagen,
  Formulario,
  Icono,
  Imagen,
  ImagenRef,
  Incrustado,
  Instancia,
  Layout,
  Marco,
  Menu,
  Negocio,
  Nodo,
  NodoBase,
  Override,
  Pagina,
  RejillaProductos,
  Seccion,
  SelectorIdioma,
  SeoSitio,
  Sitio,
  Tabla,
  Tamano,
  Tema,
  Texto,
  Tramo,
} from './tipos';
import { ESTILOS_TEXTO, PROVEEDORES_INCRUSTADO, TIPOS_NODO, TIPOS_SECCION } from './tipos';

/** Claves que llevan todos los nodos (`NodoBase` + `type`). */
const BASE = {
  id: 's!',
  type: 'e:tipo_nodo!',
  name: 's',
  locked: 't',
  hidden: 't',
  size: 'o:tamano',
  style: 'o:estilo',
  breakpoints: 'o:puntos',
} as const;

/** Tipo de nodo → sus campos. */
export const CAMPOS_NODO = {
  section: {
    ...BASE,
    kind: 'e:tipo_seccion!',
    anchor: 's',
    full_bleed: 'b!',
    layout: 'o:layout!',
    children: 'a:nodo!',
  },
  frame: {
    ...BASE,
    tag: 'e:etiqueta_marco!',
    layout: 'o:layout!',
    children: 'a:nodo!',
    link: 'u:enlace',
  },
  text: { ...BASE, tag: 'e:etiqueta_texto!', content: 'ricos!' },
  image: {
    ...BASE,
    src: 'u:imagen!',
    alt: 'textos!',
    decorative: 't',
    fit: 'e:ajuste!',
    link: 'u:enlace',
    priority: 't',
  },
  button: {
    ...BASE,
    label: 'textos!',
    variant: 'e:variante!',
    action: 'u:enlace!',
    icon: 's',
    icon_position: 'e:posicion_icono',
  },
  // En un icono, `name` es el icono (de ICONOS; lo comprueba la fase semántica), no el de la capa.
  icon: { ...BASE, name: 's!', label: 'textos' },
  embed: { ...BASE, provider: 'e:proveedor!', ref: 's!', title: 'textos!' },
  form: {
    ...BASE,
    form: 's!',
    layout: 'o:layout!',
    fields: 'a:o:campo!',
    submit: 'textos!',
    success: 'textos!',
    consent: 'textos!',
  },
  table: { ...BASE, caption: 'textos!', header: 'a:textos!', rows: 'a:a:textos!' },
  product_grid: {
    ...BASE,
    source: 'u:fuente_productos!',
    limit: 'i!',
    columns: 'i!',
    show_price: 'b!',
    show_add_to_cart: 'b!',
  },
  cart_button: { ...BASE, label: 'textos!' },
  language_switcher: { ...BASE, display: 'e:modo_idioma!' },
  menu: {
    ...BASE,
    items: 'a:o:menu_item!',
    orientation: 'e:orientacion!',
    collapse_below: 'e:colapso!',
    label: 'textos',
  },
  instance: { ...BASE, component: 's!', overrides: 'm:o:override' },
} as const;

/** Objetos que no son nodos → sus campos. Las variantes de una unión se llaman `<unión>_<discriminante>`. */
export const CAMPOS_OBJETO = {
  sitio: {
    version: 'i!',
    languages: 'o:idiomas!',
    theme: 'o:tema!',
    header: 'nodo|null!',
    footer: 'nodo|null!',
    pages: 'a:o:pagina!',
    components: 'a:o:componente!',
    seo: 'o:seo!',
  },
  idiomas: { default: 's!', others: 'a:s!' },
  tema: {
    colors: 'm:s!',
    fonts: 'o:fuentes!',
    text_styles: 'o:estilos_texto!',
    radius: 'o:radios!',
    container: 'i!',
  },
  fuentes: { heading: 's!', body: 's!' },
  estilos_texto: {
    h1: 'o:estilo_tipografico!',
    h2: 'o:estilo_tipografico!',
    h3: 'o:estilo_tipografico!',
    h4: 'o:estilo_tipografico!',
    h5: 'o:estilo_tipografico!',
    h6: 'o:estilo_tipografico!',
    body: 'o:estilo_tipografico!',
    small: 'o:estilo_tipografico!',
    label: 'o:estilo_tipografico!',
    button: 'o:estilo_tipografico!',
  },
  estilo_tipografico: {
    font: 'e:familia!',
    size: 'i[3]!',
    line_height: 'n!',
    weight: 'e:peso!',
    letter_spacing: 'n!',
    transform: 'e:mayusculas',
  },
  radios: { sm: 'i!', md: 'i!', lg: 'i!' },
  pagina: {
    id: 's!',
    slug: 'textos!',
    title: 'textos!',
    description: 'textos!',
    sections: 'a:nodo!',
    show_header: 'b!',
    show_footer: 'b!',
    noindex: 't',
    og_image: 'u:imagen',
  },
  tamano: {
    width: 'medida!',
    height: 'medida!',
    min_width: 'i',
    max_width: 'i',
    min_height: 'i',
    max_height: 'i',
    aspect_ratio: 'n',
  },
  layout: {
    direction: 'e:direccion!',
    gap: 'i!',
    row_gap: 'i',
    padding: 'i[4]!',
    align: 'e:alinear!',
    justify: 'e:repartir!',
    wrap: 'b!',
    columns: 'i',
  },
  estilo: {
    background: 'fondo',
    color: 's',
    border: 'o:borde',
    radius: 'radio',
    shadow: 'e:sombra',
    opacity: 'n',
    text: 'o:estilo_texto',
    overflow: 'e:desborde',
  },
  borde: { width: 'i!', color: 's!', style: 'e:trazo!' },
  fondo_imagen: { image: 'u:imagen!', fit: 'e:ajuste!', position: 'n[2]!', overlay: 's' },
  estilo_texto: {
    preset: 'e:estilo_texto',
    font: 'e:familia',
    size: 'i',
    weight: 'e:peso',
    line_height: 'n',
    letter_spacing: 'n',
    align: 'e:alinear_texto',
    transform: 'e:transformar',
    italic: 'b',
  },
  puntos: { tablet: 'o:delta', mobile: 'o:delta' },
  delta: {
    hidden: 'b',
    size: 'o:tamano_delta',
    layout: 'o:layout_delta',
    style: 'o:estilo_delta',
    columns: 'i',
  },
  tamano_delta: {
    width: 'medida',
    height: 'medida',
    min_width: 'i|null',
    max_width: 'i|null',
    min_height: 'i|null',
    max_height: 'i|null',
    aspect_ratio: 'n|null',
  },
  layout_delta: {
    direction: 'e:direccion',
    gap: 'i',
    row_gap: 'i',
    padding: 'i[4]',
    align: 'e:alinear',
    justify: 'e:repartir',
    wrap: 'b',
    columns: 'i',
  },
  estilo_delta: {
    background: 'fondo|null',
    color: 's',
    border: 'o:borde|null',
    radius: 'radio',
    shadow: 'e:sombra|null',
    opacity: 'n',
    text: 'o:estilo_texto',
    overflow: 'e:desborde',
  },
  tramo: { text: 's!', bold: 't', italic: 't', underline: 't', link: 'u:enlace' },
  campo: {
    id: 's!',
    kind: 'e:tipo_campo!',
    label: 'textos!',
    required: 'b!',
    options: 'a:textos',
    max_length: 'i',
    min: 'i',
    max: 'i',
    autocomplete: 'e:autocompletar',
  },
  menu_item: { id: 's!', label: 'textos!', link: 'u:enlace!', children: 'a:o:menu_item' },
  override: {
    text: 'ricos',
    label: 'textos',
    image: 'u:imagen',
    alt: 'textos',
    link: 'u:enlace|null',
    icon: 's',
    hidden: 'b',
    style: 'o:estilo',
  },
  componente: { id: 's!', name: 's!', root: 'nodo!' },
  seo: {
    site_name: 'textos!',
    og_image: 'u:imagen',
    favicon: 'u:imagen',
    noindex: 't',
    business: 'o:negocio',
  },
  negocio: {
    type: 'e:tipo_negocio!',
    phone: 's',
    email: 's',
    address: 'o:direccion',
    geo: 'o:geo',
    opening_hours: 'a:o:horario',
  },
  direccion: { street: 's!', city: 's!', postal_code: 's!', region: 's', country: 's!' },
  geo: { lat: 'n!', lon: 'n!' },
  horario: { days: 'a:e:dia!', opens: 's!', closes: 's!' },
  enlace_page: { page: 's!', anchor: 's' },
  enlace_anchor: { anchor: 's!' },
  enlace_url: { url: 's!', new_tab: 'b' },
  enlace_product: { product: 's!' },
  enlace_file: { file: 's!' },
  enlace_legal: { legal: 'e:legal!' },
  enlace_cart: { cart: 't!' },
  imagen_asset: { asset: 's!' },
  imagen_library: { library: 's!' },
  imagen_placeholder: { placeholder: 'e:relleno!' },
  fuente_productos_all: { all: 't!' },
  fuente_productos_category: { category: 's!' },
  fuente_productos_products: { products: 'a:s!' },
} as const;

/**
 * Uniones de objetos: la variante la decide la ÚNICA clave discriminante presente (su objeto es
 * `CAMPOS_OBJETO[<unión>_<clave>]`). Si hay varias, se descarta la que sea miembro de la variante de
 * otra (`{page, anchor}` es la variante `page`); si no queda exactamente una, código `valor`.
 */
export const UNIONES = {
  enlace: ['page', 'anchor', 'url', 'product', 'file', 'legal', 'cart'],
  imagen: ['asset', 'library', 'placeholder'],
  fuente_productos: ['all', 'category', 'products'],
} as const;

/** Valores admitidos de cada enumerado (fuera de ellos, código `valor`). */
export const ENUMERADOS = {
  tipo_nodo: TIPOS_NODO,
  tipo_seccion: TIPOS_SECCION,
  estilo_texto: ESTILOS_TEXTO,
  proveedor: PROVEEDORES_INCRUSTADO,
  familia: ['heading', 'body'],
  peso: [400, 500, 600, 700, 800],
  mayusculas: ['uppercase'],
  direccion: ['row', 'column'],
  alinear: ['start', 'center', 'end', 'baseline'],
  repartir: ['start', 'center', 'end', 'space-between', 'space-around', 'space-evenly'],
  sombra: ['none', 'sm', 'md', 'lg'],
  desborde: ['visible', 'hidden'],
  trazo: ['solid', 'dashed'],
  ajuste: ['cover', 'contain'],
  alinear_texto: ['start', 'center', 'end', 'justify'],
  transformar: ['none', 'uppercase'],
  etiqueta_marco: ['div', 'header', 'footer', 'nav', 'article', 'aside', 'ul', 'ol', 'li'],
  etiqueta_texto: ['h1', 'h2', 'h3', 'h4', 'h5', 'h6', 'p', 'span', 'blockquote'],
  relleno: ['photo', 'portrait', 'product', 'landscape', 'logo'],
  variante: ['primary', 'secondary', 'ghost'],
  posicion_icono: ['start', 'end'],
  tipo_campo: ['text', 'email', 'tel', 'textarea', 'select', 'checkbox', 'date', 'time', 'number'],
  autocompletar: [
    'name',
    'given-name',
    'family-name',
    'organization',
    'email',
    'tel',
    'street-address',
    'postal-code',
    'address-level2',
    'country-name',
  ],
  modo_idioma: ['code', 'name'],
  orientacion: ['horizontal', 'vertical'],
  colapso: ['never', 'tablet', 'mobile'],
  legal: ['notice', 'privacy', 'cookies', 'terms', 'withdrawal'],
  tipo_negocio: [
    'Organization',
    'LocalBusiness',
    'Store',
    'Restaurant',
    'MedicalClinic',
    'LodgingBusiness',
    'EducationalOrganization',
    'ProfessionalService',
    'HomeAndConstructionBusiness',
  ],
  dia: ['Mo', 'Tu', 'We', 'Th', 'Fr', 'Sa', 'Su'],
} as const;

// ---------------------------------------------------------------------------------------------
// Exactitud comprobada por el compilador.

type TiposNodo = {
  section: Seccion;
  frame: Marco;
  text: Texto;
  image: Imagen;
  button: Boton;
  icon: Icono;
  embed: Incrustado;
  form: Formulario;
  table: Tabla;
  product_grid: RejillaProductos;
  cart_button: BotonCarrito;
  language_switcher: SelectorIdioma;
  menu: Menu;
  instance: Instancia;
};
type Fuente = RejillaProductos['source'];
type TiposObjeto = {
  sitio: Sitio;
  idiomas: Sitio['languages'];
  tema: Tema;
  fuentes: Tema['fonts'];
  estilos_texto: Tema['text_styles'];
  estilo_tipografico: EstiloTipografico;
  radios: Tema['radius'];
  pagina: Pagina;
  tamano: Tamano;
  layout: Layout;
  estilo: Estilo;
  borde: NonNullable<Estilo['border']>;
  fondo_imagen: FondoImagen;
  estilo_texto: EstiloTexto;
  puntos: NonNullable<NodoBase['breakpoints']>;
  delta: Delta;
  tamano_delta: NonNullable<Delta['size']>;
  layout_delta: NonNullable<Delta['layout']>;
  estilo_delta: NonNullable<Delta['style']>;
  tramo: Tramo;
  campo: CampoFormulario;
  menu_item: ElementoMenu;
  override: Override;
  componente: Componente;
  seo: SeoSitio;
  negocio: Negocio;
  direccion: NonNullable<Negocio['address']>;
  geo: NonNullable<Negocio['geo']>;
  horario: NonNullable<Negocio['opening_hours']>[number];
  enlace_page: Extract<Enlace, { page: string }>;
  enlace_anchor: Extract<Enlace, { anchor: string }>;
  enlace_url: Extract<Enlace, { url: string }>;
  enlace_product: Extract<Enlace, { product: string }>;
  enlace_file: Extract<Enlace, { file: string }>;
  enlace_legal: Extract<Enlace, { legal: string }>;
  enlace_cart: Extract<Enlace, { cart: true }>;
  imagen_asset: Extract<ImagenRef, { asset: string }>;
  imagen_library: Extract<ImagenRef, { library: string }>;
  imagen_placeholder: Extract<ImagenRef, { placeholder: string }>;
  fuente_productos_all: Extract<Fuente, { all: true }>;
  fuente_productos_category: Extract<Fuente, { category: string }>;
  fuente_productos_products: Extract<Fuente, { products: string[] }>;
};
type Enumerados = {
  tipo_nodo: Seccion['type'] | Nodo['type'];
  tipo_seccion: Seccion['kind'];
  estilo_texto: NonNullable<EstiloTexto['preset']>;
  proveedor: Incrustado['provider'];
  familia: EstiloTipografico['font'];
  peso: EstiloTipografico['weight'];
  mayusculas: NonNullable<EstiloTipografico['transform']>;
  direccion: Layout['direction'];
  alinear: Layout['align'];
  repartir: Layout['justify'];
  sombra: NonNullable<Estilo['shadow']>;
  desborde: NonNullable<Estilo['overflow']>;
  trazo: NonNullable<Estilo['border']>['style'];
  ajuste: Imagen['fit'];
  alinear_texto: NonNullable<EstiloTexto['align']>;
  transformar: NonNullable<EstiloTexto['transform']>;
  etiqueta_marco: Marco['tag'];
  etiqueta_texto: Texto['tag'];
  relleno: Extract<ImagenRef, { placeholder: string }>['placeholder'];
  variante: Boton['variant'];
  posicion_icono: NonNullable<Boton['icon_position']>;
  tipo_campo: CampoFormulario['kind'];
  autocompletar: NonNullable<CampoFormulario['autocomplete']>;
  modo_idioma: SelectorIdioma['display'];
  orientacion: Menu['orientation'];
  colapso: Menu['collapse_below'];
  legal: Extract<Enlace, { legal: string }>['legal'];
  tipo_negocio: Negocio['type'];
  dia: NonNullable<Negocio['opening_hours']>[number]['days'][number];
};

type Igual<A, B> = [Exclude<A, B>, Exclude<B, A>] extends [never, never] ? true : false;
type Requeridas<T> = {
  [K in keyof T]-?: Partial<Pick<T, K>> extends Pick<T, K> ? never : K;
}[keyof T];
type Marcadas<D> = { [K in keyof D]: D[K] extends `${string}!` ? K : never }[keyof D];
type EnumDe<D> = D extends `e:${infer E}!`
  ? E
  : D extends `e:${infer E}|null`
    ? E
    : D extends `e:${infer E}`
      ? E
      : never;
/** Campos `e:<x>` cuyo tipo no es exactamente `Enumerados[x]` (`type` se salta: es el discriminante). */
type EnumsMal<T, D> = {
  [K in keyof D & keyof T]: K extends 'type'
    ? never
    : [EnumDe<D[K]>] extends [never]
      ? never
      : EnumDe<D[K]> extends keyof Enumerados
        ? Igual<NonNullable<T[K]>, Enumerados[EnumDe<D[K]>]> extends true
          ? never
          : K
        : never;
}[keyof D & keyof T];
type Encaja<T, D> =
  Igual<keyof T, keyof D> extends true
    ? Igual<Requeridas<T>, Marcadas<D>> extends true
      ? [EnumsMal<T, D>] extends [never]
        ? true
        : false
      : false
    : false;
type Fallos<Tipos, Tabla> = {
  [K in keyof Tabla]: K extends keyof Tipos
    ? Encaja<Tipos[K], Tabla[K]> extends true
      ? never
      : K
    : K;
}[keyof Tabla];
type EnumsFallidos = {
  [K in keyof typeof ENUMERADOS]: K extends keyof Enumerados
    ? Igual<(typeof ENUMERADOS)[K][number], Enumerados[K]> extends true
      ? never
      : K
    : K;
}[keyof typeof ENUMERADOS];
// oxlint-disable typescript/no-duplicate-type-constituents, typescript/no-redundant-type-constituents
type TodosLosFallos =
  | Fallos<TiposNodo, typeof CAMPOS_NODO>
  | Fallos<TiposObjeto, typeof CAMPOS_OBJETO>
  | EnumsFallidos
  | Exclude<keyof TiposNodo, keyof typeof CAMPOS_NODO>
  | Exclude<keyof TiposObjeto, keyof typeof CAMPOS_OBJETO>;
type Afirmar<T extends true> = T;

/** Si esto no compila, el tipo que sale nombra la tabla (o el enumerado) que no casa con `tipos.ts`. */
export type CamposExactos = Afirmar<[TodosLosFallos] extends [never] ? true : TodosLosFallos>;
