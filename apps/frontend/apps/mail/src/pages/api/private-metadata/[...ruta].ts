/* SPDX-License-Identifier: Apache-2.0 */
import { rutaMetadataMailPrivado } from '@sytue/auth/server';
import { bff } from '../../../lib/bff.js';
export const prerender = false;
export const ALL = rutaMetadataMailPrivado({ bff, env: import.meta.env });
