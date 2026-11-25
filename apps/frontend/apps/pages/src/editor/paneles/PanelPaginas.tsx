/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Panel de Páginas del editor (PAGES-PLAN §14.1).
 * Gestión de páginas: crear, duplicar, reordenar, eliminar y editar ajustes SEO/slug.
 *
 * Estaba entero en castellano escrito a mano y sus acciones solo aparecían al pasar
 * el ratón: con teclado o en una tableta no había forma de subir, duplicar ni borrar
 * una página, y borrar usaba el `confirm()` del navegador. Ahora habla el idioma de
 * la persona, las acciones se ven también con el foco y en pantallas táctiles, borrar
 * pide confirmación con el diálogo de la suite, y el título y la descripción dicen
 * cuánto les queda antes de que un buscador los recorte.
 */

import { useState } from 'preact/hooks';
import type { JSX } from 'preact';
import type { Id, Pagina, Sitio } from '@sytue/pages-render';
import { Badge, Checkbox, ConfirmDialog, Icon } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import { t as traducir, type ClaveMensaje } from '../../i18n/index.js';

/** Límites a partir de los que un buscador recorta (los mismos que la revisión SEO). */
export const MAX_TITULO = 60;
export const MAX_DESCRIPCION = 160;

interface PanelPaginasProps {
  ir: Sitio;
  paginaActivaId: Id;
  /** Idioma de la interfaz (no el del contenido del sitio). */
  locale: Locale;
  onCambiarPagina: (id: Id) => void;
  onCrearPagina: (titulo: string) => void;
  onDuplicarPagina: (id: Id) => void;
  onEliminarPagina: (id: Id) => void;
  onReordenarPaginas: (ids: Id[]) => void;
  onActualizarAjustesPagina: (id: Id, ajustes: Partial<Pagina>) => void;
}

/** Contador «32/60» que se pone en aviso al pasarse del límite. */
function Contador({
  n,
  max,
  t,
}: {
  n: number;
  max: number;
  t: (k: ClaveMensaje, p?: Record<string, string | number>) => string;
}) {
  return (
    <span
      class={`self-end text-[10px] tabular-nums ${n > max ? 'text-warning-text font-semibold' : 'text-text-muted'}`}
      aria-live="polite"
    >
      {t('editor.pages.counter', { n, max })}
    </span>
  );
}

export function PanelPaginas({
  ir,
  paginaActivaId,
  locale,
  onCambiarPagina,
  onCrearPagina,
  onDuplicarPagina,
  onEliminarPagina,
  onReordenarPaginas,
  onActualizarAjustesPagina,
}: PanelPaginasProps): JSX.Element {
  const t = (k: ClaveMensaje, p?: Record<string, string | number>) => traducir(locale, k, p);
  const [nuevaPaginaTitulo, setNuevaPaginaTitulo] = useState<string>('');
  const [mostrarCrear, setMostrarCrear] = useState<boolean>(false);
  const [aEliminar, setAEliminar] = useState<{ id: Id; titulo: string } | null>(null);

  const defLang = ir.languages.default;
  const paginaSeleccionada = ir.pages.find((p) => p.id === paginaActivaId) || ir.pages[0];

  const mover = (idx: number, delta: -1 | 1) => {
    const destino = idx + delta;
    // La portada es la primera y no se mueve: subir la segunda no puede quitarle el sitio.
    if (destino < 1 || destino >= ir.pages.length) return;
    const ids = ir.pages.map((p) => p.id);
    const temp = ids[idx]!;
    ids[idx] = ids[destino]!;
    ids[destino] = temp;
    onReordenarPaginas(ids);
  };

  const handleCrear = (e: Event) => {
    e.preventDefault();
    if (!nuevaPaginaTitulo.trim()) return;
    onCrearPagina(nuevaPaginaTitulo.trim());
    setNuevaPaginaTitulo('');
    setMostrarCrear(false);
  };

  // Botón de acción de cada fila: visible al pasar el ratón, con el foco dentro de la
  // fila o en pantallas táctiles, que no tienen «pasar por encima».
  const accion = 'sytue-focus-ring hover:text-primary-text rounded p-0.5 focus-visible:opacity-100';

  const tituloActual = paginaSeleccionada?.title[defLang] ?? '';
  const descripcion = paginaSeleccionada?.description[defLang] ?? '';

  return (
    <div class="flex h-full flex-col overflow-y-auto p-3">
      <div class="mb-3 flex items-center justify-between">
        <h3 class="text-text-muted text-[11px] font-semibold tracking-wider uppercase">
          {t('editor.pages.title', { count: ir.pages.length })}
        </h3>
        <button
          type="button"
          aria-expanded={mostrarCrear}
          onClick={() => setMostrarCrear(!mostrarCrear)}
          class="sytue-focus-ring bg-primary flex items-center gap-1 rounded px-2 py-1 text-xs font-medium text-[var(--sytue-on-primary)] hover:bg-[var(--sytue-primary-hover)]"
        >
          <Icon name="plus" size={12} aria-hidden="true" />
          <span>{t('editor.pages.add')}</span>
        </button>
      </div>

      {/* Formulario para crear página */}
      {mostrarCrear && (
        <form onSubmit={handleCrear} class="border-border bg-surface mb-3 rounded-lg border p-2.5">
          <label class="text-text mb-2 flex flex-col gap-1 text-xs font-semibold">
            <span>{t('editor.pages.new_title')}</span>
            <input
              ref={(el) => el?.focus()}
              type="text"
              value={nuevaPaginaTitulo}
              onInput={(e) => setNuevaPaginaTitulo((e.target as HTMLInputElement).value)}
              onKeyDown={(e) => {
                if (e.key === 'Escape') setMostrarCrear(false);
              }}
              placeholder={t('editor.pages.new_placeholder')}
              class="border-border bg-bg text-text h-7 w-full rounded border px-2 text-xs font-normal"
            />
          </label>
          <div class="flex justify-end gap-1.5">
            <button
              type="button"
              onClick={() => setMostrarCrear(false)}
              class="sytue-focus-ring text-text-muted hover:text-text rounded px-2 py-1 text-xs"
            >
              {t('editor.pages.cancel')}
            </button>
            <button
              type="submit"
              class="sytue-focus-ring bg-primary rounded px-2.5 py-1 text-xs font-medium text-[var(--sytue-on-primary)] hover:bg-[var(--sytue-primary-hover)]"
            >
              {t('editor.pages.create')}
            </button>
          </div>
        </form>
      )}

      {/* Lista de páginas */}
      <div class="mb-4 flex flex-col gap-1">
        {ir.pages.map((p, idx) => {
          const esActiva = p.id === paginaActivaId;
          const esPortada = idx === 0;
          const titulo = p.title[defLang] || p.id;
          const slug = esPortada ? '/' : `/${p.slug[defLang] || p.id}`;

          return (
            <div
              key={p.id}
              class={`group flex items-center justify-between rounded-lg border p-2 text-xs transition-colors ${
                esActiva
                  ? 'border-primary/40 bg-selected text-primary-text font-semibold'
                  : 'border-border hover:bg-bg-subtle text-text'
              }`}
            >
              {/* El nombre es el botón: así Tab llega a la página y luego a sus acciones,
                  en vez de una fila entera que no deja enfocar nada dentro. */}
              <button
                type="button"
                aria-current={esActiva ? 'page' : undefined}
                onClick={() => onCambiarPagina(p.id)}
                class="sytue-focus-ring flex min-w-0 flex-1 flex-col overflow-hidden rounded pr-2 text-left"
              >
                <span class="flex items-center gap-1.5">
                  <span class="truncate">{titulo}</span>
                  {esPortada && (
                    <span class="py-0.2 bg-selected text-primary-text rounded px-1 text-[9px] font-bold">
                      {t('editor.pages.home')}
                    </span>
                  )}
                  {p.noindex && <Badge tone="warning">noindex</Badge>}
                </span>
                <span class="text-text-muted font-mono text-[10px]">{slug}</span>
              </button>

              {/* Botones de acción rápida */}
              <div class="flex items-center gap-1 opacity-0 group-focus-within:opacity-100 group-hover:opacity-100 pointer-coarse:opacity-100">
                {idx > 1 && (
                  <button
                    type="button"
                    aria-label={t('editor.pages.move_up', { titulo })}
                    onClick={() => mover(idx, -1)}
                    class={accion}
                  >
                    <Icon name="chevronUp" size={12} aria-hidden="true" />
                  </button>
                )}
                {idx > 0 && idx < ir.pages.length - 1 && (
                  <button
                    type="button"
                    aria-label={t('editor.pages.move_down', { titulo })}
                    onClick={() => mover(idx, 1)}
                    class={accion}
                  >
                    <Icon name="chevronDown" size={12} aria-hidden="true" />
                  </button>
                )}
                <button
                  type="button"
                  aria-label={t('editor.pages.duplicate', { titulo })}
                  onClick={() => onDuplicarPagina(p.id)}
                  class={accion}
                >
                  <Icon name="copies" size={12} aria-hidden="true" />
                </button>
                {!esPortada && (
                  <button
                    type="button"
                    aria-label={t('editor.pages.delete', { titulo })}
                    onClick={() => setAEliminar({ id: p.id, titulo })}
                    class="sytue-focus-ring hover:text-danger-text rounded p-0.5"
                  >
                    <Icon name="trash" size={12} aria-hidden="true" />
                  </button>
                )}
              </div>
            </div>
          );
        })}
      </div>

      {/* Edición de ajustes de la página seleccionada */}
      {paginaSeleccionada && (
        <div class="border-border bg-surface mt-2 rounded-lg border p-3">
          <h4 class="text-text mb-2 text-xs font-semibold">
            {t('editor.pages.settings', {
              titulo: paginaSeleccionada.title[defLang] || paginaSeleccionada.id,
            })}
          </h4>

          {/* Título */}
          <label class="text-text-muted mb-2 flex flex-col gap-1 text-[11px]">
            <span>{t('editor.pages.field_title', { idioma: defLang })}</span>
            <input
              type="text"
              value={tituloActual}
              onInput={(e) => {
                const val = (e.target as HTMLInputElement).value;
                onActualizarAjustesPagina(paginaSeleccionada.id, {
                  title: { ...paginaSeleccionada.title, [defLang]: val },
                });
              }}
              class="border-border bg-bg text-text h-7 w-full rounded border px-2 text-xs"
            />
            <Contador n={tituloActual.length} max={MAX_TITULO} t={t} />
          </label>

          {/* Slug (solo si no es portada) */}
          {paginaSeleccionada.id !== ir.pages[0]?.id && (
            <label class="text-text-muted mb-2 flex flex-col gap-1 text-[11px]">
              <span>{t('editor.pages.field_slug', { idioma: defLang })}</span>
              <input
                type="text"
                value={paginaSeleccionada.slug[defLang] || ''}
                onInput={(e) => {
                  const val = (e.target as HTMLInputElement).value
                    .toLowerCase()
                    .replace(/[^a-z0-9-]+/g, '');
                  onActualizarAjustesPagina(paginaSeleccionada.id, {
                    slug: { ...paginaSeleccionada.slug, [defLang]: val },
                  });
                }}
                class="border-border bg-bg text-text h-7 w-full rounded border px-2 font-mono text-xs"
              />
            </label>
          )}

          {/* Meta Descripción */}
          <label class="text-text-muted mb-2 flex flex-col gap-1 text-[11px]">
            <span>{t('editor.pages.field_description')}</span>
            <textarea
              rows={2}
              value={descripcion}
              onInput={(e) => {
                const val = (e.target as HTMLTextAreaElement).value;
                onActualizarAjustesPagina(paginaSeleccionada.id, {
                  description: { ...paginaSeleccionada.description, [defLang]: val },
                });
              }}
              placeholder={t('editor.pages.description_placeholder')}
              class="border-border bg-bg text-text w-full rounded border p-1.5 text-xs"
            />
            <Contador n={descripcion.length} max={MAX_DESCRIPCION} t={t} />
          </label>

          {/* Interruptores */}
          <div class="flex flex-col gap-1.5 pt-1">
            <Checkbox
              label={t('editor.pages.show_header')}
              checked={paginaSeleccionada.show_header}
              onChange={(e) =>
                onActualizarAjustesPagina(paginaSeleccionada.id, {
                  show_header: (e.target as HTMLInputElement).checked,
                })
              }
            />
            <Checkbox
              label={t('editor.pages.show_footer')}
              checked={paginaSeleccionada.show_footer}
              onChange={(e) =>
                onActualizarAjustesPagina(paginaSeleccionada.id, {
                  show_footer: (e.target as HTMLInputElement).checked,
                })
              }
            />
            <Checkbox
              label={t('editor.pages.noindex')}
              checked={paginaSeleccionada.noindex === true}
              onChange={(e) => {
                const chk = (e.target as HTMLInputElement).checked;
                if (chk) {
                  onActualizarAjustesPagina(paginaSeleccionada.id, { noindex: true });
                } else {
                  const copia = { ...paginaSeleccionada };
                  delete copia.noindex;
                  onActualizarAjustesPagina(paginaSeleccionada.id, copia);
                }
              }}
            />
          </div>
        </div>
      )}

      <ConfirmDialog
        open={aEliminar !== null}
        onClose={() => setAEliminar(null)}
        onConfirm={() => {
          if (aEliminar) onEliminarPagina(aEliminar.id);
          setAEliminar(null);
        }}
        title={t('editor.pages.delete_title')}
        confirmLabel={t('editor.pages.delete_confirm_button')}
        tone="danger"
      >
        <p class="text-text text-sm">
          {t('editor.pages.delete_confirm', { titulo: aEliminar?.titulo ?? '' })}
        </p>
      </ConfirmDialog>
    </div>
  );
}
