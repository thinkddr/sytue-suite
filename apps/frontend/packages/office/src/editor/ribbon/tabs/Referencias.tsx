/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Pestaña «Referencias» del ribbon (BLUEPRINT.md §6.3; solo kind document):
 * tabla de contenidos (generar + actualizar), nota al pie, vínculos y
 * comentario.
 *
 * Decisiones no cubiertas por el blueprint (§0.10, se documentan aquí):
 * - «Actualizar TDC» reescribe el <nav data-toc> EN SU SITIO (re-ejecutar
 *   exec('insertToc') lo movería al caret); si aún no hay TDC, lo genera.
 * - Sangría por nivel del TDC: margin-left inline en cada <li> (0/1,5/3 em).
 *   Inline y no en editor.css para que sobreviva a la exportación HTML/PDF
 *   (el HTML ES el modelo) — mismo criterio que el lineHeight inline de §3.1.
 * - La nota al pie no existe en EditorCommand (§2): la mutación vive aquí,
 *   sobre getEditorRoot(), y termina emitiendo `input` para que el autosave
 *   la recoja (mismo patrón que los caminos manuales de exec.ts). Markup
 *   propio: marcador <sup data-footnote-ref id="fnref-N"><a href="#fn-N">N</a></sup>
 *   y una única <section data-footnotes> al final del folio (§6.5: el folio
 *   es continuo, su final ES el final de página) con <hr> + <ol> de notas
 *   <li id="fn-N" value="N"> — `value` mantiene el número visible alineado
 *   con el marcador aunque se borren notas intermedias.
 */
import type { JSX } from 'preact';
import { preguntarEnEditor, useMobileBack } from '@sytue/ui';
import type { DocsT } from '../../../i18n/index';
import { useT } from '../../context';
import { useEffect, useRef, useState } from 'preact/hooks';
import { executeEditingCommand } from '@sytue/ui/browser/editing';
import type { EditorContext } from '../../types';
import { captionLabel, getEditorRoot } from '../../canvas/exec';
import type { CaptionKind } from '../../canvas/exec';
import { renumberEndnotes } from '../../endnotes';
import type { EndnoteEntry } from '../../endnotes';
import { EditorIcon } from '../../icons';
import {
  RibbonButton,
  RibbonGroup,
  RibbonSeparator,
  POPOVER_SHELL,
  useBloqueado,
} from '../primitives';
import { CorrespondenciaGroup } from './Correspondencia';
import { CitasBibliografiaGroup } from './Bibliografia';
import { buildIndexHtml, indexAnchorId, parseIndexTerm } from '../../alphaindex';
import type { IndexEntry } from '../../alphaindex';
import { editorT } from '../../../i18n';

// El anillo canónico vive en `@sytue/ui/styles.css` (.sytue-focus-ring): una sola
// definición para toda la suite, en vez de la copia que cada fichero perdía a su manera.
const FOCUS_RING = 'sytue-focus-ring';

/* ---------- utilidades ---------- */

const HTML_ESCAPES: Record<string, string> = {
  '&': '&amp;',
  '<': '&lt;',
  '>': '&gt;',
  '"': '&quot;',
  "'": '&#39;',
};

function escapeHtml(text: string): string {
  return text.replace(/[&<>"']/g, (ch) => HTML_ESCAPES[ch] ?? ch);
}

/** El autosave escucha `input` en el root; las mutaciones manuales lo emiten a mano (§3). */
function emitirInput(root: HTMLElement): void {
  root.dispatchEvent(new Event('input', { bubbles: true }));
}

/* ---------- tabla de contenidos ---------- */

/** Sangría por nivel de título: h1 = 0 · h2 = 1,5 em · h3 = 3 em. */
const SANGRIA_EM = 1.5;

/**
 * <li> del índice a partir de los h1–h3 del folio: ids estables `h-<n>`
 * saltando los ya ocupados (mismo algoritmo que exec.insertToc, §3.1),
 * ancla a cada título y sangría inline por nivel.
 */
function itemsTdc(root: HTMLElement): string {
  let n = 0;
  return Array.from(root.querySelectorAll<HTMLElement>('h1,h2,h3'))
    .map((titulo) => {
      if (!titulo.id) {
        do {
          n += 1;
        } while (root.querySelector(`[id="h-${n}"]`) !== null);
        titulo.id = `h-${n}`;
      }
      const nivel = titulo.tagName === 'H1' ? 1 : titulo.tagName === 'H2' ? 2 : 3;
      return (
        `<li data-level="${nivel}" style="margin-left:${(nivel - 1) * SANGRIA_EM}em">` +
        `<a href="#${escapeHtml(titulo.id)}">${escapeHtml(titulo.textContent ?? '')}</a></li>`
      );
    })
    .join('');
}

/** Reescribe el contenido del <nav data-toc> con el índice actual (mismo shell que §3.1). */
export function reconstruirTdc(root: HTMLElement, nav: Element): void {
  nav.innerHTML = `<p><b>${editorT()('ref.toc')}</b></p><ol>${itemsTdc(root)}</ol>`;
  emitirInput(root);
}

/** Genera el TDC en el caret (exec §3.1) y le aplica en sitio anclas + sangría propias. */
function generarTdc(ctx: EditorContext): void {
  ctx.exec('insertToc'); // retira el nav previo e inserta uno nuevo en el caret (B5)
  const root = getEditorRoot();
  const nav = root?.querySelector('nav[data-toc]');
  if (root && nav) reconstruirTdc(root, nav); // contenido idéntico + sangría inline
}

/** Actualiza el TDC donde está; sin TDC previo, lo genera en el caret. */
function actualizarTdc(ctx: EditorContext): void {
  const root = getEditorRoot();
  if (!root) return;
  const nav = root.querySelector('nav[data-toc]');
  if (nav) reconstruirTdc(root, nav);
  else generarTdc(ctx);
}

/* ---------- nota al pie ---------- */

/**
 * Coloca el caret al final del cuerpo (justo antes de la sección de notas),
 * garantizando que exista un bloque donde insertar el marcador.
 */
function caretAlFinalDelCuerpo(root: HTMLElement, notas: Element | null): void {
  let objetivo = notas ? notas.previousElementSibling : root.lastElementChild;
  if (!objetivo) {
    const p = document.createElement('p');
    p.innerHTML = '<br>';
    root.insertBefore(p, notas);
    objetivo = p;
  }
  const range = document.createRange();
  range.selectNodeContents(objetivo);
  range.collapse(false);
  const sel = document.getSelection();
  if (sel) {
    sel.removeAllRanges();
    sel.addRange(range);
  }
}

/**
 * Inserta un marcador superíndice numerado en el caret y su nota en la
 * sección <section data-footnotes> del final del folio (la crea si falta).
 */
function insertarNotaAlPie(): void {
  const root = getEditorRoot();
  if (!root) return;

  // Número: marcadores existentes + 1, saltando ids aún ocupados por notas
  // borradas a medias (mismo criterio de estabilidad que los ids del TDC).
  let n = root.querySelectorAll('sup[data-footnote-ref]').length;
  do {
    n += 1;
  } while (root.querySelector(`[id="fn-${n}"], [id="fnref-${n}"]`) !== null);

  let notas: Element | null = root.querySelector('section[data-footnotes]');

  // El marcador va al caret; si la selección no vive en el cuerpo (o cae
  // dentro de las propias notas), va al final del contenido.
  const sel = document.getSelection();
  const ancla = sel && sel.rangeCount > 0 ? sel.getRangeAt(0).commonAncestorContainer : null;
  const enCuerpo =
    ancla !== null && root.contains(ancla) && !(notas !== null && notas.contains(ancla));
  const activo = document.activeElement;
  if (!activo || !root.contains(activo)) root.focus({ preventScroll: true });
  if (!enCuerpo) caretAlFinalDelCuerpo(root, notas);

  // execCommand para que el marcador participe del historial nativo de undo.
  let insertado = false;
  try {
    insertado = executeEditingCommand(
      'insertHTML',
      `<sup data-footnote-ref="${n}" id="fnref-${n}"><a href="#fn-${n}">${n}</a></sup>`,
    );
  } catch {
    insertado = false;
  }
  if (!insertado) return;

  // Sección única de notas al final del folio: <hr> separador + lista <ol>.
  let lista = notas?.querySelector('ol') ?? null;
  if (!notas || !lista) {
    notas?.remove(); // sección corrupta sin <ol>: se regenera
    notas = document.createElement('section');
    notas.setAttribute('data-footnotes', '');
    notas.innerHTML = '<hr><ol></ol>';
    root.append(notas);
    lista = notas.querySelector('ol');
  }
  if (!lista) return; // inalcanzable: la lista se acaba de crear

  const li = document.createElement('li');
  li.id = `fn-${n}`;
  li.value = n; // numeración visible = número del marcador, aunque falten notas intermedias
  li.textContent = editorT()('ref.note.placeholder');
  lista.append(li);

  // Deja el texto provisional seleccionado: teclear lo sustituye directamente.
  const range = document.createRange();
  range.selectNodeContents(li);
  if (sel) {
    sel.removeAllRanges();
    sel.addRange(range);
  }
  li.scrollIntoView({ block: 'nearest' });

  // ponytail: el alta de la sección es DOM manual y queda fuera del undo nativo
  // (deshacer retira el marcador pero no la nota); historial propio y
  // renumeración tras borrar notas llegarán con la migración a modelo Range.
  emitirInput(root);
}

/* ---------- notas al final (endnotes) ---------- */
/*
 * Hermanas de las notas al pie, pero recopiladas al FINAL del documento y
 * numeradas en ROMANO (i, ii, iii…) para distinguirlas de las notas al pie
 * (arábigo). El número visible de cada nota es su ENLACE DE RETORNO al marcador
 * (data-endnote-back, contenteditable=false), así la navegación es
 * marcador→nota (href="#en-N") y nota→marcador (href="#enref-N"). A diferencia
 * de las notas al pie, aquí SÍ se renumera al insertar/borrar: renumberEndnotes
 * (ayuda pura) asigna la secuencia 1..N en orden de documento y este pegamento
 * relabela los marcadores y reconstruye la sección. El borrado de un marcador se
 * recoge con «Actualizar notas al final» (mismo patrón que «Actualizar TDC»).
 */

const ENDNOTES_SELECTOR = 'section[data-endnotes]';

/** <ol> sin viñetas nativas: el número visible es el enlace de retorno (romano). */
const ENDNOTES_SHELL = '<hr><ol style="list-style:none;padding-left:0;margin:.25em 0"></ol>';

/** Contenido editable de una nota SIN su enlace de retorno (para preservarlo al renumerar). */
function contenidoNota(li: HTMLElement): string {
  const clon = li.cloneNode(true) as HTMLElement;
  clon.querySelector('[data-endnote-back]')?.remove();
  return clon.innerHTML.trim();
}

/**
 * <li> de una nota a partir de su plan: id estable en-N, enlace de retorno con el
 * romano como texto (número visible + navegación) y el contenido preservado. El
 * contenido es HTML del usuario ya presente en el folio (round-trip, no frontera
 * de confianza); seq/label son valores calculados seguros.
 */
function endnoteLi(e: EndnoteEntry): string {
  const t = editorT();
  return (
    `<li id="en-${e.seq}">` +
    `<a href="#enref-${e.seq}" data-endnote-back contenteditable="false" title="${escapeHtml(
      t('ref.endnote.back'),
    )}">${e.label}.</a> ` +
    `${e.content || escapeHtml(t('ref.note.placeholder'))}</li>`
  );
}

/**
 * Renumera los marcadores en orden de documento (1..N, romano) y reconstruye la
 * <section data-endnotes> con las notas recopiladas. Sin marcadores, retira la
 * sección. Camino manual: emite `input` para el autosave.
 */
function renumberEndnotesDom(root: HTMLElement): void {
  const markers = Array.from(root.querySelectorAll<HTMLElement>('sup[data-endnote-ref]'));
  const orderedIds = markers.map((m) => m.getAttribute('data-endnote-ref') ?? '');

  let section = root.querySelector<HTMLElement>(ENDNOTES_SELECTOR);
  const contentById = new Map<string, string>();
  section?.querySelectorAll<HTMLElement>('li[id^="en-"]').forEach((li) => {
    contentById.set(li.id.slice(3), contenidoNota(li)); // 'en-3' → '3'
  });

  const plan = renumberEndnotes(orderedIds, contentById);

  // Relabela los marcadores IN SITU. Se recorren por referencia retenida, así que
  // los ids duplicados transitorios durante la reasignación no afectan al bucle;
  // al terminar, todos los ids son únicos (enref-1..N).
  markers.forEach((m, i) => {
    const e = plan[i];
    if (!e) return;
    m.setAttribute('data-endnote-ref', String(e.seq));
    m.id = `enref-${e.seq}`;
    const a = m.querySelector('a');
    if (a) {
      a.setAttribute('href', `#en-${e.seq}`);
      a.textContent = e.label;
    }
  });

  if (plan.length === 0) {
    section?.remove(); // sin marcadores: no queda sección de notas
    emitirInput(root);
    return;
  }
  if (!section) {
    section = document.createElement('section');
    section.setAttribute('data-endnotes', '');
    section.innerHTML = ENDNOTES_SHELL;
    root.appendChild(section); // al final del documento (tras notas al pie si las hay)
  }
  let ol = section.querySelector('ol');
  if (!ol) {
    section.innerHTML = ENDNOTES_SHELL; // sección corrupta sin <ol>: se regenera
    ol = section.querySelector('ol');
  }
  if (ol) ol.innerHTML = plan.map(endnoteLi).join('');
  emitirInput(root);
}

/**
 * Inserta un marcador de nota al final en el caret y su nota en la
 * <section data-endnotes> del final del documento (la crea si falta), luego
 * renumera todo. Espejo de insertarNotaAlPie.
 */
function insertarNotaAlFinal(): void {
  const root = getEditorRoot();
  if (!root) return;

  // Id estable provisional (lo normaliza renumberEndnotesDom): siguiente libre,
  // saltando los ocupados (mismo criterio que fn-N/cap-N).
  let n = root.querySelectorAll('sup[data-endnote-ref]').length;
  do {
    n += 1;
  } while (root.querySelector(`[id="en-${n}"], [id="enref-${n}"]`) !== null);

  let notas: Element | null = root.querySelector(ENDNOTES_SELECTOR);

  // El marcador va al caret; si la selección no vive en el cuerpo (o cae dentro de
  // las propias notas), va al final del contenido (misma lógica que la nota al pie).
  const sel = document.getSelection();
  const ancla = sel && sel.rangeCount > 0 ? sel.getRangeAt(0).commonAncestorContainer : null;
  const enCuerpo =
    ancla !== null && root.contains(ancla) && !(notas !== null && notas.contains(ancla));
  const activo = document.activeElement;
  if (!activo || !root.contains(activo)) root.focus({ preventScroll: true });
  if (!enCuerpo) caretAlFinalDelCuerpo(root, notas);

  // execCommand para que el marcador participe del historial nativo de undo.
  let insertado = false;
  try {
    insertado = executeEditingCommand(
      'insertHTML',
      `<sup data-endnote-ref="${n}" id="enref-${n}"><a href="#en-${n}">i</a></sup>`,
    );
  } catch {
    insertado = false;
  }
  if (!insertado) return;

  // Sección única al final del documento: <hr> + <ol> sin viñetas nativas.
  let lista = notas?.querySelector('ol') ?? null;
  if (!notas || !lista) {
    notas?.remove(); // sección corrupta sin <ol>: se regenera
    notas = document.createElement('section');
    notas.setAttribute('data-endnotes', '');
    notas.innerHTML = ENDNOTES_SHELL;
    root.append(notas);
    lista = notas.querySelector('ol');
  }
  if (!lista) return; // inalcanzable: la lista se acaba de crear

  const li = document.createElement('li');
  li.id = `en-${n}`;
  const tNota = editorT();
  li.innerHTML =
    `<a href="#enref-${n}" data-endnote-back contenteditable="false" title="${escapeHtml(
      tNota('ref.endnote.back'),
    )}">i.</a> ` + escapeHtml(tNota('ref.note.placeholder'));
  lista.append(li);

  // Renumera en orden de documento (arregla el marcador recién insertado y
  // recopila las notas); el marcador (referencia retenida) queda con su id final.
  const marcador = root.querySelector<HTMLElement>(`[id="enref-${n}"]`);
  renumberEndnotesDom(root);

  // Deja el texto provisional de la nueva nota seleccionado (teclear lo sustituye),
  // sin tocar el enlace de retorno (contenteditable=false).
  const finalId = marcador ? marcador.id.replace('enref-', 'en-') : '';
  const nueva = finalId ? root.querySelector<HTMLElement>(`[id="${finalId}"]`) : null;
  if (nueva) {
    const range = document.createRange();
    range.selectNodeContents(nueva);
    const back = nueva.querySelector('[data-endnote-back]');
    if (back) range.setStartAfter(back);
    if (sel) {
      sel.removeAllRanges();
      sel.addRange(range);
    }
    nueva.scrollIntoView({ block: 'nearest' });
  }

  // ponytail: como en la nota al pie, el alta de la sección es DOM manual fuera del
  // undo nativo (deshacer retira el marcador, no la nota); «Actualizar notas al
  // final» recoge el borrado. Historial propio con la migración a modelo Range.
}

/** Renumera/recopila las notas al final (tras insertar o borrar marcadores). */
function actualizarNotasAlFinal(): void {
  const root = getEditorRoot();
  if (root) renumberEndnotesDom(root);
}

/* ---------- leyendas (captions) y referencias cruzadas ---------- */

/** Ancestro de `node` que es hijo directo del folio (unidad de bloque). */
function bloqueSuperior(root: HTMLElement, node: Node | null): HTMLElement | null {
  let el: Element | null = node instanceof Element ? node : (node?.parentElement ?? null);
  while (el && el.parentElement && el.parentElement !== root) el = el.parentElement;
  return el && el.parentElement === root ? (el as HTMLElement) : null;
}

/** Id estable `cap-<n>` saltando los ocupados (mismo criterio que TDC/notas). */
function idLeyenda(root: HTMLElement): string {
  let n = root.querySelectorAll('[data-caption]').length;
  do {
    n += 1;
  } while (root.querySelector(`[id="cap-${n}"]`) !== null);
  return `cap-${n}`;
}

/**
 * Inserta una leyenda numerada bajo la tabla/figura (o bajo el bloque del caret):
 * `<p data-caption=kind id=cap-N><span data-caption-label>Tabla N</span>. texto</p>`.
 * El número inicial = leyendas del mismo tipo + 1; «Actualizar» lo recalcula en orden.
 */
async function insertarLeyenda(kind: CaptionKind): Promise<void> {
  const root = getEditorRoot();
  if (!root) return;
  // El punto del documento se lee ANTES de preguntar: el diálogo se lleva el foco
  // y con él la selección viva, y sin ancla la leyenda acabaría al final del folio
  // en vez de bajo la tabla (mismo cuidado que markIndexEntry).
  const sel = document.getSelection();
  const anchor = sel && sel.rangeCount > 0 ? sel.getRangeAt(0).commonAncestorContainer : null;
  const enRoot = anchor !== null && root.contains(anchor);

  const respuesta = await preguntarEnEditor({
    mensaje: editorT()(kind === 'figura' ? 'dialog.caption.figure' : 'dialog.caption.table'),
    permitirVacio: true, // la leyenda puede ser solo «Tabla 3»
  });
  if (respuesta === null) return; // cancelar no inserta nada
  const texto = respuesta.trim();

  const num = root.querySelectorAll(`[data-caption="${kind}"]`).length + 1;
  const id = idLeyenda(root);
  const html =
    `<p data-caption="${kind}" id="${id}" style="font-size:0.9em;color:#475569;text-align:center;margin:.25em 0 .75em">` +
    `<span data-caption-label style="font-weight:600">${escapeHtml(captionLabel(kind, num))}</span>` +
    `${texto ? `. ${escapeHtml(texto)}` : ''}</p>`;

  // Tabla: bajo la tabla más cercana. Figura (o sin tabla): bajo el bloque del caret.
  const tabla =
    kind === 'tabla' && enRoot
      ? ((anchor instanceof Element ? anchor : anchor?.parentElement)?.closest('table') ?? null)
      : null;
  const destino =
    tabla && root.contains(tabla) ? tabla : enRoot ? bloqueSuperior(root, anchor) : null;
  if (destino) destino.insertAdjacentHTML('afterend', html);
  else root.insertAdjacentHTML('beforeend', html);

  const nuevo = root.querySelector(`[id="${id}"]`);
  nuevo?.scrollIntoView({ block: 'nearest' });
  // ponytail: inserción DOM directa (fuera del undo nativo, como notas/TDC);
  // historial propio con la migración a modelo Range.
  emitirInput(root);
}

/** Renumera todas las leyendas en orden de documento y actualiza las refs cruzadas. */
function actualizarLeyendas(): void {
  const root = getEditorRoot();
  if (!root) return;
  const contador: Record<CaptionKind, number> = { tabla: 0, figura: 0 };
  root.querySelectorAll<HTMLElement>('[data-caption]').forEach((cap) => {
    const kind: CaptionKind = cap.getAttribute('data-caption') === 'figura' ? 'figura' : 'tabla';
    contador[kind] += 1;
    const label = cap.querySelector('[data-caption-label]');
    if (label) label.textContent = captionLabel(kind, contador[kind]);
  });
  root.querySelectorAll<HTMLAnchorElement>('a[data-xref]').forEach((a) => {
    const id = a.getAttribute('data-xref') ?? '';
    const label = root.querySelector(`[id="${id}"] [data-caption-label]`)?.textContent;
    if (label) a.textContent = label;
  });
  emitirInput(root);
}

/** Datos de una leyenda para el selector de referencia cruzada. */
interface LeyendaRef {
  id: string;
  label: string;
  /** Texto de la leyenda (sin la etiqueta), recortado. */
  texto: string;
}

/** Lista las leyendas actuales del folio (para el popover de referencia cruzada). */
function leerLeyendas(): LeyendaRef[] {
  const root = getEditorRoot();
  if (!root) return [];
  return Array.from(root.querySelectorAll<HTMLElement>('[data-caption]')).map((cap) => {
    const label = cap.querySelector('[data-caption-label]')?.textContent ?? '';
    const texto = (cap.textContent ?? '')
      .slice(label.length)
      .replace(/^\.\s*/, '')
      .trim();
    return { id: cap.id, label, texto: texto.length > 40 ? `${texto.slice(0, 39)}…` : texto };
  });
}

/** Inserta la referencia cruzada en el caret vivo del folio (enlace al ancla). */
function insertarReferencia(ref: LeyendaRef): void {
  const root = getEditorRoot();
  if (!root) return;
  if (!root.contains(document.activeElement)) root.focus({ preventScroll: true });
  const html = `<a data-xref="${escapeHtml(ref.id)}" href="#${escapeHtml(ref.id)}">${escapeHtml(
    ref.label,
  )}</a>`;
  try {
    executeEditingCommand('insertHTML', html);
  } catch {
    /* navegador sin soporte: no-op */
  }
}

/** Botón + popover que lista las leyendas; al elegir una inserta su referencia. */
function ReferenciaCruzada(): JSX.Element {
  const bloqueado = useBloqueado('crossReference');
  const t = useT();
  const [open, setOpen] = useState(false);
  useMobileBack(open, () => setOpen(false));
  const [refs, setRefs] = useState<LeyendaRef[]>([]);
  const rootRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent): void => {
      if (!rootRef.current?.contains(e.target as Node)) setOpen(false);
    };
    const onKey = (e: KeyboardEvent): void => {
      if (e.key === 'Escape') setOpen(false);
    };
    document.addEventListener('mousedown', onDown);
    document.addEventListener('keydown', onKey);
    return () => {
      document.removeEventListener('mousedown', onDown);
      document.removeEventListener('keydown', onKey);
    };
  }, [open]);

  return (
    <div ref={rootRef} class="relative">
      <button
        type="button"
        class={
          'disabled:pointer-events-none disabled:opacity-40 ' +
          'text-text-muted inline-flex h-7 w-7 items-center justify-center rounded-sm ' +
          'hover:bg-bg-subtle hover:text-text ' +
          FOCUS_RING
        }
        aria-label={t('ref.crossReference.insert')}
        title={t('ref.crossReference.insert')}
        aria-haspopup="true"
        aria-expanded={open}
        disabled={bloqueado}
        onMouseDown={(e) => e.preventDefault()} // conserva el caret del folio
        onClick={() => {
          if (!open) setRefs(leerLeyendas());
          setOpen((v) => !v);
        }}
      >
        <EditorIcon name="xref" />
      </button>
      {open && (
        // onMouseDown solo evita perder el foco del lienzo; no es interacción de usuario.
        // eslint-disable-next-line jsx-a11y/no-noninteractive-element-interactions
        <div
          role="group"
          aria-label={t('ref.crossReference')}
          class={`${POPOVER_SHELL} left-0 max-h-64 w-60 overflow-auto p-1`}
          onMouseDown={(e) => e.preventDefault()} // el foco no sale del lienzo
        >
          {refs.length === 0 ? (
            <p class="text-text-muted m-0 px-2 py-1.5 text-xs">{t('ref.captions.empty')}</p>
          ) : (
            refs.map((ref) => (
              <button
                key={ref.id}
                type="button"
                class={
                  'hover:bg-bg-subtle flex w-full flex-col items-start gap-0.5 rounded-sm px-2 py-1 text-left ' +
                  FOCUS_RING
                }
                onClick={() => {
                  insertarReferencia(ref);
                  setOpen(false);
                }}
              >
                <span class="text-text text-xs font-medium">{ref.label}</span>
                {ref.texto && <span class="sytue-meta-dense">{ref.texto}</span>}
              </button>
            ))
          )}
        </div>
      )}
    </div>
  );
}

/* ---------- índice alfabético ---------- */
/*
 * Hermano del TDC pero por TÉRMINOS del usuario en vez de por títulos: «Marcar
 * entrada» envuelve la selección en <span data-index-entry="término"
 * data-index-sub="sub"> (marcador transparente que no altera el texto);
 * «Insertar/actualizar índice» recopila esos marcadores, los numera en orden de
 * documento (id idx-N, ancla del enlace) y vuelca la lista ORDENADA+AGRUPADA en
 * <section data-index>. La lógica pura (parsear «término:subtérmino», agrupar,
 * ordenar y escapar) vive en ../../alphaindex; aquí solo el pegamento con el folio.
 */

const INDEX_SELECTOR = 'section[data-index]';

/** node → su Element (él mismo o su padre); null si no hay. */
function elementOf(node: Node | null): Element | null {
  return node instanceof Element ? node : (node?.parentElement ?? null);
}

/**
 * Rango vivo de la selección si cae en el CUERPO: dentro del root y FUERA de una
 * sección generada (notas al pie/final, bibliografía, índice), para no marcar ni
 * anclar dentro de ellas. null en caso contrario.
 */
function rangoEnCuerpo(root: HTMLElement): Range | null {
  const sel = document.getSelection();
  const range = sel && sel.rangeCount > 0 ? sel.getRangeAt(0) : null;
  if (!range || !root.contains(range.commonAncestorContainer)) return null;
  return elementOf(range.commonAncestorContainer)?.closest('section') ? null : range;
}

/**
 * Marca la selección como entrada de índice. El término por defecto = el texto
 * seleccionado, editable en el prompt; «término:subtérmino» crea una subentrada.
 * El marcador conserva el formato interno y no cambia el aspecto del texto; su
 * número de aparición se asigna al generar el índice.
 */
async function markIndexEntry(): Promise<void> {
  const root = getEditorRoot();
  if (!root) return;
  const activo = document.activeElement;
  if (!activo || !root.contains(activo)) root.focus({ preventScroll: true });

  const live = rangoEnCuerpo(root);
  if (!live) return; // sin punto válido en el cuerpo: no-op

  // Clona el rango ANTES del prompt (modal): al cerrarlo se envuelve este rango,
  // pues la selección viva puede perderse al abrir el diálogo.
  const range = live.cloneRange();
  const raw = await preguntarEnEditor({
    mensaje: editorT()('dialog.index.entry'),
    valor: range.toString().trim(),
  });
  if (raw === null) return; // cancelado
  const parsed = parseIndexTerm(raw);
  if (!parsed) return; // sin término principal

  const span = document.createElement('span');
  span.setAttribute('data-index-entry', parsed.term); // setAttribute escapa el valor (anti-XSS)
  if (parsed.sub) span.setAttribute('data-index-sub', parsed.sub);
  try {
    range.surroundContents(span); // conserva el formato interno (rango colapsado → marcador vacío)
  } catch {
    // El rango cruza fronteras de elementos: se envuelve el contenido extraído.
    span.appendChild(range.extractContents());
    range.insertNode(span);
  }
  span.scrollIntoView({ block: 'nearest' });
  // ponytail: alta DOM directa fuera del undo nativo (como notas/leyendas/TDC);
  // historial propio con la migración a modelo Range.
  emitirInput(root);
}

/**
 * Coloca una sección de índice NUEVA: tras el bloque del caret (§ «donde el
 * cursor»), o al final antes de las notas al pie/final que cierran el folio.
 */
function colocarSeccionIndice(root: HTMLElement, section: HTMLElement): void {
  const live = rangoEnCuerpo(root);
  const bloque = live ? bloqueSuperior(root, live.commonAncestorContainer) : null;
  if (bloque) {
    bloque.insertAdjacentElement('afterend', section);
    return;
  }
  const notas = root.querySelector('section[data-footnotes], section[data-endnotes]');
  if (notas) root.insertBefore(section, notas);
  else root.appendChild(section);
}

/**
 * (Re)genera el índice: numera cada marcador en orden de documento (id idx-N,
 * ancla del enlace), construye la lista ordenada+agrupada (buildIndexHtml) y la
 * vuelca en <section data-index>. Si ya existe se reescribe EN SU SITIO (como el
 * TDC/bibliografía); si no, se coloca en el caret o al final. Camino manual: emite
 * `input` para el autosave.
 */
function regenerarIndice(root: HTMLElement, t: DocsT): void {
  const entries: IndexEntry[] = Array.from(
    root.querySelectorAll<HTMLElement>('span[data-index-entry]'),
  ).map((span, i) => {
    const ref = String(i + 1); // número de aparición en orden de documento
    span.id = indexAnchorId(ref); // ancla estable para el enlace del índice
    return {
      term: span.getAttribute('data-index-entry') ?? '',
      sub: span.getAttribute('data-index-sub') ?? '',
      ref,
    };
  });

  const cuerpo = buildIndexHtml(entries);
  // El índice se INSERTA como HTML en el documento: el texto va resuelto aquí, no
  // como JSX, porque dentro de una cadena no hay nada que evaluar.
  const inner =
    `<p><b>${t('ref.index')}</b></p>` +
    (cuerpo || `<p style="color:#64748b"><i>${t('ref.index.empty')}</i></p>`);

  let section = root.querySelector<HTMLElement>(INDEX_SELECTOR);
  if (section) {
    section.innerHTML = inner; // actualiza EN SU SITIO
  } else {
    section = document.createElement('section');
    section.setAttribute('data-index', '');
    section.innerHTML = inner;
    colocarSeccionIndice(root, section);
  }
  section.scrollIntoView({ block: 'nearest' });
  emitirInput(root);
}

/* ---------- pestaña ---------- */

export function ReferenciasTab({ ctx }: { ctx: EditorContext }): JSX.Element {
  const t = useT();
  const insertarEnlace = async (): Promise<void> => {
    // Mismo flujo que la pestaña Insertar (§6.3): prompt nativo; vacío o cancelar aborta.
    const url = (await preguntarEnEditor({ mensaje: editorT()('dialog.link.url') }))?.trim();
    if (url) ctx.exec('insertLink', url);
  };

  return (
    <>
      <RibbonGroup label={t('ref.toc')}>
        <RibbonButton commandId="insertToc" icon="toc" onAction={() => generarTdc(ctx)} />
        {/* ponytail: se reutiliza el icono 'history' (flecha circular) como
         * «actualizar»; trazado propio si A3 añade un 'refresh' a icons.tsx. */}
        <RibbonButton commandId="updateToc" icon="history" onAction={() => actualizarTdc(ctx)} />
      </RibbonGroup>
      <RibbonSeparator />
      <RibbonGroup label={t('group.index')}>
        <RibbonButton
          commandId="indexMark"
          icon="indexEntry"
          onAction={() => void markIndexEntry()}
        />
        <RibbonButton
          commandId="indexBuild"
          icon="indexAlpha"
          onAction={() => {
            const root = getEditorRoot();
            if (root) regenerarIndice(root, t);
          }}
        />
      </RibbonGroup>
      <RibbonSeparator />
      <RibbonGroup label={t('group.captions')}>
        <RibbonButton
          commandId="caption"
          icon="caption"
          onAction={() => void insertarLeyenda('tabla')}
        />
        <RibbonButton
          commandId="caption"
          icon="image"
          label={t('ref.captionFigure')}
          onAction={() => void insertarLeyenda('figura')}
        />
        <ReferenciaCruzada />
        <RibbonButton commandId="captionsUpdate" icon="history" onAction={actualizarLeyendas} />
      </RibbonGroup>
      <RibbonSeparator />
      <RibbonGroup label={t('group.notes')}>
        <RibbonButton commandId="footnote" icon="footnote" onAction={insertarNotaAlPie} />
        <RibbonButton commandId="endnote" icon="endnote" onAction={insertarNotaAlFinal} />
        <RibbonButton commandId="endnotesUpdate" icon="history" onAction={actualizarNotasAlFinal} />
      </RibbonGroup>
      <RibbonSeparator />
      <CitasBibliografiaGroup ctx={ctx} />
      <RibbonSeparator />
      <RibbonGroup label={t('group.links')}>
        <RibbonButton commandId="insertLink" icon="link" onAction={() => void insertarEnlace()} />
      </RibbonGroup>
      <RibbonSeparator />
      <RibbonGroup label={t('group.comment')}>
        <RibbonButton
          commandId="commentsPanel"
          icon="commentList"
          label={t('comments.add')}
          onAction={() => ctx.setUi({ panel: 'comentarios' })}
        />
      </RibbonGroup>
      <RibbonSeparator />
      <CorrespondenciaGroup ctx={ctx} />
    </>
  );
}
