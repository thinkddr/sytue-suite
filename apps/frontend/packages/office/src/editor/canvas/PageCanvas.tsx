/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * PageCanvas — lienzo del editor para kind `document` (BLUEPRINT.md §6.5):
 * folio A4 blanco centrado con sombra sobre el gris del lienzo, contenteditable
 * cuyo HTML ES el modelo (solo se re-aplica al DOM cuando cambia contentEpoch,
 * nunca por tecla), zoom por transform scale con origen superior-centro y
 * paginación visual mediante líneas de corte cada alto de página. La tipografía
 * de contenido del folio (.sytue-page: Inter, h1–h3, listas, tablas con bordes)
 * vive en editor.css (§8), igual que --editor-canvas-bg y --editor-ruler-margin.
 */
import type { JSX } from 'preact';
import { useEffect, useRef, useState } from 'preact/hooks';
import { saneaHtmlDeDocumento, useFitInViewport, useMobileBack } from '@sytue/ui';
import type { CursorPos, EditorCommand, EditorContext } from '../types';
import { comandoDelAtajo } from '../comandos';
import { parseOptions } from '../formfields';
import { anclarPos, cursorPosKey, peerName } from '../state/cursors';
import { readTrackFlag, withTrackFlag } from '../trackchanges';
import { pageFieldText, setEditorRoot, trackDelete, trackInsert } from './exec';
import { hydrateEmbeds, stripEmbedLive } from './embedDom';
import { quitarTabulaciones, tabular } from './tabulaciones';
import { MIN_ZOOM } from './metrics';
import { leerPagina, margenesPagina, medidasPagina } from '../pagedecor';
import { Ruler } from './Ruler';
import { ImageOverlay } from './ImageOverlay';
import { DibujoHost } from '../dibujo/DibujoHost';
import './superficies.css';

/**
 * Soporte de encabezado/pie (§ tarea 1): estilo mínimo inyectado una vez.
 * El posicionado de las regiones va INLINE en su HTML (que ES el modelo y viaja
 * al export); aquí solo lo que no puede ir inline: la variante «primera página
 * distinta» (oculta el encabezado del primer folio) y una pista visible cuando
 * la región queda vacía. Mismo patrón de <style> inyectado que Disposición/Vista.
 * ponytail: folio continuo ⇒ una banda de encabezado (folio 1) y una de pie
 * (último folio); repetición por folio real cuando el lienzo pagine (fase 2).
 */
const HF_STYLE_ID = 'sytue-editor-doc-hf';
function ensureHeaderFooterStyle(header: string, footer: string): void {
  if (typeof document === 'undefined') return;
  let el = document.getElementById(HF_STYLE_ID);
  if (!el) {
    el = document.createElement('style');
    el.id = HF_STYLE_ID;
    document.head.appendChild(el);
  }
  // Las pistas son TEXTO VISIBLE y van en el idioma de la interfaz; JSON.stringify
  // las entrecomilla y escapa para `content` (una comilla suelta en la traducción
  // rompería la regla entera y con ella el resto del bloque).
  el.textContent =
    '.sytue-editor .sytue-page [data-doc-header],.sytue-editor .sytue-page [data-doc-footer]{color:#475569;}' +
    '.sytue-editor .sytue-page [data-doc-header][data-first-page-different]{display:none;}' +
    // Región vacía: pista tenue para poder localizarla y hacer clic.
    `.sytue-editor .sytue-page [data-doc-header]:empty::before{content:${JSON.stringify(header)};opacity:.4;}` +
    `.sytue-editor .sytue-page [data-doc-footer]:empty::before{content:${JSON.stringify(footer)};opacity:.4;}`;
}

/** Aire vertical del folio sobre el lienzo gris (§6.5: margin 32px auto). */
const PAGE_GAP = 32;
/** Alto de reserva del caret cuando el rect colapsado no da altura (línea vacía). */
const CARET_FALLBACK_H = 18;

/* ---------- Cursores remotos (Docs): mapeo offset ↔ DOM ----------
 * El caret/selección de un peer se serializa como offsets de texto dentro del
 * folio (CursorPos kind 'doc'); aquí se convierten a/desde el DOM para pintar un
 * caret fino, el resaltado de selección y la etiqueta. Todo EFÍMERO: si un offset
 * no resuelve (contenido divergente entre peers entre adopciones), no se pinta.
 * Mismo recorrido de nodos de texto que setCaretOffsetIn de sheets. */

/** Offset de texto de (node, offset) dentro de root (cuenta el texto visible). */
const textOffsetIn = (root: HTMLElement, node: Node, offset: number): number => {
  const range = document.createRange();
  range.selectNodeContents(root);
  range.setEnd(node, offset);
  return range.toString().length;
};

/** Ancla/foco de la selección actual como offsets en root; null si cae fuera. */
const docSelectionOffsets = (root: HTMLElement): { anchor: number; focus: number } | null => {
  const sel = window.getSelection();
  if (!sel || sel.rangeCount === 0) return null;
  const { anchorNode, anchorOffset, focusNode, focusOffset } = sel;
  if (!anchorNode || !focusNode) return null;
  if (!root.contains(anchorNode) || !root.contains(focusNode)) return null;
  return {
    anchor: textOffsetIn(root, anchorNode, anchorOffset),
    focus: textOffsetIn(root, focusNode, focusOffset),
  };
};

/** Localiza (node, offset) de un offset de texto recorriendo los nodos de texto. */
const pointAtOffset = (
  root: HTMLElement,
  offset: number,
): { node: Node; offset: number } | null => {
  let remaining = Math.max(0, offset);
  const walk = (node: Node): { node: Node; offset: number } | null => {
    if (node.nodeType === Node.TEXT_NODE) {
      const len = node.textContent?.length ?? 0;
      if (remaining <= len) return { node, offset: remaining };
      remaining -= len;
      return null;
    }
    for (const child of Array.from(node.childNodes)) {
      const hit = walk(child);
      if (hit) return hit;
    }
    return null;
  };
  return walk(root);
};

/** Rect del caret (colapsado) en un offset; reserva al rect del contenedor. */
const caretRectAtOffset = (root: HTMLElement, offset: number): DOMRect | null => {
  const pt = pointAtOffset(root, offset);
  if (!pt) return null;
  const range = document.createRange();
  range.setStart(pt.node, pt.offset);
  range.collapse(true);
  const rects = range.getClientRects();
  if (rects.length > 0) return rects[0] ?? null;
  const el = pt.node.nodeType === Node.TEXT_NODE ? pt.node.parentElement : (pt.node as HTMLElement);
  return el ? el.getBoundingClientRect() : null;
};

/** Rects (por línea) de la selección [a,b] de un peer; vacío si no resuelve. */
const selectionRectsAt = (root: HTMLElement, a: number, b: number): DOMRect[] => {
  const p1 = pointAtOffset(root, Math.min(a, b));
  const p2 = pointAtOffset(root, Math.max(a, b));
  if (!p1 || !p2) return [];
  const range = document.createRange();
  range.setStart(p1.node, p1.offset);
  range.setEnd(p2.node, p2.offset);
  return Array.from(range.getClientRects());
};

/** Posición de un peer reubicada por las ediciones ajenas desde que llegó (ver el efecto). */
interface Anclado {
  /** La posición tal como llegó: si el peer manda otra, se parte de ella. */
  origen: CursorPos;
  pos: Extract<CursorPos, { kind: 'doc' }>;
  /** El texto del folio sobre el que `pos` era cierta. */
  texto: string;
}

/** Caret de un peer ya en coordenadas del folio (px de maqueta, sin escalar). */
interface PeerCaretView {
  id: string;
  name: string;
  color: string;
  /** Clave de posición: reinicia el desvanecido de la etiqueta al moverse. */
  posKey: string;
  caret: { left: number; top: number; height: number };
  sel: { left: number; top: number; width: number; height: number }[];
}

export function PageCanvas({ ctx }: { ctx: EditorContext }): JSX.Element {
  const t = ctx.t;
  const scrollRef = useRef<HTMLDivElement>(null);
  const pageRef = useRef<HTMLDivElement>(null);
  /** Wrapper escalado (transform: scale(zoom)): sistema de coordenadas de los
   *  cursores remotos (sus offsets → rects se pasan a este espacio, sin escalar). */
  const wrapRef = useRef<HTMLDivElement>(null);
  const throttleRef = useRef<number | null>(null);
  const measureRef = useRef<() => void>(() => undefined);
  const lastStatsRef = useRef({ pages: 0, page: 0 });
  /** Cursores remotos ya resueltos a coordenadas del folio (para el render). */
  const [peerCarets, setPeerCarets] = useState<PeerCaretView[]>([]);
  const ancladosRef = useRef<Map<string, Anclado>>(new Map());
  /* Menú del desplegable de un campo de formulario (modo rellenar): opciones a
   * mostrar y posición en viewport. null = cerrado. */
  const [selectMenu, setSelectMenu] = useState<{
    field: HTMLElement;
    options: string[];
    top: number;
    left: number;
  } | null>(null);
  const selectMenuRef = useRef<HTMLDivElement>(null);
  useMobileBack(selectMenu !== null, () => setSelectMenu(null));
  useFitInViewport(selectMenuRef, selectMenu !== null, { vertical: true });
  const formFill = ctx.ui.formFill;
  const track = ctx.ui.track;
  // El contenido solo se monta al cambiar de época. Estas refs dan al efecto los
  // valores más recientes sin convertir cada pulsación en un remontaje del DOM.
  const loadRef = useRef({
    content: ctx.content,
    track: ctx.ui.track,
    setUi: ctx.setUi,
    setPageSetup: ctx.setPageSetup,
  });
  loadRef.current = {
    content: ctx.content,
    track: ctx.ui.track,
    setUi: ctx.setUi,
    setPageSetup: ctx.setPageSetup,
  };

  const zoom = ctx.ui.zoom / 100;
  const { w: pageW, h: pageH } = medidasPagina(ctx.pageSetup);
  const margenes = margenesPagina(ctx.pageSetup);
  /* Centrado del folio a cualquier zoom: `margin:auto` colapsa a 0 cuando el
   * interior (pageW, sin escalar) es más ancho que el exterior (pageW·zoom) y el
   * folio se va a la derecha pageW·(1−zoom)/2. Este margen admite valores
   * negativos y centra la caja escalada (= lo que auto daría ya a zoom ≥ 1). */
  const centerMargin = (pageW * (zoom - 1)) / 2;

  /* Estadísticas (§6.5): folio continuo medido contra el alto A4. offsetHeight
   * va en px de maqueta (sin escalar); scrollTop va en px visuales, así que el
   * divisor escala con el zoom. Solo se informa cuando algo cambia. */
  const measure = (): void => {
    const page = pageRef.current;
    const scroller = scrollRef.current;
    // offsetHeight 0 = el folio no se está pintando (en móvil le cede la pantalla
    // al panel lateral, EditorLayout): medirlo daría «1 de 1» y borraría de la
    // barra de estado el recuento bueno, que sigue siendo válido.
    if (!page || !scroller || page.offsetHeight === 0) return;
    const pages = Math.max(1, Math.ceil(page.offsetHeight / pageH));
    const current = Math.min(
      pages,
      // El aire ya no escala con el zoom (padding ÷ zoom), así que en pantalla mide
      // PAGE_GAP a secas; el folio sí escala.
      Math.max(1, Math.floor((scroller.scrollTop - PAGE_GAP) / (pageH * zoom)) + 1),
    );
    const last = lastStatsRef.current;
    if (last.pages !== pages || last.page !== current) {
      lastStatsRef.current = { pages, page: current };
      ctx.reportStats({ pages, page: current });
    }
  };
  measureRef.current = measure;

  /* Throttle de 500 ms para las mediciones disparadas por tecleo (§6.5). */
  const measureSoon = (): void => {
    if (throttleRef.current !== null) return;
    throttleRef.current = window.setTimeout(() => {
      throttleRef.current = null;
      measureRef.current();
    }, 500);
  };

  /* Pistas de las regiones vacías, en el idioma de la interfaz. */
  useEffect(() => {
    ensureHeaderFooterStyle(t('insert.header'), t('insert.footer'));
  }, [t]);

  /* Registra el root editable en exec (§3) al montar; lo retira al desmontar. */
  useEffect(() => {
    setEditorRoot(pageRef.current);
    return () => {
      setEditorRoot(null);
      if (throttleRef.current !== null) window.clearTimeout(throttleRef.current);
    };
  }, []);

  /* Campo de número de página (§ tarea 1): «Página X de Y» en vivo. Se escribe
   * por DOM directo (no emite `input`) para no disparar autosave en cada scroll;
   * el ELEMENTO [data-page-field] sí persiste en el HTML del folio. Se refresca
   * al cambiar el recuento/página visible o al re-aplicarse el contenido. */
  useEffect(() => {
    const page = pageRef.current;
    if (!page) return;
    const text = pageFieldText(ctx.stats.page, ctx.stats.pages);
    page.querySelectorAll('[data-page-field]').forEach((el) => {
      if (el.textContent !== text) el.textContent = text;
    });
  }, [ctx.stats.page, ctx.stats.pages, ctx.contentEpoch]);

  /* El HTML solo se re-aplica al DOM en cambio de época (carga, adopción remota,
   * restauración de versión): así el caret sobrevive a cada tecla (§6.5). El modo
   * «control de cambios» viaja como sentinela invisible al frente del contenido
   * (readTrackFlag): se retira antes de montar el HTML y se refleja en ui.track,
   * de modo que reabrir un doc «en revisión» lo restaura (persistencia por-doc).
   * Se lee ui.track SIN declararlo dependencia a propósito: este efecto solo debe
   * correr al cambiar la época, no al conmutar el modo (re-montar el HTML pisaría
   * la edición en vuelo). */
  useEffect(() => {
    const page = pageRef.current;
    if (!page) return;
    const latest = loadRef.current;
    const { on, html } = readTrackFlag(latest.content);
    // El contenido lo pudo escribir un coautor (o quien canjeó un enlace de
    // edición, ADR 0052, también de otra organización): se sanea antes de montarlo
    // para que un `<img onerror>`/`<svg onload>` no ejecute en esta sesión.
    page.innerHTML = saneaHtmlDeDocumento(html) || '<p><br></p>';
    if (on !== latest.track) latest.setUi({ track: on });
    // La página (tamaño, márgenes y papel) la guarda el documento: la de este, o la de siempre.
    latest.setPageSetup(leerPagina(page.querySelector('[data-sytue-page]')));
    // Paradas de tabulación de Word: dependen de lo que mide el texto, así que se recolocan
    // cuando llegan las fuentes.
    tabular(page);
    void document.fonts?.ready.then(() => page.isConnected && tabular(page));
    measureRef.current();
  }, [ctx.contentEpoch]);

  /* Bloques de embed (informes Power BI): tras montar el HTML, mintea el artefacto
   * server-side (BFF /api/embed) y enmarca el informe en un iframe sandbox. El iframe
   * vive FUERA del modelo (stripEmbedLive lo retira del autosave); solo el placeholder
   * + snapshot persisten. El AbortController cancela fetches/listeners al re-montar
   * (nueva época) o desmontar; cada época re-hidrata desde el HTML ya limpio. */
  useEffect(() => {
    const page = pageRef.current;
    if (!page) return;
    const ctrl = new AbortController();
    hydrateEmbeds(page, ctrl.signal);
    return () => ctrl.abort();
  }, [ctx.contentEpoch]);

  /* Zoom, orientación o márgenes alteran el recuento/posición de páginas. */
  useEffect(() => {
    measureRef.current();
  }, [ctx.ui.zoom, ctx.pageSetup]);

  /* Ajuste inicial al ancho. Un folio A4 mide 816px y el editor abría siempre al
   * 100 %: en un teléfono de 320px se salían 2.226px y el documento se leía
   * arrastrando de lado. Solo se hace UNA vez y solo si no cabe — a partir de
   * ahí manda el deslizador de la barra de estado. */
  const autoFit = useRef(false);
  useEffect(() => {
    const scroller = scrollRef.current;
    if (autoFit.current || !scroller || scroller.clientWidth === 0) return;
    autoFit.current = true;
    const disponible = scroller.clientWidth - PAGE_GAP;
    if (disponible >= pageW) return;
    ctx.setUi({ zoom: Math.max(MIN_ZOOM, Math.floor((disponible / pageW) * 100)) });
    // pageW entra por si el documento abre apaisado; `ctx` no, para no re-entrar
    // en cada cambio de estado del editor (la guarda ya lo impediría, pero esto
    // evita recorrer el efecto en cada tecla).
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pageW]);

  /* Cursor remoto — ENVÍO: publica el caret/selección local (offsets de texto en
   * el folio) a los peers en cada cambio de selección; el transporte agrupa por
   * tiempo (throttle) y degrada sin WS. Locales estables para exhaustive-deps. */
  const { sendCursor, peerCursors, contentEpoch, pageSetup, cursorDots } = ctx;
  useEffect(() => {
    const page = pageRef.current;
    if (!page) return;
    const onSel = (): void => {
      const off = docSelectionOffsets(page);
      if (!off) return;
      // Anclado a la identidad de los caracteres si la secuencia coincide con el folio;
      // si no (se acaba de teclear), solo offsets.
      const dots = cursorDots.aPuntos((page.textContent ?? '').length, off);
      sendCursor({ kind: 'doc', anchor: off.anchor, focus: off.focus, ...dots });
    };
    document.addEventListener('selectionchange', onSel);
    return () => document.removeEventListener('selectionchange', onSel);
  }, [sendCursor, cursorDots]);

  /* Cursor remoto — RENDER: resuelve los offsets de cada peer a rects en el
   * espacio del folio (resta el rect del wrapper y divide por el zoom, que la capa
   * vuelve a aplicar). useEffect (no layout) declarado DESPUÉS del montaje del
   * contenido (arriba): lee el HTML ya aplicado en el mismo commit de contentEpoch.
   * Las coordenadas son relativas al wrapper (se desplaza entero) → estables bajo scroll. */
  useEffect(() => {
    const page = pageRef.current;
    const wrap = wrapRef.current;
    if (!page || !wrap) return;
    const wr = wrap.getBoundingClientRect();
    const z = zoom || 1;
    const out: PeerCaretView[] = [];
    const texto = page.textContent ?? '';
    const anclados = new Map<string, Anclado>();
    for (const pc of peerCursors) {
      if (pc.pos.kind !== 'doc') continue;
      // Anclado al texto: el offset llegó con el texto de ENTONCES; si otra persona
      // editó antes de él desde que lo recibimos, se reubica (state/cursors.ts).
      const previo = ancladosRef.current.get(pc.id);
      // Con los puntos de la secuencia, exacto; sin ellos (o sin coincidir con el folio), la
      // heurística sobre el texto.
      const exacto =
        pc.pos.anchorDot && pc.pos.focusDot
          ? cursorDots.aOffsets(texto.length, {
              anchorDot: pc.pos.anchorDot,
              focusDot: pc.pos.focusDot,
            })
          : null;
      const pos: Extract<CursorPos, { kind: 'doc' }> = exacto
        ? { ...pc.pos, ...exacto }
        : previo && previo.origen === pc.pos
          ? anclarPos(previo.pos, previo.texto, texto)
          : pc.pos;
      anclados.set(pc.id, { origen: pc.pos, pos, texto });
      const caretR = caretRectAtOffset(page, pos.focus);
      if (!caretR) continue;
      const sel =
        pos.anchor === pos.focus
          ? []
          : selectionRectsAt(page, pos.anchor, pos.focus).map((r) => ({
              left: (r.left - wr.left) / z,
              top: (r.top - wr.top) / z,
              width: r.width / z,
              height: r.height / z,
            }));
      out.push({
        id: pc.id,
        name: peerName(pc.name),
        color: pc.color,
        posKey: cursorPosKey(pc.pos),
        caret: {
          left: (caretR.left - wr.left) / z,
          top: (caretR.top - wr.top) / z,
          height: caretR.height / z || CARET_FALLBACK_H,
        },
        sel,
      });
    }
    ancladosRef.current = anclados;
    setPeerCarets(out);
  }, [peerCursors, contentEpoch, zoom, pageSetup, cursorDots]);

  /* Modo «Rellenar formulario» (§ campos): el folio pasa a solo lectura
   * (contentEditable=false, abajo en el JSX) pero los campos siguen interactivos.
   * Aquí se marca el folio (data-form-fill, para editor.css) y se ajusta cada
   * campo: el de texto se hace isla editable; casilla/desplegable se hacen
   * focalizables por teclado. Se re-aplica también al recargar el contenido
   * (contentEpoch), porque los campos se re-parsean desde el HTML. */
  useEffect(() => {
    const page = pageRef.current;
    if (!page) return;
    page.toggleAttribute('data-form-fill', formFill);
    page.querySelectorAll<HTMLElement>('[data-formfield]').forEach((field) => {
      const type = field.getAttribute('data-formfield');
      const name = field.getAttribute('data-name') ?? '';
      if (type === 'text') {
        if (formFill) field.contentEditable = 'true';
        else field.removeAttribute('contenteditable'); // hereda del folio (editable como autor)
      } else if (formFill) {
        field.setAttribute('tabindex', '0');
      } else {
        field.removeAttribute('tabindex');
      }
      // Semántica accesible SOLO en modo rellenar (cuando los campos son interactivos):
      // se aplica al DOM vivo, no al HTML persistido, para sobrevivir al saneado. Sin ella
      // el lector de pantalla leía un <span> desnudo (casilla/desplegable sin rol ni estado).
      if (formFill) {
        field.setAttribute('aria-label', name);
        if (type === 'checkbox') {
          field.setAttribute('role', 'checkbox');
          field.setAttribute(
            'aria-checked',
            field.getAttribute('data-checked') === '1' ? 'true' : 'false',
          );
        } else if (type === 'select') {
          field.setAttribute('role', 'button');
          field.setAttribute('aria-haspopup', 'true');
          const val = field.getAttribute('data-value');
          if (val) field.setAttribute('aria-label', `${name}, ${val}`);
        }
      } else {
        for (const a of ['role', 'aria-checked', 'aria-haspopup', 'aria-label'])
          field.removeAttribute(a);
      }
    });
    if (!formFill) setSelectMenu(null);
  }, [formFill, ctx.contentEpoch]);

  /* Interceptación de la edición SOLO con el modo activo (sin él: cero coste, la
   * edición nativa queda intacta). `beforeinput` permite preventDefault y aplicar
   * la transformación rastreada; getTargetRanges da EXACTAMENTE lo que el navegador
   * iba a borrar (un grafema o la selección). Lo no cubierto (Intro, formato, IME,
   * pegar con formato/imágenes) se deja pasar nativo: follow-up documentado en exec.ts. */
  useEffect(() => {
    const page = pageRef.current;
    if (!page || !track) return;
    const onBeforeInput = (event: InputEvent): void => {
      const type = event.inputType;
      if (type === 'insertText' && event.data !== null) {
        event.preventDefault();
        trackInsert(event.data);
      } else if (type === 'insertFromPaste' || type === 'insertFromDrop') {
        const text = event.dataTransfer?.getData('text/plain') ?? '';
        // Sin texto plano (imagen/HTML sin texto, o navegador sin dataTransfer aquí):
        // NO se cancela, se deja el pegado nativo (sin rastrear) para no perder datos.
        if (!text) return;
        event.preventDefault();
        trackInsert(text); // MVP: solo texto plano
      } else if (type.startsWith('delete')) {
        const target = event.getTargetRanges()[0];
        // Borde/estructura (Retroceso al inicio del párrafo…): sin nada que borrar,
        // se deja el comportamiento nativo (unir párrafos sin rastrear).
        if (!target || target.collapsed) return;
        event.preventDefault();
        trackDelete(target, type.includes('Backward') || type === 'deleteByCut');
      }
    };
    page.addEventListener('beforeinput', onBeforeInput as EventListener);
    return () => page.removeEventListener('beforeinput', onBeforeInput as EventListener);
  }, [track]);

  /* Interacción de los campos en modo rellenar: casilla conmuta y desplegable
   * abre su menú (por clic o teclado). El campo de texto se edita nativo (isla
   * contentEditable). Cada cambio emite `input` (burbujea al folio → autosave). */
  useEffect(() => {
    const page = pageRef.current;
    if (!page || !formFill) return;
    const activate = (field: HTMLElement): void => {
      const type = field.getAttribute('data-formfield');
      if (type === 'checkbox') {
        const on = field.getAttribute('data-checked') === '1';
        field.setAttribute('data-checked', on ? '0' : '1');
        field.setAttribute('aria-checked', on ? 'false' : 'true'); // espeja el nuevo estado para el SR
        field.dispatchEvent(new Event('input', { bubbles: true }));
      } else if (type === 'select') {
        const rect = field.getBoundingClientRect();
        setSelectMenu({
          field,
          options: parseOptions(field.getAttribute('data-options') ?? ''),
          top: rect.bottom + 2,
          left: rect.left,
        });
      }
    };
    const fieldOf = (target: EventTarget | null): HTMLElement | null => {
      const el = (target as HTMLElement | null)?.closest?.(
        '[data-formfield]',
      ) as HTMLElement | null;
      return el && page.contains(el) ? el : null;
    };
    const onFieldClick = (event: MouseEvent): void => {
      const field = fieldOf(event.target);
      const type = field?.getAttribute('data-formfield');
      if (field && (type === 'checkbox' || type === 'select')) {
        event.preventDefault();
        activate(field);
      }
    };
    const onFieldKey = (event: KeyboardEvent): void => {
      if (event.key !== 'Enter' && event.key !== ' ') return;
      const field = fieldOf(event.target);
      const type = field?.getAttribute('data-formfield');
      if (field && (type === 'checkbox' || type === 'select')) {
        event.preventDefault();
        activate(field);
      }
    };
    page.addEventListener('click', onFieldClick);
    page.addEventListener('keydown', onFieldKey);
    return () => {
      page.removeEventListener('click', onFieldClick);
      page.removeEventListener('keydown', onFieldKey);
    };
  }, [formFill]);

  /* Cierre del menú del desplegable: clic fuera, Escape o scroll del lienzo. */
  useEffect(() => {
    if (!selectMenu) return;
    const close = (): void => setSelectMenu(null);
    const onDown = (event: MouseEvent): void => {
      if (!(event.target as HTMLElement | null)?.closest?.('[data-formfield-menu]')) close();
    };
    const onKey = (event: KeyboardEvent): void => {
      if (event.key === 'Escape') close();
    };
    const scroller = scrollRef.current;
    document.addEventListener('mousedown', onDown);
    document.addEventListener('keydown', onKey);
    scroller?.addEventListener('scroll', close);
    return () => {
      document.removeEventListener('mousedown', onDown);
      document.removeEventListener('keydown', onKey);
      scroller?.removeEventListener('scroll', close);
    };
  }, [selectMenu]);

  /* Atajos del lienzo (§6.8): los del registro de comandos (editor/comandos.json).
   * Ctrl/Cmd+Z e Y no se capturan: van marcados `canvas` y los atiende el historial
   * nativo del contenteditable. */
  const onKeyDown = (event: KeyboardEvent): void => {
    const atajo = comandoDelAtajo(event, 'document');
    if (!atajo) return;
    event.preventDefault();
    ctx.exec(atajo.id as EditorCommand, atajo.value);
  };

  /* Paginación VISUAL: una línea de corte cada alto de página, pintada como
   * gradiente repetido desde el borde superior del folio (sin nodos extra;
   * el folio no tiene borde, luego el origen del fondo coincide con su borde).
   * ponytail: folio continuo que crece; cortes reales con salto de página en fase 2. */
  const pageStyle: Record<string, string> = {
    width: `${pageW}px`,
    minHeight: `${pageH}px`,
    padding: margenes.map((m) => `${m}px`).join(' '),
    margin: '0 auto',
    /* Contexto de posicionado para el numerado (::after de Disposición) y las
     * bandas de encabezado/pie, que se anclan al folio. La var expone el margen
     * VIVO para que las bandas se alineen con el área de contenido y sigan los
     * cambios de margen (su HTML referencia var(--doc-page-margin)). */
    position: 'relative',
    // ponytail: una sola variable para las bandas; con márgenes distintos, el izquierdo.
    '--doc-page-margin': `${margenes[3]}px`,
  };
  if (ctx.pageSetup.color) pageStyle.backgroundColor = ctx.pageSetup.color;
  if (ctx.stats.pages > 1) {
    pageStyle.backgroundImage = `repeating-linear-gradient(to bottom, transparent 0, transparent ${
      pageH - 2
    }px, var(--editor-page-break) ${pageH - 2}px, var(--editor-page-break) ${pageH}px)`;
  }

  return (
    <div
      ref={scrollRef}
      class="h-full min-h-0 w-full flex-1 overflow-auto"
      style={{ background: 'var(--editor-canvas-bg)' }}
      onScroll={measure}
    >
      {/* Reserva el ancho VISUAL (escalado) para centrar el folio y alcanzarlo
          con scroll horizontal a cualquier zoom. */}
      <div style={{ width: `${pageW * zoom}px`, margin: '0 auto' }}>
        {/* Zoom por transform scale con origen superior-centro (tarea A2). La regla
            (Ruler, B6) vive dentro del wrapper para escalar y alinearse con el folio.
            ponytail: la regla se desplaza con el scroll (sticky no convive con el
            transform del wrapper); fijarla exigiría sacarla del área escalada.
            ponytail: el área de scroll vertical se apoya en que los navegadores
            evergreen incluyen el overflow de cajas transformadas; compensar la
            altura de maqueta solo si algún navegador soportado recortase. */}
        <div
          ref={wrapRef}
          style={{
            position: 'relative', // sistema de coordenadas de la capa de cursores remotos
            width: `${pageW}px`,
            margin: `0 ${centerMargin}px`,
            // El aire es del LIENZO, no del folio: dentro de esta caja lo escala el
            // transform, así que al 50 % quedaban 16 px y al 200 %, 64. Se divide por el
            // zoom para que en pantalla midan siempre los 32 px de §6.5.
            padding: `${PAGE_GAP / zoom}px 0`,
            transform: `scale(${zoom})`,
            transformOrigin: 'top center',
          }}
        >
          <Ruler ctx={ctx} />
          <div
            ref={pageRef}
            class="sytue-page"
            // El papel: lo que haya dentro es contenido del documento, no cromo (suelo táctil).
            data-sin-tactil
            // En solo lectura el folio NO es editable. La barrera de verdad está en
            // `exec` y en `onContentInput`; esto es para que el cursor no parpadee
            // invitando a escribir donde no se puede.
            contentEditable={formFill || ctx.soloLectura ? 'false' : 'true'}
            aria-readonly={ctx.soloLectura ? 'true' : undefined}
            role="textbox"
            tabIndex={0}
            aria-multiline="true"
            aria-label={ctx.title || t('kind.document')}
            lang="es"
            spellcheck
            style={pageStyle}
            onInput={(event) => {
              // El modo control de cambios viaja como sentinela al frente del HTML
              // persistido (vive en el contenido, que ES el modelo; §control de cambios).
              // stripEmbedLive retira el iframe vivo del embed: solo el placeholder +
              // snapshot persisten (el token ni está en el DOM). Fast-path sin embeds.
              // Al teclear solo cambia el párrafo del cursor; lo demás (pegar, un comando)
              // puede tocar cualquiera. quitarTabulaciones retira sus envoltorios vivos.
              const tipo = (event as InputEvent).inputType ?? '';
              const tecleo =
                /^(insertText|insertCompositionText|insertReplacementText|delete)/.test(tipo);
              tabular(
                event.currentTarget,
                tecleo ? (window.getSelection()?.anchorNode ?? null) : null,
              );
              ctx.onContentInput(
                withTrackFlag(
                  quitarTabulaciones(stripEmbedLive(event.currentTarget.innerHTML)),
                  track,
                ),
              );
              measureSoon();
            }}
            onKeyDown={onKeyDown}
          />
          {/* Capa de cursores remotos: caret fino + resaltado de selección +
              etiqueta, en el espacio del folio (bajo el transform del wrapper).
              FUERA del contenteditable: no contamina el innerHTML (que ES el modelo). */}
          {peerCarets.length > 0 && (
            <div class="pointer-events-none absolute inset-0 overflow-hidden" aria-hidden="true">
              {peerCarets.map((pc) => (
                <div key={pc.id} class="contents">
                  {pc.sel.map((r, i) => (
                    <div
                      key={i}
                      class="absolute rounded-[1px]"
                      style={{
                        left: `${r.left}px`,
                        top: `${r.top}px`,
                        width: `${r.width}px`,
                        height: `${r.height}px`,
                        background: pc.color,
                        opacity: 0.18,
                      }}
                    />
                  ))}
                  <div
                    class="absolute"
                    style={{
                      left: `${pc.caret.left}px`,
                      top: `${pc.caret.top}px`,
                      width: '2px',
                      height: `${pc.caret.height}px`,
                      background: pc.color,
                    }}
                  >
                    <span
                      key={pc.posKey}
                      class="sytue-peer-label text-2xs absolute -top-[15px] left-0 max-w-[160px] truncate rounded-sm px-1 py-px leading-tight font-medium text-white"
                      style={{ background: pc.color }}
                    >
                      {pc.name}
                    </span>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
      {/* Overlay de imagen: marco + tiradores + mini-toolbar al seleccionar un
          `<img>`. Fuera del wrapper transformado (contenedor `fixed` a viewport),
          por el mismo motivo que el menú de campos. */}
      <ImageOverlay ctx={ctx} />
      <DibujoHost ctx={ctx} />
      {/* Menú del desplegable (modo rellenar). Vive FUERA del wrapper con transform
          scale: un ancestro transformado sería el bloque contenedor de un fixed y
          descuadraría las coordenadas de viewport de getBoundingClientRect. */}
      {selectMenu && (
        // onMouseDown solo evita perder el foco; cada opción es un <button> accesible.
        // eslint-disable-next-line jsx-a11y/no-noninteractive-element-interactions
        <div
          data-formfield-menu
          ref={selectMenuRef}
          role="group"
          aria-label={t('formfield.options')}
          class="sytue-menu-surface border-border fixed z-[var(--z-popover)] flex max-h-60 min-w-[8rem] flex-col overflow-auto rounded-lg border p-1 shadow-[var(--shadow-md)]"
          style={{ top: `${selectMenu.top}px`, left: `${selectMenu.left}px` }}
          onMouseDown={(e) => e.preventDefault()}
        >
          {selectMenu.options.length === 0 ? (
            <p class="text-text-muted m-0 px-2 py-1.5 text-xs">{t('formfield.options.empty')}</p>
          ) : (
            selectMenu.options.map((opt) => {
              const current = selectMenu.field.getAttribute('data-value') === opt;
              return (
                <button
                  key={opt}
                  type="button"
                  class={
                    'hover:bg-bg-subtle flex w-full items-center rounded-sm px-2 py-1 text-left text-xs ' +
                    'sytue-focus-ring ' +
                    (current ? 'text-text font-medium' : 'text-text-muted')
                  }
                  onClick={() => {
                    selectMenu.field.setAttribute('data-value', opt);
                    // El botón anuncia «nombre, valor» al elegir (nombre solo no basta para el SR).
                    selectMenu.field.setAttribute(
                      'aria-label',
                      `${selectMenu.field.getAttribute('data-name') ?? ''}, ${opt}`,
                    );
                    selectMenu.field.dispatchEvent(new Event('input', { bubbles: true }));
                    setSelectMenu(null);
                  }}
                >
                  {opt}
                </button>
              );
            })
          )}
        </div>
      )}
    </div>
  );
}
