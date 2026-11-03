/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { CARD_TONES, type BadgeTone, type CardTone, type IconName } from '@sytue/ui';
import type { BasesField, BasesFieldKind } from '@sytue/api-clients/bases';

// paridad-fyne: apps/fyne/apps/bases/aspecto.go
export const ICONO_DE_TIPO: Record<BasesFieldKind, IconName> = {
  text: 'text',
  number: 'chart',
  date: 'calendar',
  checkbox: 'check',
  select: 'tag',
  multiselect: 'list',
  link: 'link',
  formula: 'code',
  attachment: 'dosPaginas',
};

const TONOS_DE_OPCION: BadgeTone[] = ['primary', 'success', 'warning', 'accent', 'info', 'danger'];

export function tonoDeOpcion(campo: BasesField, valor: string): BadgeTone {
  const i = campo.choices?.indexOf(valor) ?? -1;
  return i < 0 ? 'neutral' : TONOS_DE_OPCION[i % TONOS_DE_OPCION.length]!;
}

/** El color identifica la base y se conserva al reordenar la lista. */
export function tonoDeBase(id: string): CardTone {
  let hash = 0;
  for (const c of id) hash = (Math.imul(hash, 31) + c.codePointAt(0)!) >>> 0;
  return CARD_TONES[hash % CARD_TONES.length]!;
}

export function esNumerico(campo: BasesField): boolean {
  return campo.kind === 'number' || (campo.kind === 'formula' && campo.result === 'number');
}
