/**
 * FieldHint / InlineHelp — la capa de AYUDA de la configuración guiada.
 *
 * FieldHint: una línea bajo un campo que dice qué es y DÓNDE encontrarlo, y que
 * cambia a estado en vivo (ok/error) sin gritar — mismo sitio, mismo tamaño, solo
 * cambia el icono y el color. Va dentro de un FieldGroup (o suelta bajo el control)
 * y se enlaza con `aria-describedby` del control mediante `id`.
 *
 *   <FieldGroup label="Workspace de Slack" htmlFor="ws">
 *     <Input id="ws" aria-describedby="ws-hint" />
 *     <FieldHint id="ws-hint" tone={err ? 'danger' : val ? 'success' : 'neutral'}>
 *       {err ?? (val ? 'Tiene buena pinta.' : 'Está en tu URL: https://ACME.slack.com')}
 *     </FieldHint>
 *   </FieldGroup>
 *
 * InlineHelp: bloque de contexto dentro del flujo («qué necesitas a mano», el
 * resultado de una prueba, un aviso). Sobrio: borde + fondo tenue del tono, sin
 * degradados ni sombras.
 */
import type { ComponentChildren, JSX } from 'preact';
import { cx } from '../lib/cx.js';
import { Icon } from './Icon.js';
import type { IconName } from './Icon.js';

export type HintTone = 'neutral' | 'info' | 'success' | 'warning' | 'danger';

export interface FieldHintProps {
  /** Estado en vivo. neutral = ayuda; success/danger = validación. */
  tone?: HintTone;
  /** Oculta el icono del tono (por defecto se muestra salvo en neutral). */
  hideIcon?: boolean;
  /** Para enlazarlo con `aria-describedby` del control. */
  id?: string;
  children: ComponentChildren;
  class?: string;
}

const toneIcon: Record<HintTone, IconName> = {
  neutral: 'info',
  info: 'info',
  success: 'check',
  warning: 'warning',
  danger: 'danger',
};

const hintText: Record<HintTone, string> = {
  neutral: 'text-text-muted',
  info: 'text-info-text',
  success: 'text-success-text',
  warning: 'text-warning-text',
  danger: 'text-danger-text',
};

export function FieldHint({
  tone = 'neutral',
  hideIcon,
  id,
  children,
  class: cls,
}: FieldHintProps): JSX.Element {
  const showIcon = !hideIcon && tone !== 'neutral';
  return (
    <p
      id={id}
      // `overflow-wrap:anywhere`: una dirección larga («Se pedirá: GET https://…») parte
      // donde haga falta en vez de salirse del panel (78 px en el teléfono, Calcus).
      class={cx(
        'flex min-w-0 items-start gap-1.5 text-xs leading-snug [overflow-wrap:anywhere]',
        hintText[tone],
        cls,
      )}
      // El error se anuncia al vuelo; la ayuda neutral no interrumpe.
      role={tone === 'danger' ? 'alert' : undefined}
    >
      {showIcon ? <Icon name={toneIcon[tone]} size={14} class="mt-px shrink-0" /> : null}
      <span class="min-w-0">{children}</span>
    </p>
  );
}

export interface InlineHelpProps {
  tone?: HintTone;
  /** Titular breve; el cuerpo va en `children`. */
  title?: string;
  /** Oculta el icono del tono. */
  hideIcon?: boolean;
  /** Acción a la derecha (enlace o botón pequeño). */
  action?: ComponentChildren;
  children?: ComponentChildren;
  class?: string;
}

const helpSurface: Record<HintTone, string> = {
  neutral: 'border-border bg-bg-subtle',
  info: 'border-info bg-info-bg',
  success: 'border-success bg-success-bg',
  warning: 'border-warning bg-warning-bg',
  danger: 'border-danger bg-danger-bg',
};

export function InlineHelp({
  tone = 'neutral',
  title,
  hideIcon,
  action,
  children,
  class: cls,
}: InlineHelpProps): JSX.Element {
  return (
    <div
      class={cx('flex gap-3 rounded-md border p-3', helpSurface[tone], cls)}
      // Mismo criterio que FieldHint: el error se anuncia al vuelo y el aviso
      // espera turno; la ayuda neutral no interrumpe. Sin esto, un error que
      // aparece DESPUÉS de pintar la página —el de un asistente que falla al
      // conectar, por ejemplo— era invisible para un lector de pantalla, que es
      // justo el momento en el que hace falta enterarse (WCAG 4.1.3).
      role={tone === 'danger' ? 'alert' : tone === 'warning' ? 'status' : undefined}
    >
      {hideIcon ? null : (
        <Icon
          name={toneIcon[tone]}
          size={16}
          class={cx('mt-0.5 shrink-0', tone === 'neutral' ? 'text-text-subtle' : hintText[tone])}
        />
      )}
      <div class="min-w-0 flex-1 text-sm leading-relaxed">
        {title ? (
          <p class={cx('font-semibold', tone === 'neutral' ? 'text-text' : hintText[tone])}>
            {title}
          </p>
        ) : null}
        {children ? <div class={cx('text-text-muted', title ? 'mt-1' : '')}>{children}</div> : null}
      </div>
      {action ? <div class="shrink-0 self-center">{action}</div> : null}
    </div>
  );
}
