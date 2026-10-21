/**
 * RichTextEditor — editor de cuerpo rich-text del compositor de Sytue Mail. Un
 * contenteditable con barra de formato (negrita/cursiva/subrayado/listas/enlace)
 * sobre el adaptador nativo compartido (sin dependencias externas). El pegado se SANEA
 * (lista blanca) antes de insertarse para que un pegado con marcado hostil no
 * ejecute nada en el DOM del compositor; el envío se vuelve a sanear. Sobrio y
 * denso (anti AI-slop), WCAG AA. docs/design.md §5.
 */
import type { JSX } from 'preact';
import { useEffect, useRef, useState } from 'preact/hooks';
import {
  BARRA_DE_EDICION,
  BOTON_DE_BARRA,
  Button,
  CAMPO_DE_BARRA,
  Dropdown,
  Icon,
  Input,
  MenuItem,
  MenuSeparator,
  SEPARADOR_DE_BARRA,
  Spinner,
  Toolbar,
} from '@sytue/ui';
import { editingCommandState, executeEditingCommand } from '@sytue/ui/browser/editing';
import type { Locale } from '@sytue/i18n';
import { useT } from '../i18n/index.js';
import { Glyph, type GlyphName } from './glyphs.js';
import { sanitizeHtml, isHtmlEmpty, incrustarImagenesPegadas } from '../client/richtext.js';
import { construirPeticion, leerRespuesta, type AccionIA } from '../lib/ai.js';
import { bloquesDe, type Bloque } from '@sytue/ai';
import {
  celdaDe,
  eliminarColumna,
  eliminarFila,
  eliminarTabla,
  insertarColumna,
  insertarFila,
} from '@sytue/ui/browser/tabla';

interface Props {
  /** Idioma ya negociado en el servidor; llega por props desde la isla raíz. */
  locale: Locale;
  id?: string;
  /** HTML inicial (se fija una vez al montar; el padre remonta vía `key`). */
  initialHtml: string;
  /** Etiqueta accesible del área de edición. */
  ariaLabel: string;
  placeholder?: string;
  onChange: (html: string) => void;
  /**
   * Incrusta una imagen del disco y devuelve su `cid`, o null si no se puede.
   *
   * Lo hace QUIEN NOS USA y no el editor: los adjuntos del mensaje son suyos, y el
   * editor solo sabe escribir el `<img src="cid:…">` que apunta a la parte. Sin
   * este callback el botón solo ofrece la dirección — que es lo único que se podía
   * hacer hasta ahora.
   */
  onInsertarImagen?: (file: File) => Promise<string | null>;
  /**
   * Si se pinta la barra de formato. El compositor la trae PLEGADA y la abre
   * con «Aa», como Apple Mail o Gmail: la mayoría de los correos no llevan
   * formato y las dos filas de controles eran lo primero que se veía al
   * redactar. Los demás usos (firma, respuestas automáticas) la dejan abierta.
   */
  mostrarBarra?: boolean;
}

/**
 * Un botón de la barra de formato: aplica un comando execCommand a la selección.
 *
 * `activo` es lo que separa una barra de verdad de una decorativa: sin él, el
 * botón de negrita se ve igual dentro y fuera de un texto en negrita, y hay que
 * pulsarlo para averiguar en qué estado se estaba. Va como `aria-pressed` y no
 * solo como color: quien navega con lector de pantalla necesita el mismo dato.
 */
function ToolButton({
  icon,
  label,
  onApply,
  activo,
}: {
  icon: GlyphName;
  label: string;
  onApply: () => void;
  activo?: boolean | undefined;
}): JSX.Element {
  // `BOTON_DE_BARRA` y no el `Button` del paquete: un botón de formulario se
  // dibuja como una caja para verse pulsable en medio de un texto, y catorce de
  // esos seguidos son catorce cajas compitiendo con lo que se está escribiendo.
  // Es la misma decisión que tomó el ribbon de Sytue Docs, ahora compartida.
  //
  // El estado activo lo pinta `aria-pressed` desde la propia clase, así que el
  // aspecto y lo que anuncia el lector de pantalla no pueden separarse: antes
  // eran dos cosas —`variant="secondary"` y el atributo— y nada obligaba a que
  // fueran a la vez.
  return (
    <button
      type="button"
      class={BOTON_DE_BARRA}
      aria-label={label}
      title={label}
      {...(activo === undefined ? {} : { 'aria-pressed': activo })}
      // mousedown+preventDefault mantiene la selección del editor al pulsar.
      onMouseDown={(e: MouseEvent) => e.preventDefault()}
      onClick={onApply}
    >
      <Glyph name={icon} />
    </button>
  );
}

/** Un desplegable de la barra (tipografía, tamaño, estilo). */
function ToolSelect({
  label,
  options,
  onPick,
  width,
}: {
  label: string;
  options: { value: string; label: string }[];
  onPick: (e: Event) => void;
  width: string;
}): JSX.Element {
  return (
    <select
      aria-label={label}
      title={label}
      value=""
      onMouseDown={(e: MouseEvent) => e.stopPropagation()}
      onChange={onPick}
      // `pointer-coarse:min-h-[max(44px,2.75rem)]` — 44px con el dedo, y solo con el dedo, igual que
      // el Button de @sytue/ui. Los 28px de `h-7` se apuntan bien con un ratón y no
      // con la yema: en la barra van pegados a los botones de formato, así que
      // fallar el tamaño de letra pulsaba negrita.
      class={`${CAMPO_DE_BARRA} ${width}`}
    >
      <option value="">{label}</option>
      {options.map((o) => (
        <option key={o.value} value={o.value}>
          {o.label}
        </option>
      ))}
    </select>
  );
}

/**
 * Tipografías ofrecidas: las que existen en Windows, macOS y Linux sin instalar
 * nada. Una fuente bonita que el destinatario no tiene se sustituye por otra
 * cualquiera, y entonces el correo llega peor de lo que se escribió.
 */
const TIPOGRAFIAS = [
  { value: 'Arial, Helvetica, sans-serif', label: 'Arial' },
  { value: 'Georgia, serif', label: 'Georgia' },
  { value: '"Times New Roman", Times, serif', label: 'Times New Roman' },
  { value: 'Verdana, Geneva, sans-serif', label: 'Verdana' },
  { value: '"Courier New", Courier, monospace', label: 'Courier New' },
  { value: 'Tahoma, Geneva, sans-serif', label: 'Tahoma' },
];

/** Tamaños de `fontSize` de execCommand (1-7), con su equivalencia legible. */
const TAMANOS = [
  { value: '1', label: '8' },
  { value: '2', label: '10' },
  { value: '3', label: '12' },
  { value: '4', label: '14' },
  { value: '5', label: '18' },
  { value: '6', label: '24' },
  { value: '7', label: '36' },
];

/** Estilos de párrafo (`formatBlock`). */
const ESTILOS = [
  { value: 'p', label: 'Normal' },
  { value: 'h1', label: 'H1' },
  { value: 'h2', label: 'H2' },
  { value: 'h3', label: 'H3' },
  { value: 'pre', label: '</>' },
];

/**
 * Paleta corta y con contraste comprobado sobre blanco. Un selector de color
 * libre invita a escribir amarillo sobre blanco, que en un correo no lo lee
 * nadie —y quien lo escribe no se entera porque en su editor sí se veía—.
 */
const COLORES = [
  { value: '#2c3137', key: 'editor.colorDefault' },
  { value: '#b91c1c', key: 'editor.colorRed' },
  { value: '#b45309', key: 'editor.colorAmber' },
  { value: '#15803d', key: 'editor.colorGreen' },
  { value: '#1d4ed8', key: 'editor.colorBlue' },
  { value: '#6d28d9', key: 'editor.colorViolet' },
] as const;

/** Fondos de resalte, claros para que el texto encima siga leyéndose. */
const RESALTES = [
  { value: '#fef08a', key: 'editor.colorYellow' },
  { value: '#bbf7d0', key: 'editor.colorGreen' },
  { value: '#bfdbfe', key: 'editor.colorBlue' },
  { value: '#fecaca', key: 'editor.colorRed' },
  { value: 'transparent', key: 'editor.colorNone' },
] as const;

/**
 * Comandos cuyo estado se consulta para encender el botón.
 *
 * Solo los que `queryCommandState` sabe responder de verdad. La tipografía y el
 * tamaño se quedan fuera aposta: ahí el navegador contesta cosas distintas según
 * el motor, y un selector que se marca mal miente más que uno que no se marca.
 */
const COMANDOS_CON_ESTADO = [
  'bold',
  'italic',
  'underline',
  'strikeThrough',
  'insertUnorderedList',
  'insertOrderedList',
  'justifyLeft',
  'justifyCenter',
  'justifyRight',
  'justifyFull',
] as const;

export function RichTextEditor({
  locale,
  id,
  initialHtml,
  ariaLabel,
  placeholder,
  onChange,
  onInsertarImagen,
  mostrarBarra = true,
}: Props): JSX.Element {
  const t = useT(locale);
  const ref = useRef<HTMLDivElement>(null);
  const [empty, setEmpty] = useState(isHtmlEmpty(initialHtml));
  // Qué formato tiene lo que hay bajo el cursor. Lo consulta el navegador, que es
  // el único que sabe de verdad si esa palabra está en negrita: deducirlo del
  // HTML nuestro fallaría con cualquier cosa pegada de fuera.
  const [activos, setActivos] = useState<Record<string, boolean>>({});
  // La celda de tabla donde está el cursor. Cuando la hay, aparece la segunda
  // fila de la barra — como en Outlook, que solo enseña lo de tabla si estás en
  // una. Una barra que siempre lo enseña todo obliga a leerla entera cada vez.
  const [enTabla, setEnTabla] = useState(false);
  // El panel de abajo sirve para dos cosas: 'enlace' pide rótulo + dirección,
  // 'imagen' solo la dirección. Son el mismo trozo de interfaz porque son la
  // misma pregunta, y tener dos paneles casi iguales invita a que se separen.
  const [panel, setPanel] = useState<'enlace' | 'imagen' | null>(null);
  const [enlaceUrl, setEnlaceUrl] = useState('');
  const [enlaceTexto, setEnlaceTexto] = useState('');
  // En el panel de IMAGEN, el campo de dirección arranca oculto tras un botón
  // «URL»: lo normal es insertar una imagen del disco (una decisión, un clic), y
  // tenerlo siempre a la vista invitaba a rellenarlo antes de ver que el fichero
  // ya estaba ahí mismo, al lado. El panel de ENLACE no lo usa: ahí la dirección
  // es lo único que hay que pedir, no una alternativa al fichero.
  const [mostrarUrlImagen, setMostrarUrlImagen] = useState(false);
  // La selección con la que se abrió el panel de enlace: al escribir en el campo
  // el editor pierde el foco y con él la selección, así que hay que guardarla o
  // el enlace se insertaría al principio del cuerpo.
  const seleccion = useRef<Range | null>(null);
  // Marca que el siguiente pegado va sin formato (Ctrl/Cmd+Shift+V). El evento
  // `paste` no trae las teclas, así que se anota en el keydown que lo precede.
  const pegarPlano = useRef(false);

  // Contenido inicial: se fija una vez (el padre fuerza remonte con `key` al
  // (re)abrir), evitando el problema del caret de un contenteditable controlado.
  //
  // Se SANEA también al cargar, no solo al pegar y al enviar. Un borrador se guarda
  // ya saneado por este mismo cliente, pero lo que vuelve es lo que haya en el
  // buzón: un borrador escrito por otro cliente o traído al importar otro correo
  // no ha pasado por aquí, y esto es un `innerHTML` en el origen de la app. Para
  // lo que este compositor guardó es una no-operación (misma lista blanca).
  useEffect(() => {
    if (ref.current) ref.current.innerHTML = sanitizeHtml(initialHtml);
    setEmpty(isHtmlEmpty(initialHtml));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  /**
   * Relee el formato bajo el cursor.
   *
   * Se llama desde `selectionchange` (que es de DOCUMENTO, no del elemento) y
   * por eso lo primero es comprobar que la selección sigue dentro de ESTE
   * editor: si no, mover el cursor por el asunto apagaría los botones sin que
   * nada haya cambiado en el cuerpo.
   */
  function sincronizar(): void {
    const raiz = ref.current;
    const sel = typeof window === 'undefined' ? null : window.getSelection();
    const nodo = sel?.anchorNode ?? null;
    if (!raiz || !nodo || !raiz.contains(nodo)) return;
    const estado: Record<string, boolean> = {};
    for (const c of COMANDOS_CON_ESTADO) estado[c] = editingCommandState(c);
    setActivos(estado);
    setEnTabla(celdaDe(nodo) !== null);
  }

  // El foco entra en la dirección al abrir el panel, no con `autoFocus`: esa
  // marca se lleva el foco también cuando la página CARGA, y por eso el gate de
  // accesibilidad la prohíbe. Aquí el panel solo aparece si alguien lo pide.
  useEffect(() => {
    if (!panel) return;
    const campo = document.getElementById(`${id ?? 'compose'}-enlace-url`);
    if (campo instanceof HTMLInputElement) campo.focus();
  }, [panel, id]);

  useEffect(() => {
    // `selectionchange` dispara en cada pulsación de flecha; el trabajo de dentro
    // son siete consultas baratas al documento, no un render del cuerpo, así que
    // no hace falta amortiguarlo — y amortiguarlo haría que la barra fuera por
    // detrás del cursor, que es justo lo que la vuelve molesta.
    document.addEventListener('selectionchange', sincronizar);
    return () => document.removeEventListener('selectionchange', sincronizar);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  function emit(): void {
    const html = ref.current?.innerHTML ?? '';
    setEmpty(isHtmlEmpty(html));
    onChange(html);
    sincronizar();
  }

  // --- Asistente de IA ------------------------------------------------------
  //
  // Actúa sobre LA SELECCIÓN si la hay, y sobre el correo entero si no. Es lo que
  // espera quien ha marcado un párrafo antes de pedir «acortar»: reescribirle el
  // mensaje completo sería tirar trabajo que no había pedido tocar.
  const [iaTrabajando, setIaTrabajando] = useState<AccionIA | null>(null);
  const [iaError, setIaError] = useState(false);

  /**
   * Escribe los bloques del asistente con el formato del correo.
   *
   * Menos rico que en el editor de documentos a propósito: un correo no lleva
   * títulos de nivel: lo que en Markdown era `## Introducción` aquí se escribe
   * en NEGRITA y en su propia línea, que es como se titula un apartado en un
   * correo y lo que entiende cualquier cliente que lo reciba.
   */
  function escribirBloques(bloques: Bloque[]): void {
    bloques.forEach((bloque, i) => {
      if (i > 0) executeEditingCommand('insertParagraph');
      if (bloque.tipo === 'li') executeEditingCommand('insertUnorderedList');
      const titulo = bloque.tipo !== 'p' && bloque.tipo !== 'li';
      if (titulo) executeEditingCommand('bold');
      for (const trozo of bloque.trozos) {
        const negrita = trozo.negrita && !titulo;
        if (negrita) executeEditingCommand('bold');
        if (trozo.cursiva) executeEditingCommand('italic');
        executeEditingCommand('insertText', trozo.texto);
        if (trozo.cursiva) executeEditingCommand('italic');
        if (negrita) executeEditingCommand('bold');
      }
      if (titulo) executeEditingCommand('bold');
      if (bloque.tipo === 'li') executeEditingCommand('insertUnorderedList');
    });
  }

  async function aplicarIA(accion: AccionIA): Promise<void> {
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
      // MISMO PROBLEMA QUE EN DOCUS: el modelo escribe en Markdown aunque se le
      // pida que no, y un correo con `**Hola**` en crudo es peor que no tener
      // asistente. Se traduce a bloques y se escribe con los comandos del propio
      // editor —negrita, listas—, nunca inyectando HTML de un tercero.
      const bloques = bloquesDe(resultado);
      const conFormato =
        bloques.length > 1 ||
        bloques.some((b) => b.tipo !== 'p' || b.trozos.some((x) => x.negrita || x.cursiva));

      if (conFormato && !haySeleccion) {
        escribirBloques(bloques);
        emit();
        return;
      }

      if (haySeleccion) {
        // `insertText` respeta el deshacer del navegador y sustituye lo marcado;
        // escribir `innerHTML` a mano se lo lleva por delante y deja a la persona
        // sin Ctrl+Z justo después de que un modelo le haya cambiado el texto.
        executeEditingCommand('insertText', resultado);
      } else {
        // Sin selección se reemplaza el cuerpo: se marca todo y se inserta, otra
        // vez para no perder el historial de deshacer.
        const rango = document.createRange();
        rango.selectNodeContents(nodo);
        sel?.removeAllRanges();
        sel?.addRange(rango);
        executeEditingCommand('insertText', resultado);
      }
      emit();
    } catch {
      // Red caída o servicio sin configurar: se dice y no se toca el borrador.
      setIaError(true);
    } finally {
      setIaTrabajando(null);
    }
  }

  function exec(command: string, value?: string): void {
    executeEditingCommand(command, value);
    ref.current?.focus();
    emit();
  }

  /**
   * Aplica un valor de una lista desplegable (tipografía, tamaño, estilo de
   * párrafo) y devuelve el selector a su rótulo.
   *
   * Vuelve a «—» a propósito: `execCommand` no informa de la tipografía bajo el
   * cursor de forma fiable entre navegadores, así que un selector que se quedara
   * marcado estaría MINTIENDO en cuanto el cursor se moviera a otro texto.
   */
  function aplicarDeLista(e: Event, command: string): void {
    const sel = e.currentTarget as HTMLSelectElement;
    const v = sel.value;
    sel.value = '';
    if (v) exec(command, v);
  }

  /**
   * Inserta una tabla de `filas`×`columnas` con bordes en línea.
   *
   * Los estilos van EN LÍNEA y no en una clase: en un correo no hay hoja de
   * estilos que valga —ni Gmail ni Outlook la aplican—, así que una tabla con
   * clases llega al destinatario sin una sola línea.
   */
  function insertarTabla(filas: number, columnas: number): void {
    const celda = '<td style="border: 1px solid #d4d0c8; padding: 6px 8px">&nbsp;</td>';
    const fila = `<tr>${celda.repeat(columnas)}</tr>`;
    exec(
      'insertHTML',
      `<table style="border-collapse: collapse; width: 100%">${fila.repeat(filas)}</table><p><br></p>`,
    );
  }

  /**
   * Inserta una imagen por dirección. Solo https, que es lo único que el saneador
   * de salida deja pasar: aceptar aquí un `http:` insertaría una imagen que el
   * destinatario no ve y que además delata su lectura en claro.
   */
  /**
   * Imagen del disco: se adjunta como parte del mensaje y en el cuerpo queda un
   * `cid:` que apunta a ella. Es como se ve una imagen DENTRO de un correo; una
   * `data:` en línea la bloquean casi todos los clientes.
   */
  async function insertarImagenDeFichero(file: File): Promise<void> {
    if (!onInsertarImagen) return;
    const cid = await onInsertarImagen(file);
    // null significa que quien nos usa ya lo ha explicado (demasiado grande, no es
    // una imagen): meter aquí otro aviso sería contarlo dos veces.
    if (!cid) return;
    recuperarSeleccion();
    exec('insertHTML', `<img src="cid:${cid}" alt="" style="max-width: 100%">`);
    // La siguiente de las elegidas va DETRÁS de esta, no delante.
    guardarSeleccion();
  }

  /** Recuerda dónde está el cursor para volver después de algo que tarda. */
  function guardarSeleccion(): void {
    const sel = window.getSelection();
    seleccion.current = sel && sel.rangeCount > 0 ? sel.getRangeAt(0).cloneRange() : null;
  }

  function insertarImagen(url: string): void {
    if (!/^https:\/\//i.test(url)) return;
    exec('insertHTML', `<img src="${url.replace(/"/g, '&quot;')}" alt="" style="max-width: 100%">`);
  }

  /** Aplica una operación de tabla sobre la celda del cursor. */
  function enCelda(accion: (celda: HTMLTableCellElement) => void): void {
    const nodo = window.getSelection()?.anchorNode ?? null;
    const celda = celdaDe(nodo);
    if (!celda) return;
    accion(celda);
    ref.current?.focus();
    emit();
  }

  /**
   * Abre el panel de enlace recordando la selección.
   *
   * Si hay texto seleccionado, ese es el rótulo y solo se pide la dirección; si
   * no lo hay, se piden las dos cosas. Es la diferencia entre poder enlazar una
   * palabra del texto y tener que pegar la URL cruda, que es lo que pasaba antes
   * con el `prompt` del navegador.
   */
  function abrirPanel(cual: 'enlace' | 'imagen'): void {
    const sel = window.getSelection();
    const rango = sel && sel.rangeCount > 0 ? sel.getRangeAt(0) : null;
    seleccion.current = rango ? rango.cloneRange() : null;
    setEnlaceTexto(cual === 'enlace' && rango ? rango.toString() : '');
    setEnlaceUrl('');
    setMostrarUrlImagen(false);
    setPanel(cual);
  }

  /** Devuelve el cursor a donde estaba antes de escribir en el panel. */
  function recuperarSeleccion(): Range | null {
    const rango = seleccion.current;
    if (rango) {
      const sel = window.getSelection();
      sel?.removeAllRanges();
      sel?.addRange(rango);
    }
    ref.current?.focus();
    return rango;
  }

  function aplicarPanel(): void {
    const url = enlaceUrl.trim();
    const cual = panel;
    setPanel(null);
    if (cual === 'imagen') {
      recuperarSeleccion();
      insertarImagen(url);
      return;
    }
    // Solo esquemas seguros: un `javascript:` en un href es ejecución en el DOM
    // de quien abra el correo, y el saneador de salida lo tiraría igualmente —
    // pero entonces el enlace se perdería sin decir por qué.
    if (!/^(https?:\/\/|mailto:)/i.test(url)) return;
    const rango = recuperarSeleccion();
    const rotulo = enlaceTexto.trim();
    if (rango && !rango.collapsed) {
      exec('createLink', url);
      return;
    }
    const escapado = (v: string) =>
      v.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/"/g, '&quot;');
    exec('insertHTML', `<a href="${escapado(url)}">${escapado(rotulo || url)}</a>`);
  }

  function handleKeyDown(e: KeyboardEvent): void {
    const mod = e.ctrlKey || e.metaKey;
    if (!mod) return;
    // Ctrl/Cmd+K: enlazar, como en Gmail, Outlook y cualquier editor.
    if (e.key.toLowerCase() === 'k') {
      e.preventDefault();
      abrirPanel('enlace');
      return;
    }
    // Ctrl/Cmd+Shift+V: pegar sin formato. El evento `paste` no dice qué teclas
    // había pulsadas, así que se anota aquí y se lee en el pegado siguiente.
    if (e.shiftKey && e.key.toLowerCase() === 'v') pegarPlano.current = true;
  }

  function handlePaste(e: ClipboardEvent): void {
    e.preventDefault();
    const html = e.clipboardData?.getData('text/html');
    const text = e.clipboardData?.getData('text/plain') ?? '';
    const plano = pegarPlano.current;
    pegarPlano.current = false;
    if (html && !plano && onInsertarImagen && /<img[^>]+src\s*=\s*["']?\s*data:/i.test(html)) {
      // Una imagen `data:` entra como parte del mensaje, igual que la del equipo; hay que
      // esperar a adjuntarla, así que se recuerda dónde estaba el cursor.
      guardarSeleccion();
      void incrustarImagenesPegadas(html, onInsertarImagen).then((h) => {
        recuperarSeleccion();
        executeEditingCommand('insertHTML', sanitizeHtml(h));
        emit();
      });
      return;
    }
    if (html && !plano) {
      executeEditingCommand('insertHTML', sanitizeHtml(html));
    } else {
      executeEditingCommand('insertText', text);
    }
    emit();
  }

  return (
    // SIN CAJA ALREDEDOR. El editor vive dentro de un diálogo que ya tiene su
    // marco, debajo de una cabecera de filas separadas por pelos: encerrarlo
    // además en un rectángulo con borde y esquinas redondeadas metía un tercer
    // marco concéntrico y hacía que escribir el mensaje pareciera rellenar el
    // último campo de un formulario. Es la misma forma que tiene Sytue Docs, donde
    // el ribbon se apoya sobre el lienzo con un pelo y nada más.
    //
    // El foco NO se pierde: cada control de la barra pinta su propio anillo, y en
    // el cuerpo editable lo dice el cursor, como en cualquier área de texto.
    <div class="flex min-h-0 flex-col">
      {/* En el móvil la barra NO se pliega: se desplaza en horizontal, como en
          Outlook y Docs. Con `flex-wrap` sus veinticinco controles ocupaban
          cuatro o cinco filas —la mitad del diálogo— y el cuerpo del correo se
          quedaba en un par de líneas. Desde 768px vuelve a plegarse, que ahí
          entra casi entera.

          `data-scroll-x` marca ese desplazamiento como DELIBERADO para el
          barrido de desbordamiento (test/e2e/detector-desbordamiento.mjs), que
          si no lo cuenta como defecto: una barra que desborda a propósito y otra
          que desborda por error se ven igual desde fuera, y el hallazgo falso
          tapa a los de verdad. Mismo marcado que la cinta de Docs. */}
      {/* `!bg-transparent` — EL FALLO DE TONO DEL TEMA OSCURO. `BARRA_DE_EDICION`
          trae `bg-surface` porque en Sytue Docs la cinta se apoya sobre el lienzo,
          que es esa superficie. Aquí la barra vive dentro de un `Modal`, y el
          panel del modal es `bg-surface-raised`: en claro las dos valen
          neutral-0 y no se nota nada, pero en oscuro son neutral-900 sobre
          neutral-850, o sea una BANDA GRIS MÁS OSCURA cruzando el diálogo justo
          por encima de donde se escribe. Sin fondo propio, la barra se apoya en
          la superficie que tenga debajo, que es lo que la propia doctrina del
          cromo dice que debe hacer: pertenece al documento, no al cromo. */}
      {mostrarBarra ? (
        <Toolbar
          data-scroll-x
          aria-label={t('editor.toolbar')}
          aria-controls={id}
          class={`${BARRA_DE_EDICION} overflow-x-auto !bg-transparent !px-0 md:flex-wrap md:overflow-x-visible [&>*]:shrink-0`}
        >
          {/* Deshacer/rehacer los primeros: es lo que se busca con prisa. */}
          <ToolButton icon="undo" label={t('editor.undo')} onApply={() => exec('undo')} />
          <ToolButton icon="redo" label={t('editor.redo')} onApply={() => exec('redo')} />
          <span class={SEPARADOR_DE_BARRA} aria-hidden="true" />

          <ToolSelect
            label={t('editor.font')}
            options={TIPOGRAFIAS}
            width="w-24"
            onPick={(e) => aplicarDeLista(e, 'fontName')}
          />
          <ToolSelect
            label={t('editor.size')}
            options={TAMANOS}
            // 80px: con 64 el rótulo salía «Tama…» en castellano.
            width="w-20"
            onPick={(e) => aplicarDeLista(e, 'fontSize')}
          />
          <ToolSelect
            label={t('editor.style')}
            options={ESTILOS}
            width="w-20"
            onPick={(e) => aplicarDeLista(e, 'formatBlock')}
          />
          <span class={SEPARADOR_DE_BARRA} aria-hidden="true" />

          <ToolButton
            icon="bold"
            label={t('editor.bold')}
            onApply={() => exec('bold')}
            activo={activos.bold}
          />
          <ToolButton
            icon="italic"
            label={t('editor.italic')}
            onApply={() => exec('italic')}
            activo={activos.italic}
          />
          <ToolButton
            icon="underline"
            label={t('editor.underline')}
            onApply={() => exec('underline')}
            activo={activos.underline}
          />
          <ToolButton
            icon="strikethrough"
            label={t('editor.strikethrough')}
            onApply={() => exec('strikeThrough')}
            activo={activos.strikeThrough}
          />
          <ToolSelect
            label={t('editor.textColor')}
            options={COLORES.map((c) => ({ value: c.value, label: t(c.key) }))}
            width="w-20"
            onPick={(e) => aplicarDeLista(e, 'foreColor')}
          />
          <ToolSelect
            label={t('editor.highlight')}
            options={RESALTES.map((c) => ({ value: c.value, label: t(c.key) }))}
            width="w-20"
            onPick={(e) => aplicarDeLista(e, 'hiliteColor')}
          />
          <span class={SEPARADOR_DE_BARRA} aria-hidden="true" />

          <ToolButton
            icon="alignLeft"
            label={t('editor.alignLeft')}
            onApply={() => exec('justifyLeft')}
            activo={activos.justifyLeft}
          />
          <ToolButton
            icon="alignCenter"
            label={t('editor.alignCenter')}
            onApply={() => exec('justifyCenter')}
            activo={activos.justifyCenter}
          />
          <ToolButton
            icon="alignRight"
            label={t('editor.alignRight')}
            onApply={() => exec('justifyRight')}
            activo={activos.justifyRight}
          />
          <ToolButton
            icon="alignJustify"
            label={t('editor.alignJustify')}
            onApply={() => exec('justifyFull')}
            activo={activos.justifyFull}
          />
          <span class={SEPARADOR_DE_BARRA} aria-hidden="true" />

          <ToolButton
            icon="bulletList"
            label={t('editor.bulletList')}
            onApply={() => exec('insertUnorderedList')}
            activo={activos.insertUnorderedList}
          />
          <ToolButton
            icon="numberList"
            label={t('editor.numberList')}
            onApply={() => exec('insertOrderedList')}
            activo={activos.insertOrderedList}
          />
          <ToolButton icon="outdent" label={t('editor.outdent')} onApply={() => exec('outdent')} />
          <ToolButton icon="indent" label={t('editor.indent')} onApply={() => exec('indent')} />
          <ToolButton
            icon="quote"
            label={t('editor.quote')}
            onApply={() => exec('formatBlock', 'blockquote')}
          />
          <span class={SEPARADOR_DE_BARRA} aria-hidden="true" />

          {/* Objetos: tabla 3×3 de partida —el tamaño que se pide el 90 % de las
              veces— y de ahí se crece con Tab, como en Outlook. */}
          <ToolButton icon="table" label={t('editor.table')} onApply={() => insertarTabla(3, 3)} />
          <ToolButton icon="image" label={t('editor.image')} onApply={() => abrirPanel('imagen')} />
          <ToolButton
            icon="rule"
            label={t('editor.rule')}
            onApply={() => exec('insertHorizontalRule')}
          />
          <ToolButton icon="link" label={t('editor.link')} onApply={() => abrirPanel('enlace')} />
          <ToolButton
            icon="clearFormat"
            label={t('editor.clearFormat')}
            onApply={() => {
              exec('removeFormat');
              exec('unlink');
            }}
          />
          <span class={SEPARADOR_DE_BARRA} aria-hidden="true" />
          {/* EL ASISTENTE VA AL FINAL Y TRAS SEPARADOR: no es formato, es
              reescribir lo que has puesto. Mezclarlo entre negrita y cursiva
              invita a pulsarlo sin querer, y esto cambia el texto entero. */}
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
                {iaTrabajando !== null ? (
                  <Spinner size={16} label={t('editor.aiWorking')} />
                ) : (
                  <Icon name="ai" size={16} />
                )}
              </button>
            )}
          >
            <MenuItem onClick={() => void aplicarIA('mejorar')}>{t('editor.ai.improve')}</MenuItem>
            <MenuItem onClick={() => void aplicarIA('corregir')}>{t('editor.ai.fix')}</MenuItem>
            <MenuItem onClick={() => void aplicarIA('acortar')}>{t('editor.ai.shorten')}</MenuItem>
            <MenuSeparator />
            <MenuItem onClick={() => void aplicarIA('formal')}>{t('editor.ai.formal')}</MenuItem>
            <MenuItem onClick={() => void aplicarIA('cercano')}>{t('editor.ai.casual')}</MenuItem>
            <MenuSeparator />
            <MenuItem onClick={() => void aplicarIA('responder')}>{t('editor.ai.reply')}</MenuItem>
            <MenuItem onClick={() => void aplicarIA('traducir')}>
              {t('editor.ai.translate')}
            </MenuItem>
          </Dropdown>
        </Toolbar>
      ) : null}

      {/* El fallo del asistente NO se traga: sin esto, pulsar «mejorar» y no ver
          nada se lee como que la aplicación está rota. `role="status"` para que
          lo anuncie también quien no lo ve. */}
      {iaError ? (
        <p role="status" class="text-danger-text px-1 py-0.5 text-xs">
          {t('editor.aiError')}
        </p>
      ) : null}

      {/* Barra de tabla: SOLO con el cursor dentro de una. Antes, una tabla
          insertada era de usar y tirar —quedarse corto de filas obligaba a
          borrarla y empezar otra vez— porque execCommand sabe crearla y nada
          más. Aparece y desaparece como en Outlook: enseñarla siempre obligaría
          a leer una barra el doble de larga en cada correo. */}
      {enTabla ? (
        <Toolbar
          data-scroll-x
          aria-label={t('editor.tableTools')}
          // Sin fondo propio, por lo mismo que la barra de arriba: `bg-bg-subtle`
          // pintaba una segunda banda gris —en oscuro, más oscura que el diálogo—
          // para decir algo que ya dice el rótulo «Tabla» que abre la fila y el
          // hecho de que la barra solo exista cuando el cursor está en una.
          class="border-border flex items-center gap-0.5 overflow-x-auto border-b px-1.5 py-1 md:flex-wrap md:overflow-x-visible [&>*]:shrink-0"
        >
          <span class="text-text-subtle px-1 text-xs">{t('editor.table')}</span>
          <ToolButton
            icon="rowAbove"
            label={t('editor.rowAbove')}
            onApply={() => enCelda((c) => insertarFila(c, 'antes'))}
          />
          <ToolButton
            icon="rowBelow"
            label={t('editor.rowBelow')}
            onApply={() => enCelda((c) => insertarFila(c, 'despues'))}
          />
          <ToolButton
            icon="colBefore"
            label={t('editor.colBefore')}
            onApply={() => enCelda((c) => insertarColumna(c, 'antes'))}
          />
          <ToolButton
            icon="colAfter"
            label={t('editor.colAfter')}
            onApply={() => enCelda((c) => insertarColumna(c, 'despues'))}
          />
          <span class={SEPARADOR_DE_BARRA} aria-hidden="true" />
          <ToolButton
            icon="rowDelete"
            label={t('editor.rowDelete')}
            onApply={() => enCelda(eliminarFila)}
          />
          <ToolButton
            icon="colDelete"
            label={t('editor.colDelete')}
            onApply={() => enCelda(eliminarColumna)}
          />
          <ToolButton
            icon="tableDelete"
            label={t('editor.tableDelete')}
            onApply={() => enCelda(eliminarTabla)}
          />
        </Toolbar>
      ) : null}

      {/* Panel de enlace. Sustituye al `window.prompt`, que no dejaba poner el
          rótulo, no se podía cancelar con Escape sin perder la selección y no se
          parece a nada del resto de la suite.

          Los anchos fijos de dentro van acotados en el móvil. A 320px el hueco real
          de este panel es de 214px —288 de diálogo, menos los 48 del `px-6` del
          cuerpo, menos el borde y el `px-3` de aquí—, así que el `w-72` (288px) del
          campo de dirección se salía 74px; y como el marco del editor es
          `overflow-hidden`, eso no daba scroll: RECORTABA. Se escribía una URL larga
          sin ver el final, que es justo cuando hace falta mirarla. Con
          `max-md:w-full` cada campo ocupa su propia fila y cabe entero. */}
      {panel ? (
        <div class="border-border flex flex-wrap items-end gap-2 border-b px-3 py-2">
          {panel === 'enlace' ? (
            <>
              <label class="text-text-muted flex min-w-0 flex-col gap-1 text-xs max-md:w-full">
                {t('editor.linkText')}
                <Input
                  value={enlaceTexto}
                  class="h-8 w-48 max-md:w-full"
                  onInput={(e: Event) =>
                    setEnlaceTexto((e.currentTarget as HTMLInputElement).value)
                  }
                />
              </label>
              <label class="text-text-muted flex min-w-0 flex-col gap-1 text-xs max-md:w-full">
                {t('editor.linkUrl')}
                <Input
                  type="url"
                  inputMode="url"
                  placeholder="https://"
                  value={enlaceUrl}
                  class="h-8 w-72 max-md:w-full"
                  id={`${id ?? 'compose'}-enlace-url`}
                  onInput={(e: Event) => setEnlaceUrl((e.currentTarget as HTMLInputElement).value)}
                  onKeyDown={(e: KeyboardEvent) => {
                    if (e.key === 'Enter') {
                      e.preventDefault();
                      aplicarPanel();
                    }
                    if (e.key === 'Escape') setPanel(null);
                  }}
                />
              </label>
              <Button type="button" size="sm" onClick={aplicarPanel}>
                {t('editor.linkApply')}
              </Button>
            </>
          ) : (
            // Imagen: PRIMERO el fichero —es la elección normal, una sola
            // acción— y la dirección queda detrás del botón «URL»: tenerla
            // siempre a la vista invitaba a escribir ahí antes de ver que el
            // fichero estaba al lado, más corto de rellenar.
            <>
              {onInsertarImagen ? (
                <label class="text-text-muted flex min-w-0 flex-col gap-1 text-xs max-md:w-full">
                  {t('editor.imageFile')}
                  <input
                    type="file"
                    accept="image/*"
                    multiple
                    aria-label={t('editor.imageFile')}
                    class="text-text-muted file:bg-bg-subtle file:text-text-muted h-8 max-w-full text-xs file:mr-2 file:rounded file:border-0 file:px-2 file:py-1"
                    onChange={(e: Event) => {
                      // Varias a la vez, en el orden en que se eligen.
                      const fs = Array.from((e.currentTarget as HTMLInputElement).files ?? []);
                      setPanel(null);
                      void (async () => {
                        for (const f of fs) await insertarImagenDeFichero(f);
                      })();
                    }}
                  />
                </label>
              ) : null}
              {!mostrarUrlImagen ? (
                <Button
                  type="button"
                  size="sm"
                  variant="secondary"
                  onClick={() => setMostrarUrlImagen(true)}
                >
                  {t('editor.imageByUrl')}
                </Button>
              ) : (
                <>
                  <label class="text-text-muted flex min-w-0 flex-col gap-1 text-xs max-md:w-full">
                    {t('editor.imageUrl')}
                    <Input
                      type="url"
                      inputMode="url"
                      placeholder="https://"
                      value={enlaceUrl}
                      class="h-8 w-72 max-md:w-full"
                      id={`${id ?? 'compose'}-enlace-url`}
                      onInput={(e: Event) =>
                        setEnlaceUrl((e.currentTarget as HTMLInputElement).value)
                      }
                      onKeyDown={(e: KeyboardEvent) => {
                        if (e.key === 'Enter') {
                          e.preventDefault();
                          aplicarPanel();
                        }
                        if (e.key === 'Escape') setPanel(null);
                      }}
                    />
                  </label>
                  <Button type="button" size="sm" onClick={aplicarPanel}>
                    {t('editor.imageApply')}
                  </Button>
                </>
              )}
            </>
          )}
          <Button type="button" size="sm" variant="ghost" onClick={() => setPanel(null)}>
            {t('editor.linkCancel')}
          </Button>
        </div>
      ) : null}

      <div class="relative">
        <div
          id={id}
          ref={ref}
          role="textbox"
          aria-multiline="true"
          aria-label={ariaLabel}
          contentEditable
          tabIndex={0}
          // Sin anillo: el cuerpo del mensaje es un lienzo, no un campo, y un cerco
          // de latón alrededor de media ventana es exactamente la caja que se le
          // quitó. Lo que dice dónde está el teclado aquí es el CURSOR, que en un
          // área de texto es el indicador de foco de siempre. `shadow-none` sigue
          // haciendo falta: sin él, el `contenteditable` recogía el anillo global
          // del tema (`outline-none` quita el contorno, no el box-shadow).
          // EL ALTO SE ATA AL VIEWPORT VISUAL, no solo a `28rem`.
          //
          // Con el teclado abierto en un teléfono quedan unos 350px de alto útil, y
          // el cromo del diálogo —cabecera, Para, Asunto, barra de formato y pie— se
          // come casi 290. Un editor de 28rem (448px) no cabe ahí ni de lejos: crecía
          // por debajo del teclado y la línea que se estaba escribiendo quedaba
          // tapada, que es justo lo que se informó desde un iPhone. `--sytue-vvh` la
          // publica la shell (viewportVisual.ts) y ya vale para toda la suite; aquí
          // se usa para que el editor NUNCA pida más de lo que hay, con `overflow-y`
          // encargándose de mantener el cursor a la vista.
          //
          // El `min-h` baja a 6rem en pantallas cortas por la misma razón: con 12rem
          // fijos el mínimo ganaba al máximo y el recorte no servía de nada. En
          // escritorio no cambia nada: 45% de 900px son 405, y el `min()` deja los
          // 28rem de siempre.
          class="sytue-campo-tactil text-text [&_a]:text-primary-text max-h-[min(28rem,calc(var(--sytue-vvh,100dvh)*0.45))] min-h-[12rem] overflow-y-auto px-1 py-3 text-base leading-relaxed [overflow-wrap:anywhere] focus:outline-none focus-visible:shadow-none max-[820px]:min-h-[6rem] [&_a]:underline [&_ol]:list-decimal [&_ol]:pl-6 [&_ul]:list-disc [&_ul]:pl-6"
          onInput={emit}
          onBlur={emit}
          onKeyDown={handleKeyDown}
          onKeyUp={sincronizar}
          onMouseUp={sincronizar}
          onPaste={handlePaste}
        />
        {empty && placeholder ? (
          /* MISMO SITIO QUE EL CURSOR, exactamente. Estaba en `top-2.5 left-3` y
             el área editable tiene `px-1 py-3`: la pista aparecía 8px a la derecha
             y 2px por encima de donde iba a salir la primera letra, así que al
             escribir el texto DABA UN SALTO. Ahora comparte relleno
             (`top-3 left-1`) y también el interlineado, que si no la desplaza
             medio píxel por su cuenta. */
          <span
            class="sytue-campo-tactil text-text-subtle pointer-events-none absolute top-3 left-1 text-base leading-relaxed"
            aria-hidden="true"
          >
            {placeholder}
          </span>
        ) : null}
      </div>
    </div>
  );
}
