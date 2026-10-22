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
import { useEffect, useState } from 'preact/hooks';
import {
  SettingsShell,
  SettingsSection,
  ChoiceCard,
  InlineHelp,
  Button,
  Spinner,
  useToast,
} from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import { useT } from '../i18n/index.js';
import { organizacionesDe } from '../lib/firmas-de-fuera.js';

type Modo = 'abierta' | 'cerrada' | 'confianza';

export default function ExternalSignaturesPanel({ locale }: { locale: Locale }): JSX.Element {
  const t = useT(locale);
  const toast = useToast();
  const [cargando, setCargando] = useState(true);
  const [fallo, setFallo] = useState(false);
  const [modo, setModo] = useState<Modo>('abierta');
  const [confianza, setConfianza] = useState('');
  const [guardando, setGuardando] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    void fetch('/api/admin/external-signatures', { credentials: 'include' })
      .then((r) => (r.ok ? r.json() : Promise.reject(new Error(String(r.status)))))
      .then((d: { mode: Modo; trusted: string[] }) => {
        setModo(d.mode);
        setConfianza(d.trusted.join('\n'));
      })
      .catch(() => setFallo(true))
      .finally(() => setCargando(false));
  }, []);

  async function guardar(): Promise<void> {
    setGuardando(true);
    setError('');
    const res = await fetch('/api/admin/external-signatures', {
      method: 'POST',
      credentials: 'include',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({
        mode: modo,
        trusted: modo === 'confianza' ? organizacionesDe(confianza) : [],
      }),
    }).catch(() => null);
    setGuardando(false);
    if (res?.ok) {
      const d = (await res.json()) as { trusted: string[] };
      setConfianza(d.trusted.join('\n'));
      toast.notify({ tone: 'success', title: t('externalSignatures.saved') });
      return;
    }
    const e = (await res?.json().catch(() => null)) as {
      error?: string;
      details?: { org?: string } | null;
    } | null;
    setError(
      e?.error === 'pdf_signature_policy_org'
        ? t('externalSignatures.badOrg', { org: e.details?.org ?? '' })
        : t('externalSignatures.saveFailed'),
    );
  }

  return (
    <SettingsShell
      title={t('externalSignatures.title')}
      description={t('externalSignatures.description')}
    >
      <SettingsSection title={t('externalSignatures.policy')}>
        {cargando ? (
          <div class="flex justify-center py-8" aria-busy="true">
            <Spinner size={22} />
          </div>
        ) : fallo ? (
          <InlineHelp tone="danger" title={t('externalSignatures.loadFailed')}>
            {t('externalSignatures.loadFailedHelp')}
          </InlineHelp>
        ) : (
          <div class="flex flex-col gap-3">
            {(['abierta', 'cerrada', 'confianza'] as const).map((m) => (
              <ChoiceCard
                key={m}
                name="politica-firmas"
                value={m}
                title={t(`externalSignatures.mode.${m}`)}
                description={t(`externalSignatures.mode.${m}.help`)}
                selected={modo === m}
                onSelect={() => setModo(m)}
              />
            ))}
            {modo === 'confianza' ? (
              <label class="text-text-muted flex flex-col gap-1 text-sm">
                {t('externalSignatures.trusted')}
                <textarea
                  rows={4}
                  value={confianza}
                  onInput={(e) => setConfianza((e.target as HTMLTextAreaElement).value)}
                  class="border-border bg-surface text-text sytue-focus-ring rounded-md border px-2 py-1.5 font-mono text-sm"
                />
                <span class="sytue-meta">{t('externalSignatures.trustedHelp')}</span>
              </label>
            ) : null}
            <InlineHelp tone="info" title={t('externalSignatures.silent')}>
              {t('externalSignatures.silentHelp')}
            </InlineHelp>
            {error ? (
              <p role="alert" class="text-danger-text text-sm">
                {error}
              </p>
            ) : null}
            <div class="flex justify-end">
              <Button variant="primary" loading={guardando} onClick={() => void guardar()}>
                {t('externalSignatures.save')}
              </Button>
            </div>
          </div>
        )}
      </SettingsSection>
    </SettingsShell>
  );
}
