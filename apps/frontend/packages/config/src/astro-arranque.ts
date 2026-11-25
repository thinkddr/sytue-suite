/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import type { AstroIntegration } from 'astro';

/**
 * `/arranque.js` en cada superficie SSR: los scripts de arranque del layout (prefijo
 * de `/api` y `/auth`, tema guardado y, con `serviceWorker`, el registro del worker)
 * servidos como fichero de este origen. Es lo que deja a la CSP de scripts sin
 * `'unsafe-inline'` (`sytueCsp()`, astro-security.ts): en línea habría que firmarlos y
 * su contenido depende del entorno del contenedor.
 *
 * Como `sytueNoEncontrada()`, la ruta vive en `@sytue/ui` (pinta lo que es de la
 * interfaz) y aquí solo se declara, con un especificador de paquete que resuelve cada
 * app. La página 404 de la suite también lo carga, así que va en TODAS las apps SSR.
 */
export function sytueArranque(opciones: { serviceWorker?: boolean } = {}): AstroIntegration {
  return {
    name: 'sytue-arranque',
    hooks: {
      'astro:config:setup': ({ injectRoute }) => {
        injectRoute({
          pattern: '/arranque.js',
          entrypoint: opciones.serviceWorker ? '@sytue/ui/arranque-sw' : '@sytue/ui/arranque',
          // En el servidor: el prefijo sale de SYTUE_BASE_PATH del contenedor.
          prerender: false,
        });
      },
    },
  };
}
