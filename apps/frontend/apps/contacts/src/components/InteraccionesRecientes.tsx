/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Interacciones recientes» de la ficha: los últimos correos y eventos del usuario
 * con esa persona, de la búsqueda federada del Hub. Solo lectura; cada fila abre
 * el correo o el evento en su app.
 *
 * Se pide al abrir la ficha y no antes: es una búsqueda federada por contacto, y
 * recorrer la lista no debe lanzar una por fila.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { Card, CardHeader, CardTitle, Icon } from '@sytue/ui';
import { formatTimeAgo, type Locale } from '@sytue/i18n';
import type { SearchResult } from '@sytue/api-clients';
import type { T } from '../i18n/index.js';
import { interaccionesRecientes } from '../client/contactsApi.js';

type Estado = { fase: 'cargando' } | { fase: 'lista'; items: SearchResult[] } | { fase: 'error' };

/** Solo enlaces http(s) o rutas de la suite: el `url` viene de otro servicio. */
function enlaceSeguro(url: string): string | null {
  if (url.startsWith('/') && !url.startsWith('//')) return url;
  try {
    const u = new URL(url);
    return u.protocol === 'https:' || u.protocol === 'http:' ? u.href : null;
  } catch {
    return null;
  }
}

export function InteraccionesRecientes({
  t,
  locale,
  emails,
  cargar = interaccionesRecientes,
}: {
  t: T;
  locale: Locale;
  emails: readonly string[];
  /** Inyectable para las pruebas; en la app, el BFF. */
  cargar?: (emails: readonly string[]) => Promise<SearchResult[]>;
}): JSX.Element | null {
  const clave = emails.join('\n');
  const [estado, setEstado] = useState<Estado>({ fase: 'cargando' });

  useEffect(() => {
    if (!clave) return;
    // Al cambiar de ficha deprisa, la respuesta de la anterior no puede pintar
    // en esta: se descarta si llega tarde.
    let vigente = true;
    setEstado({ fase: 'cargando' });
    cargar(clave.split('\n')).then(
      (items) => vigente && setEstado({ fase: 'lista', items }),
      () => vigente && setEstado({ fase: 'error' }),
    );
    return () => {
      vigente = false;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [clave]);

  // Sin correo no hay a quién buscar: la sección ni aparece.
  if (!clave) return null;

  return (
    <Card class="mt-6">
      <CardHeader>
        <CardTitle>{t('detail.interactionsTitle')}</CardTitle>
      </CardHeader>
      <div class="px-4 pb-4 sm:px-6" aria-live="polite" aria-busy={estado.fase === 'cargando'}>
        {estado.fase === 'cargando' ? (
          <p class="text-text-subtle text-sm">{t('detail.interactionsLoading')}</p>
        ) : estado.fase === 'error' ? (
          <p class="text-text-subtle text-sm">{t('detail.interactionsError')}</p>
        ) : estado.items.length === 0 ? (
          <p class="text-text-subtle text-sm">{t('detail.interactionsEmpty')}</p>
        ) : (
          <ul class="divide-border -mx-2 flex flex-col divide-y">
            {estado.items.map((r) => (
              <Fila key={`${r.source}:${r.id}`} t={t} locale={locale} r={r} />
            ))}
          </ul>
        )}
      </div>
    </Card>
  );
}

function Fila({ t, locale, r }: { t: T; locale: Locale; r: SearchResult }): JSX.Element {
  const href = enlaceSeguro(r.url);
  const tipo = r.source === 'calendar' ? t('detail.interactionEvent') : t('detail.interactionMail');
  const cuerpo = (
    <>
      <Icon name={r.source === 'calendar' ? 'calendar' : 'mail'} class="text-text-muted shrink-0" />
      <span class="min-w-0 flex-1">
        <span class="text-text block truncate">{r.title || '—'}</span>
        <span class="sytue-meta block truncate">
          {tipo}
          {r.timestamp ? ` · ${formatTimeAgo(locale, r.timestamp)}` : ''}
        </span>
      </span>
    </>
  );
  const clase =
    'flex min-h-11 items-center gap-3 rounded-sm px-2 py-2 focus-visible:outline-none sytue-focus-ring-inset';
  return (
    <li>
      {href ? (
        <a href={href} class={`${clase} hover:bg-bg-subtle`}>
          {cuerpo}
        </a>
      ) : (
        <div class={clase}>{cuerpo}</div>
      )}
    </li>
  );
}
