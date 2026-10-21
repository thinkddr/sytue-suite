/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Barra de estado inferior del editor de Sytue Web (PAGES-PLAN §14.1).
 * Muestra el nodo seleccionado, tamaño del IR (formatBytes) con alerta > 1.5 MiB,
 * y resumen de avisos del validador.
 */

import type { JSX } from 'preact';
import type { Nodo, Seccion } from '@sytue/pages-render';
import { formatBytes } from '@sytue/ui';
import { Alerta } from '../iconos';

export interface BarraEstadoProps {
  nodoSeleccionado: Nodo | Seccion | null;
  tamanoBytes: number;
  avisosCount: number;
}

const LIMITE_ALERTA_BYTES = 1.5 * 1024 * 1024; // 1.5 MiB
const LIMITE_MAXIMO_BYTES = 2.0 * 1024 * 1024; // 2.0 MiB

export function BarraEstado({
  nodoSeleccionado,
  tamanoBytes,
  avisosCount,
}: BarraEstadoProps): JSX.Element {
  const tamanoTexto = formatBytes('es', tamanoBytes);
  const esCasiLleno = tamanoBytes >= LIMITE_ALERTA_BYTES;
  const esExcedido = tamanoBytes >= LIMITE_MAXIMO_BYTES;

  const tipoTexto =
    nodoSeleccionado?.type === 'section'
      ? `sección (${nodoSeleccionado.kind})`
      : nodoSeleccionado?.type;

  return (
    <footer class="border-border bg-bg-subtle text-text-muted flex h-8 w-full items-center justify-between border-t px-3 text-[11px]">
      {/* Información del nodo seleccionado */}
      <div class="flex items-center gap-2 truncate">
        {nodoSeleccionado ? (
          <div class="flex items-center gap-1.5 font-mono">
            <span class="bg-selected text-primary-text rounded px-1 py-0.5 font-semibold">
              {tipoTexto}
            </span>
            <span class="text-text-muted">#</span>
            <span class="text-text max-w-[200px] truncate">{nodoSeleccionado.id}</span>
          </div>
        ) : (
          <span class="text-text-muted">Ningún nodo seleccionado</span>
        )}
      </div>

      {/* Avisos del validador y Tamaño del IR */}
      <div class="flex shrink-0 items-center gap-4">
        {avisosCount > 0 && (
          <span
            class="flex items-center gap-1 text-amber-600"
            title={`${avisosCount} avisos de validación`}
          >
            <span class="h-2 w-2 rounded-full bg-amber-500" />
            <span>
              {avisosCount} {avisosCount === 1 ? 'aviso' : 'avisos'}
            </span>
          </span>
        )}

        <div
          class={`flex items-center gap-1 font-mono ${
            esExcedido
              ? 'font-bold text-red-600'
              : esCasiLleno
                ? 'font-semibold text-amber-600'
                : 'text-text-muted'
          }`}
          title={
            esExcedido
              ? 'El IR supera el límite de 2 MiB'
              : esCasiLleno
                ? 'El IR se aproxima al límite de 2 MiB'
                : 'Tamaño del IR'
          }
        >
          {esCasiLleno && <Alerta size={11} />}
          <span>{tamanoTexto}</span>
          <span class="text-gray-400">/ 2 MB</span>
        </div>
      </div>
    </footer>
  );
}
