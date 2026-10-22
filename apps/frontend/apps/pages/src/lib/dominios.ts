/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import type { ClaveMensaje } from '../i18n/index.js';

export interface DominioSitio {
  id: string;
  host: string;
  status: string;
  detail: string;
  primary: boolean;
  txt?: { name: string; value: string };
  target?: { cname: string; a: string; type?: string; name?: string; value?: string };
  checked_at?: string;
  verified_at?: string;
  activated_at?: string;
}

/** Los hitos reflejan el resultado actual del servidor; los avisos invalidan un activo anterior. */
export function progresoDominio(dominio: DominioSitio): readonly boolean[] {
  const dns =
    (dominio.status === 'verified' || dominio.status === 'active') &&
    (!dominio.detail || dominio.detail === 'certificate');
  const https = dominio.status === 'active' && !dominio.detail;
  return [true, dns, https, https && dominio.primary];
}

const DETALLE: Record<string, ClaveMensaje> = {
  txt_missing: 'domains.detail.txt_missing',
  txt_mismatch: 'domains.detail.txt_mismatch',
  dns_unreachable: 'domains.detail.dns_unreachable',
  routing: 'domains.detail.routing',
  certificate: 'domains.detail.certificate',
};

export function detalleDominio(dominio: DominioSitio): ClaveMensaje {
  if (dominio.detail) return DETALLE[dominio.detail] ?? 'domains.detail.unknown';
  return dominio.status === 'active'
    ? 'domains.detail.active'
    : dominio.status === 'verified'
      ? 'domains.detail.certificate'
      : 'domains.detail.pending';
}
