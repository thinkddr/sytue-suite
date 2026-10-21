/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Enlaces públicos de un formulario (solo el propietario): crear uno con su cupo de
 * envíos, ver el estado de los que hay y revocarlos. El ENLACE completo solo se enseña al
 * crearlo: el servicio guarda solo su huella y no se puede volver a leer.
 */
import type { JSX } from 'preact';
import { useCallback, useEffect, useState } from 'preact/hooks';
import { Badge, Button, FieldGroup, Input } from '@sytue/ui';
import { formatDate, type Locale } from '@sytue/i18n';
import type { T } from '../i18n/index.js';
import { cliente, mensajeDeError } from '../lib/api.js';
import { rutaPublica } from '../lib/rutas.js';
import type { BasesFormLink } from '@sytue/api-clients/bases';

function estadoDe(e: BasesFormLink): 'revoked' | 'expired' | 'full' | 'active' {
  if (e.revoked_at) return 'revoked';
  if (e.expires_at && new Date(e.expires_at).getTime() <= Date.now()) return 'expired';
  if (e.responses >= e.max_responses) return 'full';
  return 'active';
}

export function PanelFormulario({
  t,
  locale,
  vista,
}: {
  t: T;
  locale: Locale;
  vista: string;
}): JSX.Element {
  const [enlaces, setEnlaces] = useState<BasesFormLink[]>([]);
  const [cupo, setCupo] = useState('500');
  const [nuevo, setNuevo] = useState('');
  const [error, setError] = useState('');
  const [copiado, setCopiado] = useState(false);

  const cargar = useCallback(async () => {
    try {
      setEnlaces(await cliente().listFormLinks(vista));
    } catch (e) {
      setError(mensajeDeError(e, t));
    }
  }, [vista, t]);
  useEffect(() => {
    setNuevo('');
    void cargar();
  }, [cargar]);

  async function crear(e: Event): Promise<void> {
    e.preventDefault();
    setError('');
    const n = Number.parseInt(cupo, 10);
    if (!Number.isInteger(n) || n < 1 || n > 10000) {
      setError(t('formlink.cupoInvalid'));
      return;
    }
    try {
      const r = await cliente().createFormLink(vista, { max_responses: n });
      setNuevo(`${window.location.origin}${rutaPublica(`/f/${r.token}`)}`);
      setCopiado(false);
      await cargar();
    } catch (err) {
      setError(mensajeDeError(err, t));
    }
  }

  async function revocar(id: string): Promise<void> {
    try {
      await cliente().revokeFormLink(id);
      setNuevo('');
      await cargar();
    } catch (err) {
      setError(mensajeDeError(err, t));
    }
  }

  return (
    <section
      aria-labelledby="enlaces-formulario"
      class="border-border flex flex-col gap-3 rounded-lg border p-3"
    >
      <h3 id="enlaces-formulario" class="text-text text-sm font-semibold">
        {t('formlink.title')}
      </h3>
      <p class="text-text-muted text-xs">{t('formlink.help')}</p>
      <form class="flex flex-wrap items-end gap-2" onSubmit={(e) => void crear(e)}>
        <FieldGroup label={t('formlink.cupo')} htmlFor="cupo-formulario">
          <Input
            id="cupo-formulario"
            inputMode="numeric"
            value={cupo}
            onInput={(e) => setCupo(e.currentTarget.value)}
          />
        </FieldGroup>
        <Button type="submit" variant="secondary">
          {t('formlink.create')}
        </Button>
      </form>
      {nuevo ? (
        <div class="flex flex-col gap-2" role="status">
          <p class="text-text text-sm">{t('formlink.once')}</p>
          <div class="flex flex-wrap items-center gap-2">
            <Input
              aria-label={t('formlink.url')}
              readOnly
              value={nuevo}
              onFocus={(e) => e.currentTarget.select()}
              class="min-w-0 flex-1"
            />
            <Button
              variant="ghost"
              size="sm"
              onClick={() => {
                void navigator.clipboard?.writeText(nuevo).then(() => setCopiado(true));
              }}
            >
              {copiado ? t('formlink.copied') : t('formlink.copy')}
            </Button>
          </div>
        </div>
      ) : null}
      {error ? (
        <p role="alert" class="text-danger-text text-sm">
          {error}
        </p>
      ) : null}
      {enlaces.length === 0 ? (
        <p class="text-text-muted text-sm">{t('formlink.none')}</p>
      ) : (
        <ul class="flex flex-col gap-2">
          {enlaces.map((e) => {
            const st = estadoDe(e);
            return (
              <li key={e.id} class="flex flex-wrap items-center justify-between gap-2 text-sm">
                <span class="text-text flex min-w-0 flex-wrap items-center gap-2">
                  <Badge tone={st === 'active' ? 'success' : 'neutral'}>
                    {t(`formlink.state.${st}`)}
                  </Badge>
                  {t('formlink.usage', { n: e.responses, max: e.max_responses })}
                  <span class="text-text-muted">{formatDate(locale, e.created_at)}</span>
                </span>
                {st === 'active' ? (
                  <Button variant="ghost" size="sm" onClick={() => void revocar(e.id)}>
                    {t('formlink.revoke')}
                  </Button>
                ) : null}
              </li>
            );
          })}
        </ul>
      )}
    </section>
  );
}
