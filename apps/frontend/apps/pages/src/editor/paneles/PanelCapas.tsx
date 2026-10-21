/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Panel de Capas del editor (PAGES-PLAN §14.1).
 * Árbol completo con arrastre/reordenación, renombrar (F2), ocultar, bloquear y navegación accesible.
 */

import { useState } from 'preact/hooks';
import type { JSX } from 'preact';
import type { Id, Nodo, Seccion, Sitio } from '@sytue/pages-render';
import type { PuntoDeCorte } from '../modelo/tipos';
import {
  IconoBloqueado,
  IconoChevronDown,
  IconoChevronRight,
  IconoDesbloqueado,
  IconoOculto,
  IconoTrash,
  IconoVisible,
} from '../iconos';

interface PanelCapasProps {
  ir: Sitio;
  paginaActivaId: Id;
  nodoSeleccionadoId: Id | null;
  puntoDeCorte: PuntoDeCorte;
  onSeleccionarNodo: (id: Id) => void;
  onRenombrarNodo: (id: Id, nuevoNombre: string) => void;
  onBloquearNodo: (id: Id) => void;
  onOcultarNodo: (id: Id) => void;
  onEliminarNodo: (id: Id) => void;
  onReordenarHermanos: (id: Id, direccion: 'arriba' | 'abajo') => void;
}

export function PanelCapas({
  ir,
  paginaActivaId,
  nodoSeleccionadoId,
  puntoDeCorte,
  onSeleccionarNodo,
  onRenombrarNodo,
  onBloquearNodo,
  onOcultarNodo,
  onEliminarNodo,
  onReordenarHermanos,
}: PanelCapasProps): JSX.Element {
  const [nodoEnEdicion, setNodoEnEdicion] = useState<Id | null>(null);
  const [nombreEdicion, setNombreEdicion] = useState<string>('');
  const [expandidos, setExpandidos] = useState<Record<Id, boolean>>({});

  const paginaActiva = ir.pages.find((p) => p.id === paginaActivaId) || ir.pages[0];

  const alternarExpansion = (id: Id) => {
    setExpandidos((prev) => ({ ...prev, [id]: prev[id] === undefined ? false : !prev[id] }));
  };

  const iniciarRenombrar = (id: Id, nombreActual: string) => {
    setNodoEnEdicion(id);
    setNombreEdicion(nombreActual);
  };

  const confirmarRenombrar = (id: Id) => {
    if (nombreEdicion.trim()) {
      onRenombrarNodo(id, nombreEdicion);
    }
    setNodoEnEdicion(null);
  };

  const renderizarNodoArbol = (
    nodo: Nodo | Seccion,
    nivel: number,
    esPrimero: boolean,
    esUltimo: boolean,
  ): JSX.Element => {
    const tieneHijos =
      'children' in nodo && Array.isArray(nodo.children) && nodo.children.length > 0;
    const estaExpandido = expandidos[nodo.id] ?? true;
    const esSeleccionado = nodo.id === nodoSeleccionadoId;
    const estaBloqueado = nodo.locked === true;
    const estaOculto =
      puntoDeCorte === 'desktop'
        ? nodo.hidden === true
        : nodo.breakpoints?.[puntoDeCorte]?.hidden === true;

    const tieneOverride =
      puntoDeCorte !== 'desktop' && nodo.breakpoints?.[puntoDeCorte] !== undefined;

    const nombreMostrado =
      nodo.name ||
      (nodo.type === 'section' ? `Sección ${nodo.kind || ''}` : nodo.type.toUpperCase());

    return (
      <div key={nodo.id} class="flex flex-col">
        <div
          role="treeitem"
          aria-selected={esSeleccionado}
          tabIndex={esSeleccionado ? 0 : -1}
          onClick={() => onSeleccionarNodo(nodo.id)}
          onKeyDown={(e) => {
            if (e.key === 'F2') {
              e.preventDefault();
              iniciarRenombrar(nodo.id, nombreMostrado);
            } else if (e.key === 'Delete' || e.key === 'Backspace') {
              e.preventDefault();
              if (!estaBloqueado) onEliminarNodo(nodo.id);
            } else if (e.key === 'ArrowUp' && e.altKey) {
              e.preventDefault();
              onReordenarHermanos(nodo.id, 'arriba');
            } else if (e.key === 'ArrowDown' && e.altKey) {
              e.preventDefault();
              onReordenarHermanos(nodo.id, 'abajo');
            }
          }}
          class={`group flex h-8 cursor-pointer items-center justify-between rounded px-2 text-xs font-medium transition-colors ${
            esSeleccionado
              ? 'bg-selected text-primary-text font-semibold'
              : 'text-text-muted hover:bg-bg-subtle hover:text-text'
          }`}
          style={{ paddingLeft: `${nivel * 12 + 8}px` }}
        >
          <div class="flex items-center gap-1.5 overflow-hidden">
            {tieneHijos ? (
              <button
                type="button"
                aria-label={estaExpandido ? 'Plegar capa' : 'Desplegar capa'}
                onClick={(e) => {
                  e.stopPropagation();
                  alternarExpansion(nodo.id);
                }}
                class="text-text-muted hover:text-text rounded p-0.5"
              >
                {estaExpandido ? <IconoChevronDown size={12} /> : <IconoChevronRight size={12} />}
              </button>
            ) : (
              <span class="w-3" />
            )}

            {nodoEnEdicion === nodo.id ? (
              <input
                ref={(el) => el?.focus()}
                type="text"
                value={nombreEdicion}
                onInput={(e) => setNombreEdicion((e.target as HTMLInputElement).value)}
                onBlur={() => confirmarRenombrar(nodo.id)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') confirmarRenombrar(nodo.id);
                  if (e.key === 'Escape') setNodoEnEdicion(null);
                }}
                class="bg-surface border-border text-text h-6 w-32 rounded border px-1 text-xs"
              />
            ) : (
              <span
                class="max-w-[120px] truncate"
                onDblClick={(e) => {
                  e.stopPropagation();
                  iniciarRenombrar(nodo.id, nombreMostrado);
                }}
              >
                {nombreMostrado}
              </span>
            )}

            {tieneOverride && (
              <span class="py-0.2 rounded bg-amber-500/20 px-1 text-[9px] text-amber-600">
                {puntoDeCorte}
              </span>
            )}
          </div>

          {/* Acciones de capa */}
          <div class="flex items-center gap-1 opacity-0 group-hover:opacity-100 focus-within:opacity-100">
            {/* Reordenar */}
            {!esPrimero && (
              <button
                type="button"
                aria-label="Subir capa"
                onClick={(e) => {
                  e.stopPropagation();
                  onReordenarHermanos(nodo.id, 'arriba');
                }}
                class="hover:text-primary-text p-0.5"
              >
                ↑
              </button>
            )}
            {!esUltimo && (
              <button
                type="button"
                aria-label="Bajar capa"
                onClick={(e) => {
                  e.stopPropagation();
                  onReordenarHermanos(nodo.id, 'abajo');
                }}
                class="hover:text-primary-text p-0.5"
              >
                ↓
              </button>
            )}

            {/* Ocultar / Mostrar */}
            <button
              type="button"
              aria-label={estaOculto ? 'Mostrar capa' : 'Ocultar capa'}
              onClick={(e) => {
                e.stopPropagation();
                onOcultarNodo(nodo.id);
              }}
              class={`p-0.5 ${estaOculto ? 'text-amber-600 opacity-100' : 'hover:text-text'}`}
            >
              {estaOculto ? <IconoOculto size={13} /> : <IconoVisible size={13} />}
            </button>

            {/* Bloquear / Desbloquear */}
            <button
              type="button"
              aria-label={estaBloqueado ? 'Desbloquear capa' : 'Bloquear capa'}
              onClick={(e) => {
                e.stopPropagation();
                onBloquearNodo(nodo.id);
              }}
              class={`p-0.5 ${estaBloqueado ? 'text-amber-600 opacity-100' : 'hover:text-text'}`}
            >
              {estaBloqueado ? <IconoBloqueado size={13} /> : <IconoDesbloqueado size={13} />}
            </button>

            {/* Eliminar */}
            {!estaBloqueado && (
              <button
                type="button"
                aria-label="Eliminar capa"
                onClick={(e) => {
                  e.stopPropagation();
                  onEliminarNodo(nodo.id);
                }}
                class="p-0.5 hover:text-red-600"
              >
                <IconoTrash size={13} />
              </button>
            )}
          </div>
        </div>

        {/* Hijos */}
        {tieneHijos && estaExpandido && (
          <div class="flex flex-col">
            {(nodo as Nodo & { children: Nodo[] }).children.map((hijo, idx, arr) =>
              renderizarNodoArbol(hijo, nivel + 1, idx === 0, idx === arr.length - 1),
            )}
          </div>
        )}
      </div>
    );
  };

  return (
    <div role="tree" aria-label="Capas" class="flex h-full flex-col overflow-y-auto p-2">
      <div class="mb-2 flex items-center justify-between px-2 pt-1">
        <span class="text-text-muted text-[11px] font-semibold tracking-wider uppercase">
          Capas de {paginaActiva?.title?.es || paginaActiva?.id}
        </span>
      </div>

      <div class="flex flex-col gap-0.5">
        {ir.header && renderizarNodoArbol(ir.header, 0, true, false)}

        {paginaActiva?.sections.map((sec, idx, arr) =>
          renderizarNodoArbol(
            sec,
            0,
            idx === 0 && !ir.header,
            idx === arr.length - 1 && !ir.footer,
          ),
        )}

        {ir.footer && renderizarNodoArbol(ir.footer, 0, false, true)}
      </div>
    </div>
  );
}
