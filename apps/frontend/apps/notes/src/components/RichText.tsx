/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * RichText — el editor de una nota de texto y su barra, que es la otra mitad
 * del producto: lo que tenía el BasKet clásico de KDE (negrita, cursiva,
 * subrayado, tachado, color, alineación, listas, sangría, enlaces) y lo que le
 * faltaba (títulos hasta h5, cita, código, resaltado, deshacer y rehacer, y el
 * enlace a otra nota o a un título de otra nota escribiendo su nombre, en la
 * herramienta de enlaces o tras `[[`; y «@» para mencionar a alguien).
 *
 * Se apoya en el adaptador de edición compartido de `@sytue/ui`, el mismo que
 * usa el compositor de Sytue Mail: dos implementaciones del mismo gesto acaban
 * comportándose distinto en la tercera pulsación. `styleWithCSS` va activado
 * para que color y alineación salgan como `style` —que es lo que el saneador
 * del servicio conserva— y no como `<font>` o `align=`, que tiraría. Y APAGADO
 * para negrita, cursiva, subrayado y tachado (`ordenar`): con él encendido salían
 * como `<span style="font-weight: bold">`, y el saneador solo deja en `style` el
 * color y la alineación, así que la negrita se perdía al guardar. Los colores, que el
 * navegador escribe en `rgb()`, se entregan en `#rrggbb` (`client/salida.ts`).
 *
 * EL PEGADO CONSERVA LA ESTRUCTURA Y TOMA EL ESTILO DE LA NOTA. Insertar el HTML
 * del portapapeles tal cual lo EJECUTARÍA aquí y ahora, antes de que ningún
 * servidor pueda opinar: pegar desde una página hostil sería XSS en tu propia
 * sesión. Por eso pasa por `pegadoConFormato` (`@sytue/ui/browser/portapapeles`),
 * que reconstruye un árbol nuevo con lista blanca —títulos, negrita, cursiva,
 * listas, enlaces…— sin copiar ningún atributo salvo un `href` seguro: ni
 * manejadores, ni `style`, ni imágenes. Sin HTML, entra como texto; y si Safari
 * solo trae HTML, también. El texto que parece Markdown (dos o más marcas de
 * bloque) llega con su formato, por el mismo saneado; Ctrl/⌘+Mayús+V lo pega
 * plano. La decisión vive en `queSePega` (`client/markdown.ts`).
 */
import { Fragment, type JSX } from 'preact';
import { useCallback, useEffect, useRef, useState } from 'preact/hooks';
import { createPortal } from 'preact/compat';
import {
  BOTON_DE_BARRA,
  Button,
  Dropdown,
  Icon,
  Input,
  MenuItem,
  MenuSeparator,
  Popover,
  SEPARADOR_DE_BARRA,
  ShortcutsModal,
  Spinner,
  Toolbar,
  Tooltip,
  cx,
  useFitInViewport,
  useMobileBack,
  atajoDelSistema,
  type IconName,
} from '@sytue/ui';
import { candidatos, consultaMencion, moverActiva, type Persona } from '@sytue/shell';
import {
  editingCommandEnabled,
  editingCommandState,
  editingCommandValue,
  executeEditingCommand,
} from '@sytue/ui/browser/editing';
import {
  celdaDe,
  eliminarColumna,
  eliminarFila,
  eliminarTabla,
  insertarColumna,
  insertarFila,
} from '@sytue/ui/browser/tabla';
import type { NotesMessageKey, T } from '../i18n/index.js';
import { escapar } from '../client/captura.js';
import { atajoMarkdown, queSePega } from '../client/markdown.js';
import { normalizarSalida } from '../client/salida.js';
import { atajoEnLinea } from '../client/autoformato.js';
import { estadisticas, textoDeNota } from '../client/extracto.js';
import { atajosDelEditor } from './atajos-editor.js';
import {
  consultaDeBarra,
  consultaDeEnlace,
  filtrarComandos,
  type Comando,
  type IdComando,
} from '../client/comandos.js';
import { coincidenciasEn, liPlegable } from '../client/en-nota.js';
import { Glyph, type GlyphName } from './glyphs.js';
import { COLORES_DE_TEXTO, nombreDeColor } from './colores.js';
import { construirPeticion, leerRespuesta, type AccionIA } from '../lib/ai.js';
import { bloquesDe, type Bloque } from '@sytue/ai';
import { LinkTool, useOpcionesDeEnlace, type FuentesDeEnlace, type Opcion } from './LinkTool.js';
import { BarraConMas, OpcionDeMas, type ItemDeBarra } from './BarraConMas.js';
import { DibujoDeComando, MenuInsertar } from './MenuInsertar.js';
import { SelectorDeSimbolos } from './SelectorDeSimbolos.js';
import {
  apuntarReciente,
  buscarSimbolos,
  consultaDeEmoji,
  type IdCategoria,
} from '../client/simbolos.js';
import {
  codigoDe,
  contextoDe,
  entrarEnNuevo,
  fechaLegible,
  hoyISO,
  lineasDe,
  plantillaColumnas,
  plantillaDecisiones,
  plantillaEstado,
  plantillaExpandir,
  plantillaFecha,
  plantillaPanel,
  type Contexto,
  type TipoDePanel,
} from './bloques.js';
import {
  alinearColumna,
  alternarCabeceraDeColumna,
  alternarCabeceraDeFila,
  alternarFilasNumeradas,
  celdaEn,
  celdaVecina,
  entrarEnCelda,
  fondoDeCelda,
  posicionDe,
} from './tabla-notas.js';
import { BarraDeBloque, type AccionesDeBloque } from './BarraDeBloque.js';
import { copiarRango, leerPortapapeles } from './portapapeles-editor.js';
import {
  bloqueDe,
  duplicarBloque,
  enPanel,
  llevarBloque,
  moverBloque,
  quitarBloque,
} from './mover-bloques.js';
import { documentLocale } from '@sytue/i18n';

interface Props {
  t: T;
  /** HTML inicial. Se fija UNA vez al montar; el padre remonta con `key`. */
  initialHtml: string;
  ariaLabel: string;
  placeholder?: string;
  onChange: (html: string) => void;
  /** Sale del editor: Escape (false) o Ctrl/Cmd+Enter (true). Quien lo monta decide
   *  qué hacer con lo escrito; NoteCard guarda en los dos casos. */
  onDone?: (guardar: boolean) => void;
  /** Notas y títulos que la herramienta de enlaces puede ofrecer. */
  enlaces?: FuentesDeEnlace;
  /**
   * La tarjeta que lo contiene está estirada: el editor crece con ella y la zona
   * de escribir llega hasta abajo, en vez de quedarse en las primeras líneas.
   */
  estirado?: boolean;
  /**
   * Sin anillo de foco alrededor del cuerpo. En la hoja del árbol y en la fila de
   * lista o diario el editor ES la página («papel», ADR 0079): un recuadro azul
   * alrededor del texto la parte en dos, y el foco ya se ve en el cursor y en la
   * barra de formato abierta encima, como en Apple Notas o Bear. En una tarjeta
   * (columnas, lienzo) el aro se queda: allí sí delimita qué ficha se edita.
   */
  sinAro?: boolean;
  /**
   * La hoja del árbol: a todo el ancho del panel (`true`) o estrechada a la columna de
   * lectura. Con `onAncho`, la barra ofrece «Ancho completo» como en Confluence; la
   * elección es de la nota (`data.reading_width`, NoteCard).
   */
  anchoCompleto?: boolean;
  onAncho?: () => void;
}

interface Accion {
  cmd: string;
  valor?: string;
  glyph?: GlyphName;
  icon?: IconName;
  label: string;
  atajo?: string;
  /** Con estado (aria-pressed) o solo acción. */
  conEstado?: boolean;
  desactivada?: boolean;
}

/**
 * Los estilos de texto del desplegable, con su VISTA PREVIA: cada opción se escribe con
 * el aspecto que dará (como en Confluence), con las mismas escalas que la prosa.
 */
const BLOQUES: {
  valor: string;
  clave: 'paragraph' | 'h1' | 'h2' | 'h3' | 'h4' | 'h5' | 'h6' | 'quote' | 'codeBlock';
  vista: string;
}[] = [
  { valor: 'p', clave: 'paragraph', vista: 'text-sm' },
  { valor: 'h1', clave: 'h1', vista: 'text-xl font-semibold leading-tight' },
  { valor: 'h2', clave: 'h2', vista: 'text-lg font-semibold leading-tight' },
  { valor: 'h3', clave: 'h3', vista: 'text-base font-semibold' },
  { valor: 'h4', clave: 'h4', vista: 'text-sm font-semibold' },
  { valor: 'h5', clave: 'h5', vista: 'text-xs font-semibold uppercase tracking-wide' },
  { valor: 'h6', clave: 'h6', vista: 'text-text-muted text-xs font-semibold' },
  {
    valor: 'blockquote',
    clave: 'quote',
    vista: 'border-border-strong text-text-muted border-l-2 pl-2 text-sm',
  },
  { valor: 'pre', clave: 'codeBlock', vista: 'font-mono text-xs' },
];

/** Colores de texto legibles sobre claro y oscuro. El primero es «por defecto». */
const COLORES = COLORES_DE_TEXTO;
const RESALTADO = '#fff3a3';

/** Las órdenes que tienen que salir como etiqueta (`<b>`, `<i>`, `<u>`, `<strike>`, `<sub>`, `<sup>`). */
const ETIQUETADAS = new Set([
  'bold',
  'italic',
  'underline',
  'strikeThrough',
  'subscript',
  'superscript',
]);

/** Ctrl/⌘ + letra → la orden que conmuta. */
const TECLAS_DE_FORMATO: Record<string, string | undefined> = {
  b: 'bold',
  i: 'italic',
  u: 'underline',
};

/** Los grupos de la barra, en su orden: historia | bloque | formato | color | listas | insertar | herramientas. */
type GrupoDeBarra =
  'historia' | 'bloque' | 'formato' | 'color' | 'listas' | 'insertar' | 'herramientas';

/** El atajo de cada tipo de bloque, para el menú: Ctrl+Alt+0 el párrafo, Ctrl+Alt+1…5 los títulos. */
const ATAJO_BLOQUE: Record<string, string | undefined> = {
  p: 'Ctrl+Alt+0',
  h1: 'Ctrl+Alt+1',
  h2: 'Ctrl+Alt+2',
  h3: 'Ctrl+Alt+3',
  h4: 'Ctrl+Alt+4',
  h5: 'Ctrl+Alt+5',
  h6: 'Ctrl+Alt+6',
};

/** Lo que ofrece el asistente, en su orden (las tres primeras ordenan, las demás reescriben). */
const ACCIONES_IA: readonly (readonly [AccionIA, NotesMessageKey])[] = [
  ['ordenar', 'editor.ai.tidy'],
  ['tareas', 'editor.ai.tasks'],
  ['titular', 'editor.ai.title'],
  ['resumir', 'editor.ai.summarize'],
  ['ampliar', 'editor.ai.expand'],
  ['corregir', 'editor.ai.fix'],
  ['traducir', 'editor.ai.translate'],
];

/**
 * Una orden de edición con el `styleWithCSS` que le toca (ver la cabecera). Se deja
 * puesto a propósito: con el cursor sin selección, el navegador decide el marcado al
 * TECLEAR lo siguiente, no al conmutar.
 */
function ordenar(cmd: string, valor?: string): boolean {
  executeEditingCommand('styleWithCSS', ETIQUETADAS.has(cmd) ? 'false' : 'true');
  return executeEditingCommand(cmd, valor);
}

/** Los menús que se abren al teclear: un bloque, una nota que enlazar, alguien, un emoji. */
type TipoMenu = 'cmd' | 'nota' | 'persona' | 'emoji';

/**
 * Lo que abre cada menú y cómo se lee lo escrito tras ello. `[[` va primero: el nombre
 * de una nota puede llevar « /», y mientras no se cierre se está buscando una nota.
 */
const DISPAROS: readonly (readonly [TipoMenu, string, (antes: string) => string | null])[] = [
  ['nota', '[[', consultaDeEnlace],
  ['persona', '@', consultaMencion],
  ['cmd', '/', consultaDeBarra],
  ['emoji', ':', consultaDeEmoji],
];

const ROTULO_MENU = {
  cmd: 'editor.slashMenu',
  nota: 'editor.linkNote',
  persona: 'editor.mention',
  emoji: 'editor.emoji',
} as const;

/** Una fila de cualquiera de los tres menús. */
interface Sugerencia {
  clave: string;
  dibujo: JSX.Element;
  rotulo: string;
  /** El título dentro de la nota, tras «›». */
  dentro?: string;
  /** Una línea bajo el rótulo: qué hace el elemento (catálogo de «/»). */
  descripcion?: string;
  /** A la derecha y en gris: el atajo del comando, el cuaderno, el correo. */
  detalle?: string;
  /** El grupo al que pertenece (este cuaderno, otros cuadernos). */
  grupo?: string;
  elegir: () => void;
}

/** Id del cuadro de «Buscar en la nota», para darle el foco al abrirlo. */
const ID_BUSCAR = 'notes-buscar-en-nota';
const ID_REEMPLAZO = 'notes-reemplazar-en-nota';

/** El registro de resaltados de CSS (`CSS.highlights`), donde lo hay. */
function registroResaltados(): {
  set: (k: string, v: unknown) => void;
  delete: (k: string) => void;
} | null {
  const css = (globalThis as { CSS?: { highlights?: unknown } }).CSS;
  const h = (globalThis as { Highlight?: unknown }).Highlight;
  return css?.highlights && h ? (css.highlights as ReturnType<typeof registroResaltados>) : null;
}

function resaltar(nombre: string, rangosDom: Range[]): void {
  const reg = registroResaltados();
  const H = (globalThis as { Highlight?: new (...r: Range[]) => unknown }).Highlight;
  if (!reg || !H) return;
  if (rangosDom.length === 0) reg.delete(nombre);
  else reg.set(nombre, new H(...rangosDom));
}

/**
 * Botón de la barra de formato. Va FUERA de `RichText` a propósito: declarado dentro,
 * cada render del editor creaba un tipo de componente nuevo y Preact desmontaba y
 * volvía a montar todos los botones. Basta con que la nota guarde al perder el foco el
 * texto (que es lo que pasa al enfocar un botón con el teclado) para que el botón
 * recién enfocado desaparezca y el Intro caiga en el `body`: la cursiva no se aplicaba.
 */
function BotonDeBarra({
  a,
  onPress,
  activos,
  t,
}: {
  a: Accion;
  onPress: () => void;
  activos: Record<string, boolean>;
  t: T;
}): JSX.Element {
  return (
    <Tooltip
      label={
        a.atajo ? t('editor.shortcut', { label: a.label, keys: atajoDelSistema(a.atajo) }) : a.label
      }
    >
      <button
        type="button"
        // El estado activo lo pinta `aria-pressed` (BOTON_DE_BARRA): lo que se ve y lo
        // que oye el lector de pantalla no pueden separarse.
        class={BOTON_DE_BARRA}
        aria-label={a.label}
        disabled={a.desactivada}
        {...(a.conEstado ? { 'aria-pressed': activos[a.cmd] ?? false } : {})}
        // Conservar el cursor al apoyar; activar en click también permite usar
        // Intro/Espacio y la activación táctil normal del botón.
        onMouseDown={(e) => {
          e.preventDefault();
        }}
        onClick={onPress}
      >
        {a.icon ? <Icon name={a.icon} size={16} /> : <Glyph name={a.glyph ?? 'note'} size={16} />}
      </button>
    </Tooltip>
  );
}

export function RichText({
  t,
  initialHtml,
  ariaLabel,
  placeholder,
  onChange,
  onDone,
  enlaces,
  estirado = false,
  sinAro = false,
  anchoCompleto,
  onAncho,
}: Props): JSX.Element {
  const ref = useRef<HTMLDivElement>(null);
  // Qué comandos están activos en la selección actual. Sin esto, el botón de
  // negrita se ve igual dentro y fuera de un texto en negrita y hay que pulsarlo
  // para averiguar en qué estado se estaba.
  const [activos, setActivos] = useState<Record<string, boolean>>({});
  const [bloque, setBloque] = useState('p');
  const [vacio, setVacio] = useState(initialHtml.trim() === '');
  // Palabras, caracteres y lectura del pie, las mismas cuentas que «Información»
  // (`estadisticas`, igual que Fyne). Se recalculan un momento después de escribir.
  const [cuentas, setCuentas] = useState(() => estadisticas(textoDeNota(initialHtml)));
  const recuento = useRef<ReturnType<typeof setTimeout> | null>(null);
  // La hoja de atajos (Ctrl/⌘+/).
  const [atajos, setAtajos] = useState(false);
  // El asa ⋮⋮ del bloque bajo el puntero o el del cursor (con el dedo, siempre a la
  // vista), su menú, y el arrastre: qué bloque se lleva y dónde caería.
  const [asa, setAsa] = useState<{ el: HTMLElement; top: number } | null>(null);
  const [menuBloque, setMenuBloque] = useState<{ x: number; y: number } | null>(null);
  const bloqueDelMenu = useRef<HTMLElement | null>(null);
  const arrastrado = useRef<HTMLElement | null>(null);
  const [soltarEn, setSoltarEn] = useState<{
    top: number;
    destino: HTMLElement;
    antes: boolean;
  } | null>(null);
  const [enlaceAbierto, setEnlaceAbierto] = useState(false);
  const [colorAbierto, setColorAbierto] = useState(false);
  // El «+» de la barra y el selector de símbolos (`true`) o de emoji (`'emoji'`).
  const [insertarAbierto, setInsertarAbierto] = useState(false);
  const [simbolosAbierto, setSimbolosAbierto] = useState<IdCategoria | true | false>(false);
  // Un bloque elegido a propósito (la fecha recién puesta con «Fecha», que abre su
  // calendario): manda sobre el que se deduce del cursor.
  const [elegido, setElegido] = useState<HTMLElement | null>(null);
  // El bloque con barra propia donde está el cursor (tabla, código, panel, estado,
  // fecha, sección): su barra flota junto a él (BarraDeBloque), como en Confluence. La
  // tabla sigue usando el módulo común de Mail (@sytue/ui/browser/tabla) para filas y
  // columnas: dos implementaciones del mismo gesto acaban comportándose distinto.
  const [contextoCursor, setContextoCursor] = useState<Contexto | null>(null);
  // Si el foco está en el editor o en lo que cuelga de él (su barra flotante, un menú):
  // fuera, la barra del bloque se va.
  const [conFoco, setConFoco] = useState(true);
  // El menú de una celda con el botón derecho, en el punto donde se pidió.
  const [menuCelda, setMenuCelda] = useState<{ x: number; y: number } | null>(null);
  // Los números de línea de los bloques de código que los llevan: una capa encima del
  // campo, fuera del HTML de la nota (`data-lineas`).
  const [numeros, setNumeros] = useState<{ top: number; left: number; n: number }[]>([]);
  // La celda donde está el cursor, marcada con un recuadro (también una capa).
  const [celdaActiva, setCeldaActiva] = useState<{
    top: number;
    left: number;
    width: number;
    height: number;
  } | null>(null);
  // Si hay algo que deshacer o rehacer, según el navegador: sin esto los dos
  // botones se ven iguales con la pila vacía y con veinte cambios dentro.
  const [puede, setPuede] = useState({ undo: false, redo: false });
  // Las herramientas pueden tomar el foco (menús, colores, enlaces, teclado).
  // El rango sigue perteneciendo al texto hasta volver de ellas.
  const seleccion = useRef<Range | null>(null);
  /** Ctrl/⌘+Mayús+V: el pegado que viene es «sin formato». */
  const pegarSinFormato = useRef(false);
  /** Cómo deshacer el último atajo de Markdown, mientras sea lo último que se hizo. */
  const ultimoAuto = useRef<(() => void) | null>(null);

  const guardarSeleccion = useCallback(() => {
    const rango = window.getSelection();
    if (
      rango &&
      rango.rangeCount > 0 &&
      ref.current?.contains(rango.anchorNode) &&
      ref.current.contains(rango.focusNode)
    )
      seleccion.current = rango.getRangeAt(0).cloneRange();
  }, []);

  const enfocarTexto = () => {
    if (document.activeElement === ref.current) guardarSeleccion();
    const rango = seleccion.current;
    ref.current?.focus();
    if (
      rango &&
      ref.current?.contains(rango.startContainer) &&
      ref.current.contains(rango.endContainer)
    ) {
      const actual = window.getSelection();
      actual?.removeAllRanges();
      actual?.addRange(rango);
    }
  };

  // --- Menús al teclear -----------------------------------------------------
  //
  // Tres disparadores abren una lista junto al cursor: «/» al principio de una línea o
  // tras un blanco, los bloques que se pueden poner ahí (client/comandos.ts); «[[», una
  // nota que enlazar; «@», alguien a quien mencionar. Lo que se sigue escribiendo la
  // filtra, y el texto se queda en la nota mientras tanto: si no se elige nada, es lo
  // que se escribió. Las tres se llevan con el mismo teclado y se colocan igual.
  const [menu, setMenu] = useState<{
    tipo: TipoMenu;
    q: string;
    idx: number;
    x: number;
    y: number;
    /** Encima del disparador: debajo no cabe (`max-h-72` son 252 px). */
    arriba: boolean;
  } | null>(null);
  // El disparador al que se dijo «no» con Escape: no se vuelve a abrir en él.
  const descartada = useRef<{ nodo: Node; off: number } | null>(null);
  const rotuloCmd = (c: Comando) => t(c.clave);
  const descripcionCmd = (c: Comando) => t(c.descripcion);
  // `[[`: la búsqueda de la herramienta de enlaces, la misma, sin Drive.
  const notasMenu = useOpcionesDeEnlace(menu?.tipo === 'nota' ? menu.q : null, enlaces, false);
  // `@`: la gente se pide a la primera arroba y se filtra aquí.
  const [gente, setGente] = useState<readonly Persona[] | null>(null);
  const pidiendoGente = menu?.tipo === 'persona' && gente === null;
  const personas = enlaces?.personas;
  /** Lo que aquí se puede insertar: sin cuaderno no hay notas que enlazar, ni sin gente a quién. */
  const disponible = (c: Comando) =>
    (c.id !== 'enlace' || enlaces !== undefined) && (c.id !== 'mencion' || personas !== undefined);
  useEffect(() => {
    if (!pidiendoGente || !personas) return;
    let vivo = true;
    void personas().then((g) => vivo && setGente(g));
    return () => {
      vivo = false;
    };
  }, [pidiendoGente, personas]);
  const sugerencias = ((): Sugerencia[] => {
    if (menu?.tipo === 'cmd')
      return filtrarComandos(menu.q, rotuloCmd, descripcionCmd, disponible).map((c) => ({
        clave: c.id,
        dibujo: <DibujoDeComando id={c.id} size={15} />,
        rotulo: rotuloCmd(c),
        descripcion: descripcionCmd(c),
        ...(c.pista ? { detalle: c.pista } : {}),
        elegir: () => ejecutarComando(c),
      }));
    // `:son` ofrece los emoji cuyas palabras empiezan así; ocho, que es lo que se lee.
    if (menu?.tipo === 'emoji')
      return buscarSimbolos(menu.q, 'emoji')
        .slice(0, 8)
        .map((x) => ({
          clave: x.s,
          dibujo: <span class="text-base leading-none">{x.s}</span>,
          rotulo: `:${x.claves.split(' ')[0]}`,
          elegir: () => ponerEmoji(x.s),
        }));
    if (menu?.tipo === 'nota')
      return notasMenu.opciones.map((o) => ({
        clave: o.clave,
        dibujo: <Glyph name={o.titulo ? 'heading' : 'note'} size={15} />,
        rotulo: o.nota,
        ...(o.titulo ? { dentro: o.titulo } : {}),
        ...(o.cuaderno ? { detalle: o.cuaderno } : {}),
        grupo: t(o.tipo === 'local' ? 'editor.linkThisNotebook' : 'editor.linkOtherNotebooks'),
        elegir: () => enlazarNota(o),
      }));
    // Sin nada tras la arroba no se ofrece a nadie: una «@» suelta es una arroba.
    if (menu?.tipo === 'persona' && menu.q)
      return candidatos(gente ?? [], menu.q).map((p) => ({
        clave: p.email,
        dibujo: <Icon name="user" size={15} />,
        rotulo: p.name ?? p.email,
        ...(p.name ? { detalle: p.email } : {}),
        elegir: () => mencionar(p),
      }));
    return [];
  })();
  // `[[` y «/» se ven aunque no haya nada: dicen que buscan, o que nada casa. Los otros
  // dos, solo con algo que ofrecer: una «@» o unos dos puntos sueltos son texto.
  const abierto =
    menu !== null && (sugerencias.length > 0 || menu.tipo === 'nota' || menu.tipo === 'cmd');
  const activa = Math.min(menu?.idx ?? 0, Math.max(sugerencias.length - 1, 0));
  const menuRef = useRef<HTMLDivElement>(null);
  useFitInViewport(menuRef, abierto ? menu : null, { vertical: true });
  useMobileBack(abierto, () => {
    const d = disparoEnCursor();
    descartada.current = d ? { nodo: d.nodo, off: d.off } : null;
    setMenu(null);
  });
  // La fila activa a la vista: el menú se desplaza (`max-h-72`) y las flechas bajan.
  const claveActiva = abierto ? sugerencias[activa]?.clave : undefined;
  useEffect(() => {
    if (claveActiva !== undefined)
      document.getElementById(`notes-sug-${activa}`)?.scrollIntoView({ block: 'nearest' });
  }, [claveActiva, activa]);

  // --- Burbuja de formato ---------------------------------------------------
  //
  // Encima de lo seleccionado, con lo que se hace con una selección: negrita,
  // cursiva, tachado, resaltado, código y enlace. La barra de arriba sigue ahí;
  // la burbuja es para no tener que subir a buscarla.
  const [burbuja, setBurbuja] = useState<{
    x: number;
    y: number;
    /** Con el dedo va DEBAJO de la selección: encima sale el menú del sistema. */
    abajo: boolean;
    /** En una pantalla estrecha, tachado, resaltado y código van en «Más formato». */
    estrecha: boolean;
  } | null>(null);
  // «Pegar» de la burbuja sin permiso para leer el portapapeles: se dice cómo pegar.
  const [avisoPegar, setAvisoPegar] = useState(false);
  useEffect(() => {
    if (!avisoPegar) return;
    const t = window.setTimeout(() => setAvisoPegar(false), 6000);
    return () => window.clearTimeout(t);
  }, [avisoPegar]);
  const burbujaRef = useRef<HTMLDivElement>(null);
  useFitInViewport(burbujaRef, burbuja && !menu ? burbuja : null, { vertical: true });

  // --- Buscar y reemplazar en la nota --------------------------------------
  const [buscador, setBuscador] = useState<{
    q: string;
    i: number;
    /** «Coincidir mayúsculas»: literal, mayúsculas y acentos incluidos. */
    mayus: boolean;
    /** Con la fila de reemplazar (Ctrl/⌘+H). */
    reemplazar: boolean;
    por: string;
  } | null>(null);
  const coincidencias = useRef<Range[]>([]);
  // Sin la API de resaltados de CSS (navegadores que aún no la tienen), las coincidencias
  // se marcan con recuadros en una capa encima del campo: tampoco tocan el HTML de la nota.
  const [marcasBusqueda, setMarcasBusqueda] = useState<
    { top: number; left: number; width: number; height: number; actual: boolean }[]
  >([]);

  const formato: Accion[] = [
    { cmd: 'bold', glyph: 'bold', label: t('editor.bold'), atajo: 'Ctrl+B', conEstado: true },
    { cmd: 'italic', glyph: 'italic', label: t('editor.italic'), atajo: 'Ctrl+I', conEstado: true },
    {
      cmd: 'underline',
      glyph: 'underline',
      label: t('editor.underline'),
      atajo: 'Ctrl+U',
      conEstado: true,
    },
    {
      cmd: 'strikeThrough',
      glyph: 'strikethrough',
      label: t('editor.strike'),
      atajo: 'Ctrl+Shift+X',
      conEstado: true,
    },
  ];
  const codigoEnLinea: Accion = {
    cmd: 'code',
    icon: 'code',
    label: t('editor.inlineCode'),
    atajo: 'Ctrl+E',
  };
  const resaltado: Accion = {
    cmd: 'mark',
    icon: 'highlighter',
    label: t('editor.highlight'),
    atajo: 'Ctrl+Shift+H',
  };
  const listas: Accion[] = [
    {
      cmd: 'insertUnorderedList',
      glyph: 'bulletList',
      label: t('editor.bulletList'),
      atajo: '-',
      conEstado: true,
    },
    {
      cmd: 'insertOrderedList',
      glyph: 'numberedList',
      label: t('editor.numberedList'),
      atajo: '1.',
      conEstado: true,
    },
    {
      cmd: 'insertTask',
      glyph: 'todo',
      label: t('editor.taskList'),
      atajo: '[]',
    },
    { cmd: 'outdent', glyph: 'outdent', label: t('editor.outdent'), atajo: 'Shift+Tab' },
    { cmd: 'indent', glyph: 'indent', label: t('editor.indent'), atajo: 'Tab' },
  ];
  const alineacion: Accion[] = [
    { cmd: 'justifyLeft', glyph: 'alignLeft', label: t('editor.alignLeft'), conEstado: true },
    { cmd: 'justifyCenter', glyph: 'alignCenter', label: t('editor.alignCenter'), conEstado: true },
    { cmd: 'justifyRight', glyph: 'alignRight', label: t('editor.alignRight'), conEstado: true },
  ];

  const refrescar = () => {
    const estado: Record<string, boolean> = {};
    for (const a of [...formato, ...listas, ...alineacion])
      if (a.conEstado) estado[a.cmd] = editingCommandState(a.cmd);
    setActivos(estado);
    setPuede({ undo: editingCommandEnabled('undo'), redo: editingCommandEnabled('redo') });
    const b = editingCommandValue('formatBlock').toLowerCase().replace(/[<>]/g, '');
    setBloque(BLOQUES.some((x) => x.valor === b) ? b : 'p');
    setContextoCursor(
      ref.current ? contextoDe(window.getSelection()?.anchorNode ?? null, ref.current) : null,
    );
    medirNumeros();
    if (ref.current && document.activeElement === ref.current)
      ponerAsa(bloqueDe(window.getSelection()?.anchorNode ?? null, ref.current));
  };

  // El HTML inicial se escribe en el DOM una sola vez: después manda el
  // navegador. Reescribirlo en cada render movería el cursor al principio en
  // mitad de la frase, que es el fallo clásico de un contenteditable controlado.
  useEffect(() => {
    if (ref.current && ref.current.innerHTML !== initialHtml) {
      ref.current.innerHTML = initialHtml;
    }
    executeEditingCommand('styleWithCSS', 'true');
    ref.current?.focus();
    // Las capas (números de línea, celda activa) se miden con lo ya pintado, y otra vez
    // cuando cambia el ancho: el código y la tabla se reflujan.
    medirNumeros();
    window.addEventListener('resize', medirNumeros);
    return () => {
      window.removeEventListener('resize', medirNumeros);
      if (recuento.current) clearTimeout(recuento.current);
    };
    // initialHtml queda fuera a propósito: el padre remonta con `key` cuando
    // cambia de nota, y reaccionar a él aquí es exactamente lo que no debe pasar.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const emitir = () => {
    const html = ref.current?.innerHTML ?? '';
    setVacio(ref.current?.textContent?.trim() === '' && !html.includes('<img'));
    const limpio = normalizarSalida(html);
    onChange(limpio);
    medirNumeros();
    if (recuento.current) clearTimeout(recuento.current);
    recuento.current = setTimeout(() => setCuentas(estadisticas(textoDeNota(limpio))), 250);
  };

  // --- Asistente de IA ------------------------------------------------------
  //
  // Una nota es un cuaderno: lo que se pide aquí es ordenar lo apuntado deprisa,
  // sacar las tareas escondidas en un párrafo y ampliar una idea suelta. Por eso
  // las acciones no son las del correo ni las del documento.
  const [iaTrabajando, setIaTrabajando] = useState<AccionIA | null>(null);
  const [iaError, setIaError] = useState(false);

  /**
   * Escribe los bloques del asistente con el formato de la nota.
   *
   * Una nota SÍ tiene títulos —a diferencia de un correo—, así que los bloques
   * de título se aplican con `formatBlock`. Es la misma barra que ya ofrece h1 y
   * h2 a mano, así que lo que escribe el asistente no se distingue de lo que
   * escribe una persona, que es la prueba de que el formato es el de la app.
   */
  const escribirBloques = (bloques: Bloque[]): void => {
    bloques.forEach((bloque, i) => {
      if (i > 0) executeEditingCommand('insertParagraph');
      if (bloque.tipo === 'li') executeEditingCommand('insertUnorderedList');
      else executeEditingCommand('formatBlock', bloque.tipo === 'p' ? 'p' : bloque.tipo);
      for (const trozo of bloque.trozos) {
        if (trozo.negrita) ordenar('bold');
        if (trozo.cursiva) ordenar('italic');
        executeEditingCommand('insertText', trozo.texto);
        if (trozo.cursiva) ordenar('italic');
        if (trozo.negrita) ordenar('bold');
      }
      if (bloque.tipo === 'li') executeEditingCommand('insertUnorderedList');
    });
  };

  const aplicarIA = async (accion: AccionIA): Promise<void> => {
    const nodo = ref.current;
    if (!nodo || iaTrabajando) return;
    const sel = window.getSelection();
    const haySeleccion =
      sel !== null && !sel.isCollapsed && sel.anchorNode !== null && nodo.contains(sel.anchorNode);
    const texto = haySeleccion ? sel.toString() : nodo.innerText;
    if (texto.trim() === '') return;

    setIaTrabajando(accion);
    setIaError(false);
    try {
      const res = await fetch('/api/ai', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify(construirPeticion({ accion, texto })),
      });
      const resultado = res.ok ? leerRespuesta(await res.json()) : null;
      if (resultado === null) {
        setIaError(true);
        return;
      }
      nodo.focus();
      const bloques = bloquesDe(resultado);
      const conFormato =
        bloques.length > 1 ||
        bloques.some((b) => b.tipo !== 'p' || b.trozos.some((x) => x.negrita || x.cursiva));

      if (conFormato && !haySeleccion) {
        escribirBloques(bloques);
      } else if (haySeleccion) {
        // Sustituye lo marcado conservando el deshacer del navegador.
        executeEditingCommand('insertText', resultado);
      } else {
        const rango = document.createRange();
        rango.selectNodeContents(nodo);
        sel?.removeAllRanges();
        sel?.addRange(rango);
        executeEditingCommand('insertText', resultado);
      }
      refrescar();
      emitir();
    } catch {
      setIaError(true);
    } finally {
      setIaTrabajando(null);
    }
  };

  /** Deshacer de la barra: tras un atajo de Markdown, como Ctrl+Z, devuelve lo tecleado. */
  const deshacer = () => {
    const atajo = ultimoAuto.current;
    ultimoAuto.current = null;
    if (!atajo) return aplicar('undo');
    enfocarTexto();
    atajo();
    refrescar();
    emitir();
  };

  const aplicar = (cmd: string, valor?: string) => {
    enfocarTexto();
    ordenar(cmd, valor);
    refrescar();
    emitir();
  };

  /**
   * Envuelve la selección en una etiqueta que execCommand no conoce (`code`,
   * `mark`). Va por `insertHTML` y no tocando el DOM a mano: solo lo que pasa
   * por execCommand entra en la pila de deshacer del navegador, y un botón de
   * la barra que Ctrl+Z no revierte es un botón en el que no se puede confiar.
   */
  const envolver = (tag: 'code' | 'mark') => {
    enfocarTexto();
    const sel = window.getSelection();
    if (!sel || sel.rangeCount === 0 || sel.isCollapsed) return;
    const caja = document.createElement('div');
    caja.appendChild(sel.getRangeAt(0).cloneContents());
    executeEditingCommand('insertHTML', `<${tag}>${caja.innerHTML}</${tag}>`);
    refrescar();
    emitir();
  };

  /**
   * Atajos de Markdown al escribir (client/markdown.ts). Solo actúa si el
   * marcador es TODO lo que hay antes del cursor en su línea, vive en un solo
   * nodo de texto y no está ya dentro de una lista o de un bloque de código;
   * en cualquier otro caso la tecla sigue su curso, que es lo seguro. El
   * marcador se borra por execCommand para que Ctrl+Z lo devuelva.
   */
  const aplicarAtajoMarkdown = (tecla: ' ' | 'Enter'): boolean => {
    const raiz = ref.current;
    const sel = window.getSelection();
    if (!raiz || !sel || sel.rangeCount === 0 || !sel.isCollapsed) return false;
    const rango = sel.getRangeAt(0);
    const nodo = rango.startContainer;
    if (nodo.nodeType !== Node.TEXT_NODE || !raiz.contains(nodo)) return false;
    const bloque =
      (nodo.parentElement ?? raiz).closest('p,div,h1,h2,h3,h4,h5,h6,li,blockquote,pre') ?? raiz;
    if (!raiz.contains(bloque) || bloque.tagName === 'PRE' || bloque.tagName === 'LI') return false;
    const antes = document.createRange();
    antes.setStart(bloque, 0);
    antes.setEnd(nodo, rango.startOffset);
    const atajo = atajoMarkdown(antes.toString(), tecla);
    if (!atajo || rango.startOffset < atajo.borrar) return false;

    if (
      atajo.valor?.includes('task-row') &&
      bloque.textContent?.trim() === antes.toString().trim()
    ) {
      const taskRow = document.createElement('div');
      taskRow.className = 'task-row';
      taskRow.innerHTML = '<input type="checkbox" /> <span>&nbsp;</span>';
      const original = bloque.cloneNode(true) as HTMLElement;
      bloque.replaceWith(taskRow);
      // La fila de tarea se pone a mano (con `insertHTML` el navegador le quita el
      // `<span>`), así que deshacerla también: vuelve el «[]» tal cual, con su espacio.
      ultimoAuto.current = () => {
        if (!taskRow.isConnected) return;
        taskRow.replaceWith(original);
        const r = document.createRange();
        r.selectNodeContents(original);
        r.collapse(false);
        sel.removeAllRanges();
        sel.addRange(r);
        if (tecla === ' ') executeEditingCommand('insertText', ' ');
      };
      const span = taskRow.querySelector('span');
      if (span) {
        const newRange = document.createRange();
        newRange.selectNodeContents(span);
        newRange.collapse(false);
        sel.removeAllRanges();
        sel.addRange(newRange);
      }
      refrescar();
      emitir();
      return true;
    }

    const marcador = document.createRange();
    marcador.setStart(nodo, rango.startOffset - atajo.borrar);
    marcador.setEnd(nodo, rango.startOffset);
    sel.removeAllRanges();
    sel.addRange(marcador);
    executeEditingCommand('delete');
    executeEditingCommand(atajo.cmd, atajo.valor);
    // Ctrl+Z justo después devuelve lo tecleado tal cual: deshace las dos órdenes y
    // pone la tecla que se comió el atajo (el espacio, o el salto de línea de Intro).
    ultimoAuto.current = () => {
      executeEditingCommand('undo');
      executeEditingCommand('undo');
      executeEditingCommand(tecla === ' ' ? 'insertText' : 'insertParagraph', ' ');
    };
    refrescar();
    emitir();
    return true;
  };

  /**
   * Markdown en la línea (`client/autoformato.ts`): al teclear la marca que cierra,
   * `**x**`, `*x*`, `_x_`, `` `x` `` o `~~x~~` pasan a su formato con UNA orden
   * (`insertHTML`), así que un Ctrl+Z devuelve las marcas. Detrás va un espacio de ancho
   * cero para seguir escribiendo FUERA del formato; al guardar se quita (salida.ts).
   * Dentro del código no se toca nada.
   */
  const aplicarAtajoEnLinea = (): boolean => {
    const raiz = ref.current;
    const sel = window.getSelection();
    if (!raiz || !sel || sel.rangeCount === 0 || !sel.isCollapsed) return false;
    const rango = sel.getRangeAt(0);
    const nodo = rango.startContainer;
    if (nodo.nodeType !== Node.TEXT_NODE || !raiz.contains(nodo)) return false;
    if (nodo.parentElement?.closest('pre, code')) return false;
    const fin = rango.startOffset;
    const atajo = atajoEnLinea((nodo as Text).data.slice(0, fin));
    if (!atajo) return false;
    const marcas = document.createRange();
    marcas.setStart(nodo, atajo.desde);
    marcas.setEnd(nodo, fin);
    sel.removeAllRanges();
    sel.addRange(marcas);
    const e = atajo.etiqueta;
    if (!executeEditingCommand('insertHTML', `<${e}>${escapar(atajo.texto)}</${e}>\u200B`)) {
      // El navegador no la ha hecho: el cursor vuelve a donde estaba, detrás de la marca,
      // o lo siguiente que se teclee se comería lo seleccionado.
      sel.removeAllRanges();
      sel.addRange(rango);
      return false;
    }
    ultimoAuto.current = () => executeEditingCommand('undo');
    return true;
  };

  /**
   * Tabla de `filas`×`columnas`, con la fila de cabecera puesta (como en Confluence). Sin
   * estilos en línea: el saneador de las notas no los guarda (solo color y alineación) y
   * los bordes, el relleno y la cabecera los pinta la prosa (app.css), en claro y oscuro.
   */
  const insertarTabla = (filas: number, columnas: number) => {
    enfocarTexto();
    const cabecera = `<thead><tr>${'<th><br></th>'.repeat(columnas)}</tr></thead>`;
    const fila = `<tr>${'<td><br></td>'.repeat(columnas)}</tr>`;
    executeEditingCommand(
      'insertHTML',
      `<table data-nuevo="">${cabecera}<tbody>${fila.repeat(Math.max(1, filas - 1))}</tbody></table><p><br></p>`,
    );
    const tabla = ref.current?.querySelector<HTMLTableElement>('table[data-nuevo]');
    if (tabla) {
      tabla.removeAttribute('data-nuevo');
      const primera = tabla.querySelector<HTMLTableCellElement>('th, td');
      if (primera) entrarEnCelda(primera);
    }
    refrescar();
    emitir();
  };

  /** Inserta una fila de cuadro de tarea interactiva en el texto. */
  const insertarTarea = () => {
    enfocarTexto();
    const sel = window.getSelection();
    if (sel && sel.rangeCount > 0) {
      const range = sel.getRangeAt(0);
      const node = range.startContainer;
      const el = node.nodeType === Node.TEXT_NODE ? node.parentElement : (node as HTMLElement);
      const block = el?.closest('p, h1, h2, h3, h4, h5, h6, blockquote, div:not(.task-row)');
      if (
        block &&
        ref.current?.contains(block) &&
        (block.textContent?.trim() === '' || block.innerHTML === '<br>')
      ) {
        const taskRow = document.createElement('div');
        taskRow.className = 'task-row';
        taskRow.innerHTML = '<input type="checkbox" /> <span>&nbsp;</span>';
        block.replaceWith(taskRow);
        const span = taskRow.querySelector('span');
        if (span) {
          const newRange = document.createRange();
          newRange.selectNodeContents(span);
          newRange.collapse(false);
          sel.removeAllRanges();
          sel.addRange(newRange);
        }
        refrescar();
        emitir();
        return;
      }
    }
    executeEditingCommand(
      'insertHTML',
      '<div class="task-row"><input type="checkbox" /> <span>&nbsp;</span></div>',
    );
    refrescar();
    emitir();
  };

  /**
   * Aplica una acción a la celda donde está el cursor. Las celdas nuevas del módulo común
   * traen los bordes en línea (los del correo); aquí sobran: el saneador no los guarda y
   * la prosa ya los pinta. Se deja solo la alineación.
   */
  const enCelda = (accion: (c: HTMLTableCellElement) => void) => {
    enfocarTexto();
    const celda = celdaDe(window.getSelection()?.anchorNode ?? null);
    if (!celda) return;
    const tabla = celda.closest('table');
    const donde = posicionDe(celda);
    accion(celda);
    // Cambiar una celda de etiqueta (cabeceras) o quitar su fila se lleva el cursor:
    // vuelve a la misma posición de la rejilla, o a la más cercana que quede.
    if (!celda.isConnected && tabla?.isConnected) {
      const otra = celdaEn(tabla, donde);
      if (otra) entrarEnCelda(otra);
    }
    for (const c of tabla?.querySelectorAll<HTMLElement>('td[style], th[style]') ?? []) {
      const alineado = c.style.textAlign;
      c.removeAttribute('style');
      if (alineado) c.style.setProperty('text-align', alineado);
    }
    refrescar();
    emitir();
  };

  /** Coloca el asa junto a `bloque` (un hijo directo del campo), medido sobre el envoltorio. */
  const ponerAsa = (bloque: HTMLElement | null) => {
    const caja = ref.current?.parentElement?.getBoundingClientRect();
    if (!bloque || !caja) {
      if (!menuBloque) setAsa(null);
      return;
    }
    const top = bloque.getBoundingClientRect().top - caja.top;
    setAsa((a) => (a && a.el === bloque && a.top === top ? a : { el: bloque, top }));
  };

  /**
   * Una acción del menú del bloque: el cursor va al principio del bloque (las órdenes de
   * convertir actúan donde está) y después se entrega la nota.
   */
  const conBloque = (accion: (b: HTMLElement) => void) => {
    const b = bloqueDelMenu.current;
    const raiz = ref.current;
    if (!b?.isConnected || !raiz) return;
    raiz.focus();
    const r = document.createRange();
    r.selectNodeContents(b);
    r.collapse(true);
    const sel = window.getSelection();
    sel?.removeAllRanges();
    sel?.addRange(r);
    guardarSeleccion();
    accion(b);
    emitir();
    refrescar();
  };

  /** Abre el menú del bloque en `x,y` (el asa, o el cursor con Mayús+F10). */
  const abrirMenuBloque = (bloque: HTMLElement, x: number, y: number) => {
    bloqueDelMenu.current = bloque;
    setMenuBloque({ x, y });
  };

  /** Cambia el bloque del cursor (un atributo, quitarlo) y entrega la nota. */
  const enBloque = (accion: () => void) => {
    accion();
    setElegido(null);
    emitir();
    refrescar();
  };

  const accionesDeBloque = (c: Contexto): AccionesDeBloque => {
    const el = c.el;
    const quitar = () => enBloque(() => el.remove());
    return {
      tabla: {
        fila: (lado) => enCelda((x) => insertarFila(x, lado)),
        columna: (lado) => enCelda((x) => insertarColumna(x, lado)),
        quitarFila: () => enCelda(eliminarFila),
        quitarColumna: () => enCelda(eliminarColumna),
        quitarTabla: () => enCelda(eliminarTabla),
        cabeceraFila: () => enCelda((x) => alternarCabeceraDeFila(x.closest('table')!)),
        cabeceraColumna: () => enCelda((x) => alternarCabeceraDeColumna(x.closest('table')!)),
        numerada: () => enCelda((x) => alternarFilasNumeradas(x.closest('table')!)),
        fondo: (color) => enCelda((x) => fondoDeCelda(x, color)),
        alinear: (lado) => enCelda((x) => alinearColumna(x, lado)),
      },
      codigo: {
        lenguaje: (l) =>
          enBloque(() => codigoDe(el as HTMLPreElement).setAttribute('data-lenguaje', l)),
        lineas: () => enBloque(() => el.toggleAttribute('data-lineas')),
        ajuste: () => enBloque(() => el.toggleAttribute('data-ajuste')),
        quitar,
      },
      panel: { tipo: (tipo) => enBloque(() => el.setAttribute('data-panel', tipo)), quitar },
      estado: { color: (color) => enBloque(() => el.setAttribute('data-estado', color)), quitar },
      fecha: {
        dia: (iso) =>
          enBloque(() => {
            el.setAttribute('datetime', iso);
            el.textContent = fechaLegible(iso, documentLocale());
          }),
        quitar,
      },
      expandir: { quitar },
    };
  };

  /**
   * Las capas encima del campo —los números de línea de cada `pre[data-lineas]` y el
   * recuadro de la celda activa—, medidas respecto a su envoltorio (`relative`).
   */
  const medirNumeros = () => {
    const raiz = ref.current;
    const caja = raiz?.parentElement?.getBoundingClientRect();
    if (!raiz || !caja) return;
    const nuevos = [...raiz.querySelectorAll<HTMLElement>('pre[data-lineas]')].map((pre) => {
      const r = pre.getBoundingClientRect();
      return { top: r.top - caja.top, left: r.left - caja.left, n: lineasDe(pre) };
    });
    setNumeros((antes) => (JSON.stringify(antes) === JSON.stringify(nuevos) ? antes : nuevos));
    const celda = celdaDe(window.getSelection()?.anchorNode ?? null);
    const r = celda && raiz.contains(celda) ? celda.getBoundingClientRect() : null;
    setCeldaActiva(
      r && r.width > 0
        ? { top: r.top - caja.top, left: r.left - caja.left, width: r.width, height: r.height }
        : null,
    );
  };

  const abrirEnlaces = () => {
    if (document.activeElement === ref.current) guardarSeleccion();
    setEnlaceAbierto(true);
  };

  const abrirColor = () => {
    if (document.activeElement === ref.current) guardarSeleccion();
    setColorAbierto(true);
  };

  const abrirSimbolos = (enfoque: IdCategoria | true) => {
    if (document.activeElement === ref.current) guardarSeleccion();
    setSimbolosAbierto(enfoque);
  };

  const abrirInsertar = () => {
    if (document.activeElement === ref.current) guardarSeleccion();
    setInsertarAbierto(true);
  };

  /**
   * Copiar o cortar lo seleccionado (la burbuja). Cortar borra con la orden del navegador,
   * así que Ctrl+Z lo devuelve; y solo si de verdad se copió, para no perder nada.
   */
  const copiar = async (cortar: boolean) => {
    const sel = window.getSelection();
    const rango =
      sel && sel.rangeCount > 0 && !sel.isCollapsed && ref.current?.contains(sel.anchorNode)
        ? sel.getRangeAt(0).cloneRange()
        : seleccion.current;
    if (!rango || rango.collapsed) return;
    const hecho = await copiarRango(rango);
    if (!cortar || !hecho) return;
    ref.current?.focus();
    const ahora = window.getSelection();
    ahora?.removeAllRanges();
    ahora?.addRange(rango);
    executeEditingCommand('delete');
    refrescar();
    emitir();
  };

  /**
   * Pegar desde la burbuja: lo leído entra por `queSePega`, el mismo saneado que Ctrl+V.
   * Sin permiso (o sin la API), se dice que se pegue con el teclado.
   */
  const pegar = async () => {
    if (document.activeElement === ref.current) guardarSeleccion();
    const elegido = seleccion.current?.toString() ?? '';
    const datos = await leerPortapapeles();
    if (!datos) {
      setAvisoPegar(true);
      return;
    }
    enfocarTexto();
    const ancla = window.getSelection()?.anchorNode;
    const enCodigo = !!(ancla instanceof Element ? ancla : ancla?.parentElement)?.closest('pre');
    const pegado = queSePega(datos, false, !enCodigo, elegido);
    if ('html' in pegado) executeEditingCommand('insertHTML', pegado.html);
    else if (pegado.texto) executeEditingCommand('insertText', pegado.texto);
    refrescar();
    emitir();
  };

  /** El enlace con su nombre en lugar de la selección, y un espacio para seguir fuera de él. */
  const escribirEnlace = (url: string, label: string) =>
    executeEditingCommand('insertHTML', `<a href="${escapar(url)}">${escapar(label)}</a>&nbsp;`);

  const insertarEnlace = (url: string, label: string) => {
    setEnlaceAbierto(false);
    enfocarTexto();
    const sel = window.getSelection();
    // Con texto seleccionado, ese texto es el enlace; sin él, se inserta el
    // nombre de lo enlazado. Nunca la URL pelada como texto de un enlace a una nota.
    if (sel && !sel.isCollapsed) {
      executeEditingCommand('createLink', url);
    } else {
      escribirEnlace(url, label);
    }
    refrescar();
    emitir();
  };

  /** El disparador («/», «[[» o «@») que hay justo antes del cursor, con su consulta; o null. */
  const disparoEnCursor = (): {
    tipo: TipoMenu;
    nodo: Text;
    off: number;
    largo: number;
    q: string;
  } | null => {
    const raiz = ref.current;
    const sel = window.getSelection();
    if (!raiz || !sel || sel.rangeCount === 0 || !sel.isCollapsed) return null;
    const rango = sel.getRangeAt(0);
    const nodo = rango.startContainer;
    if (nodo.nodeType !== Node.TEXT_NODE || !raiz.contains(nodo)) return null;
    if (nodo.parentElement?.closest('pre')) return null;
    const bloque =
      (nodo.parentElement ?? raiz).closest('p,div,h1,h2,h3,h4,h5,h6,li,blockquote') ?? raiz;
    const antes = document.createRange();
    antes.setStart(raiz.contains(bloque) ? bloque : raiz, 0);
    antes.setEnd(nodo, rango.startOffset);
    const texto = antes.toString();
    for (const [tipo, marca, consulta] of DISPAROS) {
      // Sin cuaderno no hay notas que ofrecer, y sin gente (un cuaderno cifrado) a nadie.
      if ((tipo === 'nota' && !enlaces) || (tipo === 'persona' && !personas)) continue;
      const q = consulta(texto);
      if (q === null) continue;
      // La consulta no lleva su marca dentro: la última es la que abre. Y tiene que
      // estar en el mismo nodo de texto que el cursor, o no hay rango que sustituir.
      const largo = texto.length - texto.lastIndexOf(marca);
      const off = rango.startOffset - largo;
      if (off < 0 || !(nodo as Text).data.startsWith(marca, off)) continue;
      return { tipo, nodo: nodo as Text, off, largo, q };
    }
    return null;
  };

  const mirarMenu = () => {
    const d = disparoEnCursor();
    if (!d) {
      descartada.current = null;
      setMenu(null);
      return;
    }
    const x = descartada.current;
    if (x && x.nodo === d.nodo && x.off === d.off) {
      setMenu(null);
      return;
    }
    const r = document.createRange();
    r.setStart(d.nodo, d.off);
    r.setEnd(d.nodo, d.off + 1);
    const caja = r.getBoundingClientRect();
    const ancho = d.tipo === 'cmd' ? 320 : 288;
    const arriba = caja.bottom + 4 + 260 > window.innerHeight && caja.top - 264 >= 0;
    setMenu((m) => ({
      tipo: d.tipo,
      q: d.q,
      idx: m && m.tipo === d.tipo && m.q === d.q ? m.idx : 0,
      x: Math.max(8, Math.min(caja.left, window.innerWidth - ancho - 8)),
      y: arriba ? caja.top - 4 : caja.bottom + 4,
      arriba,
    }));
  };

  /** Selecciona lo escrito desde el disparador («/h», «[[pla», «@an») para sustituirlo. */
  const tomarDisparo = (): boolean => {
    const d = disparoEnCursor();
    setMenu(null);
    if (!d) return false;
    const sel = window.getSelection();
    const r = document.createRange();
    r.setStart(d.nodo, d.off);
    r.setEnd(d.nodo, d.off + d.largo);
    sel?.removeAllRanges();
    sel?.addRange(r);
    return true;
  };

  /** `[[consulta` se cambia por el enlace a la nota, el mismo que pone la herramienta de enlaces. */
  const enlazarNota = (o: Opcion) => {
    if (!tomarDisparo()) return;
    escribirEnlace(o.url, o.label);
    refrescar();
    emitir();
  };

  /** «@an» se cambia por `@ana@empresa.es `, la forma con que el servicio avisa. */
  const mencionar = (p: Persona) => {
    if (!tomarDisparo()) return;
    executeEditingCommand('insertText', `@${p.email} `);
    refrescar();
    emitir();
  };

  /** «:son» se cambia por el emoji elegido. */
  const ponerEmoji = (emoji: string) => {
    if (!tomarDisparo()) return;
    apuntarReciente(emoji);
    executeEditingCommand('insertText', emoji);
    refrescar();
    emitir();
  };

  /** Un bloque de los nuevos (`bloques.ts`), con el cursor dentro al acabar. */
  const insertarBloque = (html: string) => {
    executeEditingCommand('insertHTML', html);
    if (ref.current) entrarEnNuevo(ref.current);
    refrescar();
    emitir();
  };

  /**
   * Inserta un elemento del catálogo donde está el cursor: lo comparten «/» y el «+» de
   * la barra. Los de siempre van con la MISMA orden que su botón; los bloques nuevos, con
   * su plantilla.
   */
  const insertarElemento = (id: IdComando) => {
    const bloques: Partial<Record<IdComando, string>> = {
      p: '<p>',
      h1: '<h1>',
      h2: '<h2>',
      h3: '<h3>',
      cita: '<blockquote>',
      codigo: '<pre>',
    };
    const paneles: Partial<Record<IdComando, TipoDePanel>> = {
      panelInfo: 'info',
      panelNota: 'nota',
      panelExito: 'exito',
      panelAviso: 'aviso',
      panelError: 'error',
    };
    const valor = bloques[id];
    const panel = paneles[id];
    if (valor) return aplicar('formatBlock', valor);
    if (panel) return insertarBloque(plantillaPanel(panel));
    switch (id) {
      case 'tarea':
        return insertarTarea();
      case 'vinetas':
        return aplicar('insertUnorderedList');
      case 'numeros':
        return aplicar('insertOrderedList');
      case 'regla':
        return aplicar('insertHorizontalRule');
      case 'tabla':
        return insertarTabla(3, 3);
      case 'decision':
        return insertarBloque(plantillaDecisiones());
      case 'expandir':
        return insertarBloque(plantillaExpandir(t('editor.expand')));
      case 'columnas2':
      case 'columnas3':
        return insertarBloque(plantillaColumnas(id === 'columnas2' ? 2 : 3));
      case 'estado':
        return insertarBloque(plantillaEstado(t('editor.status').toLocaleUpperCase()));
      case 'fecha':
      case 'hoy': {
        enfocarTexto();
        executeEditingCommand('insertHTML', plantillaFecha(hoyISO(), documentLocale()));
        const fecha = ref.current ? entrarEnNuevo(ref.current) : null;
        // «Fecha» abre el calendario sobre la que acaba de entrar; «Hoy» la deja puesta.
        if (id === 'fecha' && fecha) setElegido(fecha);
        refrescar();
        emitir();
        return;
      }
      case 'enlace':
        // Sin cuaderno no hay `[[`: la herramienta de enlaces, que también pega URL.
        if (!enlaces) return abrirEnlaces();
        enfocarTexto();
        executeEditingCommand('insertText', '[[');
        return mirarMenu();
      case 'mencion':
        enfocarTexto();
        executeEditingCommand('insertText', '@');
        return mirarMenu();
      case 'simbolo':
        return abrirSimbolos(true);
      case 'emoji':
        return abrirSimbolos('emoji');
    }
  };

  /** Borra «/consulta» e inserta el elemento, como si se hubiera elegido en el «+». */
  const ejecutarComando = (c: Comando) => {
    if (!tomarDisparo()) return;
    executeEditingCommand('delete');
    insertarElemento(c.id);
  };

  // La burbuja sigue a la selección: `selectionchange` es el único aviso que
  // llega igual con el ratón, con Mayús+flechas y con «seleccionar todo».
  useEffect(() => {
    const mirar = () => {
      if (document.activeElement === ref.current) guardarSeleccion();
      const raiz = ref.current;
      const sel = window.getSelection();
      if (
        !raiz ||
        !sel ||
        sel.rangeCount === 0 ||
        sel.isCollapsed ||
        document.activeElement !== raiz ||
        !raiz.contains(sel.anchorNode) ||
        !raiz.contains(sel.focusNode)
      ) {
        setBurbuja(null);
        return;
      }
      const caja = sel.getRangeAt(0).getBoundingClientRect();
      if (caja.width === 0 && caja.height === 0) {
        setBurbuja(null);
        return;
      }
      const consulta = (q: string) =>
        typeof window.matchMedia === 'function' && window.matchMedia(q).matches;
      const abajo = consulta('(pointer: coarse)');
      setBurbuja({
        x: Math.max(120, Math.min(caja.left + caja.width / 2, window.innerWidth - 120)),
        y: abajo ? caja.bottom : Math.max(caja.top, 44),
        abajo,
        estrecha: consulta('(max-width: 639px)'),
      });
    };
    document.addEventListener('selectionchange', mirar);
    window.addEventListener('scroll', mirar, true);
    return () => {
      document.removeEventListener('selectionchange', mirar);
      window.removeEventListener('scroll', mirar, true);
    };
  }, [guardarSeleccion]);

  /** Pinta las coincidencias y lleva la vista a la actual. */
  const pintarBusqueda = (
    q: string,
    i: number,
    cambios: Partial<{ mayus: boolean; reemplazar: boolean; por: string }> = {},
  ) => {
    const mayus = cambios.mayus ?? buscador?.mayus ?? false;
    const todas = ref.current ? coincidenciasEn(ref.current, q, { mayusculas: mayus }) : [];
    coincidencias.current = todas;
    const actual = todas.length ? ((i % todas.length) + todas.length) % todas.length : 0;
    if (registroResaltados()) {
      resaltar('sytue-buscar', todas);
      resaltar('sytue-buscar-actual', todas[actual] ? [todas[actual]] : []);
    } else {
      const caja = ref.current?.parentElement?.getBoundingClientRect();
      setMarcasBusqueda(
        caja
          ? todas.flatMap((r, n) =>
              [...(r.getClientRects?.() ?? [])].map((x) => ({
                top: x.top - caja.top,
                left: x.left - caja.left,
                width: x.width,
                height: x.height,
                actual: n === actual,
              })),
            )
          : [],
      );
    }
    todas[actual]?.startContainer.parentElement?.scrollIntoView({ block: 'nearest' });
    setBuscador({
      q,
      i: actual,
      mayus,
      reemplazar: cambios.reemplazar ?? buscador?.reemplazar ?? false,
      por: cambios.por ?? buscador?.por ?? '',
    });
  };

  /** Ctrl/⌘+F busca; Ctrl/⌘+H abre además la fila de reemplazar. */
  const abrirBuscador = (conReemplazo = false) => {
    const sel = window.getSelection();
    const q = sel && !sel.isCollapsed ? sel.toString().slice(0, 80) : (buscador?.q ?? '');
    pintarBusqueda(q, 0, { reemplazar: conReemplazo || (buscador?.reemplazar ?? false) });
    const destino = conReemplazo && q ? ID_REEMPLAZO : ID_BUSCAR;
    window.setTimeout(() => document.getElementById(destino)?.focus(), 0);
  };

  /**
   * Sustituye la coincidencia actual (o todas, de la última a la primera para que los
   * rangos no se muevan) con `insertText`: entra en la pila de deshacer del navegador y
   * no mete HTML, solo texto. El foco vuelve al campo de reemplazar.
   */
  const reemplazar = (todas: boolean) => {
    if (!buscador || !ref.current) return;
    const lista = coincidencias.current;
    const una = lista[buscador.i];
    const objetivo = todas ? [...lista].reverse() : una ? [una] : [];
    if (objetivo.length === 0) return;
    ref.current.focus();
    const sel = window.getSelection();
    for (const r of objetivo) {
      sel?.removeAllRanges();
      sel?.addRange(r);
      executeEditingCommand('insertText', buscador.por);
    }
    emitir();
    refrescar();
    pintarBusqueda(buscador.q, todas ? 0 : buscador.i);
    window.setTimeout(() => document.getElementById(ID_REEMPLAZO)?.focus(), 0);
  };

  /** Cierra la búsqueda dejando seleccionada la coincidencia en la que estaba. */
  const cerrarBuscador = () => {
    const actual = buscador ? coincidencias.current[buscador.i] : undefined;
    resaltar('sytue-buscar', []);
    resaltar('sytue-buscar-actual', []);
    setMarcasBusqueda([]);
    setBuscador(null);
    ref.current?.focus();
    if (actual) {
      const sel = window.getSelection();
      sel?.removeAllRanges();
      sel?.addRange(actual);
    }
  };

  // La barra del bloque solo vive con el foco en el editor o en lo que cuelga de él.
  useEffect(() => {
    const mirar = (e: FocusEvent) => {
      const d = e.target instanceof Element ? e.target : null;
      setConFoco(
        !!d &&
          (ref.current?.contains(d) === true ||
            d.closest('[data-notes-editor-tools], [role="menu"], [role="dialog"]') !== null),
      );
    };
    document.addEventListener('focusin', mirar);
    return () => document.removeEventListener('focusin', mirar);
  }, []);

  // Al desmontar (otra nota, salir del editor) no se quedan resaltados colgando.
  useEffect(
    () => () => {
      resaltar('sytue-buscar', []);
      resaltar('sytue-buscar-actual', []);
    },
    [],
  );

  /** Pliega o despliega la lista en la que está el cursor (Ctrl+.). */
  const alternarPlegadoAqui = () => {
    const li = liPlegable(window.getSelection()?.anchorNode ?? null);
    if (!li || !ref.current?.contains(li)) return;
    li.classList.toggle('plegada');
    if (li.classList.length === 0) li.removeAttribute('class');
    emitir();
  };

  // La fecha recién puesta con «Fecha» abre su calendario aunque el cursor quede detrás.
  const contexto: Contexto | null =
    elegido?.isConnected && elegido.tagName === 'TIME' && ref.current?.contains(elegido)
      ? { tipo: 'fecha', el: elegido as HTMLTimeElement }
      : contextoCursor;

  const etiquetaBloque = t(
    `editor.${BLOQUES.find((b) => b.valor === bloque)?.clave ?? 'paragraph'}` as never,
  );
  const alineada = alineacion.find((a) => activos[a.cmd]) ?? alineacion[0]!;
  const dibujoDe = (a: Accion) =>
    a.icon ? <Icon name={a.icon} size={16} /> : <Glyph name={a.glyph ?? 'note'} size={16} />;

  /** Un botón de la barra y su opción en «Más». */
  // El estado de lo que no es una orden de edición (el ancho de la hoja).
  const activosDeBarra: Record<string, boolean> = {
    ...activos,
    anchoCompleto: anchoCompleto === true,
  };
  const itemBoton = (
    a: Accion,
    grupo: GrupoDeBarra,
    prioridad: number,
    onPress: () => void,
  ): ItemDeBarra => ({
    id: a.cmd,
    grupo,
    prioridad,
    barra: () => <BotonDeBarra t={t} activos={activosDeBarra} a={a} onPress={onPress} />,
    menu: (cerrar) => (
      <OpcionDeMas
        dibujo={dibujoDe(a)}
        rotulo={a.label}
        atajo={a.atajo}
        marcada={a.conEstado ? (activosDeBarra[a.cmd] ?? false) : undefined}
        desactivada={a.desactivada}
        cerrar={cerrar}
        onElegir={onPress}
      />
    ),
  });

  /** Elegir en un panel lo cierra y devuelve el cursor al texto (no al botón). */
  const trasElegir = (cerrar: () => void, despues?: () => void) => {
    cerrar();
    // Después de que el panel se cierre y devuelva el foco a su botón: entonces el
    // cursor vuelve al texto, donde estaba, y se hace lo elegido.
    window.setTimeout(() => {
      enfocarTexto();
      despues?.();
    }, 0);
  };

  const muestrasDeColor = () => (
    <div class="grid grid-cols-4 gap-1.5 p-2">
      <button
        type="button"
        class="border-border sytue-focus-ring rounded-control hover:bg-bg-subtle col-span-4 min-w-0 border px-2 py-1 text-sm"
        onMouseDown={(e) => e.preventDefault()}
        onClick={() => {
          aplicar('foreColor', 'inherit');
          trasElegir(() => setColorAbierto(false));
        }}
      >
        {t('editor.colorDefault')}
      </button>
      {COLORES.map((c) => (
        <button
          key={c}
          type="button"
          // Acción, no estado: aplica y cierra, así que botones con el nombre
          // del tono (no un radiogroup, que elegiría al moverse con flechas).
          aria-label={nombreDeColor(t, c)}
          title={nombreDeColor(t, c)}
          style={{ background: c }}
          class="sytue-focus-ring border-border h-7 w-7 rounded-full border transition-transform duration-150 hover:scale-110"
          onMouseDown={(e) => e.preventDefault()}
          onClick={() => {
            aplicar('foreColor', c);
            trasElegir(() => setColorAbierto(false));
          }}
        />
      ))}
    </div>
  );

  const herramientaDeEnlace = () => (
    <LinkTool
      t={t}
      {...(enlaces ? { fuentes: enlaces } : {})}
      onPick={(e) => insertarEnlace(e.url, e.label)}
      onClose={() => {
        setEnlaceAbierto(false);
        enfocarTexto();
      }}
    />
  );

  /** El disparador de un desplegable o un panel de la barra, con su ayuda. */
  const disparador =
    (
      rotulo: string,
      dibujo: JSX.Element,
      extra?: { ancho?: string; texto?: string; flecha?: boolean },
    ) =>
    (props: Record<string, unknown>) => (
      <Tooltip label={rotulo}>
        <button
          {...props}
          type="button"
          class={cx(
            BOTON_DE_BARRA,
            'aria-expanded:bg-bg-subtle aria-expanded:text-text',
            extra?.texto !== undefined && 'w-auto gap-1 px-1.5 text-sm',
            extra?.flecha && 'w-auto gap-0.5 px-1',
          )}
          aria-label={extra?.texto ? `${rotulo}: ${extra.texto}` : rotulo}
          onMouseDown={(e) => e.preventDefault()}
        >
          {dibujo}
          {extra?.texto !== undefined ? (
            <>
              {/* ANCHO FIJO: «Párrafo» y «Título 1» no miden lo mismo, y la barra no
                  puede moverse al pasar de un bloque a otro ni de una nota a otra. */}
              <span class={cx('hidden truncate text-left sm:inline', extra.ancho)}>
                {extra.texto}
              </span>
              <Icon name="chevronDown" size={12} />
            </>
          ) : extra?.flecha ? (
            <Icon name="chevronDown" size={12} />
          ) : null}
        </button>
      </Tooltip>
    );

  /** Un desplegable de la barra con sus acciones; en «Más», las mismas en fila. */
  const itemMenu = (
    id: string,
    grupo: GrupoDeBarra,
    prioridad: number,
    rotulo: string,
    dibujo: JSX.Element,
    acciones: { a: Accion; hacer: () => void; marcada?: boolean }[],
  ): ItemDeBarra => ({
    id,
    grupo,
    prioridad,
    ancho: 40,
    barra: () => (
      <Dropdown label={rotulo} trigger={disparador(rotulo, dibujo, { flecha: true })}>
        {acciones.map(({ a, hacer, marcada }) => (
          <MenuItem
            key={a.cmd}
            size="sm"
            icon={dibujoDe(a)}
            {...(marcada !== undefined || a.conEstado
              ? { role: 'menuitemcheckbox', 'aria-checked': marcada ?? activos[a.cmd] ?? false }
              : {})}
            onMouseDown={(e: MouseEvent) => e.preventDefault()}
            onClick={hacer}
          >
            <span class="min-w-0 flex-1">{a.label}</span>
            {(marcada ?? (a.conEstado && activos[a.cmd])) ? (
              <Icon name="check" size={14} class="text-text-muted shrink-0" />
            ) : null}
            {a.atajo ? <span class="sytue-meta shrink-0">{atajoDelSistema(a.atajo)}</span> : null}
          </MenuItem>
        ))}
      </Dropdown>
    ),
    menu: (cerrar) =>
      acciones.map(({ a, hacer, marcada }) => (
        <OpcionDeMas
          key={a.cmd}
          dibujo={dibujoDe(a)}
          rotulo={a.label}
          atajo={a.atajo}
          marcada={marcada ?? (a.conEstado ? (activos[a.cmd] ?? false) : undefined)}
          cerrar={cerrar}
          onElegir={hacer}
        />
      )),
  });

  /** Una herramienta con panel (símbolos, el «+»): su botón y, sin sitio, colgada de «Más». */
  const itemPanel = (
    id: string,
    grupo: GrupoDeBarra,
    prioridad: number,
    rotulo: string,
    dibujo: JSX.Element,
    abierto: boolean,
    abrir: () => void,
    cerrar: () => void,
    contenido: () => JSX.Element,
  ): ItemDeBarra => ({
    id,
    grupo,
    prioridad,
    barra: () => (
      <Popover
        label={rotulo}
        open={abierto}
        onOpenChange={(o) => (o ? abrir() : cerrar())}
        trigger={disparador(rotulo, dibujo)}
      >
        {contenido()}
      </Popover>
    ),
    menu: (cerrarMenu) => (
      <OpcionDeMas dibujo={dibujo} rotulo={rotulo} cerrar={cerrarMenu} onElegir={abrir} />
    ),
    panel: abierto ? { label: rotulo, contenido, cerrar } : null,
  });

  const selectorDeSimbolos = () => (
    <SelectorDeSimbolos
      t={t}
      {...(simbolosAbierto === 'emoji' ? { enfoque: 'emoji' as const } : {})}
      onElegir={(simbolo) =>
        trasElegir(
          () => setSimbolosAbierto(false),
          () => {
            executeEditingCommand('insertText', simbolo);
            refrescar();
            emitir();
          },
        )
      }
    />
  );

  const menuInsertar = () => (
    <MenuInsertar
      t={t}
      disponible={disponible}
      onElegir={(id) =>
        trasElegir(
          () => setInsertarAbierto(false),
          () => insertarElemento(id),
        )
      }
    />
  );

  const listaDe = (cmd: string) => listas.find((a) => a.cmd === cmd)!;
  const masFormato: { a: Accion; hacer: () => void }[] = [
    { a: formato[2]!, hacer: () => aplicar('underline') },
    { a: formato[3]!, hacer: () => aplicar('strikeThrough') },
    { a: codigoEnLinea, hacer: () => envolver('code') },
    {
      a: { cmd: 'subscript', glyph: 'subindice', label: t('editor.subscript'), conEstado: true },
      hacer: () => aplicar('subscript'),
    },
    {
      a: {
        cmd: 'superscript',
        glyph: 'superindice',
        label: t('editor.superscript'),
        conEstado: true,
      },
      hacer: () => aplicar('superscript'),
    },
    {
      a: { cmd: 'removeFormat', glyph: 'eraser', label: t('editor.clearFormat'), atajo: 'Ctrl+\\' },
      hacer: () => aplicar('removeFormat'),
    },
  ];
  const parrafo: { a: Accion; hacer: () => void; marcada?: boolean }[] = [
    ...alineacion.map((a) => ({ a, hacer: () => aplicar(a.cmd), marcada: alineada.cmd === a.cmd })),
    { a: listaDe('outdent'), hacer: () => aplicar('outdent') },
    { a: listaDe('indent'), hacer: () => aplicar('indent') },
  ];

  /*
   * LA BARRA, COMO LA DE CONFLUENCE: historia | estilo del texto | negrita, cursiva y «más
   * formato» | color | listas | alineación y sangría | insertar (enlace, mención, emoji,
   * tabla y el «+» con todo) | herramientas. Lo que no cabe va a «Más» (BarraConMas).
   */
  const items: ItemDeBarra[] = [
    itemBoton(
      {
        cmd: 'undo',
        glyph: 'undo',
        label: t('editor.undo'),
        atajo: 'Ctrl+Z',
        desactivada: !puede.undo,
      },
      'historia',
      7,
      deshacer,
    ),
    itemBoton(
      {
        cmd: 'redo',
        glyph: 'redo',
        label: t('editor.redo'),
        atajo: 'Ctrl+Y',
        desactivada: !puede.redo,
      },
      'historia',
      5,
      () => aplicar('redo'),
    ),
    {
      id: 'bloque',
      grupo: 'bloque',
      prioridad: 8,
      ancho: 116,
      barra: () => (
        <Dropdown
          label={t('editor.block')}
          trigger={disparador(t('editor.block'), <Glyph name="heading" size={16} />, {
            texto: etiquetaBloque,
            ancho: 'w-20',
          })}
        >
          {BLOQUES.map((b) => (
            <MenuItem
              key={b.valor}
              size="sm"
              role="menuitemradio"
              aria-checked={bloque === b.valor}
              // EL preventDefault NO ES OPCIONAL, y su ausencia aquí es lo que
              // rompía «Título 1»: al pulsar la opción, el navegador movía el
              // foco al elemento del menú y sacaba el cursor del texto. El menú
              // se cerraba acto seguido, el elemento enfocado desaparecía y la
              // tarjeta leía ese salto como «me he ido de la nota»: guardaba y
              // salía del editor. Encima, sin selección, `formatBlock` no tenía
              // dónde aplicarse. El botón que abre el menú ya lo hacía; sus
              // opciones, no.
              onMouseDown={(e: MouseEvent) => e.preventDefault()}
              onClick={() => aplicar('formatBlock', `<${b.valor}>`)}
            >
              <span class={cx('text-text min-w-0 flex-1 truncate', b.vista)}>
                {t(`editor.${b.clave}` as never)}
              </span>
              {bloque === b.valor ? (
                <Icon name="check" size={14} class="text-text-muted shrink-0" />
              ) : null}
              {ATAJO_BLOQUE[b.valor] ? (
                <span class="sytue-meta shrink-0">{atajoDelSistema(ATAJO_BLOQUE[b.valor]!)}</span>
              ) : null}
            </MenuItem>
          ))}
        </Dropdown>
      ),
      menu: (cerrar) =>
        BLOQUES.map((b) => (
          <OpcionDeMas
            key={b.valor}
            rotulo={t(`editor.${b.clave}` as never)}
            atajo={ATAJO_BLOQUE[b.valor]}
            marcada={bloque === b.valor}
            cerrar={cerrar}
            onElegir={() => aplicar('formatBlock', `<${b.valor}>`)}
          />
        )),
    },
    itemBoton(formato[0]!, 'formato', 10, () => aplicar('bold')),
    itemBoton(formato[1]!, 'formato', 10, () => aplicar('italic')),
    itemMenu(
      'masFormato',
      'formato',
      6,
      t('editor.moreFormatting'),
      <Icon name="text" size={16} />,
      masFormato,
    ),
    {
      id: 'color',
      grupo: 'color',
      prioridad: 4,
      barra: () => (
        <Popover
          label={t('editor.textColor')}
          open={colorAbierto}
          onOpenChange={(o) => (o ? abrirColor() : setColorAbierto(false))}
          trigger={disparador(t('editor.textColor'), <Glyph name="palette" size={16} />)}
        >
          {muestrasDeColor()}
        </Popover>
      ),
      menu: (cerrar) => (
        <OpcionDeMas
          dibujo={<Glyph name="palette" size={16} />}
          rotulo={t('editor.textColor')}
          cerrar={cerrar}
          onElegir={abrirColor}
        />
      ),
      panel: colorAbierto
        ? {
            label: t('editor.textColor'),
            contenido: muestrasDeColor,
            cerrar: () => setColorAbierto(false),
          }
        : null,
    },
    itemBoton(resaltado, 'color', 3, () => aplicar('hiliteColor', RESALTADO)),
    itemBoton(listaDe('insertUnorderedList'), 'listas', 9, () => aplicar('insertUnorderedList')),
    itemBoton(listaDe('insertOrderedList'), 'listas', 7, () => aplicar('insertOrderedList')),
    itemBoton(listaDe('insertTask'), 'listas', 9, () => insertarTarea()),
    itemMenu('parrafo', 'listas', 3, t('editor.alignIndent'), dibujoDe(alineada), parrafo),
    {
      id: 'enlace',
      grupo: 'insertar',
      prioridad: 6,
      barra: () => (
        <Popover
          label={t('editor.linkTool')}
          open={enlaceAbierto}
          onOpenChange={(o) => (o ? abrirEnlaces() : setEnlaceAbierto(false))}
          trigger={disparador(
            t('editor.shortcut', {
              label: t('editor.linkTool'),
              keys: atajoDelSistema('Ctrl+K'),
            }),
            <Glyph name="link" size={16} />,
          )}
        >
          {herramientaDeEnlace()}
        </Popover>
      ),
      menu: (cerrar) => (
        <OpcionDeMas
          dibujo={<Glyph name="link" size={16} />}
          rotulo={t('editor.linkTool')}
          atajo="Ctrl+K"
          cerrar={cerrar}
          onElegir={abrirEnlaces}
        />
      ),
      panel: enlaceAbierto
        ? {
            label: t('editor.linkTool'),
            contenido: herramientaDeEnlace,
            cerrar: () => setEnlaceAbierto(false),
          }
        : null,
    },
    ...(personas
      ? [
          itemBoton(
            { cmd: 'mencion', glyph: 'arroba', label: t('editor.mention'), atajo: '@' },
            'insertar',
            2,
            () => insertarElemento('mencion'),
          ),
        ]
      : []),
    itemPanel(
      'emoji',
      'insertar',
      3,
      t('editor.emoji'),
      <Glyph name="carita" size={16} />,
      simbolosAbierto === 'emoji',
      () => abrirSimbolos('emoji'),
      () => setSimbolosAbierto(false),
      selectorDeSimbolos,
    ),
    itemPanel(
      'simbolos',
      'insertar',
      2,
      t('editor.symbols'),
      <Glyph name="omega" size={16} />,
      simbolosAbierto === true,
      () => abrirSimbolos(true),
      () => setSimbolosAbierto(false),
      selectorDeSimbolos,
    ),
    // Tabla 3×3 de partida: es la que sale en cualquier editor al pulsar el botón, y
    // crece con las acciones de tabla.
    itemBoton({ cmd: 'insertTable', glyph: 'table', label: t('editor.table') }, 'insertar', 4, () =>
      insertarTabla(3, 3),
    ),
    itemPanel(
      'insertar',
      'insertar',
      8,
      t('editor.shortcut', { label: t('editor.insert'), keys: '/' }),
      <Icon name="plus" size={16} />,
      insertarAbierto,
      abrirInsertar,
      () => setInsertarAbierto(false),
      menuInsertar,
    ),
    itemBoton(
      { cmd: 'buscar', icon: 'search', label: t('editor.find'), atajo: 'Ctrl+F' },
      'herramientas',
      2,
      () => abrirBuscador(),
    ),
    ...(onAncho
      ? [
          itemBoton(
            {
              cmd: 'anchoCompleto',
              icon: 'fitWidth',
              label: t('editor.fullWidth'),
              conEstado: true,
            },
            'herramientas',
            1,
            onAncho,
          ),
        ]
      : []),
    itemBoton(
      { cmd: 'atajos', icon: 'teclado', label: t('editor.shortcuts'), atajo: 'Ctrl+/' },
      'herramientas',
      1,
      () => {
        if (document.activeElement === ref.current) guardarSeleccion();
        setAtajos(true);
      },
    ),
    // EL ASISTENTE VA AL FINAL Y APARTE: no es formato, reescribe lo que has
    // apuntado. Entre negrita y tabla se pulsa sin querer.
    {
      id: 'ia',
      grupo: 'herramientas',
      prioridad: 1,
      barra: () => (
        <Dropdown
          label={t('editor.ai')}
          align="end"
          trigger={(props) => (
            <Tooltip label={t('editor.ai')}>
              <button
                {...props}
                type="button"
                class={cx(BOTON_DE_BARRA, 'aria-expanded:bg-bg-subtle aria-expanded:text-text')}
                aria-label={t('editor.ai')}
                disabled={iaTrabajando !== null}
              >
                {iaTrabajando !== null ? (
                  <Spinner size={16} />
                ) : (
                  <Glyph name="sparkles" size={16} />
                )}
              </button>
            </Tooltip>
          )}
        >
          {ACCIONES_IA.map(([accion, clave], i) => (
            <Fragment key={accion}>
              {i === 3 ? <MenuSeparator /> : null}
              <MenuItem size="sm" onClick={() => void aplicarIA(accion)}>
                {t(clave)}
              </MenuItem>
            </Fragment>
          ))}
        </Dropdown>
      ),
      menu: (cerrar) =>
        ACCIONES_IA.map(([accion, clave]) => (
          <OpcionDeMas
            key={accion}
            dibujo={<Glyph name="sparkles" size={16} />}
            rotulo={t(clave)}
            desactivada={iaTrabajando !== null}
            cerrar={cerrar}
            onElegir={() => void aplicarIA(accion)}
          />
        )),
    },
  ];

  return (
    <div class={cx('flex min-h-0 flex-col', estirado && 'flex-1')}>
      <BarraConMas items={items} aria-label={ariaLabel} etiquetaMas={t('editor.more')} />

      {/* El fallo del asistente no se traga: sin aviso, pulsar «ordenar» y no ver
          nada se lee como que la nota está rota. */}
      {avisoPegar ? (
        <p role="status" class="text-text-muted px-1 py-0.5 text-xs">
          {t('editor.pasteBlocked')}
        </p>
      ) : null}
      {iaError ? (
        <p role="status" class="text-danger-text px-1 py-0.5 text-xs">
          {t('editor.aiError')}
        </p>
      ) : null}

      {/* La barra del bloque donde está el cursor flota junto a él (BarraDeBloque). */}
      {contexto && conFoco ? (
        <BarraDeBloque t={t} contexto={contexto} acciones={accionesDeBloque(contexto)} />
      ) : null}

      {/* Buscar en la nota (Ctrl+F dentro del editor). Las coincidencias se
          pintan con la API de resaltados de CSS: no tocan el DOM de la nota, así
          que no hay nada que deshacer ni que se cuele al guardar. */}
      {buscador ? (
        <div class="border-border bg-surface flex flex-col border-b" role="search">
          <div class="flex items-center gap-1.5 px-1.5 py-1">
            <Input
              id={ID_BUSCAR}
              type="search"
              size="sm"
              value={buscador.q}
              class="min-w-0 flex-1"
              placeholder={t('find.placeholder')}
              aria-label={t('find.placeholder')}
              autocomplete="off"
              onInput={(e) => pintarBusqueda(e.currentTarget.value, 0)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') {
                  e.preventDefault();
                  pintarBusqueda(buscador.q, buscador.i + (e.shiftKey ? -1 : 1));
                } else if (e.key === 'Escape') {
                  e.preventDefault();
                  e.stopPropagation();
                  cerrarBuscador();
                }
              }}
            />
            {/* Lo que se ve: «2 de 5»; lo que se anuncia, cuántas hay. */}
            <span
              class="sytue-meta shrink-0 tabular-nums"
              title={t('editor.matches', { count: coincidencias.current.length })}
            >
              <span aria-hidden="true">
                {coincidencias.current.length > 0
                  ? t('find.count', {
                      current: String(buscador.i + 1),
                      total: String(coincidencias.current.length),
                    })
                  : buscador.q.trim()
                    ? t('find.none')
                    : ''}
              </span>
              <span class="sr-only" aria-live="polite">
                {buscador.q.trim()
                  ? t('editor.matches', { count: coincidencias.current.length })
                  : ''}
              </span>
            </span>
            <Toolbar aria-label={t('editor.find')} class="flex shrink-0 items-center gap-0.5">
              <BotonDeBarra
                t={t}
                activos={{ mayus: buscador.mayus }}
                a={{
                  cmd: 'mayus',
                  glyph: 'mayusculas',
                  label: t('editor.matchCase'),
                  conEstado: true,
                }}
                onPress={() => pintarBusqueda(buscador.q, 0, { mayus: !buscador.mayus })}
              />
              <BotonDeBarra
                t={t}
                activos={activos}
                a={{
                  cmd: 'findPrev',
                  icon: 'chevronUp',
                  label: t('editor.findPrev'),
                  atajo: 'Shift+Enter',
                }}
                onPress={() => pintarBusqueda(buscador.q, buscador.i - 1)}
              />
              <BotonDeBarra
                t={t}
                activos={activos}
                a={{
                  cmd: 'findNext',
                  icon: 'chevronDown',
                  label: t('editor.findNext'),
                  atajo: 'Enter',
                }}
                onPress={() => pintarBusqueda(buscador.q, buscador.i + 1)}
              />
              {/* Abre o cierra la fila de reemplazar: dice si está abierta, no si «está
                  reemplazando», y por eso no se llama igual que el botón de la fila. */}
              <Tooltip
                label={t('editor.shortcut', {
                  label: `${t('editor.replace')}…`,
                  keys: atajoDelSistema('Ctrl+H'),
                })}
              >
                <button
                  type="button"
                  class={cx(BOTON_DE_BARRA, 'aria-expanded:bg-selected aria-expanded:text-text')}
                  aria-label={`${t('editor.replace')}…`}
                  aria-expanded={buscador.reemplazar}
                  aria-controls={ID_REEMPLAZO}
                  onMouseDown={(e) => e.preventDefault()}
                  onClick={() =>
                    pintarBusqueda(buscador.q, buscador.i, { reemplazar: !buscador.reemplazar })
                  }
                >
                  <Icon name="swap" size={16} />
                </button>
              </Tooltip>
              <BotonDeBarra
                t={t}
                activos={activos}
                a={{ cmd: 'findClose', icon: 'close', label: t('find.close'), atajo: 'Escape' }}
                onPress={cerrarBuscador}
              />
            </Toolbar>
          </div>
          {buscador.reemplazar ? (
            <div class="flex items-center gap-1.5 px-1.5 pb-1">
              <Input
                id={ID_REEMPLAZO}
                type="text"
                size="sm"
                value={buscador.por}
                class="min-w-0 flex-1"
                placeholder={t('editor.replaceWith')}
                aria-label={t('editor.replaceWith')}
                autocomplete="off"
                onInput={(e) => setBuscador({ ...buscador, por: e.currentTarget.value })}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') {
                    e.preventDefault();
                    reemplazar(e.metaKey || e.ctrlKey);
                  } else if (e.key === 'Escape') {
                    e.preventDefault();
                    e.stopPropagation();
                    cerrarBuscador();
                  }
                }}
              />
              <Button
                size="sm"
                variant="secondary"
                disabled={coincidencias.current.length === 0}
                onClick={() => reemplazar(false)}
              >
                {t('editor.replace')}
              </Button>
              <Button
                size="sm"
                variant="secondary"
                disabled={coincidencias.current.length === 0}
                onClick={() => reemplazar(true)}
              >
                {t('editor.replaceAll')}
              </Button>
            </div>
          ) : null}
        </div>
      ) : null}

      {/* EL ENVOLTORIO TIENE QUE CRECER TAMBIÉN, y no basta con que crezcan el de
          arriba y el de abajo. Es `relative` porque el aviso de «escribe algo» va
          posicionado sobre el campo; al ser `display:block` y no crecer, el
          `flex-1` del campo no tenía contra qué medirse y se quedaba en su
          mínimo. Medido en el piloto: tarjeta 630px, editor 84px, 446px muertos
          debajo con la clase correcta puesta. */}
      <div
        class={cx('notes-editor-campo relative', estirado && 'flex min-h-0 flex-1 flex-col')}
        onMouseMove={(e) => {
          if (ref.current && !arrastrado.current) ponerAsa(bloqueDe(e.target as Node, ref.current));
        }}
        onDragOver={(e) => {
          const b = arrastrado.current;
          const raiz = ref.current;
          if (!b || !raiz) return;
          // Solo los bloques propios se sueltan aquí: lo demás lo pega el navegador.
          e.preventDefault();
          if (e.dataTransfer) e.dataTransfer.dropEffect = 'move';
          const destino = bloqueDe(e.target as Node, raiz);
          const caja = raiz.parentElement?.getBoundingClientRect();
          if (!destino || !caja) return;
          const r = destino.getBoundingClientRect();
          const antes = e.clientY < r.top + r.height / 2;
          setSoltarEn({ top: (antes ? r.top : r.bottom) - caja.top, destino, antes });
        }}
        onDrop={(e) => {
          const b = arrastrado.current;
          if (!b) return;
          e.preventDefault();
          if (soltarEn && llevarBloque(b, soltarEn.destino, soltarEn.antes)) emitir();
          arrastrado.current = null;
          setSoltarEn(null);
          ponerAsa(b);
        }}
      >
        <div
          ref={ref}
          contentEditable
          role="textbox"
          // `contenteditable` ya es enfocable, pero el rol explícito obliga a
          // declararlo: sin esto, un lector de pantalla anuncia un campo de texto
          // al que la tabulación no llega.
          tabIndex={0}
          aria-multiline="true"
          {...(abierto && sugerencias.length > 0
            ? {
                'aria-controls': 'notes-menu-sugerencias',
                'aria-activedescendant': `notes-sug-${activa}`,
              }
            : {})}
          aria-label={ariaLabel}
          class={cx(
            'sytue-prose text-text min-h-24 rounded-b-md px-3 py-2 text-base leading-relaxed',
            // `outline-none` y no solo quitar la clase: sin ella, el navegador pinta
            // su propio contorno de `contenteditable` y el recuadro vuelve.
            sinAro ? 'outline-none' : 'sytue-focus-ring',
            // EN UNA TARJETA ESTIRADA, LA ZONA DE ESCRIBIR LLEGA HASTA ABAJO.
            // `min-h-24` son 96px: en una nota ensanchada a 70dvh eso dejaba
            // más de quinientos píxeles de blanco donde pulsar no hacía nada, y
            // la nota parecía rota. Es el mismo arreglo que ya llevaba el cuerpo
            // en lectura, que aquí faltaba.
            estirado && 'flex-1',
          )}
          onInput={(e) => {
            const ev = e as unknown as InputEvent;
            // DESPUÉS de que el navegador acabe la edición que avisa (una microtarea): una
            // orden de edición dentro del `input` de otra la rechaza Chromium, y la
            // selección de las marcas se quedaba puesta para que la siguiente tecla se
            // las comiera. Visto en Chromium sin cabeza con el banco del editor.
            if (ev.inputType === 'insertText' && !ev.isComposing && /[*_`~]$/.test(ev.data ?? ''))
              queueMicrotask(() => {
                if (!aplicarAtajoEnLinea()) return;
                emitir();
                refrescar();
              });
            guardarSeleccion();
            emitir();
            refrescar();
            mirarMenu();
          }}
          onBlur={() => {
            ultimoAuto.current = null;
            guardarSeleccion();
            emitir();
            setMenu(null);
          }}
          onKeyUp={() => {
            guardarSeleccion();
            refrescar();
            mirarMenu();
          }}
          onMouseUp={() => {
            guardarSeleccion();
            refrescar();
            mirarMenu();
          }}
          onClick={(e) => {
            const target = e.target as HTMLElement | null;
            // El título de una sección que se pliega se escribe: pulsarlo pone el cursor.
            // Solo el triángulo de su izquierda (los primeros 24 px) la abre o la cierra,
            // como en Confluence; si no, cada clic para escribir la plegaría.
            const titulo = target?.closest('summary');
            if (titulo && ref.current?.contains(titulo)) {
              const izquierda = titulo.getBoundingClientRect().left;
              if (e.clientX > izquierda + 24) e.preventDefault();
            }
            if (target instanceof HTMLInputElement && target.type === 'checkbox') {
              if (target.checked) {
                target.setAttribute('checked', '');
              } else {
                target.removeAttribute('checked');
              }
              emitir();
            }
          }}
          onPaste={(e) => {
            // Ver la cabecera: la estructura del origen, sin su estilo ni sus atributos;
            // el Markdown, convertido; y con Mayúsculas, el texto plano.
            e.preventDefault();
            e.stopPropagation();
            // En un bloque de código el Markdown se queda en letras, como al teclear.
            const ancla = window.getSelection()?.anchorNode;
            const enCodigo = !!(ancla instanceof Element ? ancla : ancla?.parentElement)?.closest(
              'pre',
            );
            const pegado = queSePega(
              e.clipboardData,
              pegarSinFormato.current,
              !enCodigo,
              window.getSelection()?.toString() ?? '',
            );
            pegarSinFormato.current = false;
            if ('html' in pegado) executeEditingCommand('insertHTML', pegado.html);
            else if (pegado.texto) executeEditingCommand('insertText', pegado.texto);
            emitir();
          }}
          onMouseDown={() => {
            setElegido(null);
            ultimoAuto.current = null;
          }}
          onContextMenu={(e) => {
            // El menú de la celda con el botón derecho (o Mayús+F10), con lo mismo que la
            // barra de la tabla. Fuera de una tabla, el del navegador.
            const celda = celdaDe(e.target as Node);
            if (!celda) return;
            e.preventDefault();
            // El menú actúa sobre la celda pulsada: el cursor va a ella si no estaba ya.
            if (celdaDe(window.getSelection()?.anchorNode ?? null) !== celda) entrarEnCelda(celda);
            guardarSeleccion();
            refrescar();
            setMenuCelda({ x: e.clientX, y: e.clientY });
          }}
          onKeyDown={(e) => {
            const mod = e.metaKey || e.ctrlKey;
            if (elegido) setElegido(null);
            // Ctrl/⌘+Z JUSTO después de un atajo de Markdown devuelve lo tecleado tal
            // cual; cualquier otra tecla (salvo un modificador suelto) lo olvida.
            const deshacerAtajo = ultimoAuto.current;
            if (!/^(Shift|Control|Meta|Alt)$/.test(e.key)) ultimoAuto.current = null;
            if (deshacerAtajo && mod && !e.shiftKey && !e.altKey && e.key.toLowerCase() === 'z') {
              e.preventDefault();
              deshacerAtajo();
              refrescar();
              emitir();
              return;
            }
            // El evento `paste` no dice qué teclas lo trajeron: Ctrl/⌘+Mayús+V se apunta
            // aquí, justo antes, y lo consume `onPaste`. Cualquier otra tecla lo olvida.
            pegarSinFormato.current = mod && e.shiftKey && e.key.toLowerCase() === 'v';
            // Con un menú abierto, las flechas, Intro, Tab y Escape son suyas.
            if (menu && abierto && !mod && !e.altKey) {
              const n = sugerencias.length;
              if ((e.key === 'ArrowDown' || e.key === 'ArrowUp') && n > 0) {
                e.preventDefault();
                setMenu({ ...menu, idx: moverActiva(activa, n, e.key) });
                return;
              }
              if ((e.key === 'Enter' || e.key === 'Tab') && n > 0) {
                e.preventDefault();
                sugerencias[activa]?.elegir();
                return;
              }
              if (e.key === 'Escape') {
                e.preventDefault();
                e.stopPropagation();
                const d = disparoEnCursor();
                descartada.current = d ? { nodo: d.nodo, off: d.off } : null;
                setMenu(null);
                return;
              }
            }
            // En el título de una sección que se pliega, Espacio escribe un espacio (no la
            // pliega, que es lo que hace Chromium con un `<summary>` editable) e Intro baja
            // al contenido, abriéndola si estaba cerrada.
            const enTitulo = (() => {
              const n = window.getSelection()?.anchorNode ?? null;
              const el = n?.nodeType === Node.TEXT_NODE ? n.parentElement : (n as Element | null);
              const t0 = el?.closest('summary');
              return t0 && ref.current?.contains(t0) ? t0 : null;
            })();
            if (enTitulo && !mod && !e.altKey && (e.key === ' ' || e.key === 'Enter')) {
              e.preventDefault();
              if (e.key === ' ') {
                executeEditingCommand('insertText', ' ');
              } else {
                const seccion = enTitulo.parentElement as HTMLDetailsElement;
                seccion.open = true;
                let destino = enTitulo.nextElementSibling;
                if (!destino) {
                  destino = document.createElement('p');
                  destino.innerHTML = '<br>';
                  seccion.append(destino);
                }
                const r = document.createRange();
                r.selectNodeContents(destino);
                r.collapse(true);
                const sel = window.getSelection();
                sel?.removeAllRanges();
                sel?.addRange(r);
              }
              emitir();
              refrescar();
              return;
            }
            // Alt+Mayús+↑/↓ sube o baja el bloque del cursor (el cursor va con él), y
            // Mayús+F10 abre el menú del bloque: el asa, para quien no usa el ratón.
            if (e.altKey && e.shiftKey && !mod && (e.key === 'ArrowUp' || e.key === 'ArrowDown')) {
              const b = ref.current
                ? bloqueDe(window.getSelection()?.anchorNode ?? null, ref.current)
                : null;
              if (b) {
                e.preventDefault();
                if (moverBloque(b, e.key === 'ArrowUp' ? -1 : 1)) {
                  emitir();
                  refrescar();
                }
                return;
              }
            }
            if (
              e.shiftKey &&
              e.key === 'F10' &&
              !celdaDe(window.getSelection()?.anchorNode ?? null)
            ) {
              const b = ref.current
                ? bloqueDe(window.getSelection()?.anchorNode ?? null, ref.current)
                : null;
              if (b) {
                e.preventDefault();
                const r = b.getBoundingClientRect();
                guardarSeleccion();
                abrirMenuBloque(b, r.left, r.top);
                return;
              }
            }
            // Ctrl/⌘+/ abre la hoja de atajos. Por `key`: en un teclado español la barra es
            // Mayús+7, así que no se mira Mayúsculas.
            if (mod && !e.altKey && (e.key === '/' || e.code === 'Slash')) {
              e.preventDefault();
              guardarSeleccion();
              setAtajos(true);
              return;
            }
            if (mod && !e.altKey && !e.shiftKey && /^[fh]$/.test(e.key.toLowerCase())) {
              e.preventDefault();
              abrirBuscador(e.key.toLowerCase() === 'h');
              return;
            }
            if (mod && e.key === '.') {
              e.preventDefault();
              alternarPlegadoAqui();
              return;
            }
            // Ctrl/⌘+B, I y U por la misma puerta que la barra: el navegador los aplica
            // por su cuenta con el `styleWithCSS` que haya puesto, y con el del color
            // la negrita salía como `<span style>` y se perdía al guardar.
            const conmuta =
              mod && !e.altKey && !e.shiftKey && TECLAS_DE_FORMATO[e.key.toLowerCase()];
            if (conmuta) {
              e.preventDefault();
              aplicar(conmuta);
              return;
            }
            // Los atajos que la barra enseña en su ayuda: tachado, código, resaltado y
            // quitar formato. Por `code` y no por `key`: con Mayúsculas la tecla cambia.
            if (mod && !e.altKey) {
              const otro =
                e.shiftKey && e.code === 'KeyX'
                  ? () => aplicar('strikeThrough')
                  : e.shiftKey && e.code === 'KeyH'
                    ? () => aplicar('hiliteColor', RESALTADO)
                    : !e.shiftKey && e.code === 'KeyE'
                      ? () => envolver('code')
                      : !e.shiftKey && (e.code === 'Backslash' || e.key === '\\')
                        ? () => aplicar('removeFormat')
                        : null;
              if (otro) {
                e.preventDefault();
                otro();
                return;
              }
            }
            if (e.key === 'Escape') {
              e.stopPropagation();
              onDone?.(false);
            } else if (e.key === 'Enter' && mod) {
              e.preventDefault();
              onDone?.(true);
            } else if (mod && e.key.toLowerCase() === 'k') {
              e.preventDefault();
              abrirEnlaces();
            } else if (e.key === 'Tab') {
              e.preventDefault();
              // En una tabla, Tab va a la celda siguiente y Mayús+Tab a la anterior; en la
              // última, Tab añade una fila, como en cualquier hoja.
              const celda = celdaDe(window.getSelection()?.anchorNode ?? null);
              if (celda && ref.current?.contains(celda)) {
                let destino = celdaVecina(celda, e.shiftKey ? -1 : 1);
                if (!destino && !e.shiftKey) {
                  enCelda((x) => insertarFila(x, 'despues'));
                  destino = celdaVecina(celda, 1);
                }
                if (destino) entrarEnCelda(destino);
                refrescar();
                return;
              }
              if (e.shiftKey) {
                aplicar('outdent');
              } else {
                aplicar('indent');
              }
              return;
            }

            const sel = window.getSelection();
            const anchor = sel?.anchorNode;
            const taskRow = (
              anchor?.nodeType === Node.TEXT_NODE
                ? anchor.parentElement
                : (anchor as HTMLElement | null)
            )?.closest('.task-row');

            if (taskRow && ref.current?.contains(taskRow) && !mod && !e.altKey && !e.shiftKey) {
              if (e.key === 'Enter') {
                e.preventDefault();
                const span = taskRow.querySelector('span');
                const taskText = span?.textContent?.replace(/\u00a0/g, ' ').trim() ?? '';
                if (taskText === '') {
                  const p = document.createElement('p');
                  p.innerHTML = '<br>';
                  taskRow.replaceWith(p);
                  const newRange = document.createRange();
                  newRange.setStart(p, 0);
                  newRange.collapse(true);
                  sel?.removeAllRanges();
                  sel?.addRange(newRange);
                } else {
                  let afterText = '';
                  const range = sel?.getRangeAt(0);
                  if (range && span && span.contains(range.startContainer)) {
                    const afterRange = document.createRange();
                    afterRange.setStart(range.startContainer, range.startOffset);
                    afterRange.setEnd(span, span.childNodes.length);
                    afterText = afterRange.toString();
                    afterRange.deleteContents();
                  }
                  const newTask = document.createElement('div');
                  newTask.className = 'task-row';
                  const cleanAfter = afterText.trim();
                  newTask.innerHTML = `<input type="checkbox" /> <span>${cleanAfter ? escapar(cleanAfter) : '&nbsp;'}</span>`;
                  taskRow.after(newTask);
                  const newSpan = newTask.querySelector('span');
                  if (newSpan) {
                    const newRange = document.createRange();
                    newRange.selectNodeContents(newSpan);
                    newRange.collapse(!cleanAfter);
                    sel?.removeAllRanges();
                    sel?.addRange(newRange);
                  }
                }
                refrescar();
                emitir();
                return;
              }

              if (e.key === 'Backspace') {
                const span = taskRow.querySelector('span');
                const range = sel?.getRangeAt(0);
                if (range && range.collapsed && span) {
                  const isAtStart =
                    (range.startContainer === span && range.startOffset === 0) ||
                    (range.startContainer.nodeType === Node.TEXT_NODE &&
                      span.contains(range.startContainer) &&
                      range.startOffset === 0 &&
                      (range.startContainer === span.firstChild ||
                        range.startContainer.previousSibling === null));
                  const taskText = span.textContent?.replace(/\u00a0/g, ' ').trim() ?? '';
                  if (isAtStart || taskText === '') {
                    e.preventDefault();
                    const p = document.createElement('p');
                    p.innerHTML = taskText ? span.innerHTML : '<br>';
                    taskRow.replaceWith(p);
                    const newRange = document.createRange();
                    newRange.selectNodeContents(p);
                    newRange.collapse(true);
                    sel?.removeAllRanges();
                    sel?.addRange(newRange);
                    refrescar();
                    emitir();
                    return;
                  }
                }
              }
            }

            if ((e.key === ' ' || e.key === 'Enter') && !mod && !e.altKey && !e.shiftKey) {
              if (aplicarAtajoMarkdown(e.key)) e.preventDefault();
            } else if (mod && e.altKey && /^[0-6]$/.test(e.key)) {
              // Ctrl+Alt+0 párrafo, Ctrl+Alt+1..5 títulos: Ctrl+1..5 cambia de
              // pestaña en varios navegadores.
              e.preventDefault();
              aplicar('formatBlock', e.key === '0' ? '<p>' : `<h${e.key}>`);
            }
          }}
        />
        {menu && abierto && typeof document !== 'undefined'
          ? createPortal(
              <div
                ref={menuRef}
                class={cx('fixed max-w-[calc(100vw-16px)]', menu.tipo === 'cmd' ? 'w-80' : 'w-72')}
                style={{
                  zIndex: 'var(--z-dropdown)',
                  left: `${menu.x}px`,
                  top: `${menu.y}px`,
                  transform: menu.arriba ? 'translateY(-100%)' : undefined,
                }}
              >
                <div class="sytue-pop-in sytue-menu-surface border-border max-h-80 overflow-y-auto rounded-lg border p-1 shadow-[var(--shadow-md)]">
                  <div
                    id="notes-menu-sugerencias"
                    role="listbox"
                    aria-label={t(ROTULO_MENU[menu.tipo])}
                  >
                    {sugerencias.map((s, i) => (
                      <Fragment key={s.clave}>
                        {/* El rótulo de cada grupo va sobre su primera fila, sin llevarse
                            el teclado: es texto, no una opción. */}
                        {s.grupo && s.grupo !== sugerencias[i - 1]?.grupo ? (
                          <p class="sytue-meta px-2.5 pt-1 pb-0.5" aria-hidden="true">
                            {s.grupo}
                          </p>
                        ) : null}
                        <div
                          id={`notes-sug-${i}`}
                          role="option"
                          tabIndex={-1}
                          aria-selected={i === activa}
                          class={cx(
                            'text-text flex cursor-pointer items-center gap-2 rounded-md px-2.5 py-1.5 text-sm',
                            i === activa ? 'bg-selected' : 'hover:bg-bg-subtle',
                          )}
                          onMouseDown={(e) => {
                            e.preventDefault();
                            s.elegir();
                          }}
                          onMouseEnter={() => setMenu({ ...menu, idx: i })}
                        >
                          <span class="text-text-muted grid h-5 w-5 shrink-0 place-items-center">
                            {s.dibujo}
                          </span>
                          <span class="min-w-0 flex-1">
                            <span class="block truncate">
                              {s.rotulo}
                              {s.dentro ? <span class="text-text-muted"> › {s.dentro}</span> : null}
                            </span>
                            {s.descripcion ? (
                              <span class="text-text-muted block truncate text-xs">
                                {s.descripcion}
                              </span>
                            ) : null}
                          </span>
                          {s.detalle ? (
                            <span class="sytue-meta max-w-[45%] shrink-0 truncate">
                              {s.detalle}
                            </span>
                          ) : null}
                        </div>
                      </Fragment>
                    ))}
                  </div>
                  {menu.tipo === 'cmd' && sugerencias.length === 0 ? (
                    <p class="text-text-muted px-2.5 py-1.5 text-sm" role="status">
                      {t('editor.slashNoResults')}
                    </p>
                  ) : null}
                  {menu.tipo === 'nota' && (sugerencias.length === 0 || notasMenu.buscando) ? (
                    <p
                      class="text-text-muted flex items-center gap-2 px-2.5 py-1.5 text-sm"
                      role="status"
                    >
                      {notasMenu.buscando ? (
                        <>
                          <Spinner size={14} /> {t('editor.linkSearching')}
                        </>
                      ) : (
                        t('editor.linkNoResults')
                      )}
                    </p>
                  ) : null}
                </div>
              </div>,
              document.body,
            )
          : null}
        {burbuja && !menu && typeof document !== 'undefined'
          ? createPortal(
              <div
                ref={burbujaRef}
                class="fixed w-max"
                style={{
                  zIndex: 'var(--z-dropdown)',
                  left: `${burbuja.x}px`,
                  top: burbuja.abajo ? `${burbuja.y + 8}px` : `${burbuja.y - 6}px`,
                  transform: burbuja.abajo ? 'translateX(-50%)' : 'translate(-50%, -100%)',
                }}
              >
                <Toolbar
                  aria-label={t('editor.selectionTools')}
                  omitir='[role="menu"]'
                  class="sytue-pop-in sytue-menu-surface border-border flex items-center gap-0.5 rounded-lg border p-1 shadow-[var(--shadow-md)]"
                >
                  <BotonDeBarra
                    t={t}
                    activos={activos}
                    a={formato[0]!}
                    onPress={() => aplicar('bold')}
                  />
                  <BotonDeBarra
                    t={t}
                    activos={activos}
                    a={formato[1]!}
                    onPress={() => aplicar('italic')}
                  />
                  {/* En el teléfono, tachado, resaltado y código van en «Más formato»: así
                      la burbuja cabe en 390 px, como la de Fyne. */}
                  {burbuja.estrecha ? (
                    <Dropdown
                      label={t('editor.moreFormatting')}
                      trigger={disparador(
                        t('editor.moreFormatting'),
                        <Icon name="text" size={16} />,
                        {
                          flecha: true,
                        },
                      )}
                    >
                      {[
                        { a: formato[3]!, hacer: () => aplicar('strikeThrough') },
                        { a: resaltado, hacer: () => aplicar('hiliteColor', RESALTADO) },
                        { a: codigoEnLinea, hacer: () => envolver('code') },
                      ].map(({ a, hacer }) => (
                        <MenuItem
                          key={a.cmd}
                          size="sm"
                          icon={dibujoDe(a)}
                          onMouseDown={(e: MouseEvent) => e.preventDefault()}
                          onClick={hacer}
                        >
                          {a.label}
                        </MenuItem>
                      ))}
                    </Dropdown>
                  ) : (
                    <>
                      <BotonDeBarra
                        t={t}
                        activos={activos}
                        a={formato[3]!}
                        onPress={() => aplicar('strikeThrough')}
                      />
                      <BotonDeBarra
                        t={t}
                        activos={activos}
                        a={resaltado}
                        onPress={() => aplicar('hiliteColor', RESALTADO)}
                      />
                      <BotonDeBarra
                        t={t}
                        activos={activos}
                        a={codigoEnLinea}
                        onPress={() => envolver('code')}
                      />
                    </>
                  )}
                  <BotonDeBarra
                    t={t}
                    activos={activos}
                    a={{
                      cmd: 'link',
                      glyph: 'link',
                      label: t('editor.linkTool'),
                      atajo: 'Ctrl+K',
                    }}
                    onPress={abrirEnlaces}
                  />
                  {/* El portapapeles, en su propio grupo: no es formato. */}
                  <span class={SEPARADOR_DE_BARRA} aria-hidden="true" />
                  <BotonDeBarra
                    t={t}
                    activos={activos}
                    a={{ cmd: 'copy', glyph: 'copy', label: t('editor.copy'), atajo: 'Ctrl+C' }}
                    onPress={() => void copiar(false)}
                  />
                  <BotonDeBarra
                    t={t}
                    activos={activos}
                    a={{ cmd: 'cut', glyph: 'tijeras', label: t('editor.cut'), atajo: 'Ctrl+X' }}
                    onPress={() => void copiar(true)}
                  />
                  <BotonDeBarra
                    t={t}
                    activos={activos}
                    a={{ cmd: 'paste', glyph: 'pegar', label: t('editor.paste'), atajo: 'Ctrl+V' }}
                    onPress={() => void pegar()}
                  />
                </Toolbar>
              </div>,
              document.body,
            )
          : null}
        {vacio && placeholder ? (
          <span
            class="text-text-subtle pointer-events-none absolute top-2 left-3 text-base"
            aria-hidden="true"
          >
            {placeholder}
          </span>
        ) : null}
        {/* Los números de línea del código: una capa encima del campo, no texto de la
            nota (no se guardan ni se copian). */}
        {asa?.el.isConnected ? (
          <Tooltip label={t('editor.blockMenu')} placement="right">
            <button
              type="button"
              class="notes-asa sytue-focus-ring"
              style={{ top: `${asa.top}px` }}
              aria-label={t('editor.blockMenu')}
              aria-haspopup="menu"
              aria-expanded={menuBloque !== null}
              draggable
              onMouseDown={(e) => e.preventDefault()}
              onClick={(e) => {
                const r = e.currentTarget.getBoundingClientRect();
                guardarSeleccion();
                abrirMenuBloque(asa.el, r.left, r.bottom);
              }}
              onDragStart={(e) => {
                arrastrado.current = asa.el;
                e.dataTransfer?.setData('text/plain', '');
                if (e.dataTransfer) e.dataTransfer.effectAllowed = 'move';
              }}
              onDragEnd={() => {
                arrastrado.current = null;
                setSoltarEn(null);
              }}
            >
              <Icon name="grip" size={14} />
            </button>
          </Tooltip>
        ) : null}
        {soltarEn ? (
          <span class="notes-soltar" aria-hidden="true" style={{ top: `${soltarEn.top}px` }} />
        ) : null}
        {menuBloque ? (
          <Dropdown
            label={t('editor.blockMenu')}
            abiertoEn={menuBloque}
            onCerrar={() => setMenuBloque(null)}
          >
            {(cerrar) => {
              const opcion = (rotulo: string, dibujo: JSX.Element, hacer: () => void) => (
                <MenuItem
                  size="sm"
                  icon={dibujo}
                  onMouseDown={(e: MouseEvent) => e.preventDefault()}
                  onClick={() => {
                    cerrar();
                    hacer();
                  }}
                >
                  {rotulo}
                </MenuItem>
              );
              const convertir = (rotulo: string, dibujo: JSX.Element, hacer: () => void) =>
                opcion(rotulo, dibujo, () => conBloque(hacer));
              return (
                <>
                  {opcion(t('editor.moveUp'), <Icon name="chevronUp" size={16} />, () =>
                    conBloque((b) => moverBloque(b, -1)),
                  )}
                  {opcion(t('editor.moveDown'), <Icon name="chevronDown" size={16} />, () =>
                    conBloque((b) => moverBloque(b, 1)),
                  )}
                  {opcion(t('editor.duplicate'), <Glyph name="copy" size={16} />, () =>
                    conBloque((b) => duplicarBloque(b)),
                  )}
                  {opcion(t('editor.delete'), <Glyph name="trash" size={16} />, () =>
                    conBloque((b) => quitarBloque(b, ref.current!)),
                  )}
                  <MenuSeparator />
                  <p class="sytue-meta px-2.5 pt-1 pb-0.5" aria-hidden="true">
                    {t('editor.convertTo')}
                  </p>
                  {convertir(t('editor.paragraph'), <Icon name="text" size={16} />, () =>
                    ordenar('formatBlock', '<p>'),
                  )}
                  {convertir(t('editor.heading1'), <Glyph name="heading" size={16} />, () =>
                    ordenar('formatBlock', '<h1>'),
                  )}
                  {convertir(t('editor.heading2'), <Glyph name="heading" size={16} />, () =>
                    ordenar('formatBlock', '<h2>'),
                  )}
                  {convertir(t('editor.heading3'), <Glyph name="heading" size={16} />, () =>
                    ordenar('formatBlock', '<h3>'),
                  )}
                  {convertir(t('editor.bulletList'), <Glyph name="bulletList" size={16} />, () =>
                    ordenar('insertUnorderedList'),
                  )}
                  {convertir(
                    t('editor.numberedList'),
                    <Glyph name="numberedList" size={16} />,
                    () => ordenar('insertOrderedList'),
                  )}
                  {convertir(t('editor.quote'), <Glyph name="quote" size={16} />, () =>
                    ordenar('formatBlock', '<blockquote>'),
                  )}
                  {convertir(t('editor.codeBlock'), <Icon name="code" size={16} />, () =>
                    ordenar('formatBlock', '<pre>'),
                  )}
                  {opcion(t('editor.panelInfo'), <Glyph name="panel" size={16} />, () =>
                    conBloque((b) => enPanel(b)),
                  )}
                </>
              );
            }}
          </Dropdown>
        ) : null}
        {marcasBusqueda.map((m, n) => (
          <span
            key={n}
            aria-hidden="true"
            class={cx('notes-coincidencia', m.actual && 'notes-coincidencia--actual')}
            style={{
              top: `${m.top}px`,
              left: `${m.left}px`,
              width: `${m.width}px`,
              height: `${m.height}px`,
            }}
          />
        ))}
        {celdaActiva && conFoco ? (
          <span
            class="notes-celda-activa"
            aria-hidden="true"
            style={{
              top: `${celdaActiva.top}px`,
              left: `${celdaActiva.left}px`,
              width: `${celdaActiva.width}px`,
              height: `${celdaActiva.height}px`,
            }}
          />
        ) : null}
        {numeros.map((x, i) => (
          <span
            key={i}
            class="notes-lineas"
            aria-hidden="true"
            style={{ top: `${x.top}px`, left: `${x.left}px` }}
          >
            {Array.from({ length: x.n }, (_, k) => k + 1).join('\n')}
          </span>
        ))}
        {menuCelda && contexto?.tipo === 'tabla' ? (
          <Dropdown
            label={t('editor.tableTools')}
            abiertoEn={menuCelda}
            onCerrar={() => setMenuCelda(null)}
          >
            {(cerrar) => {
              const a = accionesDeBloque(contexto).tabla;
              const opcion = (rotulo: string, dibujo: GlyphName, hacer: () => void) => (
                <MenuItem
                  size="sm"
                  icon={<Glyph name={dibujo} size={16} />}
                  onMouseDown={(e: MouseEvent) => e.preventDefault()}
                  onClick={() => {
                    cerrar();
                    hacer();
                  }}
                >
                  {rotulo}
                </MenuItem>
              );
              return (
                <>
                  {opcion(t('editor.rowAbove'), 'filaArriba', () => a.fila('antes'))}
                  {opcion(t('editor.rowBelow'), 'filaAbajo', () => a.fila('despues'))}
                  {opcion(t('editor.colBefore'), 'columnaAntes', () => a.columna('antes'))}
                  {opcion(t('editor.colAfter'), 'columnaDespues', () => a.columna('despues'))}
                  <MenuSeparator />
                  {opcion(t('editor.headerRow'), 'cabeceraFila', a.cabeceraFila)}
                  {opcion(t('editor.headerColumn'), 'cabeceraColumna', a.cabeceraColumna)}
                  <MenuSeparator />
                  {opcion(t('editor.rowDelete'), 'filaQuitar', a.quitarFila)}
                  {opcion(t('editor.colDelete'), 'columnaQuitar', a.quitarColumna)}
                  {opcion(t('editor.tableDelete'), 'trash', a.quitarTabla)}
                </>
              );
            }}
          </Dropdown>
        ) : null}
      </div>
      {/* El pie: palabras, caracteres y lectura, en voz baja y con altura fija (no
          mueve nada al aparecer). No se anuncia: cambiaría con cada tecla. */}
      <div class="text-text-subtle flex h-6 shrink-0 items-center justify-end gap-3 px-3 text-xs tabular-nums select-none">
        <span>{t('editor.words', { count: cuentas.palabras })}</span>
        <span class="max-sm:hidden">{t('editor.chars', { count: cuentas.caracteres })}</span>
        <span>
          {t('editor.readingTime', {
            minutes: cuentas.palabras ? Math.max(1, cuentas.minutos) : 0,
          })}
        </span>
      </div>
      <ShortcutsModal
        open={atajos}
        title={t('editor.shortcuts')}
        groups={atajos ? atajosDelEditor(t) : []}
        onClose={() => {
          setAtajos(false);
          window.setTimeout(enfocarTexto, 0);
        }}
      />
    </div>
  );
}
