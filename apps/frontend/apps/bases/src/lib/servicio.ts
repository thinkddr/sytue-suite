/**
 * Dirección interna del servicio Go de Sytue Bases (puerto 8102 por defecto).
 */
import { serverEnv } from '@sytue/auth/server';

export const DESTINO = serverEnv(import.meta.env).SYTUE_BASES_API_URL || 'http://bases:8102';
