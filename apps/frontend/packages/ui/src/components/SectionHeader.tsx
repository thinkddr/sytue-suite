/**
 * SectionHeader — cabecera de panel/vista sobre el contenido. Estilo Mattermost:
 * fondo BLANCO con texto oscuro y un hairline inferior (el color del producto vive
 * en el menú lateral y la topbar, no en las cabeceras de contenido). Es el patrón
 * canónico para el título de bandeja/lista/vista de cada app.
 */
import type { ComponentChildren, JSX } from 'preact';
import { cx } from '../lib/cx.js';

export interface SectionHeaderProps {
  /** Título de la sección. */
  title: ComponentChildren;
  /** Subtítulo/contexto breve bajo el título. */
  subtitle?: ComponentChildren;
  /** Icono o marca a la izquierda del título. */
  icon?: ComponentChildren;
  /** Acciones a la derecha (botones, filtros). */
  actions?: ComponentChildren;
  /**
   * Nivel semántico del encabezado (1–6). Opt-in: por defecto h2 (comportamiento
   * histórico). Usa `level={1}` cuando el SectionHeader titula la página entera,
   * para que exista un único h1 (WCAG 1.3.1/2.4.6). Solo cambia la semántica; el
   * tamaño visual sigue siendo `sytue-h4`.
   */
  level?: 1 | 2 | 3 | 4 | 5 | 6;
  class?: string;
}

export function SectionHeader({
  title,
  subtitle,
  icon,
  actions,
  level = 2,
  class: cls,
}: SectionHeaderProps): JSX.Element {
  // Etiqueta real (h1..h6); el cast a un intrínseco concreto es seguro (todos
  // comparten atributos) y Preact renderiza la cadena real en runtime.
  const Heading = `h${level}` as 'h2';
  return (
    // flex-wrap: en un móvil el título y las acciones no caben en la misma línea
    // —«Nuevo proyecto» junto a «Plantillas» ya se salía 218px de un iPhone SE—,
    // así que las acciones bajan a una segunda fila en vez de empujar la vista.
    <div
      class={cx(
        'border-border bg-surface text-text flex min-h-12 flex-wrap items-center gap-x-3 gap-y-2 border-b px-4 py-2.5',
        cls,
      )}
    >
      {icon ? (
        <span class="text-text-muted grid h-5 w-5 shrink-0 place-items-center">{icon}</span>
      ) : null}
      {/* grow (basis auto), no flex-1 (basis 0): con basis 0 el título se encoge
          a cero y el salto de línea no llega a producirse. */}
      <div class="min-w-0 grow">
        <Heading class="sytue-h4 truncate">{title}</Heading>
        {subtitle ? <p class="text-text-muted truncate text-sm">{subtitle}</p> : null}
      </div>
      {actions ? <div class="flex flex-wrap items-center gap-2">{actions}</div> : null}
    </div>
  );
}
