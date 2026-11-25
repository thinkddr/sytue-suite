/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Una política de la organización con gente de otras organizaciones: abierta, cerrada
 * o solo con las de confianza, y en ese caso la lista por el nombre de su dirección.
 * La comparten las peticiones de firma de CertiPDF (ADR 0059) y las conversaciones de
 * Rooms (mig 0083 de Rooms): la forma es la misma; cambian la ruta del BFF, los
 * textos y el código del error de un nombre que no es de organización.
 */
import type { ComponentChildren, JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { SettingsSection, ChoiceCard, InlineHelp, Button, Spinner, useToast } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import { useT } from '../i18n/index.js';
import { organizacionesDe } from '../lib/firmas-de-fuera.js';

export type ModoPolitica = 'abierta' | 'cerrada' | 'confianza';
export const MODOS: readonly ModoPolitica[] = ['abierta', 'cerrada', 'confianza'];

export interface TextosPolitica {
  seccion: string;
  ayudaModo: (m: ModoPolitica) => string;
  ayudaConfianza: string;
  cargaFallida: string;
}

export function PoliticaDeFuera({
  locale,
  ruta,
  nombre,
  codigoOrg,
  textos,
  children,
}: {
  locale: Locale;
  /** El BFF de la consola: GET → { mode, trusted }, POST { mode, trusted } guarda. */
  ruta: string;
  /** `name` de las opciones: dos políticas en una página no se pisan. */
  nombre: string;
  /** El código con que el servicio rechaza un nombre que no es de organización. */
  codigoOrg: string;
  textos: TextosPolitica;
  /** Lo que la política tiene que decir además (debajo de las opciones). */
  children?: ComponentChildren;
}): JSX.Element {
  const t = useT(locale);
  const toast = useToast();
  const [cargando, setCargando] = useState(true);
  const [fallo, setFallo] = useState(false);
  const [modo, setModo] = useState<ModoPolitica>('abierta');
  const [confianza, setConfianza] = useState('');
  const [guardando, setGuardando] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    void fetch(ruta, { credentials: 'include' })
      .then((r) => (r.ok ? r.json() : Promise.reject(new Error(String(r.status)))))
      .then((d: { mode: ModoPolitica; trusted: string[] }) => {
        setModo(d.mode);
        setConfianza(d.trusted.join('\n'));
      })
      .catch(() => setFallo(true))
      .finally(() => setCargando(false));
  }, [ruta]);

  async function guardar(): Promise<void> {
    setGuardando(true);
    setError('');
    const res = await fetch(ruta, {
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
      e?.error === codigoOrg
        ? t('externalSignatures.badOrg', { org: e.details?.org ?? '' })
        : t('externalSignatures.saveFailed'),
    );
  }

  return (
    <SettingsSection title={textos.seccion}>
      {cargando ? (
        <div class="flex justify-center py-8" aria-busy="true">
          <Spinner size={22} />
        </div>
      ) : fallo ? (
        <InlineHelp tone="danger" title={t('externalSignatures.loadFailed')}>
          {textos.cargaFallida}
        </InlineHelp>
      ) : (
        <div class="flex flex-col gap-3">
          {MODOS.map((m) => (
            <ChoiceCard
              key={m}
              name={nombre}
              value={m}
              title={t(`externalSignatures.mode.${m}`)}
              description={textos.ayudaModo(m)}
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
              <span class="sytue-meta">{textos.ayudaConfianza}</span>
            </label>
          ) : null}
          {children}
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
  );
}
