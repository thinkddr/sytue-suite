/**
 * Utilidades de configuración del fondo del lienzo (canvas) en Sytue Fotos.
 *
 * Al editar o visualizar imágenes con transparencias (PNG, SVG, etc.), los elementos
 * oscuros (p. ej. texto negro) quedan invisibles sobre fondos oscuros, y los elementos
 * claros sobre fondos blancos.
 *
 * Permite alternar entre cuatro opciones, todas de la escala de neutros de la paleta
 * (`--sytue-neutral-*`), que lleva un punto del color de la suite en vigor:
 * - 'crema': el lienzo claro de la suite (neutral-100), óptimo para logotipos oscuros. El
 *   identificador se queda por compatibilidad con la preferencia guardada; el rótulo
 *   ya dice «Claro»: el crema era de la paleta antigua.
 * - 'blanco': blanco puro (neutral-0).
 * - 'oscuro': la superficie oscura de la suite (neutral-900), para siluetas blancas.
 * - 'damero': cuadrícula ajedrezada (neutral-200 sobre blanco) para ver transparencias.
 */

export type FondoLienzo = 'crema' | 'blanco' | 'oscuro' | 'damero';

export const FONDOS_DISPONIBLES: readonly FondoLienzo[] = [
  'crema',
  'blanco',
  'oscuro',
  'damero',
] as const;

export const CLAVE_STORAGE_FONDO = 'sytue_photos_canvas_bg';

export const FONDO_POR_DEFECTO: FondoLienzo = 'crema';

/** Lee la preferencia guardada en localStorage o devuelve el valor por defecto ('crema'). */
export function obtenerFondoLienzo(): FondoLienzo {
  if (typeof localStorage === 'undefined') return FONDO_POR_DEFECTO;
  try {
    const val = localStorage.getItem(CLAVE_STORAGE_FONDO);
    if (val && (FONDOS_DISPONIBLES as readonly string[]).includes(val)) {
      return val as FondoLienzo;
    }
  } catch {
    // Modo incógnito o sin permisos de storage
  }
  return FONDO_POR_DEFECTO;
}

/** Guarda la preferencia del fondo del lienzo en localStorage. */
export function guardarFondoLienzo(fondo: FondoLienzo): void {
  if (typeof localStorage === 'undefined') return;
  try {
    localStorage.setItem(CLAVE_STORAGE_FONDO, fondo);
  } catch {
    // No-op
  }
}

export interface EstiloFondo {
  className: string;
  style: Record<string, string>;
}

/**
 * Devuelve las propiedades CSS y clases correspondientes a cada modo de fondo.
 *
 * Son escalones de la escala de neutros y no roles de tema (`bg`, `surface`…) a
 * propósito: los neutros no cambian con el modo, así que el fondo claro sigue claro
 * aunque la suite esté en oscuro, que es justo lo que se elige aquí. Sí siguen al color
 * de la suite (petróleo, cobalto…), como el resto de la interfaz.
 */
export function estilosFondoLienzo(fondo: FondoLienzo): EstiloFondo {
  switch (fondo) {
    case 'blanco':
      return {
        className: 'bg-neutral-0',
        style: { backgroundColor: 'var(--sytue-neutral-0)' },
      };
    case 'oscuro':
      return {
        className: 'bg-neutral-900',
        style: { backgroundColor: 'var(--sytue-neutral-900)' },
      };
    case 'damero':
      return {
        className: 'bg-neutral-0',
        style: {
          backgroundColor: 'var(--sytue-neutral-0)',
          backgroundImage:
            'repeating-conic-gradient(var(--sytue-neutral-200) 0% 25%, transparent 0% 50%)',
          backgroundSize: '16px 16px',
        },
      };
    case 'crema':
    default:
      return {
        className: 'bg-neutral-100',
        style: { backgroundColor: 'var(--sytue-neutral-100)' },
      };
  }
}

/**
 * Las clases de color de la muestra redonda de cada fondo (el menú del visor y el
 * panel del editor): el mismo relleno que el lienzo y un borde que la despega del menú.
 * Los cuadros del damero son un escalón más oscuros que los del lienzo: con baldosas de
 * 6 px en un círculo de 12, los del lienzo apenas se verían.
 */
export function clasesMuestraFondo(fondo: FondoLienzo): string {
  switch (fondo) {
    case 'blanco':
      return 'border-neutral-300 bg-neutral-0';
    case 'oscuro':
      return 'border-neutral-700 bg-neutral-900';
    case 'damero':
      return 'border-neutral-300 bg-neutral-0 bg-[repeating-conic-gradient(var(--sytue-neutral-300)_0%_25%,transparent_0%_50%)] bg-[length:6px_6px]';
    case 'crema':
    default:
      return 'border-neutral-300 bg-neutral-100';
  }
}
