/* SPDX-License-Identifier: Apache-2.0 */
/**
 * Solo para el banco de desarrollo (`pages/dev/app.astro`, 404 en producción): la galería
 * REAL detrás de su puerta de almacenamiento privado, que también es la real, con un
 * dispositivo sintético ya verificado. Sin sesión no hay homeserver.
 */
import type { ComponentProps, JSX } from 'preact';
import { MatrixStorageFixture } from '@sytue/shell/matrix/fixture';
import { PhotosApp } from '../PhotosApp.js';

export function PhotosAppBanco(props: ComponentProps<typeof PhotosApp>): JSX.Element {
  return (
    <MatrixStorageFixture>
      <PhotosApp {...props} />
    </MatrixStorageFixture>
  );
}
