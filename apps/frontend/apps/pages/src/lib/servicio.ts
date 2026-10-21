/**
 * Dirección interna del servicio Go de Sytue Web (puerto 8100 por defecto).
 */
import { serverEnv } from '@sytue/auth/server';

export const DESTINO = serverEnv(import.meta.env).SYTUE_PAGES_API_URL || 'http://pages:8100';
