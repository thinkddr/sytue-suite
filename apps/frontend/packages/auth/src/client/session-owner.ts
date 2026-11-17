/* SPDX-License-Identifier: Apache-2.0 */
// One opaque host implementation; auth/server owns the sealed BFF session.
export {
  openSessionOwner,
  verifySessionOwner,
  SessionOwnerError,
  type OpaqueSessionOwner,
  type SessionOwnerExpected,
} from '@sytue/offline/session-owner';
