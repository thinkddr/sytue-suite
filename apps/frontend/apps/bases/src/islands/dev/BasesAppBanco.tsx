/* SPDX-License-Identifier: Apache-2.0 */
/**
 * Solo para el banco de desarrollo (`pages/dev/app.astro`, 404 en producción): la lista
 * REAL de bases detrás de su puerta de almacenamiento privado, que también es la real,
 * con un dispositivo sintético ya verificado. Sin sesión no hay homeserver.
 */
import type { ComponentProps, JSX } from 'preact';
import { MatrixStorageFixture } from '@sytue/shell/matrix/fixture';
import { BasesApp } from '../BasesApp.js';

export function BasesAppBanco(props: ComponentProps<typeof BasesApp>): JSX.Element {
  return (
    <MatrixStorageFixture>
      <BasesApp {...props} />
    </MatrixStorageFixture>
  );
}
