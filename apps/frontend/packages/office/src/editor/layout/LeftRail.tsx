/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Franja izquierda de 40 px + panel desplegable (BLUEPRINT.md §6.4): tres
 * botones de alternancia (Buscar, Esquema, Comentarios) que abren el panel
 * lateral con cabecera y botón de cierre. El ancho del panel usa el token
 * compartido --editor-panel-w (288 px, editor.css §8); la tarea sugería
 * 260 px, pero gana el token para no desalinear con el panel de ajustes.
 */
import type { JSX } from 'preact';
import { EditorIcon, type EditorIconName } from '../icons';
import { ActividadPanel } from '../panels/ActividadPanel';
import { AiPanel } from '../panels/AiPanel';
import { CommentsPanel } from '../panels/CommentsPanel';
import { OutlinePanel } from '../panels/OutlinePanel';
import { RevisionPanel } from '../panels/RevisionPanel';
import { SearchPanel } from '../panels/SearchPanel';
import type { DocsKey } from '../../i18n';
import type { EditorContext, PanelId } from '../types';

interface PanelDef {
  id: PanelId;
  icon: EditorIconName;
  /** Clave del aria-label y title del botón de la franja (nombre largo). */
  labelKey: DocsKey;
  /** Clave del título visible en la cabecera del panel (nombre corto). */
  titleKey: DocsKey;
  Body: (props: { ctx: EditorContext }) => JSX.Element;
}

const PANELS: readonly PanelDef[] = [
  {
    id: 'buscar',
    icon: 'search',
    labelKey: 'rail.search',
    titleKey: 'rail.search.title',
    Body: SearchPanel,
  },
  {
    id: 'esquema',
    icon: 'outline',
    labelKey: 'rail.outline',
    titleKey: 'rail.outline.title',
    Body: OutlinePanel,
  },
  {
    id: 'comentarios',
    icon: 'commentList',
    labelKey: 'rail.comments',
    titleKey: 'rail.comments.title',
    Body: CommentsPanel,
  },
  {
    id: 'revision',
    icon: 'trackChanges',
    labelKey: 'rail.review',
    titleKey: 'rail.review.title',
    Body: RevisionPanel,
  },
  {
    id: 'actividad',
    icon: 'history',
    labelKey: 'rail.activity',
    titleKey: 'rail.activity.title',
    Body: ActividadPanel,
  },
  { id: 'ia', icon: 'sparkles', labelKey: 'rail.ai', titleKey: 'rail.ai.title', Body: AiPanel },
];

// El botón del panel abierto lleva, además del relleno de «seleccionado», la marca de «estás
// aquí» del sistema: una barra de cobre pegada al borde de la franja (como el ítem activo
// del menú lateral de la suite). Va a `-left-1`: el botón está a 4 px del borde de la franja.
const RAIL_BTN =
  'relative inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-sm text-text-muted ' +
  'transition-colors duration-150 ' +
  'hover:bg-bg-subtle hover:text-text aria-pressed:bg-selected aria-pressed:text-text ' +
  'aria-pressed:before:bg-indicator aria-pressed:before:absolute aria-pressed:before:top-1.5 ' +
  'aria-pressed:before:-left-1 aria-pressed:before:h-5 aria-pressed:before:w-0.5 ' +
  'aria-pressed:before:rounded-full ' +
  'sytue-focus-ring';

// Botón sobre la cabecera sólida del producto (bg-header): texto claro AA, velo
// blanco en hover (mismo criterio que la topbar de la shell).
const CLOSE_BTN =
  'inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-sm text-header-fg ' +
  'hover:bg-chrome-hover active:bg-chrome-active ' +
  'focus-visible:outline-none sytue-focus-ring-chrome';

export function LeftRail({ ctx }: { ctx: EditorContext }): JSX.Element {
  const t = ctx.t;
  const openPanel = ctx.ui.panel;
  // El asistente solo entra si hay servicio de IA desplegado (SYTUE_AI_URL): sin
  // él el panel solo puede enseñar un 503, y eso no es una función.
  // La actividad es del propietario: la API contesta 403 al resto, así que ni se ofrece.
  const panels = PANELS.filter(
    (p) => (p.id !== 'ia' || ctx.aiEnabled) && (p.id !== 'actividad' || ctx.esPropietario),
  );
  const activeDef = panels.find((panel) => panel.id === openPanel) ?? null;

  return (
    <>
      {/* `sytue-no-print`: al imprimir un documento sale el documento, no el
          carril de herramientas con el que se escribió.

          `overflow-y-auto`: la franja DESPLAZA cuando no cabe. Sus botones no encogen, y
          en un móvil apaisado (340 px de alto) los dos últimos —«Actividad» y el
          asistente— quedaban por debajo del editor, recortados y sin forma de llegar a
          ellos (medido en la v3.11.1: −52 y −100 px). Sin barra a la vista: son 40 px de
          ancho. `relative`, como todo panel que desplaza. */}
      <div class="sytue-no-print border-border bg-surface relative flex min-h-0 w-[var(--editor-rail-w)] shrink-0 [scrollbar-width:none] flex-col items-center gap-1 overflow-y-auto border-r py-2">
        {panels.map((panel) => (
          <button
            key={panel.id}
            type="button"
            class={RAIL_BTN}
            aria-label={t(panel.labelKey)}
            title={t(panel.labelKey)}
            aria-pressed={openPanel === panel.id}
            onClick={() => ctx.setUi({ panel: openPanel === panel.id ? null : panel.id })}
          >
            <EditorIcon name={panel.icon} />
          </button>
        ))}
      </div>

      {activeDef !== null && (
        // En <768px el panel no cabe junto al folio (288 + las dos franjas de 40
        // se pasan de un teléfono de 360): ahí ocupa el ancho que quede —el lienzo
        // se retira, ver EditorLayout— en vez de empujar la pantalla de lado.
        <aside
          aria-label={t(activeDef.titleKey)}
          class="border-border bg-surface flex w-[var(--editor-panel-w)] min-w-0 shrink-0 flex-col border-r max-md:w-auto max-md:flex-1"
        >
          <div class="bg-header text-header-fg border-chrome-divider flex h-10 shrink-0 items-center justify-between border-b pr-1.5 pl-3">
            <h2 class="truncate text-sm font-semibold">{t(activeDef.titleKey)}</h2>
            <button
              type="button"
              class={CLOSE_BTN}
              aria-label={t('rail.close')}
              title={t('rail.close')}
              onClick={() => ctx.setUi({ panel: null })}
            >
              <EditorIcon name="close" size="md" />
            </button>
          </div>
          <div class="min-h-0 flex-1 overflow-y-auto">
            <activeDef.Body ctx={ctx} />
          </div>
        </aside>
      )}
    </>
  );
}
