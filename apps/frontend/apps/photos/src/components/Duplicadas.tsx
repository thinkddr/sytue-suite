/**
 * «Fotos que se repiten»: ráfagas y reintentos del mismo momento, agrupados.
 *
 * SE PROPONE, NO SE EJECUTA. Cada grupo enseña sus fotos y quien mira elige con
 * cuál se queda; las demás van a la papelera, de donde todavía se recuperan.
 * Un botón de «limpiar todo» sería más cómodo y es exactamente cómo se pierde
 * la única foto buena de una ráfaga.
 *
 * El grupo desaparece de la lista al resolverlo, sin recargar: lo que queda en
 * pantalla son las decisiones que faltan.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { Button, EmptyState, Icon, Illustration, Spinner, cx } from '@sytue/ui';
import type { SytueUser } from '@sytue/auth';
import type { Locale } from '@sytue/i18n';
import {
  alaPapelera,
  duplicadas,
  textoAlternativo,
  urlDeVariante,
  type Foto,
  type GrupoRepetido,
} from '../client/photosApi.js';
import { useT } from '../i18n/index.js';
import { Marco } from './Marco.js';
import { Nav } from './Nav.js';

export function Duplicadas({
  locale,
  user,
  hubBaseUrl,
}: {
  locale: Locale;
  user: SytueUser;
  hubBaseUrl: string;
}): JSX.Element {
  const t = useT(locale);
  const [grupos, setGrupos] = useState<GrupoRepetido[] | null>(null);
  const [error, setError] = useState('');
  const [aviso, setAviso] = useState('');
  const [trabajando, setTrabajando] = useState(false);

  useEffect(() => {
    duplicadas()
      .then(setGrupos)
      .catch((e) => setError(String(e)));
  }, []);

  const quedarse = async (indice: number, elegida: Foto) => {
    const grupo = grupos?.[indice];
    if (!grupo || trabajando) return;
    const sobran = grupo.items.filter((f) => f.id !== elegida.id);
    setTrabajando(true);
    try {
      // En paralelo y tolerando fallos sueltos: que una no se pueda tirar no
      // puede dejar el grupo a medias sin decir nada.
      const hechas = await Promise.allSettled(sobran.map((f) => alaPapelera(f.id)));
      const bien = hechas.filter((r) => r.status === 'fulfilled').length;
      setGrupos((gs) => (gs ?? []).filter((_, i) => i !== indice));
      setAviso(t('dup.done', { count: bien }));
      if (bien < sobran.length) setError(t('gallery.trashFailed', { count: sobran.length - bien }));
    } catch (e) {
      setError(String(e));
    } finally {
      setTrabajando(false);
    }
  };

  return (
    <Marco user={user} hubBaseUrl={hubBaseUrl} locale={locale} title={t('dup.title')}>
      <div class="mx-auto flex h-full w-full max-w-7xl flex-col px-4 py-6">
        <Nav t={t} locale={locale} activa="duplicates" />
        <header class="mb-4">
          <h1 class="text-lg font-semibold">{t('dup.title')}</h1>
          <p class="text-text-muted max-w-2xl text-sm">{t('dup.explain')}</p>
        </header>
        <p role="status" aria-live="polite" class="text-text-muted min-h-5 text-sm">
          {aviso}
        </p>
        {error ? (
          <p class="text-danger-text mb-4 text-sm" role="alert">
            {error}
          </p>
        ) : null}

        {grupos === null ? (
          <Spinner />
        ) : grupos.length === 0 ? (
          <EmptyState
            class="flex-1"
            title={t('dup.empty.title')}
            description={t('dup.empty.description')}
            illustration={<Illustration name="emptyPhotos" />}
          />
        ) : (
          <>
            <p class="text-text-muted mb-3 text-sm">{t('dup.count', { count: grupos.length })}</p>
            <ul class="flex flex-col gap-6">
              {grupos.map((g, i) => (
                <li key={g.items[0]?.id ?? i} class="border-border rounded-md border p-3">
                  <ul class="flex flex-wrap gap-3">
                    {g.items.map((f) => (
                      <li key={f.id} class="w-40">
                        <img
                          src={urlDeVariante(f.id, 'miniatura')}
                          alt={textoAlternativo(f)}
                          width={f.width}
                          height={f.height}
                          loading="lazy"
                          decoding="async"
                          class="bg-bg-subtle h-40 w-40 rounded-sm object-cover"
                        />
                        <Button
                          variant="secondary"
                          size="sm"
                          class={cx('mt-2 w-full', trabajando && 'opacity-60')}
                          disabled={trabajando}
                          onClick={() => void quedarse(i, f)}
                        >
                          <Icon name="check" size={16} />
                          {t('dup.keep')}
                        </Button>
                      </li>
                    ))}
                  </ul>
                  <p class="text-text-subtle mt-2 text-xs">
                    {t('dup.trashRest', { count: g.items.length - 1 })}
                  </p>
                </li>
              ))}
            </ul>
          </>
        )}
      </div>
    </Marco>
  );
}
