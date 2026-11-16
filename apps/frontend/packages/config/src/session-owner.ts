/* SPDX-License-Identifier: Apache-2.0 */
/** Public session metadata, never an access token or an authorization grant. */
export interface SessionOwnerDescriptor {
  readonly v: 1;
  readonly actor: string;
  readonly tenant: string;
  readonly client_id: string;
  readonly generation: string;
}
export const sessionOwnerHeader = 'X-Sytue-Session-Generation';
export const sessionChangedHeader = 'X-Sytue-Session-Changed';
export function isSessionGeneration(value: unknown): value is string {
  return typeof value === 'string' && /^[a-f0-9]{64}$/.test(value);
}
export function isSessionOwnerDescriptor(value: unknown): value is SessionOwnerDescriptor {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false;
  const owner = value as Partial<SessionOwnerDescriptor>;
  return (
    owner.v === 1 &&
    isSessionGeneration(owner.generation) &&
    [owner.actor, owner.tenant, owner.client_id].every(
      (field) =>
        typeof field === 'string' &&
        field.length > 0 &&
        field.length <= 16384 &&
        !field.includes('\u0000') &&
        !field.includes('\r') &&
        !field.includes('\n'),
    )
  );
}
