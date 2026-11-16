/* SPDX-License-Identifier: Apache-2.0 */
import type { MatrixStorageScope } from './matrix.js';

export interface StorageRootAuthority {
  v: 1;
  scope: MatrixStorageScope;
  user_id: string;
  root_key: string;
  generation: number;
  status: 'active' | 'revoked';
}
export interface StorageRootEnrollment {
  v: 1;
  scope: MatrixStorageScope;
  user_id: string;
  target: { room_id: string; device_id: string };
  root_key: string;
  generation: number;
  previous_generation?: number;
  previous_root_key?: string;
}
export type StorageControlKind =
  | 'root.enroll'
  | 'root.resume'
  | 'root.rotate'
  | 'key.enroll'
  | 'task.claim'
  | 'task.ack'
  | 'task.handoff';
export interface StorageControlIntent {
  authority_generation: number;
  scope: MatrixStorageScope;
  epoch: string;
  kind: StorageControlKind;
  target: { room_id: string; device_id: string };
  request_sha256: string;
}
export interface StorageControlAction extends StorageControlIntent {
  v: 1;
  root_key: string;
  nonce: string;
  issued_at: number;
  expires_at: number;
}
export interface SignedStorageControlAction extends StorageControlAction {
  signatures: Record<string, Record<string, string>>;
}
export interface StorageRootChallenge {
  action: StorageControlAction;
  request: StorageRootEnrollment;
}
/** Transport uses the host's authenticated, scope-bound route. A missing root
 * is recognized by its specific API conflict, never by 404 or network failure. */
export interface StorageRootService {
  read: (signal: AbortSignal) => Promise<StorageRootAuthority>;
  enrollmentRequired: (error: unknown) => boolean;
  challenge: (
    epoch: string,
    kind: 'root.enroll' | 'root.resume',
    expected: StorageRootEnrollment,
    signal: AbortSignal,
  ) => Promise<StorageRootChallenge>;
  confirm: (
    proof: SignedStorageControlAction,
    signal: AbortSignal,
  ) => Promise<StorageRootAuthority>;
}
