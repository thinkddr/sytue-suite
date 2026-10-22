/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * PaginaNoEncontrada — el cuerpo de la página 404 de las superficies SSR.
 *
 * POR QUÉ EXISTE. Ninguna de las trece apps SSR tenía `404.astro`, así que Astro
 * servía la suya: fondo casi negro, el cohete de Astro y «404: Not found» en
 * inglés, con la ruta en monoespaciada. Medido contra producción el 2026-09-21
 * (`test/e2e/medidas-web.mjs`, pasada de ruta inexistente): 15 de 15 pantallas.
 * Un enlace viejo de Drive o un marcador mal copiado sacaban a la persona de la
 * suite sin salida y en otro idioma.
 *
 * Los textos llegan por props y no con `useT()`: la página la pinta el servidor
 * sin isla ni proveedor de idioma, y así el componente no depende de ninguno.
 * La monta `src/astro/PaginaNoEncontrada.astro`, que es la ruta que inyecta
 * `sytueNoEncontrada()` de `@sytue/config/astro-404`.
 */
import type { JSX } from 'preact';
import { Button } from './Button.js';
import { Illustration } from './Illustration.js';

export interface PaginaNoEncontradaProps {
  /** «Error 404». */
  overline: string;
  title: string;
  description: string;
  /** Rótulo del botón que vuelve al inicio de la app. */
  action: string;
  /** Inicio de la app, con su prefijo (`/mail/`). */
  href: string;
}

export function PaginaNoEncontrada({
  overline,
  title,
  description,
  action,
  href,
}: PaginaNoEncontradaProps): JSX.Element {
  return (
    // `min-h-dvh` y no `h-full`: el documento no trae el `h-full` del layout de
    // las apps, y con él la columna medía lo que el contenido y quedaba arriba.
    <main
      id="contenido"
      class="bg-bg text-text flex min-h-dvh flex-col items-center justify-center px-4 py-12 text-center"
    >
      {/* La lupa del estado vacío de búsqueda: dice «buscamos y no está», que es lo
          que ha pasado, sin inventar un dibujo nuevo para una sola pantalla. */}
      <Illustration name="emptySearch" size={160} class="mb-6" />
      <p class="sytue-overline text-text-subtle">{overline}</p>
      <h1 class="sytue-h3 text-text mt-2 max-w-md text-balance">{title}</h1>
      <p class="text-text-muted mt-2 max-w-md text-base text-pretty">{description}</p>
      <Button href={href} class="mt-6">
        {action}
      </Button>
    </main>
  );
}
