/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * CONTRATO DE UI DECLARATIVA — el puente entre los componentes web y las apps
 * nativas SIN webviews.
 *
 * Cada pantalla de la suite se DECLARA aquí una sola vez como árbol de nodos,
 * con las mismas variantes y semántica que sus componentes web (AppShell,
 * Card, EmptyState, Badge…). La suite en Gio (web en WebAssembly, escritorio y
 * móvil) embebía este árbol y lo pintaba con un renderer de widgets equivalentes;
 * se retiró el 2026-09-23 y hoy no lo pinta nadie.
 *
 * NO es server-driven UI: el árbol va embebido en el binario (go:embed) y los
 * DATOS (items, contadores, sesiones) los inyecta la vista nativa en runtime
 * en los `slots` declarados — el mismo patrón de componentes con props.
 *
 * Tipos cerrados a propósito: un tipo que el renderer no conoce debe FALLAR
 * al construir, no pintarse como un hueco invisible.
 */

export type EstadoVariante = 'principal' | 'secundario' | 'exito' | 'aviso' | 'peligro';

export type NodoUI =
  /** La AppShell de la suite: rail + panel secundario + contenido. */
  | { tipo: 'shell'; rail: NodoUI[]; panel?: NodoUI[]; contenido: NodoUI[] }
  /** Card de la web: superficie + borde + esquina continua. */
  | { tipo: 'tarjeta'; titulo?: string; hijos: NodoUI[]; clave?: string }
  /** Sección vertical u horizontal. */
  | { tipo: 'columna'; hijos: NodoUI[]; separacion?: 'apretada' | 'normal' | 'ancha' }
  | { tipo: 'fila'; hijos: NodoUI[]; separacion?: 'apretada' | 'normal' | 'ancha' }
  /** Texto con la escala tipográfica de la web. */
  | {
      tipo: 'texto';
      contenido: string;
      escala?: 'titulo' | 'seccion' | 'cuerpo' | 'detalle' | 'micro';
      peso?: 'normal' | 'medio' | 'negrita';
      variante?: EstadoVariante | 'mudo';
      clave?: string;
    }
  /** Botón; `accion` es el nombre de la acción que la vista nativa conecta. */
  | {
      tipo: 'boton';
      texto: string;
      accion: string;
      variante?: EstadoVariante;
      icono?: string;
      clave?: string;
    }
  /** Campo de texto; `accion` conecta OnChanged/OnSubmitted. */
  | {
      tipo: 'entrada';
      placeholder: string;
      accion?: 'buscar' | 'mensaje' | 'titulo';
      clave?: string;
    }
  /** EmptyState con ilustración de marca. */
  | { tipo: 'vacio'; spot: string; titulo: string; texto?: string; clave?: string }
  /** Lista virtualizada; los datos los pone la vista (slot `items`). */
  | { tipo: 'lista'; slot: string; item: NodoUI[] }
  /** Badge numérico de contador (no leídos, pendientes…). */
  | { tipo: 'contador'; slot: string; variante?: EstadoVariante }
  /** Divisor del sistema. */
  | { tipo: 'divisor' }
  /** Espaciador fijo o flexible. */
  | { tipo: 'espacio'; tamano?: 'pequeno' | 'medio' | 'normal' | 'grande' | 'flexible' }
  /** Icono de producto o del sistema. */
  | { tipo: 'icono'; nombre: string; tamano?: number }
  /** Avatar de usuario o contacto. */
  | { tipo: 'avatar'; nombre: string; slot?: string }
  /** Barra de título o acciones. */
  | { tipo: 'barra'; titulo: string; acciones?: NodoUI[]; clave?: string }
  /** Ranura dinámica para inyección nativa (custom widget). */
  | { tipo: 'ranura'; nombre: string };

/** Helper de construcción para que las declaraciones de pantallas sean legibles. */
/** Los tipos de nodo que llevan un rótulo escrito: los que `ui.t` puede marcar. */
const CON_ROTULO: ReadonlySet<NodoUI['tipo']> = new Set([
  'texto',
  'boton',
  'entrada',
  'vacio',
  'barra',
  'tarjeta',
]);

export const ui = {
  /**
   * Marca un nodo con la CLAVE DE CATÁLOGO de su rótulo: `ui.t('note.new', ui.boton('Nueva
   * nota', …))`.
   *
   * Los rótulos de estas pantallas iban en español a fuego, y una pantalla declarada una
   * vez para todas las plataformas salía en español para todo el mundo: en el escritorio,
   * un usuario alemán leía «Nueva nota». La clave es la del catálogo i18n de la app
   * (`apps/<app>/src/i18n`), o la del común; quien pinta la traduce.
   *
   * El literal SE QUEDA: es el respaldo si el catálogo no trae la clave, y lo que sigue
   * leyendo un renderer que no conozca este campo. Por eso es un envoltorio y no un
   * parámetro más de cada constructor.
   */
  t: (clave: string, nodo: NodoUI): NodoUI => {
    // Tipos cerrados también aquí: una clave en un nodo sin rótulo (una lista, un
    // divisor) no la traduciría nadie, y eso tiene que verse al construir el árbol.
    if (!CON_ROTULO.has(nodo.tipo)) throw new Error(`ui.t: un nodo «${nodo.tipo}» no lleva rótulo`);
    return { ...nodo, clave } as NodoUI;
  },
  shell: (rail: NodoUI[], contenido: NodoUI[], panel?: NodoUI[]): NodoUI => ({
    tipo: 'shell',
    rail,
    contenido,
    ...(panel ? { panel } : {}),
  }),
  columna: (
    separacion: 'apretada' | 'normal' | 'ancha' = 'normal',
    ...hijos: NodoUI[]
  ): NodoUI => ({
    tipo: 'columna',
    hijos,
    separacion,
  }),
  fila: (separacion: 'apretada' | 'normal' | 'ancha' = 'normal', ...hijos: NodoUI[]): NodoUI => ({
    tipo: 'fila',
    hijos,
    separacion,
  }),
  texto: (
    contenido: string,
    escala: 'titulo' | 'seccion' | 'cuerpo' | 'detalle' | 'micro' = 'cuerpo',
    peso: 'normal' | 'medio' | 'negrita' = 'normal',
    variante: EstadoVariante | 'mudo' = 'principal',
  ): NodoUI => ({ tipo: 'texto', contenido, escala, peso, variante }),
  boton: (
    texto: string,
    accion: string,
    variante: EstadoVariante = 'secundario',
    icono?: string,
  ): NodoUI => ({
    tipo: 'boton',
    texto,
    accion,
    variante,
    ...(icono ? { icono } : {}),
  }),
  entrada: (placeholder: string, accion: 'buscar' | 'mensaje' | 'titulo'): NodoUI => ({
    tipo: 'entrada',
    placeholder,
    accion,
  }),
  tarjeta: (hijos: NodoUI[], titulo?: string): NodoUI => ({
    tipo: 'tarjeta',
    hijos,
    ...(titulo ? { titulo } : {}),
  }),
  lista: (slot: string, item: NodoUI[]): NodoUI => ({ tipo: 'lista', slot, item }),
  contador: (slot: string, variante?: EstadoVariante): NodoUI => ({
    tipo: 'contador',
    slot,
    ...(variante ? { variante } : {}),
  }),
  vacio: (spot: string, titulo: string, texto?: string): NodoUI => ({
    tipo: 'vacio',
    spot,
    titulo,
    ...(texto ? { texto } : {}),
  }),
  divisor: (): NodoUI => ({ tipo: 'divisor' }),
  espacio: (tamano: 'pequeno' | 'medio' | 'normal' | 'grande' | 'flexible' = 'normal'): NodoUI => ({
    tipo: 'espacio',
    tamano,
  }),
  icono: (nombre: string, tamano?: number): NodoUI => ({
    tipo: 'icono',
    nombre,
    ...(tamano ? { tamano } : {}),
  }),
  avatar: (nombre: string, slot?: string): NodoUI => ({
    tipo: 'avatar',
    nombre,
    ...(slot ? { slot } : {}),
  }),
  barra: (titulo: string, acciones?: NodoUI[]): NodoUI => ({
    tipo: 'barra',
    titulo,
    ...(acciones ? { acciones } : {}),
  }),
  ranura: (nombre: string): NodoUI => ({ tipo: 'ranura', nombre }),
};
