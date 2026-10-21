/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Consentimiento del manifiesto de un script. Se pide antes de la PRIMERA ejecución y de
 * nuevo ante CUALQUIER cambio del manifiesto: lo que se recuerda es el par (script, hash
 * del manifiesto canónico), así que un manifiesto distinto es otra clave y no hay
 * consentimiento heredado. El recuerdo (localStorage, por persona) es el común de
 * `@sytue/scripts`; aquí vive lo que es de Docs: cómo se cuenta su manifiesto a la persona.
 */
import { cargarConsentimientos, guardarConsentimientos } from '@sytue/scripts';

import type { ManifiestoScript } from './manifiesto';

export { consentir, haConsentido, leerConsentimientos } from '@sytue/scripts';

export const cargar = (sub: string): Set<string> => cargarConsentimientos('docs', sub);
export const guardar = (sub: string, s: ReadonlySet<string>): void =>
  guardarConsentimientos('docs', sub, s);

/** Una línea del diálogo: clave de texto + parámetros, para que la interfaz la traduzca. */
export interface LineaConsentimiento {
  clave:
    | 'scripts.consent.write'
    | 'scripts.consent.readOnly'
    | 'scripts.consent.commands'
    | 'scripts.consent.read'
    | 'scripts.consent.limits'
    | 'scripts.consent.scope'
    | 'scripts.consent.unattended';
  params?: Record<string, string | number>;
}

/** Lo que el manifiesto pide, en el orden en que importa a quien lo lee. */
export function describirManifiesto(m: ManifiestoScript): LineaConsentimiento[] {
  const l: LineaConsentimiento[] = [
    { clave: m.write ? 'scripts.consent.write' : 'scripts.consent.readOnly' },
  ];
  if (m.commands.length > 0)
    l.push({ clave: 'scripts.consent.commands', params: { list: m.commands.join(', ') } });
  if (m.read.length > 0)
    l.push({ clave: 'scripts.consent.read', params: { list: m.read.join(', ') } });
  const partes = [...(m.scope.sheets ?? []), ...(m.scope.ranges ?? [])];
  if (partes.length > 0)
    l.push({ clave: 'scripts.consent.scope', params: { scope: partes.join(', ') } });
  l.push({
    clave: 'scripts.consent.limits',
    params: { ops: m.max_ops, seconds: Math.round(m.max_ms / 100) / 10 },
  });
  if (m.unattended) l.push({ clave: 'scripts.consent.unattended' });
  return l;
}
