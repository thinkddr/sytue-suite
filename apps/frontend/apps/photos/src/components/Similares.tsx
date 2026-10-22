/**
 * «Parecidas a esta» y «más como estas»: la foto (o las fotos) de referencia
 * arriba y debajo las que más se les parecen, de más a menos.
 *
 * Se apoya en los vectores que escribe el lote semanal (deploy/fotos-gpu), así
 * que NO necesita el modelo en la mano: comparar dos vectores ya guardados es
 * un producto escalar. Buscar escribiendo «playa» sí lo necesitaría, para
 * vectorizar lo escrito, y por eso esto es lo que hay y no un buscador.
 *
 * UNA sola referencia y VARIAS no son la misma pregunta: con una se busca lo
 * que se le parece; con varias, lo que tienen EN COMÚN. Por eso las de varias
 * van por otra ruta del servicio, la que promedia los vectores.
 *
 * Reutiliza la tesela y el visor de la galería. Una cuadrícula propia se vería
 * distinta a la de al lado sin ninguna razón, y habría que arreglarla dos veces.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { Button, EmptyState, Icon, Illustration, Spinner, usePrompt } from '@sytue/ui';
import type { SytueUser } from '@sytue/auth';
import type { Locale } from '@sytue/i18n';
import {
  anyadirAlAlbum,
  crearAlbum,
  foto as pedirFoto,
  parecidas,
  parecidasAVarias,
  textoAlternativo,
  urlDeVariante,
  type Foto,
} from '../client/photosApi.js';
import { RUTA } from '../lib/ruta.js';
import { tituloSugerido } from '../lib/tituloAlbum.js';
import { useT } from '../i18n/index.js';
import { Marco } from './Marco.js';
import { Miniatura, Visor } from './PhotosApp.js';
import { Nav } from './Nav.js';

export function Similares({
  locale,
  ids,
  user,
  hubBaseUrl,
}: {
  locale: Locale;
  ids: string[];
  user: SytueUser;
  hubBaseUrl: string;
}): JSX.Element {
  const t = useT(locale);
  const [referencias, setReferencias] = useState<Foto[]>([]);
  const [fotos, setFotos] = useState<Foto[] | null>(null);
  const [abierta, setAbierta] = useState<number | null>(null);
  const [error, setError] = useState('');
  const [aviso, setAviso] = useState('');
  const { preguntar, dialogo: dialogoPreguntar } = usePrompt();

  /**
   * Guardar lo encontrado como álbum, que es lo que se quiere hacer con ello.
   *
   * Sin esto, quien llega aquí desde una búsqueda ve el montón de fotos que
   * andaba buscando y no tiene forma de quedárselo: tendría que volver a la
   * galería y marcarlas a mano una por una. El nombre viene propuesto con lo
   * que Sytue AI vio en ellas, y editable.
   */
  const guardarComoAlbum = async () => {
    if (!fotos || fotos.length === 0) return;
    const titulo = await preguntar({
      titulo: t('similar.saveAlbum'),
      mensaje: t('gallery.newAlbumPrompt', { count: fotos.length }),
      confirmLabel: t('gallery.newAlbumCreate'),
      valor: tituloSugerido(fotos),
    });
    if (titulo === null || titulo.trim() === '') return;
    try {
      const album = await crearAlbum(titulo.trim());
      await anyadirAlAlbum(
        album.id,
        fotos.map((f) => f.id),
      );
      setAviso(t('gallery.addedToAlbum', { count: fotos.length, title: album.title }));
    } catch (e) {
      setError(String(e));
    }
  };
  // Una cadena y no el array: un array nuevo en cada render volvería a lanzar
  // la consulta para siempre aunque los ids sean exactamente los mismos.
  const clave = ids.join(',');

  useEffect(() => {
    const lista = clave.split(',');
    const primera = lista[0] ?? '';
    // Solo las tres primeras para la cabecera: es para saber de qué se partió,
    // no para volver a enseñar la selección entera.
    const cabecera = lista.slice(0, 3).map((x) => pedirFoto(x).catch(() => null));
    const parecidasA = lista.length === 1 ? parecidas(primera) : parecidasAVarias(lista);
    Promise.all([Promise.all(cabecera), parecidasA])
      .then(([refs, encontradas]) => {
        setReferencias(refs.filter((r): r is Foto => r !== null));
        setFotos(encontradas);
      })
      .catch((e) => setError(String(e)));
  }, [clave]);

  const titulo = ids.length === 1 ? t('similar.title') : t('similar.titleMany');

  return (
    <Marco user={user} hubBaseUrl={hubBaseUrl} locale={locale} title={titulo}>
      <div class="mx-auto w-full max-w-7xl px-4 py-6">
        <Nav t={t} locale={locale} activa="photos" />
        <header class="mb-4 flex flex-wrap items-center gap-3">
          <Button href={RUTA} variant="ghost" size="sm" leadingIcon={<Icon name="chevronLeft" />}>
            {t('similar.back')}
          </Button>
          {/* Las referencias se ven, y pequeñas: quien llega aquí desde el visor
              ya sabe qué foto es, pero quien llega por un enlace no. */}
          {referencias.map((r) => (
            <img
              key={r.id}
              src={urlDeVariante(r.id, 'miniatura')}
              alt={textoAlternativo(r)}
              width={r.width}
              height={r.height}
              class="h-12 w-12 rounded-sm object-cover"
            />
          ))}
          <h1 class="truncate text-lg font-semibold">{titulo}</h1>
          {fotos ? (
            <span class="text-text-muted text-sm">
              {t('similar.count', { count: fotos.length })}
            </span>
          ) : null}
          {fotos && fotos.length > 0 ? (
            <Button
              variant="primary"
              size="sm"
              class="ml-auto"
              leadingIcon={<Icon name="album" />}
              onClick={() => void guardarComoAlbum()}
            >
              {t('similar.saveAlbum')}
            </Button>
          ) : null}
        </header>
        {aviso ? (
          <p class="text-text-muted mb-4 text-sm" role="status" aria-live="polite">
            {aviso}
          </p>
        ) : null}

        {error ? (
          <p class="text-danger-text mb-4 text-sm" role="alert">
            {error}
          </p>
        ) : null}

        {fotos === null ? (
          <Spinner />
        ) : fotos.length === 0 ? (
          // Lista vacía NO es un fallo: o el lote todavía no ha mirado esas
          // fotos, o de verdad no hay nada que se les parezca. Decirlo con esas
          // palabras evita que parezca una avería.
          <EmptyState
            class="flex-1"
            title={t('similar.empty.title')}
            description={t('similar.empty.description')}
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
        {dialogoPreguntar}
      </div>
    </Marco>
  );
}
