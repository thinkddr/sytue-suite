/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * PRIMEROS PASOS DE UNA ORGANIZACIÓN: la guía de configuración de la portada.
 *
 * Los avisos de la portada dicen lo que se ha torcido (invitaciones caducadas,
 * gente sin segundo factor), pero una organización recién creada no tiene nada
 * torcido todavía: tiene cosas SIN HACER. Si el alta se saltó el dominio, nada
 * se lo recordaba a nadie.
 *
 * La regla que manda aquí: un paso solo sale «hecho» si una fuente VIVA lo dice.
 * Si la fuente no contesta (o tarda más del plazo), el paso sale «sin datos». Dar
 * por configurado un dominio que no sabemos si existe es peor que no decir nada,
 * y además escondería la tarjeta a quien más la necesita.
 */
import type { SytueSession } from '@sytue/auth';
import {
  loadInsightsSourced,
  loadOidcApps,
  loadSsoProviders,
  type OrgInsights,
  type OrgMailDomainsState,
  type Sourced,
} from './admin.js';

export type SetupStepId = 'domain' | 'mail' | 'invite' | 'mfa' | 'sso' | 'apps';

/** done = hecho; pending = obligatorio sin hacer; optional = opcional sin hacer; unknown = sin datos. */
export type SetupStepStatus = 'done' | 'pending' | 'optional' | 'unknown';

export interface SetupStep {
  id: SetupStepId;
  status: SetupStepStatus;
  /** Los opcionales no cuentan para dar la guía por terminada. */
  required: boolean;
}

/** Lo que la guía necesita saber, cada cosa con su `live`. */
export interface SetupSources {
  mailDomains: Sourced<OrgMailDomainsState | null>;
  insights: Sourced<OrgInsights>;
  /** Cuántas personas hay en el directorio (basta saber si hay más de una). */
  users: Sourced<number>;
  ssoProviders: Sourced<number>;
  apps: Sourced<number>;
}

function estado(live: boolean, hecho: boolean, required: boolean): SetupStepStatus {
  if (!live) return 'unknown';
  if (hecho) return 'done';
  return required ? 'pending' : 'optional';
}

export function buildSetupSteps(s: SetupSources): SetupStep[] {
  const dominios = s.mailDomains.live ? s.mailDomains.data : null;
  const propios = dominios?.items ?? [];
  const dominioVerificado = propios.some((d) => d.verified);
  // El correo está listo cuando las direcciones nuevas nacen en un dominio
  // PROPIO y verificado, no en el subdominio de la suite.
  const correoPropio = propios.some((d) => d.primary && d.verified);
  const dominiosVivos = s.mailDomains.live && dominios !== null;

  // Invitar está hecho si ya hay alguien más que quien creó la organización, o
  // hay invitaciones esperando. Con UNA fuente viva que lo confirme basta; para
  // decir «pendiente» hacen falta las dos.
  const hayGente = s.users.live && s.users.data > 1;
  const hayInvitaciones = s.insights.live && s.insights.data.invitations_pending > 0;
  const invitar: SetupStepStatus =
    hayGente || hayInvitaciones ? 'done' : s.users.live && s.insights.live ? 'pending' : 'unknown';

  return [
    { id: 'domain', required: true, status: estado(dominiosVivos, dominioVerificado, true) },
    { id: 'mail', required: true, status: estado(dominiosVivos, correoPropio, true) },
    { id: 'invite', required: true, status: invitar },
    {
      id: 'mfa',
      required: true,
      status: estado(s.insights.live, s.insights.data.without_mfa === 0, true),
    },
    {
      id: 'sso',
      required: false,
      status: estado(s.ssoProviders.live, s.ssoProviders.data > 0, false),
    },
    { id: 'apps', required: false, status: estado(s.apps.live, s.apps.data > 0, false) },
  ];
}

/** La tarjeta sobra cuando todos los obligatorios están hechos de verdad. */
export function setupComplete(steps: SetupStep[]): boolean {
  return steps.filter((p) => p.required).every((p) => p.status === 'done');
}

/** Plazo por fuente: la portada no espera a un servicio colgado (§13.3). */
export const PLAZO_GUIA_MS = 2500;

/** Resuelve con `fallback` (que siempre es `live:false`) si `p` no llega a tiempo. */
export function conPlazo<T>(p: Promise<T>, ms: number, fallback: T): Promise<T> {
  let timer: ReturnType<typeof setTimeout> | undefined;
  const corte = new Promise<T>((res) => {
    timer = setTimeout(() => res(fallback), ms);
  });
  return Promise.race([p.catch(() => fallback), corte]).finally(() => clearTimeout(timer));
}

const contar = <T>(p: Promise<Sourced<T[]>>): Promise<Sourced<number>> =>
  p.then((r) => ({ live: r.live, data: r.data.length }));

/**
 * Carga en paralelo las fuentes que la portada aún no tenía. Las que ya cargó
 * (dominios, usuarios) se le pasan para no pedirlas dos veces.
 */
export async function loadSetupSources(
  session: SytueSession,
  ya: { mailDomains: Sourced<OrgMailDomainsState | null>; users: Sourced<number> },
  plazo = PLAZO_GUIA_MS,
): Promise<SetupSources> {
  const [insights, ssoProviders, apps] = await Promise.all([
    conPlazo(loadInsightsSourced(session), plazo, {
      live: false,
      data: {
        without_mfa: 0,
        suspended: 0,
        unverified_email: 0,
        invitations_expired: 0,
        invitations_pending: 0,
      },
    }),
    conPlazo(contar(loadSsoProviders(session)), plazo, { live: false, data: 0 }),
    conPlazo(contar(loadOidcApps(session)), plazo, { live: false, data: 0 }),
  ]);
  return { ...ya, insights, ssoProviders, apps };
}
