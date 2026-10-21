/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Pedir firma»: a otra persona, por su correo o su @usuario, sobre el PDF
 * abierto; y cómo van las que ya se pidieron de este documento (ADR 0051 §7).
 *
 * Si es de OTRA organización (ADR 0057), el servicio no crea nada hasta que quien
 * pide lo confirma: aquí se explica que el PDF saldrá por un enlace de Drive de
 * solo lectura y, al confirmar, se comparte y se pide en el mismo paso.
 */
import { useCallback, useEffect, useState } from 'preact/hooks';
import { Button } from '@sytue/ui';
import type { docsT } from '../i18n';
import { r } from '../lib/rutas';
import { ErrorPDF, firmas, quienFirma, type PeticionFirma } from './api';
import { mensajeDeError } from './errores';

export function PedirFirma({
  t,
  path,
  onCerrar,
}: {
  t: ReturnType<typeof docsT>;
  path: string;
  onCerrar: () => void;
}) {
  const [email, setEmail] = useState('');
  const [mensaje, setMensaje] = useState('');
  const [enviando, setEnviando] = useState(false);
  const [error, setError] = useState('');
  const [hecho, setHecho] = useState('');
  const [pedidas, setPedidas] = useState<PeticionFirma[] | null>(null);
  // La persona es de otra organización: su nombre y a quién se confirma, mientras se
  // pregunta.
  const [fuera, setFuera] = useState<{ name: string; confirm: string } | null>(null);

  const recargar = useCallback(
    () =>
      firmas
        .listar()
        .then((ps) => setPedidas(ps.sent.filter((p) => p.path === path)))
        .catch(() => setPedidas([])),
    [path],
  );
  useEffect(() => void recargar(), [recargar]);

  const enviar = (confirm: string) => {
    setEnviando(true);
    setError('');
    setHecho('');
    firmas
      .pedir(path, email.trim(), mensaje.trim(), confirm)
      .then(() => {
        setHecho(t('pdf.dialog.pedirFirma.sent', { name: fuera?.name ?? email.trim() }));
        setEmail('');
        setMensaje('');
        setFuera(null);
        return recargar();
      })
      .catch((err: unknown) => {
        if (err instanceof ErrorPDF && err.codigo === 'pdf_signature_recipient_external') {
          setFuera({
            name: err.detalles?.['name'] || email.trim(),
            confirm: err.detalles?.['confirm'] ?? '',
          });
          return;
        }
        setError(mensajeDeError(err, t, 'pdf.error.apply'));
      })
      .finally(() => setEnviando(false));
  };

  return (
    <div class="flex flex-col gap-4">
      <form
        class="flex flex-col gap-3"
        onSubmit={(e) => {
          e.preventDefault();
          enviar(fuera?.confirm ?? '');
        }}
      >
        <p class="text-text-muted text-xs text-pretty">{t('pdf.dialog.pedirFirma.help')}</p>
        <label class="text-text-muted flex flex-col gap-1 text-xs">
          {t('pdf.dialog.pedirFirma.email')}
          <input
            type="text"
            required
            autocomplete="email"
            inputMode="email"
            value={email}
            onInput={(e) => {
              setEmail((e.target as HTMLInputElement).value);
              setFuera(null);
            }}
            class="border-border bg-surface text-text sytue-focus-ring rounded-md border px-2 py-1.5 text-sm"
          />
        </label>
        <label class="text-text-muted flex flex-col gap-1 text-xs">
          {t('pdf.dialog.pedirFirma.message')}
          <textarea
            rows={3}
            maxLength={1000}
            value={mensaje}
            onInput={(e) => setMensaje((e.target as HTMLTextAreaElement).value)}
            class="border-border bg-surface text-text sytue-focus-ring rounded-md border px-2 py-1.5 text-sm"
          />
        </label>
        {error && (
          <p role="alert" class="text-danger-text text-xs">
            {error}
          </p>
        )}
        {hecho && (
          <p role="status" class="text-success-text text-xs">
            {hecho}
          </p>
        )}
        {fuera !== null && (
          <div
            role="alert"
            class="bg-info-bg text-info-text flex flex-col gap-1 rounded-md p-3 text-xs"
          >
            <p class="font-semibold">
              {t('pdf.dialog.pedirFirma.external.title', { name: fuera.name })}
            </p>
            <p class="text-pretty">{t('pdf.dialog.pedirFirma.external.help')}</p>
          </div>
        )}
        <div class="flex justify-end gap-2">
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={fuera !== null ? () => setFuera(null) : onCerrar}
          >
            {t('pdf.cancel')}
          </Button>
          <Button type="submit" size="sm" disabled={enviando || email.trim() === ''}>
            {enviando
              ? t('pdf.working')
              : fuera !== null
                ? t('pdf.dialog.pedirFirma.external.confirm')
                : t('pdf.dialog.pedirFirma.send')}
          </Button>
        </div>
      </form>

      <section class="border-border flex flex-col gap-2 border-t pt-3">
        <h3 class="text-text text-xs font-semibold">{t('pdf.dialog.pedirFirma.list')}</h3>
        {pedidas !== null && pedidas.length === 0 && (
          <p class="text-text-subtle text-xs">{t('pdf.dialog.pedirFirma.none')}</p>
        )}
        <ul class="flex flex-col gap-1.5">
          {(pedidas ?? []).map((p) => (
            <li key={p.id} class="flex flex-wrap items-center justify-between gap-2 text-xs">
              <span class="text-text min-w-0 truncate">{quienFirma(p, 'sent')}</span>
              <span class="flex items-center gap-2">
                <span class="text-text-muted">{t(`pdf.signatureRequest.status.${p.status}`)}</span>
                {p.status === 'firmada' && p.external && !p.signed_path && (
                  <button
                    type="button"
                    class="sytue-focus-ring rounded underline"
                    onClick={() =>
                      void firmas
                        .traer(p.id)
                        .then(recargar)
                        .catch((err: unknown) =>
                          setError(mensajeDeError(err, t, 'pdf.error.apply')),
                        )
                    }
                  >
                    {t('pdf.dialog.pedirFirma.fetchSigned')}
                  </button>
                )}
                {p.status === 'firmada' && p.signed_path && (
                  <a
                    class="sytue-focus-ring rounded underline"
                    href={r(`/pdf?path=${encodeURIComponent(p.signed_path)}`)}
                  >
                    {t('pdf.dialog.pedirFirma.openSigned')}
                  </a>
                )}
                {p.status === 'pendiente' && (
                  <button
                    type="button"
                    class="sytue-focus-ring rounded underline"
                    onClick={() =>
                      void firmas
                        .cancelar(p.id)
                        .then(recargar)
                        .catch((err: unknown) =>
                          setError(mensajeDeError(err, t, 'pdf.error.apply')),
                        )
                    }
                  >
                    {t('pdf.dialog.pedirFirma.cancel')}
                  </button>
                )}
              </span>
            </li>
          ))}
        </ul>
      </section>
    </div>
  );
}
