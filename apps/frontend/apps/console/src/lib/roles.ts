/**
 * Catálogo de permisos DELEGABLES de los roles personalizados. Espeja domain/roles.go
 * MENOS roles.manage (reservado a owner/admin: no se ofrece). Sytue ID valida el envío.
 * Vive aparte del panel porque lo comparten la tarjeta del rol, su edición y el
 * asistente de alta: tres listas que no deben divergir.
 */
import type { ConsoleMessageKey } from '../i18n/index.js';
import type { PermissionKey } from './types.js';

export const PERMISSION_CATALOG: {
  key: PermissionKey;
  label: ConsoleMessageKey;
  hint: ConsoleMessageKey;
}[] = [
  { key: 'users.manage', label: 'perm.usersManage', hint: 'perm.usersManageHint' },
  { key: 'users.read', label: 'perm.usersRead', hint: 'perm.usersReadHint' },
  { key: 'groups.manage', label: 'perm.groupsManage', hint: 'perm.groupsManageHint' },
  { key: 'sso.manage', label: 'perm.ssoManage', hint: 'perm.ssoManageHint' },
  { key: 'access_policies.manage', label: 'perm.accessManage', hint: 'perm.accessManageHint' },
  { key: 'apps.manage', label: 'perm.appsManage', hint: 'perm.appsManageHint' },
  { key: 'audit.view', label: 'perm.auditView', hint: 'perm.auditViewHint' },
  { key: 'billing.manage', label: 'perm.billingManage', hint: 'perm.billingManageHint' },
];

const PERMISSION_LABEL = new Map(PERMISSION_CATALOG.map((p) => [p.key as string, p.label]));

/** Rótulo traducible de un permiso; un permiso desconocido se muestra tal cual. */
export function permissionLabelKey(key: string): ConsoleMessageKey | null {
  return PERMISSION_LABEL.get(key) ?? null;
}

export function togglePerm(list: string[], key: string): string[] {
  return list.includes(key) ? list.filter((p) => p !== key) : [...list, key];
}
