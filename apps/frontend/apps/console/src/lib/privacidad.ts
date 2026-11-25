/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Derechos RGPD del lado del responsable (ADR 0054, enmienda del 2026-11-25): el
 * registro de solicitudes de la organización y la supresión física de una cuenta
 * suya. La organización es la responsable del tratamiento de los datos de su
 * gente; cuando alguien le pide acceso, rectificación o supresión, quien la
 * administra lo registra aquí, contesta en plazo y, si es una supresión, la ejecuta.
 *
 * Aquí van los tipos y las cuentas que la isla usa sin red (plazos, validación del
 * alta); los proxys de SOLO servidor a Sytue ID están en `privacidad-api.ts`, que
 * arrastra la configuración OIDC del BFF y no debe llegar al navegador.
 */
/** Lo que pide la persona (arts. 15 a 21). */
export const TIPOS = [
  'access',
  'rectification',
  'erasure',
  'restriction',
  'portability',
  'objection',
] as const;
export type TipoSolicitud = (typeof TIPOS)[number];

/** Por dónde llegó. */
export const CANALES = ['email', 'web', 'letter', 'phone', 'in_person', 'other'] as const;
export type CanalSolicitud = (typeof CANALES)[number];

export const ESTADOS = ['received', 'in_progress', 'completed', 'rejected'] as const;
export type EstadoSolicitud = (typeof ESTADOS)[number];

/** Una solicitud tal como la devuelve Sytue ID. */
export interface SolicitudRGPD {
  id: string;
  subject_user_id?: string;
  subject_email: string;
  subject_name: string;
  kind: TipoSolicitud;
  channel: CanalSolicitud;
  received_at: string;
  due_at: string;
  extended: boolean;
  extension_reason?: string;
  identity_verified: boolean;
  identity_method?: string;
  status: EstadoSolicitud;
  resolution?: string;
  notes?: string;
  erasure_started_at?: string;
  erasure_completed_at?: string;
  created_at: string;
  updated_at: string;
  closed_at?: string;
}

export interface EventoSolicitud {
  id: number;
  at: string;
  actor_id?: string;
  action: string;
}

/** Por qué no se ofrece la supresión física, o que sí se puede. */
export interface EstadoSupresion {
  available: boolean;
  reason?: 'not_erasure' | 'completed' | 'in_progress' | 'closed' | 'no_account' | 'not_member';
}

export interface ListadoRGPD {
  requests: SolicitudRGPD[];
  summary: { open: number; overdue: number };
}

export interface DetalleRGPD {
  request: SolicitudRGPD;
  events: EventoSolicitud[];
  erasure: EstadoSupresion;
}

/** Lo que la isla manda al registrar una solicitud. */
export interface AltaSolicitud {
  user_id?: string;
  subject_email?: string;
  subject_name?: string;
  kind: TipoSolicitud;
  channel: CanalSolicitud;
  received_at?: string;
  identity_verified: boolean;
  identity_method?: string;
  notes?: string;
}

const MS_DIA = 24 * 60 * 60 * 1000;

/**
 * Días que faltan para el plazo (negativo si venció). Por días de calendario, que
 * es como lo cuenta quien contesta: vence «el día 12», no «a las 10:14 del 12».
 */
export function diasHastaPlazo(dueAt: string, ahora: Date = new Date()): number {
  const vence = new Date(dueAt);
  const a = Date.UTC(ahora.getUTCFullYear(), ahora.getUTCMonth(), ahora.getUTCDate());
  const b = Date.UTC(vence.getUTCFullYear(), vence.getUTCMonth(), vence.getUTCDate());
  return Math.round((b - a) / MS_DIA);
}

/** Cómo está el plazo de una solicitud, para pintar su insignia. */
export type Urgencia = 'closed' | 'overdue' | 'soon' | 'ok';

export function urgencia(s: SolicitudRGPD, ahora: Date = new Date()): Urgencia {
  if (s.closed_at) return 'closed';
  const dias = diasHastaPlazo(s.due_at, ahora);
  if (dias < 0) return 'overdue';
  if (dias <= 7) return 'soon';
  return 'ok';
}

/** ¿Se puede prorrogar desde la consola? Una vez, abierta y antes de vencer. */
export function prorrogable(s: SolicitudRGPD, ahora: Date = new Date()): boolean {
  return !s.closed_at && !s.extended && new Date(s.due_at).getTime() > ahora.getTime();
}

const RE_CORREO = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

/**
 * Valida el alta antes de mandarla: la persona (cuenta de la organización o
 * correo), qué pide y por dónde llegó. Devuelve la clave del error o null. El
 * servidor valida otra vez: esto es para no hacer esperar a nadie por un 400.
 */
export function validarAlta(
  a: AltaSolicitud,
): null | 'subject' | 'email' | 'kind' | 'channel' | 'date' {
  if (!a.user_id && !a.subject_email?.trim()) return 'subject';
  if (!a.user_id && !RE_CORREO.test(a.subject_email?.trim() ?? '')) return 'email';
  if (!TIPOS.includes(a.kind)) return 'kind';
  if (!CANALES.includes(a.channel)) return 'channel';
  if (a.received_at) {
    const d = new Date(a.received_at);
    const hoy = Date.now();
    if (Number.isNaN(d.getTime()) || d.getTime() > hoy + MS_DIA || d.getTime() < hoy - 365 * MS_DIA)
      return 'date';
  }
  return null;
}
