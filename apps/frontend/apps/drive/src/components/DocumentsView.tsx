/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * DocumentsView — «Documentos» de Sytue Docs, vistos desde Sytue Drive.
 *
 * POR QUÉ EXISTE: un documento de Docs no es un fichero de Drive. Son dos almacenes
 * distintos y un documento no tiene carpeta, así que «Nuevo → Documento» creaba algo que
 * DESAPARECÍA de la vista: quedaba en Sytue Docs y Drive no sabía de él. Esta sección lo
 * enseña donde el usuario lo creó, sin copiar ni sincronizar nada — se lista lo que Docs
 * ya sabe, y abrir lleva a su editor.
 *
 * Es de solo lectura a propósito: renombrar, mover o borrar un documento son operaciones
 * de Docs, y duplicarlas aquí crearía dos sitios donde arreglar el mismo fallo.
 *
 * Idioma: lo publica <SytueShell> en el contexto de i18n a partir del locale que negoció
 * el SERVIDOR (`src/middleware.ts`); aquí solo se lee, nunca se detecta.
 */
import { useCallback, useEffect, useState } from 'preact/hooks';
import { Illustration, Button, EmptyState, Icon, Input, Skeleton } from '@sytue/ui';
import { formatDateTime, formatTimeAgo } from '@sytue/i18n';
import { useI18n } from '@sytue/i18n/preact';
import { useT } from '../i18n';
import type { DriveMessageKey } from '../i18n';
import { Glyph } from './glyphs';

/** Un documento tal y como lo devuelve el BFF (/api/documents). */
interface DocItem {
  id: string;
  title: string;
  kind: 'document' | 'spreadsheet' | 'presentation';
  updatedAt: string;
  href: string;
}

/**
 * CLAVE de catálogo por tipo. El glifo es el del propio nombre del tipo: los tres viven
 * en `glyphs.tsx` con el mismo encuadre, así que la columna usa UN solo sistema de icono.
 * Antes mezclaba `Glyph` (encuadre 20 de 24) con `Icon` (14 de 24), y al mismo tamaño el
 * documento se veía un 60 % más grande que la hoja, en la misma columna.
 */
const KIND: Record<DocItem['kind'], DriveMessageKey> = {
  document: 'docs.kind.document',
  spreadsheet: 'docs.kind.spreadsheet',
  presentation: 'docs.kind.presentation',
};

export function DocumentsView() {
  // `Intl.RelativeTimeFormat` (vía formatTimeAgo) da «ahora / hace 5 min / ayer» en los
  // cinco idiomas: no hace falta ni una tabla de plurales a mano.
  const { locale } = useI18n();
  const t = useT(locale);
  const [items, setItems] = useState<DocItem[] | null>(null);
  const [error, setError] = useState<'session' | 'network' | null>(null);
  const [q, setQ] = useState('');

  // El filtro va AL SERVICIO (índice trigram sobre el título): traer todo para filtrar en
  // el navegador se rompe en cuanto alguien tiene mil documentos.
  const load = useCallback(async (query: string) => {
    setItems(null);
    setError(null);
    try {
      const res = await fetch(`/api/documents${query ? `?q=${encodeURIComponent(query)}` : ''}`, {
        credentials: 'include',
        headers: { accept: 'application/json' },
      });
      if (res.status === 401) {
        setError('session');
        return;
      }
      if (!res.ok) throw new Error(String(res.status));
      const data = (await res.json()) as { documents?: DocItem[] };
      setItems(data.documents ?? []);
    } catch {
      setError('network');
    }
  }, []);

  // Rebote de 300 ms al teclear: una petición por pulsación castigaría al servicio y
  // haría parpadear la lista.
  useEffect(() => {
    const id = window.setTimeout(() => void load(q.trim()), q ? 300 : 0);
    return () => window.clearTimeout(id);
  }, [q, load]);

  return (
    <div class="flex h-full flex-col">
      <header class="border-border flex shrink-0 flex-wrap items-center gap-3 border-b px-4 py-3">
        <span class="text-text-muted shrink-0">
          <Glyph name="file" size={18} />
        </span>
        <span class="min-w-0 grow">
          <h1 class="text-text truncate text-base font-semibold">{t('docs.title')}</h1>
          <p class="text-text-muted truncate text-sm">{t('docs.subtitle')}</p>
        </span>
        <Input
          type="search"
          value={q}
          onInput={(e) => setQ(e.currentTarget.value)}
          placeholder={t('docs.search')}
          aria-label={t('docs.searchAria')}
          class="w-full sm:w-64"
        />
      </header>

      <div class="min-h-0 flex-1 overflow-y-auto">
        {items === null && !error ? (
          <ul class="px-3 pb-6" aria-hidden="true">
            {[0, 1, 2, 3, 4].map((i) => (
              <li key={i} class="border-border/60 flex items-center gap-3 border-b px-3 py-2">
                <Skeleton class="h-9 w-9 rounded-md" />
                <Skeleton class="h-4 w-1/3" />
              </li>
            ))}
          </ul>
        ) : error ? (
          <div class="grid h-full place-items-center p-6">
            <EmptyState
              title={t(error === 'session' ? 'files.error.sessionTitle' : 'docs.errorTitle')}
              description={t(error === 'session' ? 'docs.sessionBody' : 'docs.errorBody')}
              illustration={<Icon name="warning" size={40} class="text-danger-text" />}
              action={
                error === 'session' ? (
                  <Button size="sm" onClick={() => location.reload()}>
                    {t('files.error.sessionCta')}
                  </Button>
                ) : (
                  <Button variant="secondary" size="sm" onClick={() => void load(q.trim())}>
                    {t('files.retry')}
                  </Button>
                )
              }
            />
          </div>
        ) : items && items.length === 0 ? (
          <div class="grid h-full place-items-center p-6">
            <EmptyState
              title={t(q ? 'docs.emptyQueryTitle' : 'docs.emptyTitle')}
              description={t(q ? 'docs.emptyQueryBody' : 'docs.emptyBody')}
              illustration={<Illustration name="emptyDrive" />}
            />
          </div>
        ) : (
          <ul class="px-3 pb-6">
            {items!.map((d) => {
              const label = KIND[d.kind] ?? KIND.document;
              // 20 px, el tamaño denso de la escala del sistema (16/20/24). Los tres
              // glifos comparten encuadre, así que ninguno pesa más que otro.
              const glifo = KIND[d.kind] ? d.kind : 'document';
              return (
                <li
                  key={d.id}
                  class="group border-border/60 hover:bg-bg-subtle flex items-center gap-3 border-b px-3 py-2 transition-colors duration-150"
                >
                  <span class="text-text-muted shrink-0">
                    <Glyph name={glifo} size={20} />
                  </span>
                  <span class="min-w-0 flex-1">
                    <a
                      href={d.href}
                      class="text-text sytue-focus-ring block max-w-full truncate rounded text-left font-medium hover:underline"
                      title={t('docs.open', { title: d.title })}
                    >
                      {d.title}
                    </a>
                    <span class="text-text-muted block truncate text-sm">{t(label)}</span>
                  </span>
                  <span
                    class="text-text-muted shrink-0 text-sm tabular-nums"
                    title={formatDateTime(locale, d.updatedAt)}
                  >
                    {formatTimeAgo(locale, d.updatedAt)}
                  </span>
                </li>
              );
            })}
          </ul>
        )}
      </div>
    </div>
  );
}
