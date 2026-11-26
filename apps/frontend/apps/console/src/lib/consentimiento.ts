/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Adónde vuelve el navegador al terminar un consentimiento OAuth de una conexión.
 *
 * El canje lo hace el servicio de integraciones solo para la persona que empezó el
 * consentimiento (NEG-10), y responde con el desenlace; aquí se convierte en la
 * URL de la sección de la consola, con la query que leen `AppsPanel` y
 * Migraciones (`instance`, `oauth=connected`, `purpose=out`, `oauth_error`).
 */

/** Lo que responde `POST /api/v1/oauth/finish` del servicio de integraciones. */
export interface DesenlaceConsentimiento {
  instance?: string;
  /**
   * Con `error: 'exists'`: la conexión que YA había de esa misma cuenta. La que se estaba
   * creando se ha descartado, así que es a esta a la que se vuelve.
   */
  existing?: string;
  section?: string;
  out?: boolean;
  error?: string;
}

// Lista cerrada y no un path libre: el destino acaba en un Location.
const SECCIONES = new Set(['apps', 'migrations']);

/** URL de la consola (bajo `base`) donde se enseña el desenlace. */
export function destinoTrasConsentimiento(base: string, d: DesenlaceConsentimiento): string {
  const seccion = d.section !== undefined && SECCIONES.has(d.section) ? d.section : 'apps';
  const q = new URLSearchParams();
  const instancia = d.error === 'exists' && d.existing ? d.existing : d.instance;
  if (instancia) q.set('instance', instancia);
  if (d.error) {
    q.set('oauth_error', d.error);
  } else {
    q.set('oauth', 'connected');
    if (d.out) q.set('purpose', 'out');
  }
  return `${base.replace(/\/+$/, '')}/${seccion}?${q.toString()}`;
}
