/*
 * Middleware de Sytue Admin: la organización en el path (ADR 0056), qué se sirve
 * desde cada host, y lo estándar.
 *
 * La consola de UNA organización vive en `admin.<dominio>/admin/<org>/…` y en
 * `<slug>.<dominio>/admin/<org>/…`; el middleware compartido
 * `middlewareDeOrganizacion` va PRIMERO: reescribe por dentro las URL con org,
 * manda a la canónica las sin org cuando hay sesión, y deja la org en
 * `locals.org`. La org del path NO autoriza: quién administra qué lo siguen
 * decidiendo el token (roles/perms del guard de cada página) y el servicio.
 *
 * LA CONSOLA DE PLATAFORMA NO LLEVA ORG, y no hace falta evitarlo a propósito:
 * se sirve en la raíz de `platform.<dominio>` (`/platform`, `/conexiones`,
 * `/account`, `/auth/…`, `/api/platform…`) — rutas que NO empiezan por `/admin/`
 * — y el middleware de organización solo atiende lo que cuelga de su prefijo
 * (`lib/superficies.ts`). La misma imagen, el mismo eslabón, dos superficies: en
 * plataforma el eslabón pasa sin tocar nada, igual que hace con la plomería.
 * (Una URL `/admin/…` escrita A MANO en el host de plataforma puede saltar antes
 * de que la verja de superficie la mande al 404: llega al mismo 404, con un salto
 * de más.)
 *
 * En `sinOrg` están las rutas de página, TODAS con nombre de slug válido
 * (`users`, `tenants`, `sso`…): sin excepción colisionan con el namespace de
 * organizaciones y el middleware no puede distinguir `/admin/users` de
 * `/admin/<org>/…`. El guard de cada página de la consola de organización compone
 * ahí la canónica con la org de la sesión (`canonicaConOrg`, @sytue/config), el
 * mismo salto local que el middleware aplica a lo que no colisiona. Quedan FUERA
 * del guard —y por tanto pueden servirse sin org— `platform`, `conexiones` y
 * `account`, que la consola de plataforma sirve en la raíz de su host, donde
 * nunca hay org.
 *
 * La negociación de idioma y las cabeceras de seguridad son las de la suite
 * (`@sytue/config/astro-middleware`). Esta app NO usa el handler estándar porque
 * tiene que decidir algo ANTES de entregar la ruta a Astro (un 404 por host, una
 * reescritura de la raíz de plataforma), así que compone el middleware ella misma
 * y llama a `negociarIdioma` para que el orden de preferencia —cookie >
 * `Accept-Language` > inglés— siga siendo el mismo que en las otras once
 * superficies.
 */
import { sequence } from 'astro:middleware';
import { defineMiddleware } from 'astro:middleware';
import { applySecurityHeaders } from '@sytue/config/astro-security';
import {
  cabecerasCerradas,
  middlewareDeOrganizacion,
  negociarIdioma,
} from '@sytue/config/astro-middleware';
import { hubBaseUrl } from './lib/auth.js';
import { esHostDePlataforma, esRutaDePlataforma } from './lib/superficies.js';
import { getBff, toBffContext } from './server/bff.js';

/** CSP cerrada (FE-03): la consola habla con su BFF; del navegador solo sale la shell. */
const cerrada = (): Readonly<Record<string, string>> => cabecerasCerradas(hubBaseUrl());

/*
 * QUÉ SE SIRVE DESDE CADA HOST vive en `lib/superficies.ts`: la regla se puede
 * probar sin levantar Astro, que desde un test no se puede importar.
 *
 * La raíz de plataforma se REESCRIBE a `/platform` en vez de redirigir: la URL
 * sigue siendo platform.<dominio>/ y el callback OIDC del client apunta ahí.
 */
export const onRequest = sequence(
  middlewareDeOrganizacion({
    base: '/admin',
    sesion: async (context) => getBff().getSession(toBffContext(context)),
    sinOrg: [
      'access',
      'account',
      'apps',
      'audit',
      'compliance',
      'conexiones',
      'devices',
      'editor-de-documentos',
      'erp',
      'firmas-de-fuera',
      'governance',
      'groups',
      'migrations',
      'platform',
      'privileged-access',
      'rooms',
      'roles',
      'sensitivity-labels',
      'sso',
      'tenants',
      'users',
    ],
  }),
  defineMiddleware(async (context, next) => {
    negociarIdioma(context);
    const enPlataforma = esHostDePlataforma(context.request.headers.get('host'));
    const ruta = context.url.pathname;

    if (enPlataforma && !esRutaDePlataforma(ruta)) {
      // 404 y no 403: desde este host esa pantalla no existe, y decir «no tienes
      // permiso» sería mentira —la misma persona la abre en admin sin problema—.
      const r = new Response(null, { status: 404 });
      applySecurityHeaders(r.headers, {}, cerrada());
      return r;
    }

    // La reescritura de plataforma también sale por aquí: las cabeceras se ponen
    // sobre la respuesta FINAL, sea cual sea la rama.
    const response =
      ruta === '/' && enPlataforma ? await context.rewrite('/platform') : await next();
    applySecurityHeaders(response.headers, {}, cerrada());
    return response;
  }),
);
