/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El prefijo de las rutas internas de Sytue Fotos, con barra final.
 *
 * POR QUÉ NO VALEN LAS RELATIVAS. La app se sirve bajo `/photos`, y un
 * `href="similar?id=…"` se resuelve contra el DIRECTORIO de la página actual:
 * desde `/photos/` da `/photos/similar`, pero desde `/photos` —sin barra— da
 * `/similar`, que es un 404 en la raíz del dominio de la organización.
 *
 * Y a `/photos` sin barra se llega por el camino normal: pedirlo redirige al
 * login con `returnTo=%2Fphotos`, y al volver se aterriza justo ahí. Medido en
 * producción el 2026-09-12: «Fotos parecidas» abría
 * `https://corporate.sytue.eu/similar?id=…` → `404 page not found`.
 *
 * Es PREFIJO y no envoltorio (`ruta('/similar')`) para que cada sitio cambie en
 * un solo punto: los enlaces se arman con plantillas de varias líneas y añadir
 * un paréntesis a ocho de ellas es cómo se descuadra el noveno.
 *
 * LA ORGANIZACIÓN VIAJA DENTRO (ADR 0056). En el navegador el prefijo lleva la
 * org de la URL actual (`/photos/<org>/`), para que la navegación entre páginas
 * conserve el segmento sin pagar el 302 del middleware en cada salto. En SSR no
 * hay URL de navegador: sale sin org y es ese middleware quien la añade. Solo
 * llevan página las rutas que construyen con `RUTA`; `p` (álbum público) y la
 * plomería (`api`, `auth`) van por su lado, fuera de escopia.
 *
 * `withBase` respeta además el prefijo que publica el servidor
 * (`window.__sytueBase`), que no tiene por qué ser el del build.
 */
import { orgActual, rutaConOrg } from '@sytue/config';
import { withBase } from '@sytue/ui';

export const RUTA = withBase(
  import.meta.env.BASE_URL,
  rutaConOrg('', orgActual(import.meta.env.BASE_URL), '/'),
);
