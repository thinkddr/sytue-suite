/*
 * El middleware de la suite —idioma negociado y cabeceras de seguridad de la
 * casa— más lo único que es de esta app: la CSP estricta de scripts.
 *
 * «Mi cuenta» comparte origen con Sytue ID (docs/security.md#riesgo-aceptado-mi-cuenta-en-el-origen-del-idp),
 * así que aquí `script-src` no admite nada que no sea nuestro: Astro emite la
 * cabecera con los hashes de sus scripts, y los de arranque del layout se sirven
 * como fichero de este origen (src/pages/arranque.js.ts). `applySecurityHeaders`
 * CONSERVA esa CSP de Astro (no la pisa) y le AÑADE la de la casa, de modo que el
 * navegador aplica las dos: antes esto se hacía a mano aquí, ahora lo hace la
 * propia función (la misma regla que conserva el `sandbox` de los adjuntos).
 */
import type { MiddlewareHandler } from 'astro';
import { appDomains } from '@sytue/config';
import { cabecerasCerradas, negociarIdioma } from '@sytue/config/astro-middleware';
import { applySecurityHeaders } from '@sytue/config/astro-security';
import { serverEnv } from '@sytue/auth/server';

/**
 * CSP cerrada (FE-03), SUMADA a la de Astro: aquí importa más que en ninguna otra app,
 * porque este origen es el del IdP. Además de la shell, `PlanCard` pide la cuota a
 * Sytue Drive desde el navegador.
 */
const cerrada = (): Readonly<Record<string, string>> =>
  cabecerasCerradas(serverEnv(import.meta.env).SYTUE_HUB_BASE_URL, {
    conectar: URL.canParse(appDomains.drive) ? [new URL(appDomains.drive).origin] : [],
  });

export const onRequest: MiddlewareHandler = async (context, next) => {
  negociarIdioma(context);
  const response = await next();
  applySecurityHeaders(response.headers, {}, cerrada());
  return response;
};
