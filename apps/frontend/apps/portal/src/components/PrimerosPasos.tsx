/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «PRIMEROS PASOS» EN LA PORTADA.
 *
 * La lista «Siguientes pasos» del alta se perdía al cerrar el asistente, y quien
 * entra por invitación no la veía nunca. Esta tarjeta enseña, encima de las apps,
 * lo que a ESTA persona le queda por dejar listo; cada paso lleva a donde se
 * arregla y desaparece al cumplirse, y la tarjeta entera cuando la cuenta ya está
 * lista (verificación, recuperación y foto; conectar e instalar solo acompañan). «Ocultar»
 * la quita para siempre en este navegador (por persona y organización).
 *
 * Los pasos llegan ya calculados del SSR: la tarjeta sale pintada en el HTML y
 * las apps no se mueven al hidratar. Lo que solo sabe el navegador (oculta, ya
 * instalada) lo aplica antes del pintado `guionAntesDePintar`, y aquí, al montar,
 * se llega a lo mismo. Si el SSR no pudo, se piden al BFF y la tarjeta entra
 * solo si queda algo.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import type { Locale } from '@sytue/i18n';
import { Button, Card, Icon } from '@sytue/ui';
import { appDomains } from '@sytue/config';
import { useT } from '../i18n/index.js';
import { r } from '../lib/rutas.js';
import {
  ID_TARJETA_PASOS,
  guardarPreferencia,
  instaladaAqui,
  leerPreferencias,
  pasosVisibles,
  type PasoPendiente,
} from '../lib/primeros-pasos.js';

type Paso = PasoPendiente['clave'];

const ICONO: Record<Paso, 'shield' | 'key' | 'camera' | 'device' | 'download'> = {
  dos_pasos: 'shield',
  recuperacion: 'key',
  foto: 'camera',
  dispositivos: 'device',
  instalar: 'download',
};

// Conectar los dispositivos es el mismo paso que cerraba el alta: mismo texto.
const TEXTOS = {
  dos_pasos: ['firstSteps.mfa.title', 'firstSteps.mfa.body'],
  recuperacion: ['firstSteps.recovery.title', 'firstSteps.recovery.body'],
  foto: ['firstSteps.photo.title', 'firstSteps.photo.body'],
  dispositivos: ['ob.next.devices.title', 'ob.next.devices.body'],
  instalar: ['firstSteps.install.title', 'firstSteps.install.body'],
} as const satisfies Record<Paso, readonly [string, string]>;

/** Mi cuenta la sirve Sytue ID en `/account`; el ancla abre la sección que lo arregla. */
function hrefDelPaso(p: PasoPendiente, org: string | null): string {
  return p.destino === 'cuenta' ? `${appDomains.id}/account${p.ruta}` : r(p.ruta, org);
}

export function PrimerosPasos({
  locale,
  pasos: iniciales,
  clave,
  org,
}: {
  locale: Locale;
  /** Los del SSR; `undefined` si Sytue ID no contestó a tiempo. */
  pasos: readonly PasoPendiente[] | undefined;
  /** Clave de `localStorage` de esta persona en esta organización. */
  clave: string;
  org: string | null;
}): JSX.Element | null {
  const t = useT(locale);
  const [pasos, setPasos] = useState<readonly PasoPendiente[] | null>(iniciales ?? null);
  // Hasta montar se pinta lo mismo que el SSR (si no, Preact hidrataría otra cosa);
  // las preferencias del navegador entran en el efecto.
  const [montado, setMontado] = useState(false);
  const [oculto, setOculto] = useState(false);
  // Un booleano y no la lista: la dependencia del efecto es «¿hace falta el BFF?».
  const pedirAlBff = iniciales === undefined;

  useEffect(() => {
    setMontado(true);
    if (!pedirAlBff) return;
    const controller = new AbortController();
    fetch(r('/api/primeros-pasos'), { credentials: 'include', signal: controller.signal })
      .then((res) => (res.ok ? (res.json() as Promise<{ pasos?: PasoPendiente[] }>) : null))
      .then((data) => setPasos(data?.pasos ?? []))
      .catch(() => {
        /* silencio: sin la tarjeta la portada es la de siempre. */
      });
    return () => controller.abort();
  }, [pedirAlBff]);

  if (!pasos || oculto) return null;
  const visibles = montado ? pasosVisibles(pasos, leerPreferencias(clave), instaladaAqui()) : pasos;
  if (visibles.length === 0) return null;

  const ocultar = () => {
    guardarPreferencia(clave, 'oculto');
    setOculto(true);
  };

  return (
    <section id={ID_TARJETA_PASOS} aria-labelledby="primeros-pasos-titulo" class="mb-4">
      <Card>
        <div class="flex items-center justify-between gap-3 px-4 pt-3">
          <h2 id="primeros-pasos-titulo" class="text-text text-sm font-semibold">
            {t('firstSteps.title')}
          </h2>
          <Button
            variant="ghost"
            size="sm"
            onClick={ocultar}
            aria-label={t('firstSteps.hideAria')}
            class="max-sm:min-h-11"
          >
            {t('firstSteps.hide')}
          </Button>
        </div>
        <ul class="grid gap-1 p-2 sm:grid-cols-2">
          {visibles.map((p) => (
            <li key={p.clave} data-paso={p.clave} class="min-w-0">
              <a
                href={hrefDelPaso(p, org)}
                // Instalar no deja rastro en Sytue ID: se da por hecho al ir a las descargas.
                onClick={
                  p.clave === 'instalar' ? () => guardarPreferencia(clave, 'instalada') : undefined
                }
                class="hover:bg-bg-subtle focus-visible:bg-bg-subtle sytue-focus-ring-inset flex min-h-11 items-start gap-3 rounded-md p-2 transition-colors"
              >
                <Icon name={ICONO[p.clave]} size={18} class="text-text-muted mt-0.5 shrink-0" />
                <span class="min-w-0 flex-1">
                  <span class="text-text block text-sm font-medium">{t(TEXTOS[p.clave][0])}</span>
                  <span class="text-text-muted mt-0.5 block text-sm leading-snug">
                    {t(TEXTOS[p.clave][1])}
                  </span>
                </span>
                <Icon name="chevronRight" size={16} class="text-text-muted mt-0.5 shrink-0" />
              </a>
            </li>
          ))}
        </ul>
      </Card>
    </section>
  );
}
