/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Panel de Páginas del editor (PAGES-PLAN §14.1).
 * Gestión de páginas: crear, duplicar, reordenar, eliminar y editar ajustes SEO/slug.
 */

import { useState } from 'preact/hooks';
import type { JSX } from 'preact';
import type { Id, Pagina, Sitio } from '@sytue/pages-render';
import { Checkbox } from '@sytue/ui';
import { IconoPlus, IconoTrash } from '../iconos';

interface PanelPaginasProps {
  ir: Sitio;
  paginaActivaId: Id;
  onCambiarPagina: (id: Id) => void;
  onCrearPagina: (titulo: string) => void;
  onDuplicarPagina: (id: Id) => void;
  onEliminarPagina: (id: Id) => void;
  onReordenarPaginas: (ids: Id[]) => void;
  onActualizarAjustesPagina: (id: Id, ajustes: Partial<Pagina>) => void;
}

export function PanelPaginas({
  ir,
  paginaActivaId,
  onCambiarPagina,
  onCrearPagina,
  onDuplicarPagina,
  onEliminarPagina,
  onReordenarPaginas,
  onActualizarAjustesPagina,
}: PanelPaginasProps): JSX.Element {
  const [nuevaPaginaTitulo, setNuevaPaginaTitulo] = useState<string>('');
  const [mostrarCrear, setMostrarCrear] = useState<boolean>(false);
  const [paginaEditandoAjustes, setPaginaEditandoAjustes] = useState<Id | null>(null);

  const defLang = ir.languages.default;
  const paginaSeleccionada = ir.pages.find((p) => p.id === paginaActivaId) || ir.pages[0];

  const moverArriba = (idx: number) => {
    if (idx <= 0) return;
    const ids = ir.pages.map((p) => p.id);
    const temp = ids[idx]!;
    ids[idx] = ids[idx - 1]!;
    ids[idx - 1] = temp;
    onReordenarPaginas(ids);
  };

  const moverAbajo = (idx: number) => {
    if (idx >= ir.pages.length - 1) return;
    const ids = ir.pages.map((p) => p.id);
    const temp = ids[idx]!;
    ids[idx] = ids[idx + 1]!;
    ids[idx + 1] = temp;
    onReordenarPaginas(ids);
  };

  const handleCrear = (e: Event) => {
    e.preventDefault();
    if (!nuevaPaginaTitulo.trim()) return;
    onCrearPagina(nuevaPaginaTitulo.trim());
    setNuevaPaginaTitulo('');
    setMostrarCrear(false);
  };

  return (
    <div class="flex h-full flex-col overflow-y-auto p-3">
      <div class="mb-3 flex items-center justify-between">
        <h3 class="text-text-muted text-[11px] font-semibold tracking-wider uppercase">
          Páginas del sitio ({ir.pages.length})
        </h3>
        <button
          type="button"
          onClick={() => setMostrarCrear(!mostrarCrear)}
          class="bg-primary flex items-center gap-1 rounded px-2 py-1 text-xs font-medium text-[var(--sytue-on-primary)] hover:bg-[var(--sytue-primary-hover)]"
        >
          <IconoPlus size={12} />
          <span>Añadir</span>
        </button>
      </div>

      {/* Formulario para crear página */}
      {mostrarCrear && (
        <form onSubmit={handleCrear} class="border-border bg-surface mb-3 rounded-lg border p-2.5">
          <label class="text-text mb-2 flex flex-col gap-1 text-xs font-semibold">
            <span>Título de la nueva página</span>
            <input
              ref={(el) => el?.focus()}
              type="text"
              value={nuevaPaginaTitulo}
              onInput={(e) => setNuevaPaginaTitulo((e.target as HTMLInputElement).value)}
              placeholder="Ej. Sobre Nosotros"
              class="border-border bg-bg text-text h-7 w-full rounded border px-2 text-xs font-normal"
            />
          </label>
          <div class="flex justify-end gap-1.5">
            <button
              type="button"
              onClick={() => setMostrarCrear(false)}
              class="text-text-muted hover:text-text px-2 py-1 text-xs"
            >
              Cancelar
            </button>
            <button
              type="submit"
              class="bg-primary rounded px-2.5 py-1 text-xs font-medium text-[var(--sytue-on-primary)] hover:bg-[var(--sytue-primary-hover)]"
            >
              Crear página
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
              role="button"
              tabIndex={0}
              onClick={() => onCambiarPagina(p.id)}
              onKeyDown={(e) => {
                if (e.key === 'Enter' || e.key === ' ') {
                  e.preventDefault();
                  onCambiarPagina(p.id);
                }
              }}
              class={`group flex cursor-pointer items-center justify-between rounded-lg border p-2 text-xs transition-colors ${
                esActiva
                  ? 'border-primary/40 bg-selected text-primary-text font-semibold'
                  : 'border-border hover:bg-bg-subtle text-text'
              }`}
            >
              <div class="flex flex-col overflow-hidden pr-2">
                <div class="flex items-center gap-1.5">
                  <span class="truncate">{titulo}</span>
                  {esPortada && (
                    <span class="py-0.2 bg-selected text-primary-text rounded px-1 text-[9px] font-bold">
                      Inicio
                    </span>
                  )}
                  {p.noindex && (
                    <span class="py-0.2 rounded bg-amber-500/20 px-1 text-[9px] text-amber-600">
                      noindex
                    </span>
                  )}
                </div>
                <span class="text-text-muted font-mono text-[10px]">{slug}</span>
              </div>

              {/* Botones de acción rápida */}
              <div class="flex items-center gap-1 opacity-0 group-hover:opacity-100">
                {idx > 1 && (
                  <button
                    type="button"
                    aria-label="Subir"
                    onClick={(e) => {
                      e.stopPropagation();
                      moverArriba(idx);
                    }}
                    class="hover:text-primary-text p-0.5"
                  >
                    ↑
                  </button>
                )}
                {idx > 0 && idx < ir.pages.length - 1 && (
                  <button
                    type="button"
                    aria-label="Bajar"
                    onClick={(e) => {
                      e.stopPropagation();
                      moverAbajo(idx);
                    }}
                    class="hover:text-primary-text p-0.5"
                  >
                    ↓
                  </button>
                )}
                <button
                  type="button"
                  aria-label="Ajustes"
                  onClick={(e) => {
                    e.stopPropagation();
                    setPaginaEditandoAjustes(paginaEditandoAjustes === p.id ? null : p.id);
                  }}
                  class="hover:text-text p-0.5 font-bold"
                >
                  ⚙
                </button>
                <button
                  type="button"
                  aria-label="Duplicar"
                  onClick={(e) => {
                    e.stopPropagation();
                    onDuplicarPagina(p.id);
                  }}
                  class="hover:text-primary-text p-0.5 text-[10px]"
                >
                  📋
                </button>
                {!esPortada && (
                  <button
                    type="button"
                    aria-label="Eliminar"
                    onClick={(e) => {
                      e.stopPropagation();
                      if (confirm(`¿Eliminar la página "${titulo}"?`)) {
                        onEliminarPagina(p.id);
                      }
                    }}
                    class="p-0.5 hover:text-red-600"
                  >
                    <IconoTrash size={12} />
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
            Ajustes: {paginaSeleccionada.title[defLang] || paginaSeleccionada.id}
          </h4>

          {/* Título */}
          <label class="text-text-muted mb-2 flex flex-col gap-1 text-[11px]">
            <span>Título ({defLang})</span>
            <input
              type="text"
              value={paginaSeleccionada.title[defLang] || ''}
              onInput={(e) => {
                const val = (e.target as HTMLInputElement).value;
                onActualizarAjustesPagina(paginaSeleccionada.id, {
                  title: { ...paginaSeleccionada.title, [defLang]: val },
                });
              }}
              class="border-border bg-bg text-text h-7 w-full rounded border px-2 text-xs"
            />
          </label>

          {/* Slug (solo si no es portada) */}
          {paginaSeleccionada.id !== ir.pages[0]?.id && (
            <label class="text-text-muted mb-2 flex flex-col gap-1 text-[11px]">
              <span>Slug URL ({defLang})</span>
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
            <span>Meta descripción (SEO)</span>
            <textarea
              rows={2}
              value={paginaSeleccionada.description[defLang] || ''}
              onInput={(e) => {
                const val = (e.target as HTMLTextAreaElement).value;
                onActualizarAjustesPagina(paginaSeleccionada.id, {
                  description: { ...paginaSeleccionada.description, [defLang]: val },
                });
              }}
              placeholder="Resumen SEO"
              class="border-border bg-bg text-text w-full rounded border p-1.5 text-xs"
            />
          </label>

          {/* Interruptores */}
          <div class="flex flex-col gap-1.5 pt-1">
            <label class="text-text flex cursor-pointer items-center gap-2 text-xs">
              <Checkbox
                checked={paginaSeleccionada.show_header}
                onChange={(e) =>
                  onActualizarAjustesPagina(paginaSeleccionada.id, {
                    show_header: (e.target as HTMLInputElement).checked,
                  })
                }
              />
              <span>Mostrar cabecera común</span>
            </label>

            <label class="text-text flex cursor-pointer items-center gap-2 text-xs">
              <Checkbox
                checked={paginaSeleccionada.show_footer}
                onChange={(e) =>
                  onActualizarAjustesPagina(paginaSeleccionada.id, {
                    show_footer: (e.target as HTMLInputElement).checked,
                  })
                }
              />
              <span>Mostrar pie de página común</span>
            </label>

            <label class="text-text flex cursor-pointer items-center gap-2 text-xs">
              <Checkbox
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
              <span>Ocultar de buscadores (noindex)</span>
            </label>
          </div>
        </div>
      )}
    </div>
  );
}
