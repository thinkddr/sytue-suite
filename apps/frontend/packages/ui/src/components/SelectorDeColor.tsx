/**
 * SelectorDeColor — el selector de color de usuario de toda la suite: un
 * `radiogroup` con un radio por tono, tabulación itinerante (una sola parada de
 * Tab) y flechas para moverse, como pide el patrón de radios de WAI-ARIA.
 *
 * Sale de la receta del calendario y sustituye a seis selectores con tres modelos
 * distintos (`aria-pressed`, `radio`, botones sueltos). Lo que fija:
 *
 * - El nombre accesible es SIEMPRE el nombre traducido del tono (`{ valor, nombre }`),
 *   nunca el hex: un lector que dice «almohadilla b0413a» no ayuda a nadie.
 * - El elegido lleva anillo y visto: el estado no se comunica solo por color.
 * - Contorno en todas las muestras: sobre la superficie elevada del tema oscuro
 *   ningún tono de las paletas llega a 3:1 y sin borde la muestra no tiene forma.
 * - Con puntero grueso, cada muestra es un blanco de 44 px y el paso entre ellas
 *   también, sin agrandar el círculo (las filas de etiquetas no caben con 44).
 *
 * Cada app conserva SU paleta: se pasa por `colores`. Un `valor` vacío es «sin
 * color» (heredar), que se pinta con borde discontinuo. `pintar` traduce el valor
 * a un color CSS cuando la app guarda tokens en vez de hex (el correo).
 */
import type { JSX } from 'preact';
import { useId, useRef } from 'preact/hooks';
import { cx } from '../lib/cx.js';
import { Icon } from './Icon.js';

export interface ColorDeSelector {
  /** Lo que se guarda: un hex, un token de la app o `''` (sin color). */
  valor: string;
  /** Nombre del tono ya traducido: es el nombre accesible y el `title`. */
  nombre: string;
}

export interface SelectorDeColorProps {
  colores: ReadonlyArray<ColorDeSelector>;
  /** Valor elegido. Si no está en la paleta, no se marca ninguno. */
  valor: string;
  onChange: (valor: string) => void;
  /** Nombre del grupo cuando no hay una etiqueta visible a la que apuntar. */
  'aria-label'?: string | undefined;
  /** Id de la etiqueta visible del grupo (leyenda, rótulo del campo). */
  etiquetadoPor?: string | undefined;
  /** `sm` (24 px) para filas densas; `md` (28 px) por defecto. */
  tamano?: 'sm' | 'md';
  /** Traduce el valor a color CSS; por defecto el propio valor. */
  pintar?: (valor: string) => string;
  class?: string | undefined;
}

function iguales(a: string, b: string): boolean {
  return a.toLowerCase() === b.toLowerCase();
}

export function SelectorDeColor({
  colores,
  valor,
  onChange,
  'aria-label': ariaLabel,
  etiquetadoPor,
  tamano = 'md',
  pintar = (v) => v,
  class: cls,
}: SelectorDeColorProps): JSX.Element {
  const base = useId();
  const grupo = useRef<HTMLDivElement>(null);
  const elegido = colores.findIndex((c) => iguales(c.valor, valor));
  // Color fuera de paleta: la primera muestra recibe la parada de Tab.
  const parada = elegido < 0 ? 0 : elegido;

  function onKeyDown(e: KeyboardEvent): void {
    // Con modificador la tecla es del navegador o del sistema (Alt+← es «atrás»,
    // Cmd+← es inicio de línea): no se roba ni cambia el color por sorpresa.
    if (!colores.length || e.altKey || e.ctrlKey || e.metaKey) return;
    let destino = -1;
    if (e.key === 'ArrowRight' || e.key === 'ArrowDown') destino = (parada + 1) % colores.length;
    else if (e.key === 'ArrowLeft' || e.key === 'ArrowUp')
      destino = (parada - 1 + colores.length) % colores.length;
    else if (e.key === 'Home') destino = 0;
    else if (e.key === 'End') destino = colores.length - 1;
    if (destino < 0) return;
    e.preventDefault();
    onChange(colores[destino]!.valor);
    grupo.current?.querySelectorAll<HTMLButtonElement>('[role="radio"]')[destino]?.focus();
  }

  return (
    <div
      ref={grupo}
      role="radiogroup"
      // Focusable solo por programa: la parada de Tab es el radio elegido.
      tabIndex={-1}
      {...(ariaLabel ? { 'aria-label': ariaLabel } : {})}
      {...(etiquetadoPor ? { 'aria-labelledby': etiquetadoPor } : {})}
      // Con el dedo, el paso entre centros también es de 44: los blancos se tocan sin pisarse.
      class={cx(
        'flex flex-wrap',
        tamano === 'sm' ? 'gap-1.5 pointer-coarse:gap-5' : 'gap-2 pointer-coarse:gap-4',
        cls,
      )}
      onKeyDown={onKeyDown}
    >
      {colores.map((c, i) => {
        const marcado = i === elegido;
        const sinColor = c.valor === '';
        return (
          <button
            key={c.valor || 'none'}
            id={`${base}-${i}`}
            type="button"
            role="radio"
            aria-checked={marcado}
            aria-label={c.nombre}
            title={c.nombre}
            tabIndex={i === parada ? 0 : -1}
            onClick={() => onChange(c.valor)}
            class={cx(
              'relative grid shrink-0 place-items-center rounded-full transition-shadow duration-150 motion-reduce:transition-none',
              tamano === 'sm' ? 'h-6 w-6' : 'h-7 w-7',
              // Blanco de 44 con el dedo: el círculo más lo que asoma por cada lado.
              tamano === 'sm'
                ? "pointer-coarse:after:absolute pointer-coarse:after:-inset-2.5 pointer-coarse:after:content-['']"
                : "pointer-coarse:after:absolute pointer-coarse:after:-inset-2 pointer-coarse:after:content-['']",
              'sytue-focus-ring-inset border-border border',
              sinColor && 'border-dashed',
              marcado &&
                'shadow-[0_0_0_2px_var(--sytue-surface),0_0_0_4px_var(--sytue-border-strong)]',
            )}
            style={sinColor ? undefined : { backgroundColor: pintar(c.valor) }}
          >
            {marcado ? (
              <Icon name="check" size={14} class={sinColor ? 'text-text' : 'text-neutral-0'} />
            ) : null}
          </button>
        );
      })}
    </div>
  );
}
