/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Qué hace la organización con las peticiones de firma de CertiPDF que le llegan de
 * otras organizaciones (ADR 0059). Tres opciones y, en la de confianza, la lista de
 * organizaciones por su nombre en la dirección (el `acme` de `…/docs/acme/`).
 *
 * Quien pide desde fuera no se entera de lo que se elija aquí: su petición se «envía»
 * igual y simplemente no llega. Por eso la pantalla lo dice, para que nadie espere que
 * la otra parte reciba un «rechazado».
 */
import type { JSX } from 'preact';
import { SettingsShell, InlineHelp } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import { useT } from '../i18n/index.js';
import { PoliticaDeFuera } from './PoliticaDeFuera.js';

export default function ExternalSignaturesPanel({ locale }: { locale: Locale }): JSX.Element {
  const t = useT(locale);
  return (
    <SettingsShell
      title={t('externalSignatures.title')}
      description={t('externalSignatures.description')}
    >
      <PoliticaDeFuera
        locale={locale}
        ruta="/api/admin/external-signatures"
        nombre="politica-firmas"
        codigoOrg="pdf_signature_policy_org"
        textos={{
          seccion: t('externalSignatures.policy'),
          ayudaModo: (m) => t(`externalSignatures.mode.${m}.help`),
          ayudaConfianza: t('externalSignatures.trustedHelp'),
          cargaFallida: t('externalSignatures.loadFailedHelp'),
        }}
      >
        <InlineHelp tone="info" title={t('externalSignatures.silent')}>
          {t('externalSignatures.silentHelp')}
        </InlineHelp>
      </PoliticaDeFuera>
    </SettingsShell>
  );
}
