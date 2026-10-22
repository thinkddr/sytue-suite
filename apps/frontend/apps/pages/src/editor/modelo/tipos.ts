/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */

import type { Nodo, Pagina, Seccion } from '@sytue/pages-render';

export type PuntoDeCorte = 'desktop' | 'tablet' | 'mobile';

export const ANCHOS_PUNTO_DE_CORTE: Record<PuntoDeCorte, number> = {
  desktop: 1200,
  tablet: 768,
  mobile: 375,
};

export interface ReferenciaNodo {
  nodo: Nodo | Seccion;
  padre: Seccion | (Nodo & { children: Nodo[] }) | null;
  index: number;
  pagina: Pagina | null;
}

export type EstadoGuardado =
  'guardado' | 'guardando' | 'pendiente' | 'error' | 'conflicto' | 'offline';
