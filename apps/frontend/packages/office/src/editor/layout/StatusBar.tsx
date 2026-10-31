/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Barra de estado inferior de 28 px (BLUEPRINT.md §6.7): unidad actual
 * (página/hoja/diapositiva), palabras e idioma (solo kind document) y control
 * de zoom (botón −, deslizador MIN_ZOOM–MAX_ZOOM en pasos de 10, porcentaje, botón +).
 */
import type { JSX } from 'preact';
import type { DocKind } from '@sytue/api-clients';
import { intlTag } from '@sytue/i18n';
import { IconButton } from '@sytue/ui';
import { EditorIcon } from '../icons';
import { MIN_ZOOM, MAX_ZOOM } from '../canvas/metrics';
import type { DocsKey } from '../../i18n';
import type { EditorContext } from '../types';

/* «Página 2 de 7» es UNA frase por tipo de documento, no «unidad» + números
 * concatenados: en alemán y en francés el orden y la preposición cambian. */
const POSITION_KEY: Record<DocKind, DocsKey> = {
  document: 'status.page',
  spreadsheet: 'status.sheet',
  presentation: 'status.slide',
};

export function StatusBar({ ctx }: { ctx: EditorContext }): JSX.Element {
  const t = ctx.t;
  const { page, pages, words } = ctx.stats;
  const zoom = ctx.ui.zoom;
  const setZoom = (next: number): void => {
    ctx.setUi({ zoom: Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, next)) });
  };

  return (
    <div class="sytue-sheet-corners border-border bg-surface text-text-muted flex h-[var(--editor-status-h)] shrink-0 items-center gap-4 border-t px-3 text-xs [--sytue-sheet-tl:0px] [--sytue-sheet-tr:0px]">
      <span class="shrink-0 whitespace-nowrap">
        {t(POSITION_KEY[ctx.kind] ?? 'status.page', { page, pages })}
      </span>
      {ctx.kind === 'document' && (
        <>
          {/* Fuera en <640px: la barra mide 28px de alto y no puede envolver, y
              con los dos informativos sumaba 465px contra los 390 de un móvil.
              Se van los que solo informan; el zoom —la única acción, y la que más
              falta hace en un teléfono— se queda entero. */}
          <span class="max-sm:hidden">{t('status.words', { count: words })}</span>
          {/* ponytail: idioma único informativo en v1 —el de la interfaz—; cablear
              opciones y lang del folio cuando el documento persista el suyo. */}
          {/* 24 px de alto, no `h-5`: eran 17,5 (1,25 rem con la raíz a 14), medido en
              producción; la barra mide 28 y cabe. */}
          <select
            aria-label={t('status.language')}
            title={t('status.language')}
            class="text-text-muted hover:text-text sytue-focus-ring h-[24px] cursor-pointer rounded-sm bg-transparent text-xs max-sm:hidden"
          >
            <option value={intlTag(ctx.locale)}>{t('status.language.value')}</option>
          </select>
        </>
      )}

      {/* EL DESLIZADOR CEDE. Con el dedo los dos botones miden 44 px, y con el rótulo más
          largo —«Diapositiva 1 de 3»— el grupo se salía 4 px de la barra a 320. De todo
          lo que hay aquí, el deslizador es lo único que puede medir menos sin dejar de
          servir (los botones de al lado hacen lo mismo paso a paso), y nunca menos de
          44 px, que sigue siendo una diana para el dedo; ni el rótulo ni el porcentaje
          parten en dos líneas, que la barra mide 28 px. */}
      <div class="ml-auto flex min-w-0 items-center gap-1">
        <IconButton
          size="sm"
          label={t('status.zoom.out')}
          disabled={zoom <= MIN_ZOOM}
          onClick={() => setZoom(zoom - 10)}
        >
          <EditorIcon name="minus" size="sm" />
        </IconButton>
        <input
          type="range"
          min={MIN_ZOOM}
          max={MAX_ZOOM}
          step={10}
          value={zoom}
          aria-label={t('status.zoom')}
          aria-valuetext={`${zoom} %`}
          title={t('status.zoom')}
          onInput={(e) => setZoom(Number(e.currentTarget.value))}
          class="sytue-zoom sytue-focus-ring w-24 min-w-[44px] shrink cursor-pointer rounded-full pointer-coarse:min-h-[max(44px,2.75rem)]"
        />
        <span class="w-10 shrink-0 text-center whitespace-nowrap tabular-nums">{zoom} %</span>
        <IconButton
          size="sm"
          label={t('status.zoom.in')}
          disabled={zoom >= MAX_ZOOM}
          onClick={() => setZoom(zoom + 10)}
        >
          <EditorIcon name="plus" size="sm" />
        </IconButton>
      </div>
    </div>
  );
}
