/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * DownloadPanel — descarga de las apps nativas. Destaca la plataforma detectada
 * y deja el resto a un clic: la detección solo REORDENA, nunca esconde, así que
 * un user agent raro no deja a nadie sin su binario.
 *
 * Lo único que esta isla necesita del navegador es el user agent. Los textos y
 * los enlaces llegan ya resueltos desde la página (`downloads.astro`): si la isla
 * importara el catálogo, los 26 idiomas enteros acabarían en el bundle.
 */
import { useEffect, useState } from 'preact/hooks';
import { Button, Card, CardBody } from '@sytue/ui';
import { detectPlatform, type PlatformId } from '../lib/downloads';

/** Una tarjeta de descarga, con todo su texto ya traducido. */
export interface DownloadCard {
  id: PlatformId;
  /** Plataforma (macOS, Windows…): marca, no se traduce. */
  label: string;
  /** Requisito mínimo del sistema. */
  requirement: string;
  /** Texto del botón: descargar, ver en la tienda o «próximamente». */
  cta: string;
  /** Vacío = la ficha de tienda aún no existe: el botón se muestra desactivado. */
  href: string;
}

export interface DownloadPanelProps {
  desktop: DownloadCard[];
  mobile: DownloadCard[];
  /** Textos del panel (secciones, APK y sistema detectado), ya traducidos. */
  labels: {
    desktopTitle: string;
    desktopBody: string;
    mobileTitle: string;
    mobileBody: string;
    selfhostTitle: string;
    selfhostBody: string;
    yourSystem: string;
  };
}

function TargetCard({
  card,
  featured,
  featuredLabel,
}: {
  card: DownloadCard;
  featured: boolean;
  featuredLabel: string;
}) {
  return (
    <Card class={featured ? 'border-border-strong h-full' : 'h-full'}>
      <CardBody class="flex h-full flex-col gap-3">
        <div class="flex items-baseline justify-between gap-2">
          <p class="text-text font-semibold">{card.label}</p>
          {featured ? (
            <span class="text-primary-text text-xs font-semibold">{featuredLabel}</span>
          ) : null}
        </div>
        <p class="text-text-muted text-sm">{card.requirement}</p>
        <div class="mt-auto pt-1">
          {card.href ? (
            <a href={card.href} data-platform={card.id}>
              <Button variant={featured ? 'primary' : 'secondary'} fullWidth>
                {card.cta}
              </Button>
            </a>
          ) : (
            <Button variant="secondary" fullWidth disabled>
              {card.cta}
            </Button>
          )}
        </div>
      </CardBody>
    </Card>
  );
}

export default function DownloadPanel({ desktop, mobile, labels }: DownloadPanelProps) {
  const [current, setCurrent] = useState<PlatformId | null>(null);

  // En SSR no hay user agent: se detecta al hidratar y ninguna tarjeta nace
  // destacada, de modo que el HTML servido ya es completo y navegable sin JS.
  useEffect(() => setCurrent(detectPlatform(navigator.userAgent, navigator.maxTouchPoints)), []);

  const byCurrentFirst = (list: DownloadCard[]) =>
    [...list].sort((a, b) => Number(b.id === current) - Number(a.id === current));

  const render = (card: DownloadCard) => (
    <TargetCard
      key={card.id}
      card={card}
      featured={card.id === current}
      featuredLabel={labels.yourSystem}
    />
  );

  return (
    <div class="flex flex-col gap-12">
      <section aria-labelledby="desktop">
        <h2 id="desktop" class="sytue-h3 text-text text-balance">
          {labels.desktopTitle}
        </h2>
        <p class="text-text-muted mt-2 text-base text-pretty">{labels.desktopBody}</p>
        <div class="mt-5 grid grid-cols-1 gap-4 sm:grid-cols-3">
          {byCurrentFirst(desktop).map(render)}
        </div>
      </section>

      <section aria-labelledby="mobile">
        <h2 id="mobile" class="sytue-h3 text-text text-balance">
          {labels.mobileTitle}
        </h2>
        <p class="text-text-muted mt-2 text-base text-pretty">{labels.mobileBody}</p>
        <div class="mt-5 grid grid-cols-1 gap-4 sm:grid-cols-2">
          {byCurrentFirst(mobile).map(render)}
        </div>
      </section>

      <section aria-labelledby="selfhost" class="border-border border-t pt-8">
        <h2 id="selfhost" class="sytue-h3 text-text text-balance">
          {labels.selfhostTitle}
        </h2>
        <p class="text-text-muted mt-2 text-base text-pretty">{labels.selfhostBody}</p>
      </section>
    </div>
  );
}
