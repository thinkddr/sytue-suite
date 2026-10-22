/**
 * Datos de ejemplo (seed) de administración para Sytue Admin. En producción
 * los sirve la API de administración de Sytue ID (identidad/SCIM) y Sytue Hub a
 * través del control-plane; aquí se proveen como fuente realista para SSR/islas
 * mientras se integra el backend. NUNCA contienen secretos. docs/CONVENTIONS.md §8.
 */
import type {
  AdminGroup,
  AuditEvent,
  ConsoleMetric,
  DirectoryUser,
  Group,
  Tenant,
} from './types.js';

// `role` es el rol de quien mira, y en el seed va deliberadamente a 'admin': sin
// backend vivo no hay forma de saberlo, y de las dos opciones 'admin' es la que
// NO ofrece conceder owner. Un demo que enseña un botón que el servicio real
// rechazaría es peor que un demo con una opción de menos.
export const tenants: Tenant[] = [
  {
    slug: 'acme',
    name: 'ACME Europa',
    mailDomain: 'acme.eu',
    plan: 'business',
    userCount: 42,
    namespace: 'tenant-acme',
    createdAt: '2025-11-02T10:00:00Z',
    status: 'active',
    role: 'admin',
  },
  {
    slug: 'norte',
    name: 'Cooperativa Norte',
    mailDomain: 'norte.coop',
    plan: 'starter',
    userCount: 9,
    namespace: 'tenant-norte',
    createdAt: '2026-01-18T08:30:00Z',
    status: 'active',
    role: 'admin',
  },
  {
    slug: 'ayto-bilbo',
    name: 'Ayuntamiento de Bilbo',
    mailDomain: 'bilbo.eus',
    plan: 'enterprise',
    userCount: 318,
    namespace: 'tenant-ayto-bilbo',
    createdAt: '2025-09-12T09:00:00Z',
    status: 'active',
    role: 'admin',
  },
  {
    slug: 'demo',
    name: 'Demo Sytue',
    mailDomain: 'demo.sytue.eu',
    plan: 'starter',
    userCount: 3,
    namespace: 'tenant-demo',
    createdAt: '2026-06-01T12:00:00Z',
    status: 'suspended',
    role: 'admin',
  },
];

export function getTenant(slug: string): Tenant | undefined {
  return tenants.find((t) => t.slug === slug);
}

export const users: DirectoryUser[] = [
  {
    id: 'u_marta',
    username: 'marta.ruiz',
    email: 'marta.ruiz@acme.eu',
    fullName: 'Marta Ruiz',
    enabled: true,
    emailVerified: true,
    roles: ['owner', 'admin'],
    groups: ['org/acme', 'org/acme/direccion'],
    tenant: 'acme',
    mfaEnabled: true,
    lastLogin: '2026-06-30T07:10:00Z',
    createdAt: '2025-11-02T10:05:00Z',
    provisioning: { drive: 'synced', chat: 'synced', mail: 'synced' },
    department: 'Dirección',
    jobTitle: 'Directora general',
    location: 'Bilbao',
  },
  {
    id: 'u_jon',
    username: 'jon.alava',
    email: 'jon.alava@acme.eu',
    fullName: 'Jon Alava',
    enabled: true,
    emailVerified: true,
    roles: ['member'],
    groups: ['org/acme', 'org/acme/ingenieria'],
    tenant: 'acme',
    mfaEnabled: false,
    lastLogin: '2026-06-29T16:42:00Z',
    createdAt: '2025-11-04T11:00:00Z',
    provisioning: { drive: 'synced', chat: 'synced', mail: 'synced' },
    department: 'Ingeniería',
    jobTitle: 'Ingeniero de plataforma',
    location: 'Bilbao',
  },
  {
    id: 'u_lucia',
    username: 'lucia.mota',
    email: 'lucia.mota@acme.eu',
    fullName: 'Lucía Mota',
    enabled: true,
    emailVerified: false,
    roles: ['billing'],
    groups: ['org/acme', 'org/acme/finanzas'],
    tenant: 'acme',
    mfaEnabled: false,
    createdAt: '2026-05-20T09:00:00Z',
    provisioning: { drive: 'synced', chat: 'pending', mail: 'synced' },
    department: 'Sales',
    jobTitle: 'Responsable de cuentas',
    location: 'Madrid',
  },
  {
    id: 'u_pau',
    username: 'pau.serra',
    email: 'pau.serra@norte.coop',
    fullName: 'Pau Serra',
    enabled: true,
    emailVerified: true,
    roles: ['owner', 'admin'],
    groups: ['org/norte'],
    tenant: 'norte',
    mfaEnabled: true,
    lastLogin: '2026-06-28T10:00:00Z',
    createdAt: '2026-01-18T08:35:00Z',
    provisioning: { drive: 'synced', chat: 'synced', mail: 'error' },
    department: 'Dirección',
    jobTitle: 'Director',
    location: 'Iruñea',
  },
  {
    id: 'u_iz',
    username: 'izaro.goiri',
    email: 'izaro.goiri@bilbo.eus',
    fullName: 'Izaro Goiri',
    enabled: false,
    emailVerified: true,
    roles: ['member'],
    groups: ['org/ayto-bilbo', 'org/ayto-bilbo/cultura'],
    tenant: 'ayto-bilbo',
    mfaEnabled: false,
    lastLogin: '2026-04-02T08:00:00Z',
    createdAt: '2025-09-13T09:00:00Z',
    provisioning: { drive: 'synced', chat: 'synced', mail: 'synced' },
    department: 'Cultura',
    jobTitle: 'Técnica de cultura',
    location: 'Bilbao',
  },
];

export function usersForTenant(slug: string): DirectoryUser[] {
  return users.filter((u) => u.tenant === slug);
}

/** Seed de grupos gestionables (contrato /admin/groups) para SSR sin backend. */
export const adminGroups: AdminGroup[] = [
  {
    id: 'grp-ingenieria',
    name: 'Ingeniería',
    description: 'Producto, plataforma e infraestructura.',
    memberCount: 12,
    dynamic: false,
  },
  {
    id: 'grp-finanzas',
    name: 'Finanzas',
    description: 'Contabilidad y facturación.',
    memberCount: 4,
    dynamic: false,
  },
  { id: 'grp-todos', name: 'Toda la empresa', description: '', memberCount: 42, dynamic: false },
  {
    id: 'grp-ventas-din',
    name: 'Ventas (dinámico)',
    description: 'Membresía calculada: department = Sales.',
    memberCount: 7,
    dynamic: true,
    membershipRule: {
      match: 'all',
      conditions: [{ attribute: 'department', op: 'eq', value: 'Sales' }],
    },
  },
];

export const groups: Group[] = [
  {
    path: 'org/acme',
    name: 'ACME Europa',
    memberCount: 42,
    roles: ['member'],
    mappings: {
      chatWorkspace: 'acme',
      driveGroupFolder: 'ACME',
      mailDomain: 'acme.eu',
      namespace: 'tenant-acme',
    },
  },
  {
    path: 'org/acme/ingenieria',
    name: 'Ingeniería',
    memberCount: 18,
    roles: ['member'],
    mappings: { chatWorkspace: 'acme-ingenieria', driveGroupFolder: 'ACME/Ingenieria' },
  },
  {
    path: 'org/acme/finanzas',
    name: 'Finanzas',
    memberCount: 5,
    roles: ['billing'],
    mappings: { chatWorkspace: 'acme-finanzas', driveGroupFolder: 'ACME/Finanzas' },
  },
  {
    path: 'org/norte',
    name: 'Cooperativa Norte',
    memberCount: 9,
    roles: ['member'],
    mappings: {
      chatWorkspace: 'norte',
      driveGroupFolder: 'Norte',
      mailDomain: 'norte.coop',
      namespace: 'tenant-norte',
    },
  },
];

export function groupsForTenant(slug: string): Group[] {
  return groups.filter((g) => g.path === `org/${slug}` || g.path.startsWith(`org/${slug}/`));
}

export const auditEvents: AuditEvent[] = [
  {
    id: 'ev_1',
    source: 'id',
    type: 'USER_LOGIN',
    actor: 'marta.ruiz',
    tenant: 'acme',
    at: '2026-06-30T07:10:00Z',
    ok: true,
  },
  {
    id: 'ev_2',
    source: 'hub',
    type: 'SCIM_PROVISION',
    actor: 'system',
    target: 'lucia.mota@acme.eu',
    tenant: 'acme',
    at: '2026-06-30T06:55:00Z',
    ok: true,
  },
  {
    id: 'ev_3',
    source: 'chat',
    type: 'WORKSPACE_MEMBER_ADD',
    actor: 'system',
    target: 'jon.alava',
    tenant: 'acme',
    at: '2026-06-29T16:43:00Z',
    ok: true,
  },
  {
    id: 'ev_4',
    source: 'mail',
    type: 'MAILBOX_CREATE',
    actor: 'system',
    target: 'pau.serra@norte.coop',
    tenant: 'norte',
    at: '2026-06-28T10:01:00Z',
    ok: false,
  },
  {
    id: 'ev_5',
    source: 'id',
    type: 'MFA_ENROLL',
    actor: 'pau.serra',
    tenant: 'norte',
    at: '2026-06-28T10:00:00Z',
    ok: true,
  },
  {
    id: 'ev_6',
    source: 'id',
    type: 'USER_DISABLED',
    actor: 'admin.bilbo',
    target: 'izaro.goiri',
    tenant: 'ayto-bilbo',
    at: '2026-04-02T09:00:00Z',
    ok: true,
  },
];

export const metrics: ConsoleMetric[] = [
  {
    labelKey: 'dashboard.metric.tenants',
    value: String(tenants.length),
    hintKey: 'dashboard.metric.tenantsHint',
  },
  {
    labelKey: 'dashboard.metric.users',
    value: String(users.length),
    hintKey: 'dashboard.metric.usersHint',
  },
  {
    labelKey: 'dashboard.metric.mfa',
    value: `${users.filter((u) => u.mfaEnabled).length}/${users.length}`,
    hintKey: 'dashboard.metric.mfaHint',
  },
];
