/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * ProductGrid — rejilla de productos de la suite para el sitio público. Usa el
 * catálogo canónico (suiteApps) y los iconos de producto de @sytue/ui, de modo
 * que el marketing refleja exactamente la suite real. Tarjetas con icono de
 * marca, color de producto y enlace a la ficha. docs/design.md §9.
 *
 * Sin `client:*`: son tarjetas con enlaces y hover CSS, así que se renderiza en
 * el servidor y no manda una línea de JS al navegador.
 */
import type { Locale } from '@sytue/i18n';
import { suiteApps, Badge, Card, CardBody, ProductIcon } from '@sytue/ui';
import { localePath, useT } from '../i18n';
import {
  PRODUCT_KEYS,
  etiquetaEstado,
  rutaDeFicha,
  sePuedeAbrir,
  soloOrganizaciones,
  nombreEnWeb,
} from '../lib/product-content';

export interface ProductGridProps {
  locale: Locale;
  /** Destino de cada tarjeta: 'detail' → /product/[key] (funnel de marketing),
   *  'app' → el subdominio del producto. Por defecto 'detail'. */
  linkTo?: 'detail' | 'app';
}

export default function ProductGrid({ locale, linkTo = 'detail' }: ProductGridProps) {
  const t = useT(locale);

  return (
    <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
      {PRODUCT_KEYS.map((key) => {
        const app = suiteApps.find((a) => a.key === key);
        if (!app) return null;
        const estado = etiquetaEstado(t, key);
        // Un producto que todavía no se puede abrir SIEMPRE lleva a su ficha, aunque la
        // rejilla esté pidiendo enlaces a la app: ahí es donde se explica su estado, y
        // `app.href` daría un subdominio que hoy no responde.
        const href =
          linkTo === 'detail' || !sePuedeAbrir(key)
            ? localePath(locale, rutaDeFicha(key))
            : app.href;
        return (
          <a key={key} href={href} class="group sytue-focus-ring block rounded-lg">
            <Card class="group-hover:border-border-strong h-full transition duration-150 group-hover:-translate-y-0.5 group-hover:shadow-[var(--shadow-md)] group-active:translate-y-0 group-active:shadow-[var(--shadow-xs)]">
              <CardBody class="flex h-full flex-col gap-3">
                <div class="flex min-w-0 items-center gap-3">
                  <span
                    class="grid h-11 w-11 shrink-0 place-items-center rounded-lg"
                    style={{
                      // ponytail: color-mix acepta cualquier color CSS; `${hex}1a` solo componía alfa con hex.
                      background: `color-mix(in srgb, ${app.color} 10%, transparent)`,
                    }}
                  >
                    <ProductIcon appKey={key} size={24} />
                  </span>
                  {/* El nombre del producto es marca: nunca pasa por el catálogo. */}
                  <p class="text-text min-w-0 truncate font-semibold" title={nombreEnWeb(app)}>
                    {nombreEnWeb(app)}
                  </p>
                  {/* Lo que una cuenta personal no tiene, dicho en la tarjeta y no en
                      la letra pequeña: quien va a registrarse gratis lo ve antes de
                      hacerse ilusiones. La lista vive en product-content. */}
                  {/* El ESTADO manda sobre «solo organizaciones»: saber que algo aún no
                      existe importa más que saber para quién será cuando exista, y dos
                      distintivos en una tarjeta no caben sin partir el nombre. */}
                  {estado ? (
                    <Badge tone="warning" class="ml-auto shrink-0">
                      {estado}
                    </Badge>
                  ) : soloOrganizaciones(key) ? (
                    <Badge tone="primary" class="ml-auto shrink-0">
                      {t('grid.orgOnly')}
                    </Badge>
                  ) : null}
                </div>
                <p class="text-text-muted text-base">{t(`product.${key}.blurb`)}</p>
                <span class="text-primary-text mt-auto inline-flex items-center gap-1 text-sm font-semibold">
                  {t('grid.discover')}
                  <span aria-hidden="true" class="transition-transform group-hover:translate-x-0.5">
                    →
                  </span>
                </span>
              </CardBody>
            </Card>
          </a>
        );
      })}
    </div>
  );
}
