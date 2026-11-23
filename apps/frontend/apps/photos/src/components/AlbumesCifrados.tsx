/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Los álbumes de la galería cifrada (ADR 0086), en la página de álbumes: registros
 * cifrados en el Drive de la bóveda (`client/galeriaCifrada.ts`), creados, llenados
 * y vaciados SOLO ahí. El servidor no sabe que existen. Solo aparece si el Drive de
 * la persona está en la bóveda, y no abre la bóveda sin que se pida.
 */
import type { JSX } from 'preact';
import { useEffect, useMemo, useState } from 'preact/hooks';
import { Button, EmptyState, Icon, usePrompt } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import type { SytueUser } from '@sytue/auth';
import { useT } from '../i18n/index.js';
import {
  FotosCifradas,
  MiniaturaCifrada,
  VisorCifrado,
  useAccionesCifradas,
  useDriveCifrado,
  useGaleriaCifrada,
} from './FotosCifradas.js';

export function AlbumesCifrados({
  locale,
  user,
}: {
  locale: Locale;
  user: SytueUser;
}): JSX.Element | null {
  const t = useT(locale);
  const cifrado = useDriveCifrado(user.id, user.tenant);
  const viva = useGaleriaCifrada(cifrado);
  const acciones = useAccionesCifradas(viva, locale);
  const { preguntar, dialogo } = usePrompt();
  const [elegido, setElegido] = useState<string | null>(null);
  const [abierta, setAbierta] = useState<string | null>(null);

  const albumes = useMemo(() => viva.indice?.albumes ?? [], [viva.indice]);
  const delAlbum = useMemo(
    () => (viva.indice?.fotos ?? []).filter((f) => elegido && f.entrada.albumes.includes(elegido)),
    [viva.indice, elegido],
  );
  const portadas = useMemo(() => {
    const out = new Map<
      string,
      { cuantas: number; portada: (typeof delAlbum)[number] | undefined }
    >();
    for (const a of albumes) out.set(a.id, { cuantas: 0, portada: undefined });
    for (const f of viva.indice?.fotos ?? [])
      for (const a of f.entrada.albumes) {
        const p = out.get(a);
        if (p) {
          p.cuantas++;
          p.portada ??= f;
        }
      }
    return out;
  }, [albumes, viva.indice]);
  const { pedir } = viva;
  useEffect(() => {
    const visibles = [...portadas.values()].flatMap((p) => (p.portada ? [p.portada] : []));
    pedir([...visibles, ...delAlbum]);
  }, [pedir, portadas, delAlbum]);

  if (!cifrado.enBoveda && cifrado.estado.fase === 'cerrado') return null;
  const album = albumes.find((a) => a.id === elegido);

  return (
    <section class="mt-10" aria-label={t('gallery.encrypted.albums')}>
      <div class="mb-3 flex flex-wrap items-center gap-2">
        <Icon name="lock" size={16} />
        <h2 class="text-base font-semibold">{t('gallery.encrypted.albums')}</h2>
        {viva.galeria ? (
          <Button
            variant="ghost"
            size="sm"
            class="ml-auto"
            leadingIcon={<Icon name="plus" />}
            onClick={() =>
              void preguntar({
                titulo: t('gallery.encrypted.newAlbum'),
                mensaje: t('albums.title'),
                confirmLabel: t('gallery.newAlbumCreate'),
              }).then(async (titulo) => {
                if (!titulo?.trim()) return;
                const nuevo = await acciones.crearAlbum(titulo);
                if (nuevo) setElegido(nuevo.id);
              })
            }
          >
            {t('gallery.encrypted.newAlbum')}
          </Button>
        ) : null}
      </div>
      <FotosCifradas locale={locale} cifrado={cifrado} viva={viva} />
      {viva.indice && albumes.length > 0 ? (
        <ul class="grid grid-cols-[repeat(auto-fill,minmax(11rem,1fr))] gap-6">
          {albumes.map((a) => {
            const p = portadas.get(a.id);
            const url = p?.portada ? viva.url(p.portada) : undefined;
            return (
              <li key={a.id}>
                <button
                  type="button"
                  class="sytue-focus-ring flex w-full flex-col gap-2 rounded-md text-left"
                  aria-pressed={elegido === a.id}
                  onClick={() => setElegido(elegido === a.id ? null : a.id)}
                >
                  <span class="bg-bg-subtle relative block aspect-square w-full overflow-hidden rounded-md">
                    {url ? (
                      <img src={url} alt="" class="h-full w-full object-cover" />
                    ) : (
                      <span class="text-text-muted absolute inset-0 flex items-center justify-center">
                        <Icon name="lock" size={20} />
                      </span>
                    )}
                  </span>
                  <span class="text-sm font-medium">{a.titulo}</span>
                  <span class="text-text-muted text-xs tabular-nums">{p?.cuantas ?? 0}</span>
                </button>
              </li>
            );
          })}
        </ul>
      ) : null}
      {album ? (
        <div class="mt-6">
          <h3 class="mb-2 text-sm font-semibold">{album.titulo}</h3>
          {delAlbum.length === 0 ? (
            <EmptyState compact title={t('gallery.encrypted.albumEmpty')} />
          ) : (
            <ul class="filas">
              {delAlbum.map((f) => (
                <MiniaturaCifrada
                  key={f.id}
                  foto={f}
                  url={viva.url(f)}
                  locale={locale}
                  onOpen={() => setAbierta(f.id)}
                  onFavorite={() => void acciones.favorita(f, !f.entrada.favorita)}
                />
              ))}
            </ul>
          )}
        </div>
      ) : null}
      {abierta !== null ? (
        <VisorCifrado
          locale={locale}
          viva={viva}
          fotos={delAlbum}
          inicial={abierta}
          alCerrar={() => setAbierta(null)}
        />
      ) : null}
      {dialogo}
    </section>
  );
}
