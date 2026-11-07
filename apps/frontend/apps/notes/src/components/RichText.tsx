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
 * del servicio conserva— y no como `<font>` o `align=`, que tiraría.
 *
 * EL PEGADO ENTRA COMO TEXTO PLANO, a propósito. Insertar el HTML del
 * portapapeles en el DOM lo EJECUTA aquí y ahora, antes de que ningún servidor
 * pueda opinar: pegar desde una página hostil sería XSS en tu propia sesión. El
 * formato que se escribe con esta barra se conserva entero; lo que se pierde es
 * el formato de lo que se pega de fuera.
 * ponytail: si algún día hace falta conservarlo, lo que toca es un saneador de
 * allowlist en `@sytue/ui` (compartido con Mail), no relajar esto.
 */
import { Fragment, type JSX } from 'preact';
import { useCallback, useEffect, useRef, useState } from 'preact/hooks';
import { createPortal } from 'preact/compat';
import {
  BARRA_DE_EDICION,
  BOTON_DE_BARRA,
  Dropdown,
  FILA_UNICA_EN_MOVIL,
  Icon,
  Input,
  MASCARA_SCROLL_X,
  MenuItem,
  MenuSeparator,
  Popover,
  SEPARADOR_DE_BARRA,
  Spinner,
  Toolbar,
  Tooltip,
  cx,
  useScrollRestante,
  useFitInViewport,
  useMobileBack,
  atajoDelSistema,
} from '@sytue/ui';
import { candidatos, consultaMencion, moverActiva, type Persona } from '@sytue/shell';
import {
  editingCommandEnabled,
  editingCommandState,
  editingCommandValue,
  executeEditingCommand,
} from '@sytue/ui/browser/editing';
import {
  ESTILO_CELDA,
  celdaDe,
  eliminarColumna,
  eliminarFila,
  eliminarTabla,
  insertarColumna,
  insertarFila,
} from '@sytue/ui/browser/tabla';
import type { T } from '../i18n/index.js';
import { escapar } from '../client/captura.js';
import { atajoMarkdown } from '../client/markdown.js';
import {
  consultaDeBarra,
  consultaDeEnlace,
  filtrarComandos,
  type Comando,
  type IdComando,
} from '../client/comandos.js';
import { coincidenciasEn, liPlegable } from '../client/en-nota.js';
import { Glyph, type GlyphName } from './glyphs.js';
import { construirPeticion, leerRespuesta, type AccionIA } from '../lib/ai.js';
import { bloquesDe, type Bloque } from '@sytue/ai';
import { LinkTool, useOpcionesDeEnlace, type FuentesDeEnlace, type Opcion } from './LinkTool.js';

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
}

interface Accion {
  cmd: string;
  valor?: string;
  glyph?: GlyphName;
  icon?: 'code' | 'highlighter' | 'minus' | 'chevronUp' | 'chevronDown' | 'close';
  label: string;
  atajo?: string;
  /** Con estado (aria-pressed) o solo acción. */
  conEstado?: boolean;
  desactivada?: boolean;
}

const BLOQUES: {
  valor: string;
  clave: 'paragraph' | 'h1' | 'h2' | 'h3' | 'h4' | 'h5' | 'quote' | 'codeBlock';
}[] = [
  { valor: 'p', clave: 'paragraph' },
  { valor: 'h1', clave: 'h1' },
  { valor: 'h2', clave: 'h2' },
  { valor: 'h3', clave: 'h3' },
  { valor: 'h4', clave: 'h4' },
  { valor: 'h5', clave: 'h5' },
  { valor: 'blockquote', clave: 'quote' },
  { valor: 'pre', clave: 'codeBlock' },
];

/** Colores de texto legibles sobre claro y oscuro. El primero es «por defecto». */
const COLORES = [
  '#b91c1c',
  '#c2410c',
  '#a16207',
  '#15803d',
  '#0e7490',
  '#1d4ed8',
  '#7e22ce',
  '#be185d',
];
const RESALTADO = '#fff3a3';

/** El dibujo de cada comando de «/»: el mismo que su botón en la barra. */
const DIBUJO_COMANDO: Record<IdComando, { glyph?: GlyphName; icon?: 'text' | 'code' | 'minus' }> = {
  p: { icon: 'text' },
  h1: { glyph: 'heading' },
  h2: { glyph: 'heading' },
  h3: { glyph: 'heading' },
  tarea: { glyph: 'todo' },
  vinetas: { glyph: 'bulletList' },
  numeros: { glyph: 'numberedList' },
  cita: { glyph: 'quote' },
  codigo: { icon: 'code' },
  regla: { icon: 'minus' },
  tabla: { glyph: 'table' },
};

/** Los tres menús que se abren al teclear: un bloque, una nota que enlazar, alguien. */
type TipoMenu = 'cmd' | 'nota' | 'persona';

/**
 * Lo que abre cada menú y cómo se lee lo escrito tras ello. `[[` va primero: el nombre
 * de una nota puede llevar « /», y mientras no se cierre se está buscando una nota.
 */
const DISPAROS: readonly (readonly [TipoMenu, string, (antes: string) => string | null])[] = [
  ['nota', '[[', consultaDeEnlace],
  ['persona', '@', consultaMencion],
  ['cmd', '/', consultaDeBarra],
];

const ROTULO_MENU = {
  cmd: 'slash.title',
  nota: 'editor.linkNote',
  persona: 'editor.mention',
} as const;

/** Una fila de cualquiera de los tres menús. */
interface Sugerencia {
  clave: string;
  dibujo: JSX.Element;
  rotulo: string;
  /** El título dentro de la nota, tras «›». */
  dentro?: string;
  /** A la derecha y en gris: el atajo del comando, el cuaderno, el correo. */
  detalle?: string;
  /** El grupo al que pertenece (este cuaderno, otros cuadernos). */
  grupo?: string;
  elegir: () => void;
}

/** Id del cuadro de «Buscar en la nota», para darle el foco al abrirlo. */
const ID_BUSCAR = 'notes-buscar-en-nota';

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

export function RichText({
  t,
  initialHtml,
  ariaLabel,
  placeholder,
  onChange,
  onDone,
  enlaces,
  estirado = false,
}: Props): JSX.Element {
  const ref = useRef<HTMLDivElement>(null);
  // Qué comandos están activos en la selección actual. Sin esto, el botón de
  // negrita se ve igual dentro y fuera de un texto en negrita y hay que pulsarlo
  // para averiguar en qué estado se estaba.
  const [activos, setActivos] = useState<Record<string, boolean>>({});
  const [bloque, setBloque] = useState('p');
  const [vacio, setVacio] = useState(initialHtml.trim() === '');
  const [enlaceAbierto, setEnlaceAbierto] = useState(false);
  // Celda de tabla donde está el cursor: mientras la hay, la barra enseña las
  // acciones de tabla. Mismo trato que en el redactor de Sytue Mail, con el
  // mismo módulo (@sytue/ui/browser/tabla): dos implementaciones del mismo
  // gesto acaban comportándose distinto.
  const [enTabla, setEnTabla] = useState(false);
  // En móvil las barras NO se envuelven en filas: se desplazan de lado (ver
  // scrollX.ts). Con veinticinco controles, `flex-wrap` a secas apilaba cuatro o
  // cinco filas y la nota quedaba en un par de líneas. La de tabla es
  // condicional, así que su hook depende de `enTabla`: sin él corre con el ref
  // vacío y no se vuelve a enlazar nunca.
  const barra = useScrollRestante<HTMLDivElement>();
  const barraTabla = useScrollRestante<HTMLDivElement>(enTabla);
  // Si hay algo que deshacer o rehacer, según el navegador: sin esto los dos
  // botones se ven iguales con la pila vacía y con veinte cambios dentro.
  const [puede, setPuede] = useState({ undo: false, redo: false });
  // Las herramientas pueden tomar el foco (menús, colores, enlaces, teclado).
  // El rango sigue perteneciendo al texto hasta volver de ellas.
  const seleccion = useRef<Range | null>(null);

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
  // `[[`: la búsqueda de la herramienta de enlaces, la misma, sin Drive.
  const notasMenu = useOpcionesDeEnlace(menu?.tipo === 'nota' ? menu.q : null, enlaces, false);
  // `@`: la gente se pide a la primera arroba y se filtra aquí.
  const [gente, setGente] = useState<readonly Persona[] | null>(null);
  const pidiendoGente = menu?.tipo === 'persona' && gente === null;
  const personas = enlaces?.personas;
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
      return filtrarComandos(menu.q, rotuloCmd).map((c) => {
        const d = DIBUJO_COMANDO[c.id];
        return {
          clave: c.id,
          dibujo: d.icon ? (
            <Icon name={d.icon} size={15} />
          ) : (
            <Glyph name={d.glyph ?? 'note'} size={15} />
          ),
          rotulo: rotuloCmd(c),
          detalle: `/${c.alias[0]}`,
          elegir: () => ejecutarComando(c),
        };
      });
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
  // `[[` se ve aunque aún no haya nada: dice que busca, o que nada casa. Los otros dos,
  // solo con algo que ofrecer.
  const abierto = menu !== null && (sugerencias.length > 0 || menu.tipo === 'nota');
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
  const [burbuja, setBurbuja] = useState<{ x: number; y: number } | null>(null);
  const burbujaRef = useRef<HTMLDivElement>(null);
  useFitInViewport(burbujaRef, burbuja && !menu ? burbuja : null, { vertical: true });

  // --- Buscar en la nota ----------------------------------------------------
  const [buscador, setBuscador] = useState<{ q: string; i: number } | null>(null);
  const coincidencias = useRef<Range[]>([]);

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
    { cmd: 'strikeThrough', glyph: 'strikethrough', label: t('editor.strike'), conEstado: true },
  ];
  const listas: Accion[] = [
    {
      cmd: 'insertTask',
      glyph: 'todo',
      label: t('note.newTodo'),
      atajo: '[]',
    },
    {
      cmd: 'insertUnorderedList',
      glyph: 'bulletList',
      label: t('editor.bulletList'),
      conEstado: true,
    },
    {
      cmd: 'insertOrderedList',
      glyph: 'numberedList',
      label: t('editor.numberedList'),
      conEstado: true,
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
    setEnTabla(celdaDe(window.getSelection()?.anchorNode ?? null) !== null);
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
    // initialHtml queda fuera a propósito: el padre remonta con `key` cuando
    // cambia de nota, y reaccionar a él aquí es exactamente lo que no debe pasar.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const emitir = () => {
    const html = ref.current?.innerHTML ?? '';
    setVacio(ref.current?.textContent?.trim() === '' && !html.includes('<img'));
    onChange(html);
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
        if (trozo.negrita) executeEditingCommand('bold');
        if (trozo.cursiva) executeEditingCommand('italic');
        executeEditingCommand('insertText', trozo.texto);
        if (trozo.cursiva) executeEditingCommand('italic');
        if (trozo.negrita) executeEditingCommand('bold');
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

  const aplicar = (cmd: string, valor?: string) => {
    enfocarTexto();
    executeEditingCommand(cmd, valor);
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
      bloque.replaceWith(taskRow);
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
    refrescar();
    emitir();
    return true;
  };

  /** Tabla de `filas`×`columnas`. Los bordes van en línea, como en Mail: así se
   *  ven igual en la nota, en el HTML exportado y en el PDF. */
  const insertarTabla = (filas: number, columnas: number) => {
    enfocarTexto();
    const celda = `<td style="${ESTILO_CELDA}">&nbsp;</td>`;
    const fila = `<tr>${celda.repeat(columnas)}</tr>`;
    executeEditingCommand(
      'insertHTML',
      `<table style="border-collapse: collapse; width: 100%">${fila.repeat(filas)}</table><p><br></p>`,
    );
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

  /** Aplica una acción a la celda donde está el cursor. */
  const enCelda = (accion: (c: HTMLTableCellElement) => void) => {
    enfocarTexto();
    const celda = celdaDe(window.getSelection()?.anchorNode ?? null);
    if (!celda) return;
    accion(celda);
    refrescar();
    emitir();
  };

  const abrirEnlaces = () => {
    if (document.activeElement === ref.current) guardarSeleccion();
    setEnlaceAbierto(true);
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
    if (
      (x && x.nodo === d.nodo && x.off === d.off) ||
      (d.tipo === 'cmd' && filtrarComandos(d.q, rotuloCmd).length === 0)
    ) {
      setMenu(null);
      return;
    }
    const r = document.createRange();
    r.setStart(d.nodo, d.off);
    r.setEnd(d.nodo, d.off + 1);
    const caja = r.getBoundingClientRect();
    const ancho = d.tipo === 'cmd' ? 240 : 288;
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

  /** Borra «/consulta» y aplica el comando con la misma orden que su botón. */
  const ejecutarComando = (c: Comando) => {
    if (!tomarDisparo()) return;
    executeEditingCommand('delete');
    const bloque: Partial<Record<IdComando, string>> = {
      p: '<p>',
      h1: '<h1>',
      h2: '<h2>',
      h3: '<h3>',
      cita: '<blockquote>',
      codigo: '<pre>',
    };
    const valor = bloque[c.id];
    if (valor) aplicar('formatBlock', valor);
    else if (c.id === 'tarea') insertarTarea();
    else if (c.id === 'vinetas') aplicar('insertUnorderedList');
    else if (c.id === 'numeros') aplicar('insertOrderedList');
    else if (c.id === 'regla') aplicar('insertHorizontalRule');
    else if (c.id === 'tabla') insertarTabla(3, 3);
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
      setBurbuja({
        x: Math.max(104, Math.min(caja.left + caja.width / 2, window.innerWidth - 104)),
        y: Math.max(caja.top, 44),
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
  const pintarBusqueda = (q: string, i: number) => {
    const todas = ref.current ? coincidenciasEn(ref.current, q) : [];
    coincidencias.current = todas;
    const actual = todas.length ? ((i % todas.length) + todas.length) % todas.length : 0;
    resaltar('sytue-buscar', todas);
    resaltar('sytue-buscar-actual', todas[actual] ? [todas[actual]] : []);
    todas[actual]?.startContainer.parentElement?.scrollIntoView({ block: 'nearest' });
    setBuscador({ q, i: actual });
  };

  const abrirBuscador = () => {
    const sel = window.getSelection();
    const q = sel && !sel.isCollapsed ? sel.toString().slice(0, 80) : (buscador?.q ?? '');
    pintarBusqueda(q, 0);
    window.setTimeout(() => document.getElementById(ID_BUSCAR)?.focus(), 0);
  };

  /** Cierra la búsqueda dejando seleccionada la coincidencia en la que estaba. */
  const cerrarBuscador = () => {
    const actual = buscador ? coincidencias.current[buscador.i] : undefined;
    resaltar('sytue-buscar', []);
    resaltar('sytue-buscar-actual', []);
    setBuscador(null);
    ref.current?.focus();
    if (actual) {
      const sel = window.getSelection();
      sel?.removeAllRanges();
      sel?.addRange(actual);
    }
  };

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

  const Boton = ({ a, onPress }: { a: Accion; onPress: () => void }) => (
    <Tooltip
      label={
        a.atajo ? t('editor.shortcut', { label: a.label, keys: atajoDelSistema(a.atajo) }) : a.label
      }
    >
      <button
        type="button"
        class={cx(
          BOTON_DE_BARRA,
          a.conEstado && activos[a.cmd] && 'bg-bg-subtle text-text',
          a.desactivada && 'opacity-40',
        )}
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

  const etiquetaBloque = t(
    `editor.${BLOQUES.find((b) => b.valor === bloque)?.clave ?? 'paragraph'}` as never,
  );

  return (
    <div class={cx('flex min-h-0 flex-col', estirado && 'flex-1')}>
      <Toolbar
        data-scroll-x
        innerRef={barra.ref}
        class={cx(
          BARRA_DE_EDICION,
          'md:flex-wrap',
          FILA_UNICA_EN_MOVIL,
          barra.queda && MASCARA_SCROLL_X,
        )}
        aria-label={ariaLabel}
      >
        <Boton
          a={{
            cmd: 'undo',
            glyph: 'undo',
            label: t('editor.undo'),
            atajo: 'Ctrl+Z',
            desactivada: !puede.undo,
          }}
          onPress={() => aplicar('undo')}
        />
        <Boton
          a={{
            cmd: 'redo',
            glyph: 'redo',
            label: t('editor.redo'),
            atajo: 'Ctrl+Y',
            desactivada: !puede.redo,
          }}
          onPress={() => aplicar('redo')}
        />
        <span class={SEPARADOR_DE_BARRA} aria-hidden="true" />

        <Dropdown
          label={t('editor.block')}
          trigger={(props) => (
            <button
              {...props}
              type="button"
              class={cx(BOTON_DE_BARRA, 'w-auto gap-1 px-1.5 text-sm')}
              aria-label={t('editor.block')}
              onMouseDown={(e) => e.preventDefault()}
            >
              <Glyph name="heading" size={14} />
              <span class="max-w-24 truncate">{etiquetaBloque}</span>
              <Icon name="chevronDown" size={12} />
            </button>
          )}
        >
          {BLOQUES.map((b) => (
            <MenuItem
              key={b.valor}
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
              {t(`editor.${b.clave}` as never)}
              {bloque === b.valor ? ' ✓' : ''}
            </MenuItem>
          ))}
        </Dropdown>
        <span class={SEPARADOR_DE_BARRA} aria-hidden="true" />

        {formato.map((a) => (
          <Boton key={a.cmd} a={a} onPress={() => aplicar(a.cmd)} />
        ))}
        <Boton
          a={{ cmd: 'mark', icon: 'highlighter', label: t('editor.highlight') }}
          onPress={() => aplicar('hiliteColor', RESALTADO)}
        />
        <Boton
          a={{ cmd: 'code', icon: 'code', label: t('editor.inlineCode') }}
          onPress={() => envolver('code')}
        />
        <Popover
          label={t('editor.textColor')}
          trigger={(props) => (
            <Tooltip label={t('editor.textColor')}>
              <button
                {...props}
                type="button"
                class={BOTON_DE_BARRA}
                aria-label={t('editor.textColor')}
                onMouseDown={(e) => e.preventDefault()}
              >
                <Glyph name="palette" size={16} />
              </button>
            </Tooltip>
          )}
        >
          {(close) => (
            <div class="grid grid-cols-4 gap-1.5 p-2">
              <button
                type="button"
                class="border-border sytue-focus-ring rounded-control col-span-4 min-w-0 border px-2 py-1 text-sm"
                onMouseDown={(e) => e.preventDefault()}
                onClick={() => {
                  aplicar('foreColor', 'inherit');
                  close();
                }}
              >
                {t('editor.colorDefault')}
              </button>
              {COLORES.map((c) => (
                <button
                  key={c}
                  type="button"
                  aria-label={c}
                  style={{ background: c }}
                  class="sytue-focus-ring h-7 w-7 rounded-full"
                  onMouseDown={(e) => e.preventDefault()}
                  onClick={() => {
                    aplicar('foreColor', c);
                    close();
                  }}
                />
              ))}
            </div>
          )}
        </Popover>
        <span class={SEPARADOR_DE_BARRA} aria-hidden="true" />

        {listas.map((a) => (
          <Boton
            key={a.cmd}
            a={a}
            onPress={() => (a.cmd === 'insertTask' ? insertarTarea() : aplicar(a.cmd))}
          />
        ))}
        <span class={SEPARADOR_DE_BARRA} aria-hidden="true" />
        {alineacion.map((a) => (
          <Boton key={a.cmd} a={a} onPress={() => aplicar(a.cmd)} />
        ))}
        <span class={SEPARADOR_DE_BARRA} aria-hidden="true" />

        <Popover
          label={t('editor.linkTool')}
          open={enlaceAbierto}
          onOpenChange={(o) => (o ? abrirEnlaces() : setEnlaceAbierto(false))}
          trigger={(props) => (
            <Tooltip
              label={t('editor.shortcut', {
                label: t('editor.linkTool'),
                keys: atajoDelSistema('Ctrl+K'),
              })}
            >
              <button
                {...props}
                type="button"
                class={BOTON_DE_BARRA}
                aria-label={t('editor.linkTool')}
                onMouseDown={(e) => e.preventDefault()}
              >
                <Glyph name="link" size={16} />
              </button>
            </Tooltip>
          )}
        >
          <LinkTool
            t={t}
            {...(enlaces ? { fuentes: enlaces } : {})}
            onPick={(e) => insertarEnlace(e.url, e.label)}
            onClose={() => {
              setEnlaceAbierto(false);
              enfocarTexto();
            }}
          />
        </Popover>
        <Boton
          a={{ cmd: 'insertHorizontalRule', icon: 'minus', label: t('editor.hr') }}
          onPress={() => aplicar('insertHorizontalRule')}
        />
        <Boton
          a={{ cmd: 'removeFormat', glyph: 'eraser', label: t('editor.clearFormat') }}
          onPress={() => aplicar('removeFormat')}
        />
        {/* Tabla 3×3 de partida: es la que sale en cualquier editor al pulsar el
            botón, y crece con las acciones de abajo. */}
        <Boton
          a={{ cmd: 'insertTable', glyph: 'table', label: t('editor.table') }}
          onPress={() => insertarTabla(3, 3)}
        />
        {/* EL ASISTENTE VA AL FINAL Y APARTE: no es formato, reescribe lo que
            has apuntado. Entre negrita y tabla se pulsa sin querer. */}
        <Dropdown
          label={t('editor.ai')}
          align="end"
          trigger={(props) => (
            <button
              {...props}
              type="button"
              class={BOTON_DE_BARRA}
              title={t('editor.ai')}
              aria-label={t('editor.ai')}
              disabled={iaTrabajando !== null}
            >
              {iaTrabajando !== null ? <Spinner size={16} /> : <Glyph name="sparkles" size={16} />}
            </button>
          )}
        >
          <MenuItem onClick={() => void aplicarIA('ordenar')}>{t('editor.ai.tidy')}</MenuItem>
          <MenuItem onClick={() => void aplicarIA('tareas')}>{t('editor.ai.tasks')}</MenuItem>
          <MenuItem onClick={() => void aplicarIA('titular')}>{t('editor.ai.title')}</MenuItem>
          <MenuSeparator />
          <MenuItem onClick={() => void aplicarIA('resumir')}>{t('editor.ai.summarize')}</MenuItem>
          <MenuItem onClick={() => void aplicarIA('ampliar')}>{t('editor.ai.expand')}</MenuItem>
          <MenuItem onClick={() => void aplicarIA('corregir')}>{t('editor.ai.fix')}</MenuItem>
          <MenuItem onClick={() => void aplicarIA('traducir')}>{t('editor.ai.translate')}</MenuItem>
        </Dropdown>
      </Toolbar>

      {/* El fallo del asistente no se traga: sin aviso, pulsar «ordenar» y no ver
          nada se lee como que la nota está rota. */}
      {iaError ? (
        <p role="status" class="text-danger-text px-1 py-0.5 text-xs">
          {t('editor.aiError')}
        </p>
      ) : null}

      {/* Acciones de tabla: aparecen SOLO con el cursor dentro de una, como en
          Outlook. Enseñarlas siempre alargaría la barra para algo que la mayoría
          de las notas no usa. */}
      {enTabla ? (
        <Toolbar
          data-scroll-x
          innerRef={barraTabla.ref}
          class={cx(
            BARRA_DE_EDICION,
            'md:flex-wrap',
            FILA_UNICA_EN_MOVIL,
            barraTabla.queda && MASCARA_SCROLL_X,
          )}
          aria-label={t('editor.tableTools')}
        >
          <span class="text-text-subtle px-1 text-xs">{t('editor.table')}</span>
          <Boton
            a={{ cmd: 'rowAbove', icon: 'minus', label: t('editor.rowAbove') }}
            onPress={() => enCelda((c) => insertarFila(c, 'antes'))}
          />
          <Boton
            a={{ cmd: 'rowBelow', icon: 'minus', label: t('editor.rowBelow') }}
            onPress={() => enCelda((c) => insertarFila(c, 'despues'))}
          />
          <Boton
            a={{ cmd: 'colBefore', icon: 'minus', label: t('editor.colBefore') }}
            onPress={() => enCelda((c) => insertarColumna(c, 'antes'))}
          />
          <Boton
            a={{ cmd: 'colAfter', icon: 'minus', label: t('editor.colAfter') }}
            onPress={() => enCelda((c) => insertarColumna(c, 'despues'))}
          />
          <span class={SEPARADOR_DE_BARRA} aria-hidden="true" />
          <Boton
            a={{ cmd: 'rowDelete', glyph: 'trash', label: t('editor.rowDelete') }}
            onPress={() => enCelda(eliminarFila)}
          />
          <Boton
            a={{ cmd: 'colDelete', glyph: 'trash', label: t('editor.colDelete') }}
            onPress={() => enCelda(eliminarColumna)}
          />
          <Boton
            a={{ cmd: 'tableDelete', glyph: 'trash', label: t('editor.tableDelete') }}
            onPress={() => enCelda(eliminarTabla)}
          />
        </Toolbar>
      ) : null}

      {/* Buscar en la nota (Ctrl+F dentro del editor). Las coincidencias se
          pintan con la API de resaltados de CSS: no tocan el DOM de la nota, así
          que no hay nada que deshacer ni que se cuele al guardar. */}
      {buscador ? (
        <div class={cx(BARRA_DE_EDICION, 'gap-1.5')} role="search">
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
          <span class="sytue-meta shrink-0 tabular-nums" aria-live="polite">
            {coincidencias.current.length > 0
              ? t('find.count', {
                  current: String(buscador.i + 1),
                  total: String(coincidencias.current.length),
                })
              : buscador.q.trim()
                ? t('find.none')
                : ''}
          </span>
          <Boton
            a={{
              cmd: 'findPrev',
              icon: 'chevronUp',
              label: t('find.previous'),
              atajo: 'Shift+Enter',
            }}
            onPress={() => pintarBusqueda(buscador.q, buscador.i - 1)}
          />
          <Boton
            a={{ cmd: 'findNext', icon: 'chevronDown', label: t('find.next'), atajo: 'Enter' }}
            onPress={() => pintarBusqueda(buscador.q, buscador.i + 1)}
          />
          <Boton
            a={{ cmd: 'findClose', icon: 'close', label: t('find.close') }}
            onPress={cerrarBuscador}
          />
        </div>
      ) : null}

      {/* EL ENVOLTORIO TIENE QUE CRECER TAMBIÉN, y no basta con que crezcan el de
          arriba y el de abajo. Es `relative` porque el aviso de «escribe algo» va
          posicionado sobre el campo; al ser `display:block` y no crecer, el
          `flex-1` del campo no tenía contra qué medirse y se quedaba en su
          mínimo. Medido en el piloto: tarjeta 630px, editor 84px, 446px muertos
          debajo con la clase correcta puesta. */}
      <div class={cx('relative', estirado && 'flex min-h-0 flex-1 flex-col')}>
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
            'sytue-prose text-text sytue-focus-ring min-h-24 rounded-b-md px-3 py-2 text-base leading-relaxed',
            // EN UNA TARJETA ESTIRADA, LA ZONA DE ESCRIBIR LLEGA HASTA ABAJO.
            // `min-h-24` son 96px: en una nota ensanchada a 70dvh eso dejaba
            // más de quinientos píxeles de blanco donde pulsar no hacía nada, y
            // la nota parecía rota. Es el mismo arreglo que ya llevaba el cuerpo
            // en lectura, que aquí faltaba.
            estirado && 'flex-1',
          )}
          onInput={() => {
            guardarSeleccion();
            emitir();
            refrescar();
            mirarMenu();
          }}
          onBlur={() => {
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
            // Ver la cabecera: el portapapeles entra como texto, nunca como HTML.
            e.preventDefault();
            e.stopPropagation();
            const texto = e.clipboardData?.getData('text/plain') ?? '';
            if (texto) executeEditingCommand('insertText', texto);
            emitir();
          }}
          onKeyDown={(e) => {
            const mod = e.metaKey || e.ctrlKey;
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
            if (mod && !e.altKey && e.key.toLowerCase() === 'f') {
              e.preventDefault();
              abrirBuscador();
              return;
            }
            if (mod && e.key === '.') {
              e.preventDefault();
              alternarPlegadoAqui();
              return;
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
            } else if (mod && e.altKey && /^[0-5]$/.test(e.key)) {
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
                class={cx('fixed', menu.tipo === 'cmd' ? 'w-60' : 'w-72')}
                style={{
                  zIndex: 'var(--z-dropdown)',
                  left: `${menu.x}px`,
                  top: `${menu.y}px`,
                  transform: menu.arriba ? 'translateY(-100%)' : undefined,
                }}
              >
                <div class="sytue-pop-in sytue-menu-surface border-border max-h-72 overflow-y-auto rounded-lg border p-1 shadow-[var(--shadow-md)]">
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
                          <span class="min-w-0 flex-1 truncate">
                            {s.rotulo}
                            {s.dentro ? <span class="text-text-muted"> › {s.dentro}</span> : null}
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
                  top: `${burbuja.y - 6}px`,
                  transform: 'translate(-50%, -100%)',
                }}
              >
                <Toolbar
                  aria-label={t('editor.selectionTools')}
                  class="sytue-pop-in sytue-menu-surface border-border flex items-center gap-0.5 rounded-lg border p-1 shadow-[var(--shadow-md)]"
                >
                  {formato
                    .filter((a) => a.cmd !== 'underline')
                    .map((a) => (
                      <Boton key={a.cmd} a={a} onPress={() => aplicar(a.cmd)} />
                    ))}
                  <Boton
                    a={{ cmd: 'mark', icon: 'highlighter', label: t('editor.highlight') }}
                    onPress={() => aplicar('hiliteColor', RESALTADO)}
                  />
                  <Boton
                    a={{ cmd: 'code', icon: 'code', label: t('editor.inlineCode') }}
                    onPress={() => envolver('code')}
                  />
                  <Boton
                    a={{
                      cmd: 'link',
                      glyph: 'link',
                      label: t('editor.linkTool'),
                      atajo: 'Ctrl+K',
                    }}
                    onPress={abrirEnlaces}
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
      </div>
    </div>
  );
}
