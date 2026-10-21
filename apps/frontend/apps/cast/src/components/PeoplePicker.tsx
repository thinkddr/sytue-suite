/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * PeoplePicker — con quién se comparte un vídeo de visibilidad «personas».
 *
 * Se escribe «@ana», «ana» o un correo y el SERVICIO lo resuelve contra Sytue ID
 * (`POST /videos/{id}/people`, que llama a people:resolve con el token de quien
 * comparte): la web no tiene un directorio que consultar ni debe tenerlo. Cada
 * persona añadida es un chip con su nombre y su @usuario, y se quita con su botón.
 */
import type { JSX } from 'preact';
import { useEffect, useId, useState } from 'preact/hooks';
import { Avatar, Button, Icon, IconButton, Input } from '@sytue/ui';
import type { T } from '../i18n/index.js';
import { anadirPersona, personas, quitarPersona } from '../client/castApi.js';
import { FalloDelServicio } from '../lib/fallo.js';
import type { Persona } from '../lib/tipos.js';

export function PeoplePicker({ t, videoId }: { t: T; videoId: string }): JSX.Element {
  const id = useId();
  const [lista, setLista] = useState<Persona[] | null>(null);
  const [consulta, setConsulta] = useState('');
  const [ocupado, setOcupado] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    personas(videoId)
      .then(setLista)
      .catch(() => setLista([]));
  }, [videoId]);

  const anadir = async (e: Event): Promise<void> => {
    e.preventDefault();
    const q = consulta.trim();
    if (!q) return;
    setOcupado(true);
    setError(null);
    try {
      const p = await anadirPersona(videoId, q);
      setLista((l) => [...(l ?? []).filter((x) => x.user_id !== p.user_id), p]);
      setConsulta('');
    } catch (err) {
      setError(
        err instanceof FalloDelServicio && (err.status === 404 || err.status === 400)
          ? t('manage.people.notFound', { query: q })
          : t('manage.people.error'),
      );
    } finally {
      setOcupado(false);
    }
  };

  const quitar = async (p: Persona): Promise<void> => {
    const antes = lista;
    setLista((l) => (l ?? []).filter((x) => x.user_id !== p.user_id));
    try {
      await quitarPersona(videoId, p.user_id);
    } catch {
      setLista(antes);
    }
  };

  return (
    <div class="flex flex-col gap-3">
      <form onSubmit={(e) => void anadir(e)} class="flex flex-col gap-1.5">
        <label for={id} class="text-text text-sm font-semibold">
          {t('manage.people.label')}
        </label>
        <div class="flex gap-2">
          <Input
            id={id}
            value={consulta}
            placeholder={t('manage.people.placeholder')}
            autoComplete="off"
            invalid={Boolean(error)}
            aria-describedby={error ? `${id}-error` : undefined}
            onInput={(e) => setConsulta(e.currentTarget.value)}
            class="min-w-0 flex-1"
          />
          <Button type="submit" variant="secondary" loading={ocupado} disabled={!consulta.trim()}>
            {t('manage.people.add')}
          </Button>
        </div>
        {error ? (
          <p
            id={`${id}-error`}
            class="text-danger-text flex items-center gap-1 text-xs"
            role="alert"
          >
            <Icon name="danger" size={12} />
            {error}
          </p>
        ) : null}
      </form>
      {lista === null ? null : lista.length === 0 ? (
        <p class="text-text-muted text-sm">{t('manage.people.empty')}</p>
      ) : (
        <ul class="flex flex-wrap gap-2">
          {lista.map((p) => (
            <li
              key={p.user_id}
              class="border-border bg-surface flex items-center gap-2 rounded-full border py-1 pr-1 pl-1.5"
            >
              <Avatar name={p.name} size="xs" />
              <span class="text-text text-sm">{p.name}</span>
              {p.username ? <span class="text-text-subtle text-xs">@{p.username}</span> : null}
              <IconButton
                size="sm"
                label={t('manage.people.remove', { name: p.name })}
                onClick={() => void quitar(p)}
              >
                <Icon name="close" size={14} />
              </IconButton>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
