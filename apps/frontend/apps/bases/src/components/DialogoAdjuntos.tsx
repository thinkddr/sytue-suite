/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Los ficheros de un campo de adjuntos de un registro: verlos, descargarlos y, quien puede
 * editar, añadir y borrar. El servicio decide el tipo por el CONTENIDO y vuelve a
 * autorizar cada descarga; aquí solo se evita mandar lo que ya se sabe que no cabe.
 */
import type { JSX } from 'preact';
import { useRef, useState } from 'preact/hooks';
import { Button, Icon, Modal, formatBytes } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import { HttpError } from '@sytue/api-clients';
import type { BasesAttachment, BasesField, BasesRecord } from '@sytue/api-clients/bases';
import type { T } from '../i18n/index.js';
import { cliente, mensajeDeError } from '../lib/api.js';
import { MAX_BYTES_ADJUNTO } from '../lib/adjuntos.js';
import { ruta } from '../lib/rutas.js';

export function DialogoAdjuntos(p: {
  t: T;
  locale: Locale;
  registro: BasesRecord;
  campo: BasesField;
  puedeEditar: boolean;
  /** La lista nueva de adjuntos del campo, tras añadir o borrar. */
  onCambio: (adjuntos: BasesAttachment[]) => void;
  onCerrar: () => void;
}): JSX.Element {
  const { t } = p;
  const adjuntos = p.registro.attachments?.[p.campo.id] ?? [];
  const [ocupado, setOcupado] = useState(false);
  const [error, setError] = useState('');
  const entrada = useRef<HTMLInputElement>(null);
  const url = (a: BasesAttachment, miniatura = false) =>
    ruta(`/api/records/${p.registro.id}/attachments/${a.id}${miniatura ? '/thumb' : ''}`);

  async function subir(fichero: File): Promise<void> {
    setError('');
    if (fichero.size > MAX_BYTES_ADJUNTO) {
      setError(t('attach.tooLarge'));
      return;
    }
    setOcupado(true);
    try {
      const nuevo = await cliente().uploadAttachment(p.registro.id, p.campo.id, fichero);
      p.onCambio([...adjuntos, nuevo]);
    } catch (e) {
      setError(
        e instanceof HttpError && e.status === 413
          ? t('attach.tooLarge')
          : e instanceof HttpError && e.status === 422
            ? t('attach.badType')
            : e instanceof HttpError && e.status === 503
              ? t('attach.unavailable')
              : mensajeDeError(e, t),
      );
    } finally {
      setOcupado(false);
      if (entrada.current) entrada.current.value = '';
    }
  }

  async function borrar(a: BasesAttachment): Promise<void> {
    setError('');
    try {
      await cliente().deleteAttachment(p.registro.id, a.id);
      p.onCambio(adjuntos.filter((x) => x.id !== a.id));
    } catch (e) {
      setError(mensajeDeError(e, t));
    }
  }

  return (
    <Modal open onClose={p.onCerrar} title={p.campo.name}>
      <div class="flex flex-col gap-4">
        {adjuntos.length === 0 ? (
          <p class="text-text-muted text-sm">{t('attach.none')}</p>
        ) : (
          <ul class="flex max-h-80 flex-col gap-2 overflow-y-auto">
            {adjuntos.map((a) => (
              <li key={a.id} class="border-border flex items-center gap-3 rounded-lg border p-2">
                {a.has_thumb ? (
                  <img
                    src={url(a, true)}
                    alt=""
                    width={48}
                    height={48}
                    loading="lazy"
                    class="h-12 w-12 shrink-0 rounded object-cover"
                  />
                ) : (
                  <span
                    class="bg-bg-subtle text-text-muted flex h-12 w-12 shrink-0 items-center justify-center rounded"
                    aria-hidden="true"
                  >
                    <Icon name="download" size={20} />
                  </span>
                )}
                <span class="min-w-0 flex-1">
                  <a
                    class="text-text block truncate text-sm underline"
                    href={url(a)}
                    download={a.name}
                    aria-label={t('attach.download', { name: a.name })}
                  >
                    {a.name}
                  </a>
                  <span class="text-text-muted text-xs">{formatBytes(p.locale, a.size)}</span>
                </span>
                {p.puedeEditar ? (
                  <Button
                    variant="ghost"
                    size="sm"
                    iconOnly
                    aria-label={t('attach.delete', { name: a.name })}
                    leadingIcon={<Icon name="trash" size={16} />}
                    onClick={() => void borrar(a)}
                  />
                ) : null}
              </li>
            ))}
          </ul>
        )}
        {p.puedeEditar ? (
          <div class="flex flex-col gap-1">
            <label class="text-text text-sm font-medium" for="adjunto-fichero">
              {t('attach.add')}
            </label>
            <input
              ref={entrada}
              id="adjunto-fichero"
              type="file"
              class="text-text text-sm"
              disabled={ocupado}
              aria-describedby="adjunto-ayuda"
              onChange={(e) => {
                const f = e.currentTarget.files?.[0];
                if (f) void subir(f);
              }}
            />
            <p id="adjunto-ayuda" class="text-text-muted text-xs">
              {t('attach.help')}
            </p>
          </div>
        ) : null}
        <p role="status" aria-live="polite" class="text-text-muted text-sm">
          {ocupado ? t('attach.uploading') : ''}
        </p>
        {error ? (
          <p role="alert" class="text-danger-text text-sm">
            {error}
          </p>
        ) : null}
      </div>
    </Modal>
  );
}
