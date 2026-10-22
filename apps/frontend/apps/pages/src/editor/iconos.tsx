/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Iconos propios del editor visual (no se añaden a @sytue/ui para no forzar su inclusión en Fyne).
 * El dibujo es de Pages; el envoltorio —lienzo, grosor y relleno— es `Glifo`, el de toda la suite.
 */

import type { JSX } from 'preact';
import { Glifo } from '@sytue/ui';

interface IconoProps {
  size?: number;
  class?: string;
}

export function IconoDesktop({ size = 18, class: className = '' }: IconoProps): JSX.Element {
  return (
    <Glifo size={size} class={className}>
      <rect width="20" height="14" x="2" y="3" rx="2" />
      <line x1="8" x2="16" y1="21" y2="21" />
      <line x1="12" x2="12" y1="17" y2="21" />
    </Glifo>
  );
}

export function IconoTablet({ size = 18, class: className = '' }: IconoProps): JSX.Element {
  return (
    <Glifo size={size} class={className}>
      <rect width="16" height="20" x="4" y="2" rx="2" />
      <line x1="12" x2="12.01" y1="18" y2="18" />
    </Glifo>
  );
}

export function IconoMobile({ size = 18, class: className = '' }: IconoProps): JSX.Element {
  return (
    <Glifo size={size} class={className}>
      <rect width="14" height="20" x="5" y="2" rx="2" />
      <line x1="12" x2="12.01" y1="17" y2="17" />
    </Glifo>
  );
}

export function IconoDeshacer({ size = 18, class: className = '' }: IconoProps): JSX.Element {
  return (
    <Glifo size={size} class={className}>
      <path d="M3 7v6h6" />
      <path d="M21 17a9 9 0 0 0-9-9 9 9 0 0 0-6 2.3L3 13" />
    </Glifo>
  );
}

export function IconoRehacer({ size = 18, class: className = '' }: IconoProps): JSX.Element {
  return (
    <Glifo size={size} class={className}>
      <path d="M21 7v6h-6" />
      <path d="M3 17a9 9 0 0 1 9-9 9 9 0 0 1 6 2.3L21 13" />
    </Glifo>
  );
}

export function IconoZoomIn({ size = 18, class: className = '' }: IconoProps): JSX.Element {
  return (
    <Glifo size={size} class={className}>
      <circle cx="11" cy="11" r="8" />
      <line x1="21" x2="16.65" y1="21" y2="16.65" />
      <line x1="11" x2="11" y1="8" y2="14" />
      <line x1="8" x2="14" y1="11" y2="11" />
    </Glifo>
  );
}

export function IconoZoomOut({ size = 18, class: className = '' }: IconoProps): JSX.Element {
  return (
    <Glifo size={size} class={className}>
      <circle cx="11" cy="11" r="8" />
      <line x1="21" x2="16.65" y1="21" y2="16.65" />
      <line x1="8" x2="14" y1="11" y2="11" />
    </Glifo>
  );
}

export function IconoZoomFit({ size = 18, class: className = '' }: IconoProps): JSX.Element {
  return (
    <Glifo size={size} class={className}>
      <path d="M15 3h6v6" />
      <path d="M9 21H3v-6" />
      <path d="M21 3l-7 7" />
      <path d="M3 21l7-7" />
    </Glifo>
  );
}

export function IconoGuardado({ size = 16, class: className = '' }: IconoProps): JSX.Element {
  return (
    <Glifo size={size} class={className}>
      <polyline points="20 6 9 17 4 12" />
    </Glifo>
  );
}

export function IconoSpinner({ size = 16, class: className = '' }: IconoProps): JSX.Element {
  return (
    <Glifo size={size} class={`animate-spin ${className}`}>
      <path d="M21 12a9 9 0 1 1-6.219-8.56" />
    </Glifo>
  );
}

export function IconoAlerta({ size = 16, class: className = '' }: IconoProps): JSX.Element {
  return (
    <Glifo size={size} class={className}>
      <path d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3Z" />
      <line x1="12" x2="12" y1="9" y2="13" />
      <line x1="12" x2="12.01" y1="17" y2="17" />
    </Glifo>
  );
}

export function IconoOjo({ size = 18, class: className = '' }: IconoProps): JSX.Element {
  return (
    <Glifo size={size} class={className}>
      <path d="M2 12s3-7 10-7 10 7 10 7-3 7-10 7-10-7-10-7Z" />
      <circle cx="12" cy="12" r="3" />
    </Glifo>
  );
}

export function IconoCapas({ size = 18, class: className = '' }: IconoProps): JSX.Element {
  return (
    <Glifo size={size} class={className}>
      <polygon points="12 2 2 7 12 12 22 7 12 2" />
      <polyline points="2 17 12 22 22 17" />
      <polyline points="2 12 12 17 22 12" />
    </Glifo>
  );
}

export function IconoInsertar({ size = 18, class: className = '' }: IconoProps): JSX.Element {
  return (
    <Glifo size={size} class={className}>
      <line x1="12" y1="5" x2="12" y2="19" />
      <line x1="5" y1="12" x2="19" y2="12" />
    </Glifo>
  );
}

export function IconoPaginas({ size = 18, class: className = '' }: IconoProps): JSX.Element {
  return (
    <Glifo size={size} class={className}>
      <rect width="14" height="18" x="3" y="3" rx="2" />
      <path d="M7 7h6" />
      <path d="M7 11h6" />
      <path d="M7 15h4" />
      <path d="M17 7h4v14H7v-4" />
    </Glifo>
  );
}

export function IconoSitio({ size = 18, class: className = '' }: IconoProps): JSX.Element {
  return (
    <Glifo size={size} class={className}>
      <circle cx="12" cy="12" r="3" />
      <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z" />
    </Glifo>
  );
}

export function IconoBloqueado({ size = 16, class: className = '' }: IconoProps): JSX.Element {
  return (
    <Glifo size={size} class={className}>
      <rect width="18" height="11" x="3" y="11" rx="2" ry="2" />
      <path d="M7 11V7a5 5 0 0 1 10 0v4" />
    </Glifo>
  );
}

export function IconoDesbloqueado({ size = 16, class: className = '' }: IconoProps): JSX.Element {
  return (
    <Glifo size={size} class={className}>
      <rect width="18" height="11" x="3" y="11" rx="2" ry="2" />
      <path d="M7 11V7a5 5 0 0 1 9.9-1" />
    </Glifo>
  );
}

export function IconoOjoCerrado({ size = 16, class: className = '' }: IconoProps): JSX.Element {
  return (
    <Glifo size={size} class={className}>
      <path d="m15 18-.722-3.25" />
      <path d="M2 8a10.645 10.645 0 0 0 20 0" />
      <path d="m20 15-1.726-2.05" />
      <path d="m4 15 1.726-2.05" />
      <path d="m9 18 .722-3.25" />
    </Glifo>
  );
}

export function IconoChevronRight({ size = 16, class: className = '' }: IconoProps): JSX.Element {
  return (
    <Glifo size={size} class={className}>
      <polyline points="9 18 15 12 9 6" />
    </Glifo>
  );
}

export function IconoChevronDown({ size = 16, class: className = '' }: IconoProps): JSX.Element {
  return (
    <Glifo size={size} class={className}>
      <polyline points="6 9 12 15 18 9" />
    </Glifo>
  );
}

export function IconoTrash({ size = 16, class: className = '' }: IconoProps): JSX.Element {
  return (
    <Glifo size={size} class={className}>
      <polyline points="3 6 5 6 21 6" />
      <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
    </Glifo>
  );
}

export function IconoPlus({ size = 16, class: className = '' }: IconoProps): JSX.Element {
  return (
    <Glifo size={size} class={className}>
      <line x1="12" y1="5" x2="12" y2="19" />
      <line x1="5" y1="12" x2="19" y2="12" />
    </Glifo>
  );
}

export function IconoAutoLayoutFila({ size = 18, class: className = '' }: IconoProps): JSX.Element {
  return (
    <Glifo size={size} class={className}>
      <rect width="6" height="16" x="4" y="4" rx="1" />
      <rect width="6" height="16" x="14" y="4" rx="1" />
    </Glifo>
  );
}

export function IconoAutoLayoutColumna({
  size = 18,
  class: className = '',
}: IconoProps): JSX.Element {
  return (
    <Glifo size={size} class={className}>
      <rect width="16" height="6" x="4" y="4" rx="1" />
      <rect width="16" height="6" x="4" y="14" rx="1" />
    </Glifo>
  );
}

export function IconoGrip({ size = 16, class: className = '' }: IconoProps): JSX.Element {
  return (
    <Glifo size={size} class={className}>
      <circle cx="9" cy="6" r="1" />
      <circle cx="9" cy="12" r="1" />
      <circle cx="9" cy="18" r="1" />
      <circle cx="15" cy="6" r="1" />
      <circle cx="15" cy="12" r="1" />
      <circle cx="15" cy="18" r="1" />
    </Glifo>
  );
}

export function IconoVolver({ size = 18, class: className = '' }: IconoProps): JSX.Element {
  return (
    <Glifo size={size} class={className}>
      <path d="m15 18-6-6 6-6" />
    </Glifo>
  );
}

export {
  IconoAlerta as Alerta,
  IconoSpinner as Spinner,
  IconoZoomIn as IconoZoomMas,
  IconoZoomOut as IconoZoomMenos,
  IconoZoomFit as IconoZoomAjustar,
  IconoOjo as IconoVisible,
  IconoOjoCerrado as IconoOculto,
};
