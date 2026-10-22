/**
 * Form fields — Input, Textarea, Select, Checkbox, Radio, Switch y FieldGroup.
 * Label asociado, aria-invalid, aria-describedby; el error se comunica con
 * texto + icono (no solo color). docs/design.md §9.
 */
import type { ComponentChildren, JSX } from 'preact';
import { useId } from 'preact/hooks';
import { useT } from '@sytue/i18n/preact';
import { cx } from '../lib/cx.js';
import { Icon } from './Icon.js';

// `text-ellipsis`: un `placeholder` (o un valor sin foco) que no cabe acaba en «…» y no
// cortado a tijera, que parecía el ejemplo entero (el asistente de Calendar en 390).
const controlBase =
  'w-full rounded-md border bg-surface text-text text-ellipsis placeholder:text-text-subtle ' +
  'transition-colors duration-150 focus-visible:outline-none ' +
  'disabled:opacity-50 disabled:pointer-events-none';

/**
 * El foco de un campo es el aro CANÓNICO, ni uno propio ni más grueso.
 *
 * Los campos se escribían el suyo a 3px de latón + 2px de contorno y ADEMÁS
 * pasaban el borde a `border-primary`: como borde y aro eran el mismo latón, se
 * fundían en un bloque macizo de 4px con otros 2px de contorno alrededor —6px de
 * cromo rodeando una caja de texto, el control más recargado de la suite—. El
 * borde se queda en su gris: el aro YA es la señal, y dejarlos de colores
 * distintos es lo que hace que se lea como un aro fino y no como un marco.
 */
// El aro es el azul de foco TAMBIÉN en un campo con error, y no rojo: `--sytue-danger` se
// queda en 2,65:1 sobre el cromo del tema oscuro, así que un aro de ese color
// sería un indicador de foco que incumple WCAG 1.4.11 justo donde más falta hace.
// El error ya se señala por partida doble —borde rojo y mensaje—, y el aro dice
// otra cosa: dónde está el teclado.
const controlState = (invalid?: boolean) =>
  invalid
    ? 'border-danger sytue-focus-ring'
    : 'border-border-strong hover:border-text-subtle sytue-focus-ring';

// ---- FieldGroup: label + ayuda + error en torno a un control ----

export interface FieldGroupProps {
  label: string;
  /** id del control que envuelve (se asocia con htmlFor). */
  htmlFor: string;
  help?: string;
  error?: string;
  required?: boolean;
  children: ComponentChildren;
  class?: string;
}

export function FieldGroup({
  label,
  htmlFor,
  help,
  error,
  required,
  children,
  class: cls,
}: FieldGroupProps): JSX.Element {
  const t = useT();
  const helpId = `${htmlFor}-help`;
  const errorId = `${htmlFor}-error`;
  return (
    <div class={cx('flex flex-col gap-1.5', cls)}>
      <label for={htmlFor} class="text-text text-sm font-semibold">
        {label}
        {required ? (
          <>
            <span class="text-danger-text ml-0.5" aria-hidden="true">
              *
            </span>
            {/* Se anunciaba EN CASTELLANO en toda la suite, pidieras el idioma que
                pidieras: era el unico rotulo del componente que no pasaba por `t`. */}
            <span class="sr-only"> {t('ui.required')}</span>
          </>
        ) : null}
      </label>
      {children}
      {help && !error ? (
        <p id={helpId} class="text-text-muted text-xs">
          {help}
        </p>
      ) : null}
      {error ? (
        <p id={errorId} class="text-danger-text flex items-center gap-1 text-xs" role="alert">
          <Icon name="danger" size={14} />
          {error}
        </p>
      ) : null}
    </div>
  );
}

/** Helper para enlazar aria-describedby de un control con su ayuda/error. */
export function describedBy(id: string, help?: string, error?: string): string | undefined {
  if (error) return `${id}-error`;
  if (help) return `${id}-help`;
  return undefined;
}

// ---- Input ----

/**
 * `md` (31,5 px) es el control de formulario; `sm` (24 px) el de cromo denso —cinta del
 * editor, barras de propiedades—, donde un `h-9` no cabe.
 *
 * Los 24 los pone `min-h-[24px]`, no `h-6`: con la raíz de la suite a 14 px, `h-6` son
 * 21. Así medía el filtro de notas de Notepads en producción (182×21, 2026-09-21), al
 * lado de botones de 28. Es el mismo suelo que `Button size="xs"` e `IconButton sm`.
 *
 * Sin esta talla, cada panel se escribía la suya: nueve `inputCls` locales con CUATRO
 * geometrías distintas, siete de ellas en el mismo fichero. Y como cada copia repetía
 * también el anillo de foco, cada una podía perderlo por su cuenta.
 *
 * Con el DEDO, `md` mide 44 (`pointer-coarse:min-h-[max(44px,2.75rem)]`), el mismo suelo
 * que `Button`: medido en el barrido de diálogos (2026-10-16), los campos se quedaban en
 * 36 al lado de botones de 44 en todas las hojas con formulario, y eran el blanco más
 * pequeño de cada una. `sm` es cromo denso y se queda en 24.
 */
export type InputSize = 'sm' | 'md';

export interface InputProps extends Omit<JSX.InputHTMLAttributes<HTMLInputElement>, 'size'> {
  invalid?: boolean;
  leadingIcon?: JSX.Element | undefined;
  size?: InputSize;
}

/** Geometría por talla: alto, relleno y cuerpo de letra van juntos o no cuadra. */
const INPUT_SIZE: Record<InputSize, { plain: string; withIcon: string }> = {
  sm: {
    plain: 'h-6 min-h-[24px] px-1.5 text-xs',
    withIcon: 'h-6 min-h-[24px] pr-1.5 pl-7 text-xs',
  },
  md: {
    plain: 'h-9 px-3 text-base pointer-coarse:min-h-[max(44px,2.75rem)]',
    withIcon: 'h-9 pr-3 pl-9 text-base pointer-coarse:min-h-[max(44px,2.75rem)]',
  },
};

/**
 * Colapsa el doble relleno de los gestores de contraseñas en los campos de correo.
 *
 * Algunos escriben la dirección y, al ver que el valor parece un correo, la
 * escriben otra vez SIN limpiar: el campo acaba con «correo@dominio
 * correo@dominio». Pasó en el acceso de Sytue ID —que lo corrige igual, en
 * `services/id/internal/http/login.go`— y aquí vale para los doce campos de
 * correo de la suite sin tocarlos uno a uno.
 *
 * Solo actúa si los trozos son IDÉNTICOS, que es la firma inequívoca del doble
 * relleno: colapsar cualquier espacio le borraría caracteres a quien esté
 * tecleando. Y corrige el elemento ANTES de llamar al manejador de quien usa el
 * componente, para que un campo controlado reciba ya el valor bueno y no haya
 * que acordarse de esto en cada formulario.
 */
function sinDobleRelleno(
  handler: JSX.InputEventHandler<HTMLInputElement> | undefined,
): JSX.InputEventHandler<HTMLInputElement> {
  return function (this: HTMLInputElement, e) {
    const el = e.currentTarget;
    const [primero, ...resto] = el.value.split(/\s+/).filter(Boolean);
    if (primero !== undefined && resto.length > 0 && resto.every((t) => t === primero)) {
      el.value = primero;
    }
    handler?.call(this, e);
  };
}

export function Input({
  invalid,
  leadingIcon,
  size = 'md',
  class: cls,
  ...rest
}: InputProps): JSX.Element {
  if (rest.type === 'email') {
    rest = { ...rest, onInput: sinDobleRelleno(rest.onInput) };
  }
  if (leadingIcon) {
    return (
      // `min-w-0`: este div es el que participa en el flex del padre, y sin
      // poder encoger su mínimo es el ancho intrínseco del input (size=20 chars
      // + relleno), que en alemán no cabe en un panel de 320px y empuja el
      // formulario — y con él la lista— fuera de la pantalla. El input de dentro
      // lleva `w-full` (controlBase) y sigue al div cuando este encoge.
      <div class="relative min-w-0">
        <span
          class={cx(
            'text-primary-text pointer-events-none absolute top-1/2 -translate-y-1/2',
            size === 'sm' ? 'left-1.5' : 'left-3',
          )}
        >
          {leadingIcon}
        </span>
        <input
          class={cx(controlBase, controlState(invalid), INPUT_SIZE[size].withIcon, cls)}
          aria-invalid={invalid ? 'true' : undefined}
          {...rest}
        />
      </div>
    );
  }
  return (
    <input
      class={cx(controlBase, controlState(invalid), INPUT_SIZE[size].plain, cls)}
      aria-invalid={invalid ? 'true' : undefined}
      {...rest}
    />
  );
}

// ---- Textarea ----

export interface TextareaProps extends JSX.TextareaHTMLAttributes<HTMLTextAreaElement> {
  invalid?: boolean;
}

export function Textarea({ invalid, class: cls, rows = 4, ...rest }: TextareaProps): JSX.Element {
  return (
    <textarea
      rows={rows}
      class={cx(controlBase, controlState(invalid), 'resize-y px-3 py-2 text-base', cls)}
      aria-invalid={invalid ? 'true' : undefined}
      {...rest}
    />
  );
}

// ---- Select ----

export interface SelectProps extends Omit<JSX.SelectHTMLAttributes<HTMLSelectElement>, 'size'> {
  invalid?: boolean;
  /** Misma escala que `Input`: `sm` para cromo denso, `md` para formulario. */
  size?: InputSize;
}

/** El chevron encoge y se acerca al borde con la talla, o se come el texto. */
const SELECT_SIZE: Record<InputSize, { box: string; chevron: number; right: string }> = {
  sm: {
    box: 'h-6 min-h-[24px] appearance-none pr-5 pl-1.5 text-xs',
    chevron: 12,
    right: 'right-1',
  },
  md: {
    box: 'h-9 appearance-none pr-9 pl-3 text-base pointer-coarse:min-h-[max(44px,2.75rem)]',
    chevron: 16,
    right: 'right-3',
  },
};

export function Select({
  invalid,
  size = 'md',
  class: cls,
  children,
  ...rest
}: SelectProps): JSX.Element {
  return (
    <div class="relative">
      <select
        class={cx(controlBase, controlState(invalid), SELECT_SIZE[size].box, cls)}
        aria-invalid={invalid ? 'true' : undefined}
        {...rest}
      >
        {children}
      </select>
      <Icon
        name="chevronDown"
        size={SELECT_SIZE[size].chevron}
        class={cx(
          'text-primary-text pointer-events-none absolute top-1/2 -translate-y-1/2',
          SELECT_SIZE[size].right,
        )}
      />
    </div>
  );
}

// ---- Checkbox / Radio ----

export interface ChoiceProps extends Omit<JSX.InputHTMLAttributes<HTMLInputElement>, 'type'> {
  /**
   * Sin `label` sale SOLO el control —el aro y el `<input>`—, para la casilla que ya
   * tiene su etiqueta fuera (un `<label>` que la envuelve, un `for`) o que se nombra con
   * `aria-label`: la selección de una fila, el «seleccionar todo» de una cabecera. Así
   * `class` va al `<input>` y SUSTITUYE la talla por defecto, porque no hay etiqueta que
   * vestir. `indeterminate` va por prop: Preact la fija como propiedad del DOM, sin ref.
   */
  label?: string;
  /**
   * Clases del `<span>` que pinta el aro. Los márgenes van aquí y no en el `<input>`:
   * dentro del `<span>` lo agrandarían, y el aro se separaría de la casilla. Y sobre
   * cromo oscuro se cambia el color con `sytue-focus-ring-chrome`.
   */
  ringClass?: string;
}

const CHOICE_INPUT = 'border-border-strong accent-primary h-4 w-4';

function Choice(type: 'checkbox' | 'radio') {
  return function ChoiceControl({
    label,
    ringClass,
    class: cls,
    id,
    ...rest
  }: ChoiceProps): JSX.Element {
    const auto = useId();
    const fieldId = id ?? auto;
    /* El aro lo pinta este `<span>` y no el `<input>`: Chromium deja a 0 el
       `border-radius` de una casilla o un radio nativos, así que su `box-shadow`
       es un cuadrado. El `<span>` lleva la esquina que Chromium pinta —2px la
       casilla, un círculo el radio— y el aro la sigue. */
    const control = (input: ChoiceProps['class']) => (
      <span
        class={cx(
          'sytue-focus-ring-choice',
          type === 'checkbox' ? 'rounded-[2px]' : 'rounded-full',
          ringClass,
        )}
      >
        <input id={label ? fieldId : id} type={type} class={input} {...rest} />
      </span>
    );
    if (!label) return control(cls ?? CHOICE_INPUT);
    return (
      <label
        for={fieldId}
        class={cx(
          'inline-flex cursor-pointer items-center gap-2 text-base',
          // Con el dedo, la ETIQUETA es el blanco: 44 de alto, como un botón. La casilla
          // de 16 sola dejaba 22 (medido en el barrido de diálogos, 2026-10-16).
          'pointer-coarse:min-h-[max(44px,2.75rem)]',
          // Disabled coherente: la etiqueta acompaña al control atenuado.
          'has-[:disabled]:cursor-not-allowed has-[:disabled]:opacity-50',
          cls,
        )}
      >
        {control(CHOICE_INPUT)}
        <span class="text-text">{label}</span>
      </label>
    );
  };
}

export const Checkbox = Choice('checkbox');
export const Radio = Choice('radio');

// ---- Switch ----

export interface SwitchProps extends Omit<
  JSX.InputHTMLAttributes<HTMLInputElement>,
  'type' | 'label'
> {
  label: string;
  checked?: boolean;
}

export function Switch({ label, checked, class: cls, id, ...rest }: SwitchProps): JSX.Element {
  const auto = useId();
  const fieldId = id ?? auto;
  return (
    <label
      for={fieldId}
      class={cx(
        'inline-flex cursor-pointer items-center gap-2.5 text-base',
        // Con el dedo, 44 de alto, como un botón (el carril solo daba 22).
        'pointer-coarse:min-h-[max(44px,2.75rem)]',
        'has-[:disabled]:cursor-not-allowed has-[:disabled]:opacity-50',
        cls,
      )}
    >
      <span class="relative inline-flex">
        <input
          id={fieldId}
          type="checkbox"
          // El checkbox nativo ya expone el estado por `checked`: añadir `aria-checked`
          // a mano lo duplicaría y podría desincronizarse. oxlint no contempla esa
          // excepción del control nativo, que jsx-a11y sí traía de fábrica.
          // oxlint-disable-next-line jsx-a11y/role-has-required-aria-props
          role="switch"
          checked={checked}
          class="peer sr-only"
          {...rest}
        />
        {/* El carril APAGADO es un componente de interfaz: WCAG 1.4.11 le exige 3:1
            contra el fondo.

            El contraste lo pone el BORDE y no el relleno, que es como lo resuelven
            Apple y GitHub: un carril relleno de gris oscuro parecería encendido, y
            entonces el estado se leería al revés. `neutral-500` da 4,54:1 sobre marfil.

            Y por eso el relleno es `bg-subtle` y ya no `border-strong`: aquel era un
            token de CONTORNO usado como relleno, y el 2026-09-21 subió al 54 % de tinta
            para cumplir ese mismo 3:1 como contorno. Dejarlo aquí habría pintado el
            carril apagado de gris medio, o sea, encendido. */}
        <span
          class={cx(
            'rounded-pill bg-bg-subtle h-5 w-9 ring-1 ring-neutral-500 transition-colors duration-150',
            'peer-checked:bg-primary peer-checked:ring-transparent',
            // A mano y no `sytue-focus-ring`: quien recibe el foco es el input
            // hermano, y el aro se pinta aquí. Mismo grosor que el canónico (2+1).
            'peer-focus-visible:shadow-[0_0_0_2px_var(--sytue-focus-ring)]',
          )}
          aria-hidden="true"
        />
        {/* El pomo sale de `surface`, no de `neutral-0`: en tema oscuro el blanco fijo
            no invertía y quedaba un círculo blanco sobre un carril oscuro. */}
        <span
          class={cx(
            'bg-surface absolute top-0.5 left-0.5 h-4 w-4 rounded-full shadow-[var(--shadow-xs)]',
            'transition-transform duration-150 peer-checked:translate-x-4',
          )}
          aria-hidden="true"
        />
      </span>
      <span class="text-text">{label}</span>
    </label>
  );
}
