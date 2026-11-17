/* SPDX-License-Identifier: Apache-2.0 */
// Transition only (ADR 0082): the retired ADR 0081 copy, read once to wrap it.
import { rutasRecuperacionMatrix } from '@sytue/auth/server';
import { bff } from '../../../lib/bff.js';
export const prerender = false;
export const { GET } = rutasRecuperacionMatrix({
  bff,
  env: import.meta.env,
  product: 'bases',
}).legacyKey;
