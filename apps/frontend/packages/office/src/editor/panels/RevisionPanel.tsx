/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Panel «Revisión» (BLUEPRINT.md §6.4, control de cambios): lista los cambios
 * rastreados del folio (<ins>/<del> data-track) en ORDEN DE DOCUMENTO, con QUIÉN
 * (autor), CUÁNDO (sello `data-ts` de la marca), tipo y un extracto. Por cambio:
 * ACEPTAR (inserción → conserva el texto; borrado → lo aplica) o RECHAZAR (inserción
 * → la descarta; borrado → restaura el texto); y ACEPTAR/RECHAZAR TODOS. Navegación
 * anterior/siguiente que resalta y hace scroll a la marca. Las marcas VIVEN en el
 * HTML del folio (que ES el modelo): la lista se deriva de `getEditorRoot().innerHTML`
 * con el helper PURO listTrackedChanges y se refresca al teclear (input) y al recargar
 * contenido (contentEpoch); cada acción reescribe el folio (resolveTrackedChange*,
 * exec) y emite el autosave.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { Button, IconButton, useConfirm } from '@sytue/ui';
import { formatDateTime, formatTimeAgo } from '@sytue/i18n';
import { getEditorRoot, resolveTrackedChangeAt, resolveTrackedChanges } from '../canvas/exec';
import { EditorIcon } from '../icons';
import { listTrackedChanges, type TrackedChange } from '../trackchanges';
import type { EditorContext } from '../types';
import { setEditorHighlight } from './SearchPanel';

/** Resaltado de la marca en foco durante la navegación (CSS Custom Highlight API:
 *  no muta el HTML del folio, que ES el modelo). */
const HL_CHANGE = 'sytue-track-review';

/**
 * Marcas rastreadas del folio VIVO con el MISMO criterio y orden que el scanner
 * puro de listTrackedChanges: solo las de texto plano (`[^<]*`), descartando las que
 * anidan un elemento —Intro dentro de un `<ins>`, formato inline sobre un tramo
 * rastreado, comentar texto insertado—, que aquel tampoco cuenta. Así el ordinal del
 * panel, el índice del DOM y el que reescribe el HTML son el MISMO y navegar,
 * fechar y resolver no se desincronizan (ver TrackedChange).
 */
const trackedMarks = (root: HTMLElement): HTMLElement[] =>
  [...root.querySelectorAll<HTMLElement>('ins[data-track],del[data-track]')].filter(
    (mark) => mark.firstElementChild === null,
  );

/**
 * Sello de tiempo (ms) de una marca a partir de su `data-ts` en bruto, o null si
 * falta o no describe una fecha real. El HTML del folio es una FRONTERA DE CONFIANZA
 * —llega de un documento compartido— y un `data-ts` inventado (vacío, texto, 1e20)
 * haría estallar el render con un Date inválido: sin fecha se pinta el cambio igual,
 * solo sin el «cuándo». Exportada para que el self-test la muerda sin DOM.
 */
export const trackTime = (raw: string | undefined): number | null => {
  const ts = Number(raw);
  return ts > 0 && !Number.isNaN(new Date(ts).getTime()) ? ts : null;
};

/** Un cambio de la lista pura + su CUÁNDO. El sello vive en el HTML (`data-ts`, el
 *  inicio de la racha del autor tras fundir marcas contiguas) pero listTrackedChanges
 *  no lo devuelve, así que se lee del MISMO nodo que ya sirve para navegar. */
type ReviewRow = TrackedChange & { ts: number | null };

/** Marcas rastreadas del folio VIVO en orden de documento; [] si aún no hay root. */
const readChanges = (): ReviewRow[] => {
  const root = getEditorRoot();
  if (root === null) return [];
  const marks = trackedMarks(root);
  return listTrackedChanges(root.innerHTML).map((change) => ({
    ...change,
    ts: trackTime(marks[change.id]?.dataset.ts),
  }));
};

/** Aceptar/rechazar un cambio rastreado. Era una copia LITERAL del botón de acción de
 *  CommentsPanel; la geometría la pone ahora `Button size="xs"` y aquí solo queda el color
 *  atenuado, que el `ghost` del DS no trae. */
const ACTION_BTN = 'text-text-muted hover:text-text';

export function RevisionPanel({ ctx }: { ctx: EditorContext }): JSX.Element {
  // Aceptar o rechazar TODOS los cambios es masivo y no entra en el deshacer: se
  // confirma con el diálogo de la casa, no con el del navegador.
  const { confirmar, dialogo: dialogoConfirmar } = useConfirm();
  const t = ctx.t;
  const [changes, setChanges] = useState<ReviewRow[]>([]);
  /** Índice del cambio en foco (= su id); -1 si ninguno. */
  const [activeId, setActiveId] = useState(-1);

  // Deriva la lista del folio vivo: al montar, al teclear (input, con rebote para
  // no re-escanear en cada tecla) y al recargar contenido (contentEpoch: carga,
  // adopción remota o restauración de versión, que no emiten `input`).
  useEffect(() => {
    const root = getEditorRoot();
    const refresh = (): void => setChanges(readChanges());
    refresh();
    if (root === null) return;
    let timer = 0;
    const onInput = (): void => {
      window.clearTimeout(timer);
      timer = window.setTimeout(refresh, 150);
    };
    root.addEventListener('input', onInput);
    return () => {
      window.clearTimeout(timer);
      root.removeEventListener('input', onInput);
      setEditorHighlight(HL_CHANGE, []); // no dejar resaltado al cerrar/recargar
    };
  }, [ctx.contentEpoch]);

  const total = changes.length;
  /** Foco válido solo si sigue dentro de la lista (una acción pudo encogerla). */
  const activeIndex = activeId >= 0 && activeId < total ? activeId : -1;

  /** Resalta y hace scroll a la marca `index` del folio (misma numeración que la
   *  lista pura, ver trackedMarks). */
  const focusChange = (index: number): void => {
    setActiveId(index);
    const root = getEditorRoot();
    const el = (root === null ? [] : trackedMarks(root))[index] ?? null;
    if (el === null) {
      setEditorHighlight(HL_CHANGE, []);
      return;
    }
    const range = document.createRange();
    range.selectNodeContents(el);
    setEditorHighlight(HL_CHANGE, [range]);
    el.scrollIntoView({
      block: 'center',
      behavior: matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth',
    });
  };

  /** Cambio anterior/siguiente en ciclo; arranca por los extremos si no hay foco. */
  const go = (delta: 1 | -1): void => {
    if (total === 0) return;
    const from = activeIndex === -1 ? (delta === 1 ? -1 : 0) : activeIndex;
    focusChange((from + delta + total) % total);
  };

  /** Acepta/rechaza UN cambio; reescribe el folio (autosave) y encadena al que
   *  ocupa ahora ese hueco (la lista se corrió), o limpia si no quedan. */
  const resolveOne = (index: number, accept: boolean): void => {
    resolveTrackedChangeAt(index, accept);
    const next = readChanges();
    setChanges(next);
    if (next.length === 0) {
      setActiveId(-1);
      setEditorHighlight(HL_CHANGE, []);
    } else {
      focusChange(Math.min(index, next.length - 1));
    }
  };

  /** Acepta/rechaza TODOS. Pide confirmación: es masivo y no entra en el deshacer
   *  nativo (swap de innerHTML), igual criterio que el botón del ribbon. */
  const resolveAll = async (accept: boolean): Promise<void> => {
    const ok = await confirmar({
      mensaje: t(accept ? 'review.acceptAll.confirm' : 'review.rejectAll.confirm'),
      confirmLabel: t(accept ? 'review.acceptAll' : 'review.rejectAll'),
      ...(accept ? {} : { tone: 'danger' as const }),
    });
    if (!ok) return;
    resolveTrackedChanges(accept);
    setChanges(readChanges());
    setActiveId(-1);
    setEditorHighlight(HL_CHANGE, []);
  };

  return (
    <div class="flex min-h-full flex-col gap-2 p-3">
      {/* Violeta #E9D5FF de HIGHLIGHT_PALETTE y negro #0A0A0A de CONTENT_PALETTE (§5):
          colores de CONTENIDO sobre el papel, no tokens de UI. */}
      <style>{`::highlight(${HL_CHANGE}) { background-color: #e9d5ff; color: #0a0a0a; }`}</style>

      {/* Navegación anterior/siguiente + contador «n de m». */}
      <div class="flex items-center gap-1">
        <IconButton
          size="md"
          label={t('revision.previous')}
          disabled={total === 0}
          onClick={() => go(-1)}
        >
          <EditorIcon name="chevronLeft" size="md" />
        </IconButton>
        <IconButton
          size="md"
          label={t('revision.next')}
          disabled={total === 0}
          onClick={() => go(1)}
        >
          <EditorIcon name="chevronRight" size="md" />
        </IconButton>
        <span class="text-text-muted ml-1 text-xs">
          {total === 0
            ? t('revision.none')
            : t('revision.position', {
                index: activeIndex === -1 ? t('revision.emptyPosition') : activeIndex + 1,
                total,
              })}
        </span>
      </div>

      {total === 0 ? (
        <p class="text-text-muted text-sm">{t('revision.empty')}</p>
      ) : (
        <ul class="flex flex-col gap-2" aria-label={t('revision.list')}>
          {changes.map((change) => (
            <li key={change.id}>
              <article
                class={`rounded-md border p-2 ${
                  activeIndex === change.id ? 'border-primary bg-selected' : 'border-border'
                }`}
              >
                <button
                  type="button"
                  class={'block w-full rounded-sm text-left ' + 'sytue-focus-ring'}
                  title={t('revision.goTo')}
                  onClick={() => focusChange(change.id)}
                >
                  <div class="flex items-baseline gap-1.5">
                    <EditorIcon name={change.type === 'insert' ? 'plus' : 'minus'} size="sm" />
                    <span class="text-text text-xs font-semibold">
                      {t(change.type === 'insert' ? 'revision.insertion' : 'revision.deletion')}
                    </span>
                    <span class="sytue-meta-dense truncate">· {change.author}</span>
                    {/* CUÁNDO: antigüedad legible («hace 5 min», «ayer»), que es lo que
                        el revisor lee de un vistazo; la fecha exacta va en el title y en
                        dateTime (legible por máquina). Ambas de CLDR vía ctx.locale, sin
                        clave de catálogo. Marca sin sello (documento antiguo): sin fecha. */}
                    {change.ts !== null && (
                      <time
                        class="sytue-meta-dense ml-auto shrink-0 pl-1.5"
                        dateTime={new Date(change.ts).toISOString()}
                        title={formatDateTime(ctx.locale, change.ts)}
                      >
                        {formatTimeAgo(ctx.locale, change.ts)}
                      </time>
                    )}
                  </div>
                  <p
                    class={`text-text-muted mt-1 text-xs break-words ${
                      change.type === 'delete' ? 'line-through' : ''
                    }`}
                  >
                    «{change.text}»
                  </p>
                </button>

                <div class="mt-1.5 flex items-center gap-1">
                  <Button
                    size="xs"
                    variant="ghost"
                    class={ACTION_BTN}
                    title={t('revision.accept.title')}
                    onClick={() => resolveOne(change.id, true)}
                  >
                    <EditorIcon name="check" size="sm" />
                    {t('revision.accept')}
                  </Button>
                  <Button
                    size="xs"
                    variant="ghost"
                    class={ACTION_BTN}
                    title={t('revision.reject.title')}
                    onClick={() => resolveOne(change.id, false)}
                  >
                    <EditorIcon name="close" size="sm" />
                    {t('revision.reject')}
                  </Button>
                </div>
              </article>
            </li>
          ))}
        </ul>
      )}

      <div class="border-border mt-auto flex gap-1.5 border-t pt-2">
        <Button
          type="button"
          size="sm"
          variant="ghost"
          disabled={total === 0}
          onClick={() => void resolveAll(true)}
        >
          {t('revision.acceptAll')}
        </Button>
        <Button
          type="button"
          size="sm"
          variant="ghost"
          disabled={total === 0}
          onClick={() => void resolveAll(false)}
        >
          {t('revision.rejectAll')}
        </Button>
      </div>
      {dialogoConfirmar}
    </div>
  );
}
