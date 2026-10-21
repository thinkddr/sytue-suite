/**
 * Wizard / Stepper — configuración GUIADA paso a paso (una idea por paso).
 *
 * Es el marco canónico de «conectar/configurar algo» en la suite: progreso claro,
 * atrás/siguiente, validación por paso y paso final de resumen. El asistente NO
 * decide el contenido: cada paso trae su `content` y dice si se puede avanzar.
 *
 * Accesibilidad: el progreso es un `<ol>` con `aria-current="step"`; al cambiar de
 * paso el foco va al encabezado del paso (que lo anuncian los lectores de pantalla);
 * los pasos ya completados son botones (se puede volver con teclado).
 *
 *   const [step, setStep] = useState(0);
 *   <Wizard
 *     current={step}
 *     onCurrentChange={setStep}
 *     onCancel={() => close()}
 *     steps={[
 *       { id: 'elegir', title: 'Elegir', heading: '¿Qué quieres conectar?',
 *         content: <Catalogo />, canContinue: Boolean(sel) },
 *       { id: 'listo', title: 'Listo', content: <Exito />, done: true },
 *     ]}
 *   />
 *
 * Un paso con `onNext` ejecuta SU acción en vez de avanzar (p. ej. «Activar», que
 * llama a la API y luego mueve el paso). Un paso `done` es terminal: sin barra de
 * acciones y sin vuelta atrás (la acción ya no es reversible desde aquí).
 */
import type { ComponentChildren, JSX } from 'preact';
import { createPortal } from 'preact/compat';
import { useEffect, useRef } from 'preact/hooks';
import { useT } from '@sytue/i18n/preact';
import { cx } from '../lib/cx.js';
import { Button } from './Button.js';
import { Icon } from './Icon.js';

export interface WizardStep {
  id: string;
  /** Etiqueta corta en el progreso («Configurar»). */
  title: string;
  /** Encabezado del paso en lenguaje llano; por defecto, `title`. */
  heading?: string;
  /** Una frase que explica qué se hace aquí. */
  description?: string;
  content: ComponentChildren;
  /** false deshabilita el avance (validación del paso). Por defecto true. */
  canContinue?: boolean;
  /** Etiqueta del botón de avance. Por defecto «Continuar». */
  nextLabel?: string;
  /** Acción propia del paso en vez de avanzar (p. ej. aplicar los cambios). */
  onNext?: () => void | Promise<void>;
  /** Paso terminal: sin barra de acciones ni vuelta atrás. */
  done?: boolean;
}

/**
 * Textos del armazón del asistente. Van por props porque `@sytue/ui` es el design
 * system: no puede importar el catálogo de ninguna app, y dejarlos fijos en
 * castellano —como estaban— dejaba a TODOS los asistentes de la suite en un solo
 * idioma por mucho que la app tradujera sus pasos.
 *
 * Los valores por defecto siguen siendo los de antes, así que ningún asistente
 * existente cambia hasta que le pasen los suyos.
 */
export interface WizardLabels {
  /** Botón de retroceso. */
  back?: string;
  /** Botón de avance cuando el paso no trae `nextLabel`. */
  next?: string;
  cancel?: string;
  /** Nombre accesible de la barra de progreso. */
  progress?: string;
  /** Enlace a un paso ya completado; recibe el número. */
  goToStep?: (n: number) => string;
  /** Posición del paso para lector de pantalla; recibe número y total. */
  stepOf?: (n: number, total: number) => string;
}

/**
 * Textos del cromo del asistente, TRADUCIDOS.
 *
 * Estaban en español fijo y ningún asistente pasaba `labels`, así que en una
 * consola en inglés, francés, alemán o italiano los tres botones del pie salían
 * en español. Salen del catálogo del sistema (`ui.wizard.*`) por la misma razón
 * que los de `Spinner` o `Modal`: el asistente es un componente del design
 * system, no de una app, y traducirlo en cada app sería garantizar que alguna se
 * queda sin traducir.
 */
function useDefaultLabels(): Required<WizardLabels> {
  const t = useT();
  return {
    back: t('ui.wizard.back'),
    next: t('ui.wizard.next'),
    cancel: t('ui.wizard.cancel'),
    progress: t('ui.wizard.progress'),
    goToStep: (n) => t('ui.wizard.goToStep', { n }),
    stepOf: (n, total) => t('ui.wizard.stepOf', { n, total }),
  };
}

export interface WizardProps {
  steps: WizardStep[];
  /** Paso actual (controlado). */
  current: number;
  onCurrentChange: (index: number) => void;
  /** Bloquea las acciones y muestra el avance en curso. */
  busy?: boolean;
  /** Si se pasa, aparece «Cancelar» a la izquierda. */
  onCancel?: () => void;
  /** @deprecated Usar `labels.cancel`. */
  cancelLabel?: string;
  /** Textos del armazón (atrás, continuar, progreso…). Ver WizardLabels. */
  labels?: WizardLabels;
  /** Place actions in a Modal footer so they remain reachable above the mobile keyboard. */
  footerTarget?: Element | null;
  class?: string;
}

export interface StepperProps {
  steps: { id: string; title: string }[];
  current: number;
  /** Volver a un paso ya completado. Si se omite, el progreso no es interactivo. */
  onStepClick?: (index: number) => void;
  /** Textos accesibles (progreso, «paso N de M»…). Ver WizardLabels. */
  labels?: WizardLabels;
  class?: string;
}

const focusRing = ' sytue-focus-ring-inset';

/** Progreso de un flujo por pasos. Reutilizable fuera del Wizard. */
export function Stepper({
  steps,
  current,
  onStepClick,
  labels,
  class: cls,
}: StepperProps): JSX.Element {
  const l = { ...useDefaultLabels(), ...labels };
  return (
    <ol class={cx('flex items-center gap-2', cls)} aria-label={l.progress}>
      {steps.map((s, i) => {
        const state = i < current ? 'done' : i === current ? 'current' : 'todo';
        const clickable = state === 'done' && Boolean(onStepClick);
        const marker = (
          <>
            <span
              aria-hidden="true"
              class={cx(
                'grid h-6 w-6 shrink-0 place-items-center rounded-full text-xs font-semibold',
                state === 'done'
                  ? 'bg-primary text-[var(--sytue-on-primary)]'
                  : state === 'current'
                    ? 'border-primary text-primary-text border-2'
                    : 'border-border-strong text-text-subtle border',
              )}
            >
              {state === 'done' ? <Icon name="check" size={14} /> : i + 1}
            </span>
            <span
              class={cx(
                'truncate text-sm',
                state === 'current'
                  ? 'text-text font-semibold'
                  : state === 'done'
                    ? 'text-text-muted'
                    : 'text-text-subtle',
                // En pantallas estrechas no se leen las etiquetas: la del paso actual se
                // quedaba en 6 px y asomaba un glifo suelto («¿», «P»), y el título del
                // paso ya va debajo, en el `<h2>`. El lector la sigue oyendo.
                state === 'current' ? 'max-sm:sr-only' : 'hidden sm:inline',
              )}
            >
              {s.title}
            </span>
          </>
        );
        return (
          <li key={s.id} class="flex min-w-0 flex-1 items-center gap-2 last:flex-none">
            {clickable ? (
              <button
                type="button"
                class={cx(
                  'rounded-control flex min-w-0 items-center gap-2 px-1 py-0.5',
                  // «Volver al paso N» con el dedo: 44, con el círculo en el centro.
                  'pointer-coarse:min-h-[max(44px,2.75rem)] pointer-coarse:min-w-[max(44px,2.75rem)] pointer-coarse:justify-center',
                  'hover:text-text transition-colors duration-150',
                  focusRing,
                )}
                onClick={() => onStepClick?.(i)}
              >
                <span class="sr-only">{l.goToStep(i + 1)}</span>
                {marker}
              </button>
            ) : (
              <span
                class="flex min-w-0 items-center gap-2 px-1 py-0.5"
                aria-current={state === 'current' ? 'step' : undefined}
              >
                {state === 'current' ? (
                  <span class="sr-only">{l.stepOf(i + 1, steps.length)}</span>
                ) : null}
                {marker}
              </span>
            )}
            {i < steps.length - 1 ? (
              <span
                aria-hidden="true"
                // `max-sm:min-w-2`: con cinco pasos y sus blancos de 44, los filetes de 16
                // sacaban el indicador 9 px del diálogo del teléfono.
                class={cx(
                  'h-px min-w-4 flex-1 max-sm:min-w-2',
                  i < current ? 'bg-primary' : 'bg-border',
                )}
              />
            ) : null}
          </li>
        );
      })}
    </ol>
  );
}

export function Wizard({
  steps,
  current,
  onCurrentChange,
  busy = false,
  onCancel,
  cancelLabel,
  labels,
  footerTarget,
  class: cls,
}: WizardProps): JSX.Element | null {
  // cancelLabel sigue ganando si alguien lo pasaba: era la única de estas seis
  // cadenas que ya se podía traducir, y romperla no aportaría nada.
  const l = { ...useDefaultLabels(), ...labels, ...(cancelLabel ? { cancel: cancelLabel } : {}) };
  const headingRef = useRef<HTMLHeadingElement>(null);
  const prevStepRef = useRef(current);

  // Foco al encabezado SOLO al cambiar de paso: en el montaje inicial robaría el
  // foco a quien ya estuviera navegando la página.
  useEffect(() => {
    if (prevStepRef.current !== current) headingRef.current?.focus();
    prevStepRef.current = current;
  }, [current]);

  const step = steps[current];
  if (!step) return null;
  const terminal = step.done === true;

  const advance = () => {
    if (step.onNext) void step.onNext();
    else if (current < steps.length - 1) onCurrentChange(current + 1);
  };
  const actions = terminal ? null : (
    <div
      class={cx(
        'flex flex-wrap items-center gap-2',
        // En el pie de un `Modal` ocupa la fila entera («Cancelar» a la izquierda y
        // «Atrás / Continuar» a la derecha) y sus botones miden 44 en el teléfono, como
        // el resto de pies de la suite.
        footerTarget ? 'w-full max-md:[&_button]:min-h-11' : 'border-border-subtle border-t pt-4',
      )}
    >
      {onCancel ? (
        <Button variant="ghost" onClick={onCancel} disabled={busy}>
          {l.cancel}
        </Button>
      ) : null}
      <div class="ml-auto flex max-w-full flex-wrap items-center justify-end gap-2">
        {current > 0 ? (
          <Button
            variant="secondary"
            disabled={busy}
            leadingIcon={<Icon name="chevronLeft" size={16} />}
            onClick={() => onCurrentChange(current - 1)}
          >
            {l.back}
          </Button>
        ) : null}
        <Button
          variant="primary"
          loading={busy}
          disabled={busy || step.canContinue === false}
          onClick={advance}
        >
          {step.nextLabel ?? l.next}
        </Button>
      </div>
    </div>
  );

  return (
    <div class={cx('flex flex-col gap-6', cls)}>
      <Stepper
        steps={steps}
        current={current}
        {...(terminal || busy ? {} : { onStepClick: onCurrentChange })}
        {...(labels ? { labels } : {})}
      />

      <div>
        <h2
          ref={headingRef}
          tabIndex={-1}
          class="sytue-h3 text-text scroll-mt-4 focus-visible:outline-none"
        >
          {step.heading ?? step.title}
        </h2>
        {step.description ? (
          <p class="text-text-muted mt-1.5 max-w-2xl text-base">{step.description}</p>
        ) : null}
      </div>

      <div class="min-w-0">{step.content}</div>

      {footerTarget ? createPortal(actions, footerTarget) : actions}
    </div>
  );
}
