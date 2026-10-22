// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//
// `Astro.locals.locale`, que deja puesto `middlewareDeIdioma`.
//
// EN UN `.d.ts` Y NO EN EL FICHERO DEL MIDDLEWARE, que es donde nació. Ampliar
// un espacio de nombres global no tiene equivalente con módulos ES, así que
// `typescript/no-namespace` —que el repo tiene en `error`— lo rechaza en
// cualquier `.ts`. El lint excluye `**/*.d.ts` justamente por esto, y es donde
// lo declaran las once apps en su `env.d.ts`.
//
// Se descubrió tarde: el `quality` de la release v2.70.0 falló por estas cuatro
// líneas cuando ya estaba todo fusionado.
import type { Locale } from '@sytue/i18n';

declare global {
  namespace App {
    interface Locals {
      locale: Locale;
    }
  }
}
