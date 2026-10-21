/**
 * «Fotos de por aquí»: las hechas cerca de una, de la más próxima a la más
 * lejana.
 *
 * Cerca son DOS KILÓMETROS —el mismo sitio, no la misma ciudad—, y lo decide el
 * servicio: un radio elegible acabaría siendo un deslizador que nadie mueve
 * salvo para descubrir que a diez kilómetros sale media galería.
 *
 * No tiene nada que ver con «parecidas»: aquí no se mira ni un vector. Son las
 * coordenadas del EXIF, que están desde el día que se subió la foto y no
 * dependen de que ningún lote la haya mirado.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { Button, EmptyState, Icon, Illustration, Spinner } from '@sytue/ui';
import type { SytueUser } from '@sytue/auth';
import type { Locale } from '@sytue/i18n';
import { cerca as pedirCerca, foto as pedirFoto, type Foto } from '../client/photosApi.js';
import { useT } from '../i18n/index.js';
import { RUTA } from '../lib/ruta.js';
import { Marco } from './Marco.js';
import { Miniatura, Visor } from './PhotosApp.js';
import { Nav } from './Nav.js';

export function Cerca({
  locale,
  id,
  user,
  hubBaseUrl,
}: {
  locale: Locale;
  id: string;
  user: SytueUser;
  hubBaseUrl: string;
}): JSX.Element {
  const t = useT(locale);
  const [referencia, setReferencia] = useState<Foto | null>(null);
  const [fotos, setFotos] = useState<Foto[] | null>(null);
  const [abierta, setAbierta] = useState<number | null>(null);
  const [error, setError] = useState('');

  useEffect(() => {
    Promise.all([pedirFoto(id), pedirCerca(id)])
      .then(([r, lista]) => {
        setReferencia(r);
        setFotos(lista);
      })
      .catch((e) => setError(String(e)));
  }, [id]);

  return (
    <Marco user={user} hubBaseUrl={hubBaseUrl} locale={locale} title={t('nearby.title')}>
      <div class="mx-auto w-full max-w-7xl px-4 py-6">
        <Nav t={t} locale={locale} activa="photos" />
        <header class="mb-4 flex flex-wrap items-center gap-3">
          <Button href={RUTA} variant="ghost" size="sm" leadingIcon={<Icon name="chevronLeft" />}>
            {t('similar.back')}
          </Button>
          <h1 class="truncate text-lg font-semibold">{t('nearby.title')}</h1>
          {/* Las coordenadas de la foto de partida, que son la pregunta. */}
          {referencia?.location ? (
            <span class="text-text-muted text-sm tabular-nums">
              {referencia.location.lat.toFixed(4)}, {referencia.location.lon.toFixed(4)}
            </span>
          ) : null}
          {fotos ? (
            <span class="text-text-muted text-sm">
              {t('nearby.count', { count: fotos.length })}
            </span>
          ) : null}
        </header>

        {error ? (
          <p class="text-danger-text mb-4 text-sm" role="alert">
            {error}
          </p>
        ) : null}

        {fotos === null ? (
          <Spinner />
        ) : fotos.length === 0 ? (
          // Vacío no es un fallo: o la foto no trae coordenadas —la mayoría de
          // las de una cámara sin GPS— o de verdad no hay otra por allí.
          <EmptyState
            class="flex-1"
            title={t('nearby.empty.title')}
            description={t('nearby.empty.description')}
            illustration={<Illustration name="emptyPhotos" />}
          />
        ) : (
          <ul class="filas">
            {fotos.map((f, i) => (
              <Miniatura key={f.id} foto={f} t={t} onOpen={() => setAbierta(i)} />
            ))}
          </ul>
        )}

        {abierta !== null && fotos?.[abierta] ? (
          <Visor
            foto={fotos[abierta]}
            t={t}
            locale={locale}
            onClose={() => setAbierta(null)}
            vecinas={[fotos[abierta - 1], fotos[abierta + 1]]}
            onPrev={abierta > 0 ? () => setAbierta(abierta - 1) : undefined}
            onNext={abierta < fotos.length - 1 ? () => setAbierta(abierta + 1) : undefined}
          />
        ) : null}
      </div>
    </Marco>
  );
}
