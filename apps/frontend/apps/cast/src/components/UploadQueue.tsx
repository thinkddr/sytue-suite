/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * UploadQueue — la lista de subidas, una fila por fichero: el título (editable
 * mientras sube: se guarda al salir del campo), el tamaño, el progreso y qué se
 * puede hacer con él (cancelar, abrir, quitar de la lista). Un fallo dice POR QUÉ
 * con palabras, no solo con el color de la barra.
 */
import type { JSX } from 'preact';
import { Button, Icon, IconButton, Input, ProgressBar, formatBytes } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import type { T } from '../i18n/index.js';
import type { Elemento } from '../lib/cola.js';
import { ruta } from '../lib/rutas.js';

export interface UploadQueueProps {
  t: T;
  locale: Locale;
  cola: readonly Elemento[];
  /** Bytes libres cuando el servicio rechazó por cuota. */
  disponibles: number | null;
  alRenombrar: (clave: string, titulo: string) => void;
  alGuardarTitulo: (clave: string) => void;
  alCancelar: (clave: string) => void;
  alQuitar: (clave: string) => void;
}

export function UploadQueue({
  t,
  locale,
  cola,
  disponibles,
  alRenombrar,
  alGuardarTitulo,
  alCancelar,
  alQuitar,
}: UploadQueueProps): JSX.Element {
  const motivo = (e: Elemento): string => {
    if (e.motivo === 'cuota' && disponibles !== null)
      return t('upload.error.cuotaQuedan', { available: formatBytes(locale, disponibles) });
    return t(`upload.error.${e.motivo ?? 'servicio'}`);
  };
  return (
    <section aria-labelledby="cola-titulo" class="flex flex-col gap-2">
      <h2 id="cola-titulo" class="text-text text-sm font-semibold">
        {t('upload.queue')}
      </h2>
      <ul class="border-border divide-border bg-surface divide-y rounded-md border">
        {cola.map((e) => (
          <li key={e.clave} class="flex flex-col gap-2 p-3 sm:flex-row sm:items-center sm:gap-4">
            <div class="flex min-w-0 flex-1 items-start gap-3">
              <span
                class={
                  e.estado === 'fallo'
                    ? 'text-danger-text mt-1.5'
                    : e.estado === 'procesando'
                      ? 'text-success-text mt-1.5'
                      : 'text-text-subtle mt-1.5'
                }
              >
                <Icon
                  name={
                    e.estado === 'fallo' ? 'danger' : e.estado === 'procesando' ? 'check' : 'video'
                  }
                  size={16}
                />
              </span>
              <div class="flex min-w-0 flex-1 flex-col gap-1.5">
                <Input
                  value={e.titulo}
                  aria-label={t('upload.titleField')}
                  maxLength={200}
                  disabled={e.estado === 'fallo'}
                  onInput={(ev) => alRenombrar(e.clave, ev.currentTarget.value)}
                  onBlur={() => alGuardarTitulo(e.clave)}
                />
                <p class="text-text-subtle truncate text-xs">
                  {e.nombre} · {formatBytes(locale, e.bytes)}
                </p>
                {e.estado === 'subiendo' ? (
                  <ProgressBar
                    value={e.progreso}
                    label={t('upload.uploading', { name: e.titulo })}
                  />
                ) : e.estado === 'esperando' ? (
                  <p class="text-text-muted text-xs">{t('upload.waiting')}</p>
                ) : e.estado === 'procesando' ? (
                  <p class="text-success-text text-xs">{t('upload.processing')}</p>
                ) : (
                  <p class="text-danger-text text-xs" role="alert">
                    {motivo(e)}
                  </p>
                )}
              </div>
            </div>
            <div class="flex shrink-0 items-center justify-end gap-1">
              {e.estado === 'procesando' && e.videoId ? (
                <Button size="sm" variant="secondary" href={ruta(`/manage/${e.videoId}`)}>
                  {t('upload.edit')}
                </Button>
              ) : null}
              {e.estado === 'subiendo' || e.estado === 'esperando' ? (
                <IconButton label={t('upload.cancel')} onClick={() => alCancelar(e.clave)}>
                  <Icon name="close" size={16} />
                </IconButton>
              ) : (
                <IconButton label={t('upload.remove')} onClick={() => alQuitar(e.clave)}>
                  <Icon name="close" size={16} />
                </IconButton>
              )}
            </div>
          </li>
        ))}
      </ul>
    </section>
  );
}
