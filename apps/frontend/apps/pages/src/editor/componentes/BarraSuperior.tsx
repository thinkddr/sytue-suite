/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Barra superior del editor de Sytue Web (PAGES-PLAN §14.1).
 */

import type { JSX } from 'preact';
import type { Idioma, Pagina, Sitio } from '@sytue/pages-render';
import type { EstadoGuardado, PuntoDeCorte } from '../modelo/tipos';
import {
  Alerta,
  IconoDesktop,
  IconoDeshacer,
  IconoGuardado,
  IconoMobile,
  IconoOjo,
  IconoRehacer,
  IconoTablet,
  IconoVolver,
  IconoZoomAjustar,
  IconoZoomMas,
  IconoZoomMenos,
  Spinner,
} from '../iconos';
import { ruta } from '../../lib/rutas';
import { t } from '../../i18n/index.js';

export interface BarraSuperiorProps {
  locale: string;
  siteId: string;
  siteName: string;
  ir: Sitio;
  paginaActivaId: string;
  puntoDeCorte: PuntoDeCorte;
  idiomaActivo: Idioma;
  zoom: number;
  estadoGuardado: EstadoGuardado;
  puedeDeshacer: boolean;
  puedeRehacer: boolean;
  onCambiarPagina: (id: string) => void;
  onCambiarPuntoDeCorte: (bp: PuntoDeCorte) => void;
  onCambiarIdioma: (idioma: Idioma) => void;
  onAbrirAsistente: (modo: 'diseno' | 'traduccion') => void;
  onDeshacer: () => void;
  onRehacer: () => void;
  onZoomMas: () => void;
  onZoomMenos: () => void;
  onZoomAjustar: () => void;
  onZoom100: () => void;
  previsualizando: boolean;
  onVistaPrevia: () => void;
}

export function BarraSuperior({
  locale,
  siteId,
  siteName,
  ir,
  paginaActivaId,
  puntoDeCorte,
  idiomaActivo,
  zoom,
  estadoGuardado,
  puedeDeshacer,
  puedeRehacer,
  onCambiarPagina,
  onCambiarPuntoDeCorte,
  onCambiarIdioma,
  onAbrirAsistente,
  onDeshacer,
  onRehacer,
  onZoomMas,
  onZoomMenos,
  onZoomAjustar,
  onZoom100,
  previsualizando,
  onVistaPrevia,
}: BarraSuperiorProps): JSX.Element {
  const zoomPct = Math.round(zoom * 100);

  return (
    <header class="border-border bg-surface text-text flex h-14 w-full items-center justify-between border-b px-3 shadow-xs">
      {/* Zona izquierda: Volver, Nombre, Selector de página */}
      <div class="flex items-center gap-3">
        <a
          href={ruta(`/site/${encodeURIComponent(siteId)}`)}
          title="Volver"
          class="text-text-muted hover:bg-bg-subtle hover:text-text rounded-control flex h-8 w-8 items-center justify-center"
        >
          <IconoVolver size={14} />
        </a>

        <div class="hidden flex-col sm:flex">
          <span class="text-text max-w-[140px] truncate text-xs leading-tight font-semibold md:max-w-[200px]">
            {siteName || 'Sitio'}
          </span>
          <span class="text-text-muted text-[10px]">Sytue Web</span>
        </div>

        <div class="bg-border h-5 w-px" />

        {/* Selector de página activa */}
        <div class="flex items-center">
          <label for="selector-pagina" class="sr-only">
            Página activa
          </label>
          <select
            id="selector-pagina"
            value={paginaActivaId}
            onChange={(e) => onCambiarPagina((e.target as HTMLSelectElement).value)}
            class="border-border bg-bg-subtle text-text sytue-focus-ring focus:border-primary rounded-md border py-1 pr-7 pl-2.5 text-xs font-medium"
          >
            {ir.pages.map((p: Pagina) => (
              <option key={p.id} value={p.id}>
                {p.title?.es || p.title?.en || p.slug?.es || p.id}{' '}
                {p.id === ir.pages[0]?.id ? '★' : ''}
              </option>
            ))}
          </select>
        </div>
      </div>

      <label class="hidden items-center gap-1 text-xs sm:flex">
        <span class="sr-only">Idioma de contenido</span>
        <select
          value={idiomaActivo}
          onChange={(e) => onCambiarIdioma((e.target as HTMLSelectElement).value as Idioma)}
          class="border-border bg-surface sytue-focus-ring rounded border px-2 py-1 text-xs"
        >
          {[ir.languages.default, ...ir.languages.others]
            .filter((v, i, a) => v && a.indexOf(v) === i)
            .map((idioma) => (
              <option key={idioma} value={idioma}>
                {idioma.toUpperCase()}
              </option>
            ))}
        </select>
      </label>

      <div class="hidden items-center gap-1 lg:flex">
        <button
          type="button"
          onClick={() => onAbrirAsistente('diseno')}
          class="border-border hover:bg-bg-subtle sytue-focus-ring rounded border px-2 py-1 text-xs font-medium"
        >
          Diseñar con IA
        </button>
        <button
          type="button"
          onClick={() => onAbrirAsistente('traduccion')}
          class="border-border hover:bg-bg-subtle sytue-focus-ring rounded border px-2 py-1 text-xs font-medium"
        >
          Traducir
        </button>
      </div>

      {/* Zona central: Breakpoints (Desktop, Tablet, Mobile) */}
      <div class="border-border bg-bg-subtle flex items-center rounded-lg border p-0.5">
        <button
          type="button"
          onClick={() => onCambiarPuntoDeCorte('desktop')}
          title="Escritorio (1200 px)"
          class={`rounded-control flex h-7 items-center gap-1.5 px-2 text-xs font-medium transition-colors ${
            puntoDeCorte === 'desktop'
              ? 'bg-surface text-text shadow-xs'
              : 'text-text-muted hover:text-text'
          }`}
        >
          <IconoDesktop size={12} />
          <span class="hidden md:inline">1200</span>
        </button>
        <button
          type="button"
          onClick={() => onCambiarPuntoDeCorte('tablet')}
          title="Tableta (768 px)"
          class={`rounded-control flex h-7 items-center gap-1.5 px-2 text-xs font-medium transition-colors ${
            puntoDeCorte === 'tablet'
              ? 'bg-surface text-text shadow-xs'
              : 'text-text-muted hover:text-text'
          }`}
        >
          <IconoTablet size={12} />
          <span class="hidden md:inline">768</span>
        </button>
        <button
          type="button"
          onClick={() => onCambiarPuntoDeCorte('mobile')}
          title="Móvil (375 px)"
          class={`rounded-control flex h-7 items-center gap-1.5 px-2 text-xs font-medium transition-colors ${
            puntoDeCorte === 'mobile'
              ? 'bg-surface text-text shadow-xs'
              : 'text-text-muted hover:text-text'
          }`}
        >
          <IconoMobile size={12} />
          <span class="hidden md:inline">375</span>
        </button>
      </div>

      {/* Zona derecha: Deshacer/Rehacer, Zoom, Estado guardado, Vista previa */}
      <div class="flex items-center gap-2">
        {/* Deshacer / Rehacer */}
        <div class="flex items-center gap-0.5">
          <button
            type="button"
            onClick={onDeshacer}
            disabled={!puedeDeshacer}
            title="Deshacer (Ctrl+Z)"
            class="text-text-muted hover:bg-bg-subtle rounded-control flex h-8 w-8 items-center justify-center disabled:opacity-35 disabled:hover:bg-transparent"
          >
            <IconoDeshacer size={14} />
          </button>
          <button
            type="button"
            onClick={onRehacer}
            disabled={!puedeRehacer}
            title="Rehacer (Ctrl+Y)"
            class="text-text-muted hover:bg-bg-subtle rounded-control flex h-8 w-8 items-center justify-center disabled:opacity-35 disabled:hover:bg-transparent"
          >
            <IconoRehacer size={14} />
          </button>
        </div>

        <div class="bg-border h-5 w-px" />

        {/* Zoom */}
        <div class="hidden items-center gap-1 sm:flex">
          <button
            type="button"
            onClick={onZoomMenos}
            title="Reducir zoom (Ctrl+-)"
            class="text-text-muted hover:bg-bg-subtle rounded-control flex h-7 w-7 items-center justify-center"
          >
            <IconoZoomMenos size={12} />
          </button>
          <button
            type="button"
            onClick={onZoom100}
            title="Zoom al 100% (Ctrl+1)"
            class="text-text hover:text-text min-w-[42px] px-1 text-center font-mono text-xs"
          >
            {zoomPct}%
          </button>
          <button
            type="button"
            onClick={onZoomMas}
            title="Aumentar zoom (Ctrl++)"
            class="text-text-muted hover:bg-bg-subtle rounded-control flex h-7 w-7 items-center justify-center"
          >
            <IconoZoomMas size={12} />
          </button>
          <button
            type="button"
            onClick={onZoomAjustar}
            title="Ajustar zoom (Ctrl+0)"
            class="text-text-muted hover:bg-bg-subtle rounded-control flex h-7 w-7 items-center justify-center"
          >
            <IconoZoomAjustar size={12} />
          </button>
        </div>

        <div class="bg-border h-5 w-px" />

        {/* Indicador de estado de guardado */}
        <div class="flex items-center gap-1.5 px-1 text-xs">
          {estadoGuardado === 'guardando' && (
            <span class="flex items-center gap-1 text-blue-600">
              <Spinner size={12} />
              <span class="hidden md:inline">Guardando...</span>
            </span>
          )}
          {estadoGuardado === 'guardado' && (
            <span class="flex items-center gap-1 text-green-600">
              <IconoGuardado size={12} />
              <span class="hidden md:inline">Guardado</span>
            </span>
          )}
          {estadoGuardado === 'pendiente' && (
            <span class="flex items-center gap-1 text-amber-600">
              <span class="h-2 w-2 rounded-full bg-amber-500" />
              <span class="hidden md:inline">Sin guardar</span>
            </span>
          )}
          {estadoGuardado === 'error' && (
            <span class="flex items-center gap-1 text-red-600" title="Error">
              <Alerta size={12} />
              <span class="hidden md:inline">Error</span>
            </span>
          )}
          {estadoGuardado === 'conflicto' && (
            <span class="flex items-center gap-1 text-red-600" title="Conflicto detectado">
              <Alerta size={12} />
              <span class="hidden md:inline">Conflicto</span>
            </span>
          )}
          {estadoGuardado === 'offline' && (
            <span class="text-text-muted flex items-center gap-1" title="Modo sin conexión">
              <span class="h-2 w-2 rounded-full bg-gray-400" />
              <span class="hidden md:inline">Sin conexión</span>
            </span>
          )}
        </div>

        {/* Botón de vista previa */}
        <button
          type="button"
          onClick={onVistaPrevia}
          disabled={previsualizando || estadoGuardado !== 'guardado'}
          title={t(
            locale,
            estadoGuardado !== 'guardado' ? 'editor.preview_wait_save' : 'editor.preview',
          )}
          class="bg-primary rounded-control flex items-center gap-1 px-3 py-1.5 text-xs font-semibold text-[var(--sytue-on-primary)] shadow-xs transition-colors hover:bg-[var(--sytue-primary-hover)] disabled:opacity-50"
        >
          {previsualizando ? <Spinner size={12} /> : <IconoOjo size={12} />}
          <span class="hidden md:inline">
            {t(locale, previsualizando ? 'editor.preview_creating' : 'editor.preview')}
          </span>
        </button>
      </div>
    </header>
  );
}
