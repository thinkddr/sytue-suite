/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Fuente ÚNICA de «qué sección de la Console exige qué permiso». La consumen el
 * layout (para decidir si se entra y qué se pinta en el menú) y el índice (para
 * llevar a un admin delegado a la primera sección que sí puede usar).
 *
 * Los permisos ESPEJAN los que exige Sytue ID en cada bloque de rutas
 * (`services/id/internal/http/server.go`): si aquí se abriera de más, el backend
 * seguiría respondiendo 403 — el enforcement real vive allí. Esto solo evita
 * ofrecer puertas que se van a cerrar en la cara.
 *
 * `null` = reservado a owner/admin (no delegable): ciclo de vida de organizaciones,
 * roles, PIM, gobernanza, cumplimiento y etiquetas de sensibilidad.
 */
import { esPlanDePago, type SytuePermission } from '@sytue/auth';

export const SECTION_PERMS: Record<string, readonly SytuePermission[] | null> = {
  dashboard: null,
  tenants: null,
  users: ['users.read', 'users.manage'],
  groups: ['groups.manage'],
  roles: null,
  pim: null,
  sso: ['sso.manage'],
  access: ['access_policies.manage'],
  devices: ['users.read', 'users.manage'],
  apps: ['apps.manage'],
  migrations: ['apps.manage'],
  // Con qué editor se abren los documentos: es configuración de las apps de la
  // organización y va con su mismo permiso. Faltaba, y como el menú se filtra por
  // ESTE mapa, la sección existía sin que nadie —tampoco un owner— viera su enlace.
  'document-editor': ['apps.manage'],
  // Canales de Rooms: administrar la mensajería de la organización es gobierno de
  // una app, igual que con qué editor se abren los documentos.
  rooms: ['apps.manage'],
  // Las peticiones de firma de fuera: el servicio las reserva a owner/admin, y no se
  // delega (ADR 0059).
  'external-signatures': null,
  // Permisos del ERP por módulo. Los guarda el ERP y solo los cambia owner/admin
  // (el servicio lo exige): no se delega.
  erp: null,
  governance: null,
  compliance: null,
  'sensitivity-labels': null,
  audit: ['audit.view'],
};

/**
 * `platform` NO está en SECTION_PERMS y no es un olvido: administrar la plataforma
 * no es un permiso de organización, así que ningún rol —tampoco `owner`— lo
 * concede. Se ofrece por SUPERFICIE (platform.<dominio>), y quién entra lo decide
 * Sytue ID. Ofrecérselo a un admin de organización cliente sería enseñarle una
 * puerta que nunca se le va a abrir.
 */
const PLATFORM_SECTION = 'platform';

/** Lo que una cuenta personal administra en Sytue Admin: lo suyo, no una organización. */
const PERSONAL_SECTIONS = ['apps', 'migrations'] as const;

/**
 * Las credenciales con las que Sytue se presenta ante Google, Microsoft o Airtable
 * son de la PLATAFORMA y sirven a todos los clientes a la vez, así que van en la
 * misma superficie y por el mismo camino: sección propia, no un bloque al final de
 * la administración global —que es donde estaba, y encontrarlo exigía saber que
 * había que bajar—.
 */
const CONNECTIONS_SECTION = 'connections';

/**
 * La cola de denuncias del operador (Cast y Rooms de todas las organizaciones).
 * Misma superficie y mismo gate: quién entra lo decide Sytue ID en cada petición.
 */
const MODERATION_SECTION = 'moderation';

/**
 * Secciones que puede ver este usuario. owner/admin lo ven todo (superusuarios,
 * mismo criterio que `HasPermission` en Go); un rol personalizado, solo aquellas
 * cuyo permiso tenga. «Mi cuenta» no está aquí: es auto-servicio y siempre visible.
 *
 * `platformSurface` añade la administración global, que no depende del rol sino de
 * en qué superficie se está.
 */
export function allowedSections(
  roles: readonly string[],
  perms: readonly string[],
  platformSurface = false,
  orgKind: 'org' | 'personal' | undefined = 'org',
  plan?: string,
): string[] {
  // Una cuenta personal no administra una organización —es una persona sola,
  // aunque sea `owner` de su organización personal—. CON PLAN DE PAGO sí es
  // dueña de sus integraciones y de sus datos: apps (conectar otras
  // aplicaciones) y migraciones (traer o llevarse lo suyo). SIN plan de pago no
  // tiene Sytue Admin, que es lo que promete el sitio público («Solo con
  // suscripción»). Sytue ID cierra el resto con 403 (requireOrgKind) y la guarda
  // del BFF cierra la consola entera a la gratuita; esto solo evita ofrecer
  // puertas que se van a cerrar.
  if (orgKind === 'personal') return esPlanDePago({ plan }) ? PERSONAL_SECTIONS.slice() : [];
  // EN PLATAFORMA, SOLO PLATAFORMA. Antes se devolvían las secciones de
  // organización MÁS la global, así que platform.sytue.eu pintaba el menú entero
  // de la consola de una organización —usuarios, grupos, roles, dispositivos— con
  // la administración global como una entrada más al final. Las dos cosas son
  // distintas: `admin` administra UNA organización y `platform` administra TODAS.
  // Mezclarlas invita a operar sobre una organización desde la consola de todas,
  // sin que quien mira sepa en cuál está actuando.
  //
  // El middleware devuelve 404 en esas rutas desde este host; esto es lo que hace
  // que además no se ofrezcan. Un enlace que lleva a un 404 es peor que no estar.
  if (platformSurface) return [PLATFORM_SECTION, CONNECTIONS_SECTION, MODERATION_SECTION];

  if (roles.includes('owner') || roles.includes('admin')) return Object.keys(SECTION_PERMS);
  return Object.entries(SECTION_PERMS)
    .filter(([, need]) => need !== null && need.some((p) => perms.includes(p)))
    .map(([id]) => id);
}

/** Ruta de cada sección delegable (las reservadas a owner/admin no hacen falta aquí). */
const SECTION_PATH: Record<string, string> = {
  users: '/users',
  groups: '/groups',
  sso: '/sso',
  access: '/access',
  devices: '/devices',
  apps: '/apps',
  migrations: '/migrations',
  audit: '/audit',
};

/**
 * Primera sección utilizable por este usuario, como ruta. Sirve para no plantarle
 * un 403 a un admin delegado que entra por la raíz: se le lleva a lo suyo. Reserva
 * a «Mi cuenta», que siempre existe.
 */
export function firstSectionPath(
  roles: readonly string[],
  perms: readonly string[],
  orgKind?: 'org' | 'personal',
  plan?: string,
): string {
  const first = allowedSections(roles, perms, false, orgKind, plan)[0];
  return first !== undefined ? (SECTION_PATH[first] ?? '/account') : '/account';
}
