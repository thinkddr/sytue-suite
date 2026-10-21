/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BORRAR MI CUENTA (ADR 0054). Es un ENLACE a la página de Sytue ID, no un
 * formulario aquí: confirmar exige reautenticarse con la cookie del IdP, que esta
 * app no ve, y esa misma página es la URL que publica Google Play. Duplicarla
 * aquí daría dos sitios donde decidir qué se borra y qué no.
 */
import type { JSX } from 'preact';
import { Card } from '@sytue/ui';
import type { T } from '../i18n/index.js';

export function BorrarCuentaCard({ t, idBaseUrl }: { t: T; idBaseUrl: string }): JSX.Element {
  return (
    <Card class="mb-6 break-inside-avoid">
      <div class="flex flex-col gap-3 p-4 sm:p-5">
        <h2 class="text-lg font-semibold">{t('delete.title')}</h2>
        <p class="text-text-muted text-sm">{t('delete.hint')}</p>
        {/* `min-h-[24px]`: el mínimo de WCAG 2.5.8 para un enlace suelto (ver PerfilCard). */}
        <a
          href={`${idBaseUrl}/account/delete`}
          class="text-danger inline-flex min-h-[24px] items-center self-start text-sm font-medium underline"
        >
          {t('delete.link')}
        </a>
      </div>
    </Card>
  );
}

export default BorrarCuentaCard;
