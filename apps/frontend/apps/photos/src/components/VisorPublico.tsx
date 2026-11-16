/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El visor de la página PÚBLICA de un álbum (`/p/{token}`): quien recibe el enlace pulsa
 * una foto y ve la foto a pantalla completa, con «5 / 30», anterior y siguiente (flechas,
 * Inicio, Fin, deslizar con el dedo), Escape y «Descargar». Antes el navegador abría la
 * imagen cruda y para ver la siguiente había que volver atrás.
 *
 * Las teselas las pinta el servidor como enlaces a la variante grande, y siguen siéndolo:
 * sin JS (o antes de hidratar) pulsar una foto la abre entera, como siempre. Esta isla
 * solo ESCUCHA los clics en esos enlaces (`data-visor`, con el índice de la foto) y, si
 * son un clic normal, abre el diálogo en vez de navegar. Un clic con Ctrl, ⌘, Mayús o el
 * botón central sigue siendo del navegador: abrir la foto en otra pestaña es legítimo.
 *
 * No hay sesión ni Sytue Shell: el visor es el del álbum compartido conmigo
 * (`VisorCompartido`), al que se le dan las URLs de la verja pública.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import type { Locale } from '@sytue/i18n';
import type { FotoCompartida } from '../client/photosApi.js';
import { useT } from '../i18n/index.js';
import { posicionVisor } from '../lib/posicionVisor.js';
import { VisorCompartido } from './VisorCompartido.js';

/** Una foto del álbum público con la URL de su variante grande, ya con la verja. */
export interface FotoPublica extends FotoCompartida {
  grande: string;
}

/** El atributo que lleva cada tesela con el índice de su foto. */
export const ATRIBUTO_VISOR = 'data-visor';

/**
 * El índice de la foto que abre un clic, o `null` si el clic no es para el visor: fuera
 * de una tesela, con modificadores o con otro botón que el principal.
 */
export function fotoDelClic(e: MouseEvent, total: number): number | null {
  if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) {
    return null;
  }
  const a = e.target instanceof Element ? e.target.closest(`a[${ATRIBUTO_VISOR}]`) : null;
  if (!a) return null;
  const i = Number(a.getAttribute(ATRIBUTO_VISOR));
  return Number.isInteger(i) && i >= 0 && i < total ? i : null;
}

export function VisorPublico({
  fotos,
  locale,
}: {
  fotos: FotoPublica[];
  locale: Locale;
}): JSX.Element | null {
  const t = useT(locale);
  const [abierta, setAbierta] = useState<number | null>(null);
  useEffect(() => {
    const alPulsar = (e: MouseEvent) => {
      const i = fotoDelClic(e, fotos.length);
      if (i === null) return;
      e.preventDefault();
      setAbierta(i);
    };
    document.addEventListener('click', alPulsar);
    return () => document.removeEventListener('click', alPulsar);
  }, [fotos.length]);
  // El foco vuelve a la tesela de la foto que se estaba viendo, no al principio. Tiene
  // que ser DESPUÉS de quitar el diálogo: mientras el `<dialog>` modal sigue abierto,
  // el resto de la página es inerte y `focus()` sobre una tesela no hace nada (con
  // «Cerrar» el foco se quedaba en `<body>`).
  const [volverA, setVolverA] = useState<number | null>(null);
  useEffect(() => {
    if (abierta !== null || volverA === null) return;
    document.querySelector<HTMLElement>(`a[${ATRIBUTO_VISOR}="${volverA}"]`)?.focus();
    setVolverA(null);
  }, [abierta, volverA]);

  const foto = abierta === null ? undefined : fotos[abierta];
  if (abierta === null || !foto) return null;
  const cerrar = () => {
    setVolverA(abierta);
    setAbierta(null);
  };
  return (
    <VisorCompartido
      src={foto.grande}
      descargar={foto.grande}
      foto={foto}
      t={t}
      onClose={cerrar}
      posicion={posicionVisor(abierta, fotos.length)}
      onIr={setAbierta}
      onPrev={abierta > 0 ? () => setAbierta(abierta - 1) : undefined}
      onNext={abierta < fotos.length - 1 ? () => setAbierta(abierta + 1) : undefined}
    />
  );
}
