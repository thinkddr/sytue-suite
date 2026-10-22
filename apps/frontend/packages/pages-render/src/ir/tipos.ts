/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El IR de Sytue Web, versión 1: el formato del diseño de una web. Es CABLE (claves en inglés
 * snake_case, enumerados que son valores CSS cuando se puede) y lo leen el renderizador, el editor,
 * la IA y los exportadores. Estos tipos son normativos (contrato de Pages §4.1): se pueden añadir
 * comentarios, no cambiar nombres ni formas. Su espejo en Go vive en `services/pages/internal/ir`
 * y dos guardianes de ese paquete leen este fichero y `campos.ts`.
 */

export const VERSION_IR = 1 as const;

/** Idiomas en que puede estar una WEB. Es un concepto distinto de los 26 de la interfaz (`LOCALES` de
 *  @sytue/i18n): añade catalán, euskera y gallego. Un test exige LOCALES ⊆ IDIOMAS. */
export const IDIOMAS = [
  'bg',
  'ca',
  'cs',
  'da',
  'de',
  'el',
  'en',
  'es',
  'et',
  'eu',
  'fi',
  'fr',
  'ga',
  'gl',
  'hr',
  'hu',
  'is',
  'it',
  'lt',
  'lv',
  'mt',
  'nb',
  'nl',
  'pl',
  'pt',
  'ro',
  'sk',
  'sl',
  'sv',
] as const;
/** Los que tienen textos de sistema para el visitante (carrito, legales, 404…). Solo estos se pueden poner en
 *  `languages`. Crece con la pieza de traducción (§18.1). */
export const IDIOMAS_VISITANTE = ['en', 'es'] as const;
export type Idioma = (typeof IDIOMAS)[number];

export const TIPOS_NODO = [
  'section',
  'frame',
  'text',
  'image',
  'button',
  'icon',
  'embed',
  'form',
  'table',
  'product_grid',
  'cart_button',
  'language_switcher',
  'menu',
  'instance',
] as const;
export const TIPOS_SECCION = [
  'header',
  'footer',
  'hero',
  'features',
  'gallery',
  'text',
  'cta',
  'contact',
  'map',
  'testimonials',
  'faq',
  'pricing',
  'team',
  'products',
  'logos',
  'custom',
] as const;
export const ICONOS = [
  'phone',
  'mail',
  'map-pin',
  'clock',
  'calendar',
  'check',
  'star',
  'heart',
  'cart',
  'bag',
  'truck',
  'gift',
  'tag',
  'user',
  'users',
  'building',
  'home',
  'globe',
  'shield',
  'leaf',
  'wrench',
  'hammer',
  'stethoscope',
  'bed',
  'utensils',
  'graduation',
  'briefcase',
  'camera',
  'image',
  'play',
  'quote',
  'chat',
  'arrow-right',
  'arrow-left',
  'chevron-down',
  'menu',
  'close',
  'plus',
  'minus',
  'external',
  'download',
  'info',
  'whatsapp',
  'instagram',
  'facebook',
  'linkedin',
  'tiktok',
  'x',
  'youtube',
] as const;
export const PROVEEDORES_INCRUSTADO = ['osm', 'cast', 'youtube', 'vimeo'] as const;
export const COLORES_OBLIGATORIOS = [
  'background',
  'text',
  'primary',
  'on_primary',
  'muted',
  'border',
] as const;
export const ESTILOS_TEXTO = [
  'h1',
  'h2',
  'h3',
  'h4',
  'h5',
  'h6',
  'body',
  'small',
  'label',
  'button',
] as const;
export const SLUGS_RESERVADOS = ['p', 'cart', 'order', 'legal', 'report'] as const;
export const ANCLAS_RESERVADAS = ['contenido'] as const;
export const LIMITES = {
  irBytes: 2_097_152,
  paginas: 50,
  seccionesPorPagina: 25,
  nodosPorPagina: 2000,
  profundidad: 12,
  componentes: 100,
  nodosPorComponente: 300,
  caracteresPorTexto: 10_000,
  coloresTema: 32,
  camposFormulario: 20,
  elementosMenu: 20,
  hijosPorElementoMenu: 12,
  productosEnRejilla: 48,
  filasTabla: 50,
  columnasTabla: 8,
  slug: 80,
  url: 2048,
} as const;

/** 10 caracteres [a-z0-9]. Un solo espacio de nombres en todo el documento (páginas, nodos, componentes y sus
 *  nodos, campos, elementos de menú, formularios). */
export type Id = string;
export type Textos = Partial<Record<Idioma, string>>;
export interface Tramo {
  text: string;
  bold?: true;
  italic?: true;
  underline?: true;
  link?: Enlace;
}
export type TextosRicos = Partial<Record<Idioma, Tramo[]>>;
/** '#rrggbb' | '#rrggbbaa' (minúsculas) | '$<token de theme.colors>' */
export type ColorValor = string;
export type Medida = 'fill' | 'hug' | number; // número = px fijos (entero)

export interface Sitio {
  version: typeof VERSION_IR;
  languages: { default: Idioma; others: Idioma[] };
  theme: Tema;
  header: Seccion | null; // kind 'header'
  footer: Seccion | null; // kind 'footer'
  pages: Pagina[]; // pages[0] es la portada
  components: Componente[];
  seo: SeoSitio;
}
export interface Tema {
  colors: Record<string, string>; // claves /^[a-z][a-z0-9_]{0,31}$/; ≥ COLORES_OBLIGATORIOS; valores '#rrggbb(aa)'
  fonts: { heading: string; body: string }; // id de familia del catálogo (§13.3)
  text_styles: Record<(typeof ESTILOS_TEXTO)[number], EstiloTipografico>;
  radius: { sm: number; md: number; lg: number }; // px
  container: number; // ancho máximo del contenido, 640..1600
}
export interface EstiloTipografico {
  font: 'heading' | 'body';
  size: [number, number, number]; // px en escritorio, tableta, móvil
  line_height: number;
  weight: 400 | 500 | 600 | 700 | 800;
  letter_spacing: number;
  transform?: 'uppercase';
}
export interface Pagina {
  id: Id;
  slug: Textos; // en TODOS los idiomas del sitio; portada '' en todos; resto /^[a-z0-9]+(-[a-z0-9]+)*$/
  title: Textos;
  description: Textos;
  sections: Seccion[];
  show_header: boolean;
  show_footer: boolean;
  noindex?: true;
  og_image?: ImagenRef;
}
export interface NodoBase {
  id: Id;
  name?: string; // ≤ 60, solo capas
  locked?: true; // solo editor
  hidden?: true; // oculto en escritorio (y abajo, salvo delta que lo muestre)
  size?: Tamano;
  style?: Estilo;
  breakpoints?: { tablet?: Delta; mobile?: Delta };
}
export interface Tamano {
  width: Medida;
  height: Medida;
  min_width?: number;
  max_width?: number;
  min_height?: number;
  max_height?: number;
  aspect_ratio?: number; // ancho/alto; solo image, embed y frame
}
export interface Layout {
  direction: 'row' | 'column';
  gap: number; // 0..400
  row_gap?: number; // solo con wrap
  padding: [number, number, number, number]; // arriba, derecha, abajo, izquierda
  align: 'start' | 'center' | 'end' | 'baseline'; // eje cruzado (align-items)
  justify: 'start' | 'center' | 'end' | 'space-between' | 'space-around' | 'space-evenly';
  wrap: boolean;
  columns?: number; // 1..12; solo con direction 'row' y wrap: columnas iguales
}
export interface Estilo {
  background?: ColorValor | FondoImagen;
  color?: ColorValor; // color del texto; se hereda
  border?: { width: number; color: ColorValor; style: 'solid' | 'dashed' };
  radius?: number | '$sm' | '$md' | '$lg';
  shadow?: 'none' | 'sm' | 'md' | 'lg';
  opacity?: number;
  text?: EstiloTexto;
  overflow?: 'visible' | 'hidden';
}
export interface FondoImagen {
  image: ImagenRef;
  fit: 'cover' | 'contain';
  position: [number, number];
  overlay?: ColorValor;
}
export interface EstiloTexto {
  preset?: (typeof ESTILOS_TEXTO)[number];
  font?: 'heading' | 'body';
  size?: number;
  weight?: 400 | 500 | 600 | 700 | 800;
  line_height?: number;
  letter_spacing?: number;
  align?: 'start' | 'center' | 'end' | 'justify';
  transform?: 'none' | 'uppercase';
  italic?: boolean;
}
/** Sobrescritura de un punto de corte: se mezcla sobre el anterior (móvil hereda de tableta).
 *  TRES ESTADOS: ausente = hereda; valor = sustituye; `null` = vuelve al valor por defecto (quita lo heredado). */
export interface Delta {
  hidden?: boolean; // false = «muéstralo en este punto de corte»
  size?: {
    width?: Medida;
    height?: Medida;
    min_width?: number | null;
    max_width?: number | null;
    min_height?: number | null;
    max_height?: number | null;
    aspect_ratio?: number | null;
  };
  layout?: Partial<Layout>; // solo section, frame y form
  style?: Omit<Partial<Estilo>, 'background' | 'border' | 'shadow'> & {
    background?: ColorValor | FondoImagen | null;
    border?: Estilo['border'] | null;
    shadow?: Estilo['shadow'] | null;
  };
  columns?: number; // solo product_grid, 1..6
}

export type Nodo =
  | Marco
  | Texto
  | Imagen
  | Boton
  | Icono
  | Incrustado
  | Formulario
  | Tabla
  | RejillaProductos
  | BotonCarrito
  | SelectorIdioma
  | Menu
  | Instancia;
export interface Seccion extends NodoBase {
  type: 'section';
  kind: (typeof TIPOS_SECCION)[number];
  anchor?: string; // /^[a-z][a-z0-9-]{0,39}$/, único en la página CONTANDO cabecera y pie; nunca ANCLAS_RESERVADAS
  full_bleed: boolean; // false ⇒ contenido centrado a theme.container
  layout: Layout;
  children: Nodo[];
} // size de una sección: width solo 'fill'; height 'hug' o px (se pinta como min-height)
export interface Marco extends NodoBase {
  type: 'frame';
  tag: 'div' | 'header' | 'footer' | 'nav' | 'article' | 'aside' | 'ul' | 'ol' | 'li';
  layout: Layout;
  children: Nodo[];
  link?: Enlace; // solo con tag 'div'
}
export interface Texto extends NodoBase {
  type: 'text';
  tag: 'h1' | 'h2' | 'h3' | 'h4' | 'h5' | 'h6' | 'p' | 'span' | 'blockquote';
  content: TextosRicos;
}
export type ImagenRef =
  | { asset: string }
  | { library: string }
  | { placeholder: 'photo' | 'portrait' | 'product' | 'landscape' | 'logo' };
export interface Imagen extends NodoBase {
  type: 'image';
  src: ImagenRef;
  alt: Textos;
  decorative?: true;
  fit: 'cover' | 'contain';
  link?: Enlace;
  priority?: true; // imagen del LCP: sin lazy, fetchpriority=high
}
export interface Boton extends NodoBase {
  type: 'button';
  label: Textos;
  variant: 'primary' | 'secondary' | 'ghost';
  action: Enlace;
  icon?: (typeof ICONOS)[number];
  icon_position?: 'start' | 'end';
}
export interface Icono extends NodoBase {
  type: 'icon';
  name: (typeof ICONOS)[number];
  label?: Textos;
}
export interface Incrustado extends NodoBase {
  type: 'embed';
  provider: (typeof PROVEEDORES_INCRUSTADO)[number];
  ref: string; // osm 'lat,lon,zoom' · cast uuid · youtube [A-Za-z0-9_-]{11} · vimeo [0-9]{6,12}
  title: Textos;
}
export interface Formulario extends NodoBase {
  type: 'form';
  form: Id;
  layout: Layout;
  fields: CampoFormulario[];
  submit: Textos;
  success: Textos;
  consent: Textos; // el formulario pinta su propio botón de envío (primary)
}
export interface CampoFormulario {
  id: Id;
  kind: 'text' | 'email' | 'tel' | 'textarea' | 'select' | 'checkbox' | 'date' | 'time' | 'number';
  label: Textos;
  required: boolean;
  options?: Textos[]; // solo select; el navegador envía el ÍNDICE de la opción
  max_length?: number; // 1..5000; solo text, email, tel y textarea
  min?: number;
  max?: number; // solo number; enteros
  autocomplete?:
    | 'name'
    | 'given-name'
    | 'family-name'
    | 'organization'
    | 'email'
    | 'tel'
    | 'street-address'
    | 'postal-code'
    | 'address-level2'
    | 'country-name';
}
export interface Tabla extends NodoBase {
  type: 'table';
  caption: Textos;
  header: Textos[];
  rows: Textos[][]; // solo texto; ≤ filasTabla × columnasTabla
}
export interface RejillaProductos extends NodoBase {
  type: 'product_grid';
  source: { all: true } | { category: string } | { products: string[] }; // uuid de products
  limit: number;
  columns: number;
  show_price: boolean;
  show_add_to_cart: boolean;
}
export interface BotonCarrito extends NodoBase {
  type: 'cart_button';
  label: Textos;
}
export interface SelectorIdioma extends NodoBase {
  type: 'language_switcher';
  display: 'code' | 'name';
}
export interface Menu extends NodoBase {
  type: 'menu';
  items: ElementoMenu[];
  orientation: 'horizontal' | 'vertical';
  collapse_below: 'never' | 'tablet' | 'mobile'; // colapsa en ESE punto de corte y en los menores
  label?: Textos; // nombre accesible del <nav> (cabecera, pie…)
}
export interface ElementoMenu {
  id: Id;
  label: Textos;
  link: Enlace;
  children?: ElementoMenu[];
}
export interface Instancia extends NodoBase {
  type: 'instance';
  component: Id;
  overrides?: Record<Id, Override>;
}
export interface Override {
  text?: TextosRicos;
  label?: Textos;
  image?: ImagenRef;
  alt?: Textos;
  link?: Enlace | null; // también sustituye button.action; null = quita el enlace del maestro
  icon?: (typeof ICONOS)[number]; // icon.name y button.icon
  hidden?: boolean;
  style?: Partial<Estilo>;
}
export interface Componente {
  id: Id;
  name: string;
  root: Marco;
} // un maestro no contiene instancias ni formularios
export type Enlace =
  | { page: Id; anchor?: string }
  | { anchor: string }
  | { url: string; new_tab?: boolean }
  | { product: string /* uuid de products */ }
  | { file: string /* uuid de assets con ext 'pdf' */ }
  | { legal: 'notice' | 'privacy' | 'cookies' | 'terms' | 'withdrawal' }
  | { cart: true };
export interface SeoSitio {
  site_name: Textos;
  og_image?: ImagenRef;
  favicon?: ImagenRef;
  noindex?: true;
  business?: Negocio;
}
export interface Negocio {
  type:
    | 'Organization'
    | 'LocalBusiness'
    | 'Store'
    | 'Restaurant'
    | 'MedicalClinic'
    | 'LodgingBusiness'
    | 'EducationalOrganization'
    | 'ProfessionalService'
    | 'HomeAndConstructionBusiness';
  phone?: string;
  email?: string;
  address?: { street: string; city: string; postal_code: string; region?: string; country: string }; // ISO 3166-1 alfa-2
  geo?: { lat: number; lon: number };
  opening_hours?: {
    days: ('Mo' | 'Tu' | 'We' | 'Th' | 'Fr' | 'Sa' | 'Su')[];
    opens: string;
    closes: string;
  }[]; // 'HH:MM'
}
