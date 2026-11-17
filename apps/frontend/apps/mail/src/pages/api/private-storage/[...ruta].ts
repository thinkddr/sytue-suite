/* SPDX-License-Identifier: Apache-2.0 */
import { rutaMailPrivado } from '@sytue/auth/server';
import { bff } from '../../../lib/bff.js';
export const prerender = false;
export const ALL = rutaMailPrivado({ bff, env: import.meta.env });
