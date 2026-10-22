/**
 * Utilidades de configuración del fondo del lienzo (canvas) en Sytue Fotos.
 *
 * Al editar o visualizar imágenes con transparencias (PNG, SVG, etc.), los elementos
 * oscuros (p. ej. texto negro) quedan invisibles sobre fondos oscuros, y los elementos
 * claros sobre fondos blancos.
 *
 * Permite alternar entre cuatro opciones:
 * - 'crema': Tono marfil/crema cálido característico de Sytue (#faf9f6), óptimo para logotipos oscuros.
 * - 'blanco': Blanco puro (#ffffff).
 * - 'oscuro': Grafito oscuro (#131619), idóneo para logotipos o siluetas blancas.
 * - 'damero': Patrón de cuadrícula ajedrezada para inspeccionar transparencias reales.
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
 * Usamos colores directos garantizados para que el modo 'crema' siempre se vea crema cálido
 * incluso si el sistema o la aplicación están en modo oscuro (donde var(--sytue-bg) cambiaría a grafito).
 */
export function estilosFondoLienzo(fondo: FondoLienzo): EstiloFondo {
  switch (fondo) {
    case 'blanco':
      return {
        className: 'bg-white',
        style: { backgroundColor: '#ffffff' },
      };
    case 'oscuro':
      return {
        className: 'bg-[#131619]',
        style: { backgroundColor: '#131619' },
      };
    case 'damero':
      return {
        className: 'bg-white',
        style: {
          backgroundColor: '#ffffff',
          backgroundImage:
            'repeating-conic-gradient(rgba(180, 180, 180, 0.35) 0% 25%, transparent 0% 50%)',
          backgroundSize: '16px 16px',
        },
      };
    case 'crema':
    default:
      return {
        className: 'bg-[#faf9f6]',
        style: { backgroundColor: '#faf9f6' },
      };
  }
}
