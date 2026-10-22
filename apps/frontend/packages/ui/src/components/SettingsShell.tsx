/**
 * SettingsShell / SettingsSection — marco de una página de AJUSTES.
 *
 * Evita el «muro de formulario»: ancho de lectura acotado, título y descripción en
 * lenguaje llano, y el contenido troceado en secciones que se explican solas. Cada
 * sección tiene su propia acción a la derecha (guardar por sección, no un botón
 * gigante al final).
 *
 *   <SettingsShell title="Integraciones"
 *     description="Conecta las aplicaciones que ya usa tu equipo…"
 *     actions={<Button>Conectar una aplicación</Button>}>
 *     <SettingsSection title="Aplicaciones conectadas" description="…">
 *       …
 *     </SettingsSection>
 *   </SettingsShell>
 *
 * El título es un `h1` por defecto (la página lo titula); si la página ya tiene su
 * h1 (p. ej. un SectionHeader `level={1}`), pasa `level={2}`.
 */
import type { ComponentChildren, JSX } from 'preact';
import { cx } from '../lib/cx.js';
import { Card, CardBody } from './Card.js';

export interface SettingsShellProps {
  title: string;
  /** Una o dos frases llanas: qué se configura aquí y para qué sirve. */
  description?: string;
  /** Acción principal de la página (a la derecha del título). */
  actions?: ComponentChildren;
  /** Aviso permanente sobre el contenido (p. ej. servicio no disponible). */
  banner?: ComponentChildren;
  level?: 1 | 2;
  children: ComponentChildren;
  class?: string;
}

export function SettingsShell({
  title,
  description,
  actions,
  banner,
  level = 1,
  children,
  class: cls,
}: SettingsShellProps): JSX.Element {
  const Heading = `h${level}` as 'h1';
  return (
    <div class={cx('mx-auto w-full max-w-5xl px-4 py-6 sm:px-6', cls)}>
      <div class="flex flex-wrap items-start gap-4">
        <div class="min-w-0 flex-1">
          <Heading class="sytue-h2 text-text">{title}</Heading>
          {description ? (
            <p class="text-text-muted mt-1.5 max-w-2xl text-base leading-relaxed">{description}</p>
          ) : null}
        </div>
        {actions ? <div class="flex shrink-0 items-center gap-2">{actions}</div> : null}
      </div>
      {banner ? <div class="mt-4">{banner}</div> : null}
      <div class="mt-6 flex flex-col gap-6">{children}</div>
    </div>
  );
}

export interface SettingsSectionProps {
  title: string;
  /** Qué hace esta sección, en una frase. */
  description?: string;
  /** Acciones de la sección (a la derecha del título). */
  actions?: ComponentChildren;
  /** Sin tarjeta: la sección es solo un encabezado sobre contenido libre. */
  plain?: boolean;
  children: ComponentChildren;
  class?: string;
}

export function SettingsSection({
  title,
  description,
  actions,
  plain,
  children,
  class: cls,
}: SettingsSectionProps): JSX.Element {
  const header = (
    <div class="flex flex-wrap items-start gap-3">
      <div class="min-w-0 flex-1">
        <h2 class="sytue-h4 text-text">{title}</h2>
        {description ? (
          <p class="text-text-muted mt-1 max-w-2xl text-sm leading-relaxed">{description}</p>
        ) : null}
      </div>
      {actions ? <div class="flex shrink-0 items-center gap-2">{actions}</div> : null}
    </div>
  );
  if (plain) {
    return (
      <section class={cx('flex flex-col gap-3', cls)}>
        {header}
        {children}
      </section>
    );
  }
  return (
    <Card sectionLabel={title} class={cls}>
      <CardBody class="flex flex-col gap-4">
        {header}
        {children}
      </CardBody>
    </Card>
  );
}
