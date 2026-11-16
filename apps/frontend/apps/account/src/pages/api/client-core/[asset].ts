/* SPDX-License-Identifier: Apache-2.0 */
import type { APIRoute } from 'astro';
import { serverEnv, clientCoreProxy, roomsServiceBase } from '@sytue/auth/server';
export const prerender = false;
export const GET: APIRoute = clientCoreProxy(roomsServiceBase(serverEnv(import.meta.env)));
