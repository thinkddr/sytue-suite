/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Con quién de otras organizaciones se habla en Rooms (mig 0083 de Rooms): mensajes
 * directos, grupos, canales, sus llamadas y la reunión de un evento. Abierta, cerrada o
 * solo con las organizaciones de confianza. Vale en los dos sentidos: la gente de aquí
 * no abre conversaciones con quien no se admite ni entra en las suyas, y nadie de una
 * organización que no se admite entra en las de aquí. La conversación es de las dos,
 * así que la otra organización también tiene que admitir a esta.
 */
import type { JSX } from 'preact';
import { ToastProvider } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import { useT } from '../i18n/index.js';
import { PoliticaDeFuera } from './PoliticaDeFuera.js';

export default function RoomsContactoExterno({ locale }: { locale: Locale }): JSX.Element {
  const t = useT(locale);
  return (
    <ToastProvider>
      <PoliticaDeFuera
        locale={locale}
        ruta="/api/admin/rooms-external-contact"
        nombre="politica-rooms"
        codigoOrg="rooms_external_policy_org"
        textos={{
          seccion: t('roomsExternal.title'),
          ayudaModo: (m) => t(`roomsExternal.mode.${m}.help`),
          ayudaConfianza: t('roomsExternal.trustedHelp'),
          cargaFallida: t('roomsExternal.loadFailedHelp'),
        }}
      >
        <p class="sytue-meta">{t('roomsExternal.description')}</p>
      </PoliticaDeFuera>
    </ToastProvider>
  );
}
