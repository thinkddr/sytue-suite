/**
 * MobileFab — la acción principal de la app al alcance del pulgar en <768px.
 *
 * En escritorio no existe: ahí la lleva el botón primario del menú lateral. En
 * móvil ese menú es un cajón cerrado, y «Redactar», «Crear evento» o «Nuevo
 * contacto» quedaban a dos toques y fuera de la vista, que es justo lo contrario
 * de lo que hace cualquier app de correo o agenda del teléfono.
 *
 * Va FUERA de `AppShell`: en <768px la shell esconde el contenido mientras enseña
 * la lista, y un botón dentro desaparecería con él. Es `fixed`, así que da igual
 * dónde se monte. docs/design.md §3.5 (primario) y §2.2 (responsive).
 */
import type { JSX } from 'preact';
import { cx } from '../lib/cx.js';

export interface MobileFabProps {
  /** Nombre accesible y tooltip: el botón es solo icono. */
  label: string;
  icon: JSX.Element;
  onClick: () => void;
  class?: string;
}

export function MobileFab({ label, icon, onClick, class: cls }: MobileFabProps): JSX.Element {
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      onClick={onClick}
      class={cx(
        // Los mismos colores que Button primary: es LA acción primaria, en otro sitio.
        'bg-primary text-[var(--sytue-on-primary)] hover:bg-[var(--sytue-primary-hover)] active:bg-[var(--sytue-primary-active)]',
        // 56px: el tamaño canónico del FAB, y por encima de los 44 del dedo.
        'rounded-control fixed right-[max(1rem,var(--sytue-safe-right,0px))] z-20 grid h-14 w-14 place-items-center',
        // Respeta la barra de gestos de iOS, que si no lo tapa a medias.
        'bottom-[max(1rem,var(--sytue-safe-bottom,0px))]',
        'shadow-[var(--shadow-lg)] transition-transform duration-150 active:scale-95',
        'sytue-focus-ring md:hidden',
        cls,
      )}
    >
      {icon}
    </button>
  );
}
