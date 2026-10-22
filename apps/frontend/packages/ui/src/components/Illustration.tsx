/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Illustration — la ilustración de marca PINTADA, no enlazada.
 *
 * `illustrations` (assets/illustrations.ts) entrega URLs, y una URL dentro de un
 * `<img>` es un documento SVG APARTE: no hereda las variables CSS de la página.
 * Nuestro line-art se dibuja entero con roles `var(--sytue-*)` —así conmuta solo
 * entre claro y oscuro—, así que por esa vía sale con TODOS los trazos sin color.
 * Invisible. Y no falla ruidosamente: se ve un hueco donde iba el dibujo, que es
 * exactamente lo que nadie reporta.
 *
 * Lo descubrieron dos apps por separado y cada una lo esquivó a su manera —Mail
 * se escribió un `EmptyArt` privado que importaba los SVG con `?raw` cruzando la
 * frontera del paquete, y Docs renunció a la ilustración y puso un `<Icon>`—,
 * mientras Calendar seguía enseñando el hueco en dos sitios. Tres soluciones para
 * el mismo defecto es la señal de que el defecto era del sistema de diseño.
 *
 * Aquí el SVG va EN LÍNEA, dentro del documento, y las variables resuelven contra
 * el tema activo. Mismo camino que `OrgMark`, que ya necesitaba teñir por dentro.
 *
 * Decorativa siempre (`aria-hidden`): el estado vacío ya dice en palabras lo que
 * pasa, y además los SVG traen un `<title>`/`aria-label` en CASTELLANO horneado
 * desde branding/ — que en línea y sin ocultar se leería tal cual a quien tiene la
 * suite en alemán.
 *
 * ponytail: el mapa entra entero en el bundle (~8 KB sin comprimir, los diez
 * spots); si alguna app necesita cargar solo el suyo, partir en imports sueltos.
 */
import type { JSX } from 'preact';
import { cx } from '../lib/cx.js';
import emptyMail from '../assets/illustrations/empty-mail.svg?raw';
import emptyDrive from '../assets/illustrations/empty-drive.svg?raw';
import emptyCalendar from '../assets/illustrations/empty-calendar.svg?raw';
import emptyChat from '../assets/illustrations/empty-chat.svg?raw';
import emptyContacts from '../assets/illustrations/empty-contacts.svg?raw';
import emptyNotes from '../assets/illustrations/empty-notes.svg?raw';
import emptySearch from '../assets/illustrations/empty-search.svg?raw';
import emptyPhotos from '../assets/illustrations/empty-photos.svg?raw';
import errorRetry from '../assets/illustrations/error-retry.svg?raw';
import allDone from '../assets/illustrations/all-done.svg?raw';
import spotSovereignty from '../assets/illustrations/spot-sovereignty.svg?raw';
import spotUpload from '../assets/illustrations/spot-upload.svg?raw';

/**
 * Solo los spots que se tiñen con el tema. `heroSuite`, `ogImage` y
 * `faviconMaskable` NO están: son superficie de marca fija (color horneado), no
 * necesitan heredar nada y pesan lo que tres spots juntos. Para esos, `<img
 * src={illustrations.heroSuite}>` sigue siendo lo correcto.
 */
const SPOTS = {
  emptyMail,
  emptyDrive,
  emptyCalendar,
  emptyChat,
  emptyContacts,
  emptyNotes,
  emptySearch,
  emptyPhotos,
  errorRetry,
  allDone,
  spotSovereignty,
  spotUpload,
} as const;

export type SpotName = keyof typeof SPOTS;

export interface IllustrationProps {
  name: SpotName;
  /** Ancho máximo en píxeles. El alto lo pone la proporción del SVG. */
  size?: number;
  class?: string;
}

export function Illustration({ name, size = 160, class: cls }: IllustrationProps): JSX.Element {
  return (
    <span
      aria-hidden="true"
      // `w-full` + tope: dentro de `EmptyState` el hueco es estrecho en móvil y el
      // dibujo tiene que encoger; el `max-width` es lo que impide que crezca hasta
      // ocupar la pantalla en escritorio.
      class={cx('block w-full [&>svg]:h-auto [&>svg]:w-full', cls)}
      style={{ maxWidth: `${size}px` }}
      // SVG vendorizado de branding/ por scripts/sync-branding.sh: contenido
      // estático del repositorio, nunca entrada de usuario.
      dangerouslySetInnerHTML={{ __html: SPOTS[name] }}
    />
  );
}
