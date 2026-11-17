/* SPDX-License-Identifier: Apache-2.0 */
// Wrapped recovery keys of this person's Matrix accounts (ADR 0082): shared handler.
import { rutasRecuperacionMatrix } from '@sytue/auth/server';
import { bff } from '../../../lib/auth.js';
export const prerender = false;
export const { GET, PUT } = rutasRecuperacionMatrix({
  bff,
  env: import.meta.env,
  product: 'drive',
}).wrappers;
