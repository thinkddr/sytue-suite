/**
 * Los álbumes de la persona: tarjetas con portada y recuento, y un formulario
 * de UNA línea para crear uno. Cada tarjeta enlaza a `album?id=` bajo el prefijo de la app.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { Button, EmptyState, Icon, Illustration, Input, Spinner, usePrompt } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import {
  albumes,
  anyadirAlAlbum,
  crearAlbum,
  temas as pedirTemas,
  textoAlternativo,
  urlDeVariante,
  type Album,
  type Foto,
} from '../client/photosApi.js';
import { tituloSugerido } from '../lib/tituloAlbum.js';
import { RUTA } from '../lib/ruta.js';
import { useT } from '../i18n/index.js';
import { Marco } from './Marco.js';
import type { SytueUser } from '@sytue/auth';
import { Nav } from './Nav.js';
import { CAMPO_TACTIL } from '../lib/tactil.js';

export function Albums({
  locale,
  user,
  hubBaseUrl,
}: {
  locale: Locale;
  user: SytueUser;
  hubBaseUrl: string;
}): JSX.Element {
  const t = useT(locale);
  const [lista, setLista] = useState<Album[] | null>(null);
  const [titulo, setTitulo] = useState('');
  const [error, setError] = useState('');
  // `null` es «todavía no se han pedido» y `[]` es «se pidieron y no hay».
  // Distinguirlos es lo que deja decir «no encontramos ninguno» en vez de
  // dejar la sección en blanco como si estuviera cargando para siempre.
  const [montones, setMontones] = useState<Foto[][] | null>(null);
  const [buscando, setBuscando] = useState(false);
  const { preguntar, dialogo: dialogoPreguntar } = usePrompt();

  const buscarTemas = async () => {
    if (buscando) return;
    setBuscando(true);
    try {
      setMontones(await pedirTemas());
    } catch (e) {
      setError(String(e));
    } finally {
      setBuscando(false);
    }
  };

  /** Un montón se convierte en álbum con el nombre que proponen sus etiquetas. */
  const albumDelMonton = async (fotos: Foto[]) => {
    const tit = await preguntar({
      titulo: t('albums.suggestionSave'),
      mensaje: t('gallery.newAlbumPrompt', { count: fotos.length }),
      confirmLabel: t('gallery.newAlbumCreate'),
      valor: tituloSugerido(fotos),
    });
    if (tit === null || tit.trim() === '') return;
    try {
      const nuevo = await crearAlbum(tit.trim());
      await anyadirAlAlbum(
        nuevo.id,
        fotos.map((f) => f.id),
      );
      setLista((l) => [{ ...nuevo, count: fotos.length }, ...(l ?? [])]);
      // El montón guardado desaparece de las sugerencias: seguir ofreciendo
      // crear un álbum que se acaba de crear es la forma de acabar con tres
      // álbumes iguales.
      setMontones((m) => (m ?? []).filter((g) => g !== fotos));
    } catch (e) {
      setError(String(e));
    }
  };

  useEffect(() => {
    albumes()
      .then(setLista)
      .catch((e) => setError(String(e)));
  }, []);

  const crear = async (e: Event) => {
    e.preventDefault();
    const tit = titulo.trim();
    if (!tit) return;
    try {
      const nuevo = await crearAlbum(tit);
      setLista((l) => [nuevo, ...(l ?? [])]);
      setTitulo('');
    } catch (err) {
      setError(String(err));
    }
  };

  return (
    <Marco user={user} hubBaseUrl={hubBaseUrl} locale={locale} title={t('nav.albums')}>
      <div class="mx-auto flex h-full w-full max-w-7xl flex-col px-4 py-6">
        <Nav t={t} locale={locale} activa="albums" />
        <form onSubmit={(e) => void crear(e)} class="mb-6 flex gap-2">
          {/* El campo de la casa, del alto de su botón: el de antes medía 28 px
              junto a uno de 32. El ancho va en el envoltorio porque `Input` trae
              el suyo (`w-full`). */}
          <div class="w-64 max-w-full min-w-0">
            <Input
              type="text"
              value={titulo}
              onInput={(e) => setTitulo((e.target as HTMLInputElement).value)}
              placeholder={t('albums.title')}
              aria-label={t('albums.title')}
              maxLength={200}
              required
              class={CAMPO_TACTIL}
            />
          </div>
          <Button type="submit" variant="primary" leadingIcon={<Icon name="plus" />}>
            {t('albums.create')}
          </Button>
        </form>
        {error ? (
          <p class="text-danger-text mb-4 text-sm" role="alert">
            {error}
          </p>
        ) : null}
        {/* Las sugerencias van ARRIBA y solo si se piden. Recorrer los vectores
            de la biblioteca para proponer montones no es gratis, y nadie abre
            «álbumes» para que le adivinen: quien lo quiere, lo pulsa. */}
        <section class="mb-6">
          <div class="mb-2 flex flex-wrap items-center gap-2">
            <h2 class="text-sm font-semibold">{t('albums.suggestions')}</h2>
            {/* De fantasma: la acción principal de esta pantalla es crear un
                álbum, y dos botones rellenos pesaban lo mismo. */}
            <Button
              variant="ghost"
              size="sm"
              leadingIcon={<Icon name="search" />}
              disabled={buscando}
              onClick={() => void buscarTemas()}
            >
              {buscando ? t('albums.suggestionsLoading') : t('albums.suggestionsFind')}
            </Button>
          </div>
          {montones !== null && montones.length === 0 ? (
            <p class="text-text-muted text-sm">{t('albums.suggestionsNone')}</p>
          ) : null}
          <ul class="flex flex-col gap-3">
            {(montones ?? []).map((grupo) => (
              <li
                key={grupo[0]?.id}
                class="border-border flex flex-wrap items-center gap-2 rounded-md border p-2"
              >
                {grupo.slice(0, 8).map((f) => (
                  <img
                    key={f.id}
                    src={urlDeVariante(f.id, 'miniatura')}
                    alt={textoAlternativo(f)}
                    width={f.width}
                    height={f.height}
                    loading="lazy"
                    decoding="async"
                    class="h-14 w-14 rounded-sm object-cover"
                  />
                ))}
                <span class="text-text-muted text-sm">
                  {t('albums.count', { count: grupo.length })}
                </span>
                <span class="ml-auto flex gap-1">
                  <Button
                    href={`${RUTA}similar?ids=${encodeURIComponent(
                      grupo
                        .slice(0, 12)
                        .map((f) => f.id)
                        .join(','),
                    )}`}
                    variant="ghost"
                    size="sm"
                    leadingIcon={<Icon name="image" />}
                  >
                    {t('albums.suggestionSee')}
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    leadingIcon={<Icon name="album" />}
                    onClick={() => void albumDelMonton(grupo)}
                  >
                    {t('albums.suggestionSave')}
                  </Button>
                </span>
              </li>
            ))}
          </ul>
        </section>
        {lista === null ? (
          <Spinner />
        ) : lista.length === 0 ? (
          <EmptyState
            class="flex-1"
            title={t('albums.empty.title')}
            description={t('albums.empty.description')}
            illustration={<Illustration name="emptyPhotos" />}
          />
        ) : (
          <ul class="grid grid-cols-[repeat(auto-fill,minmax(11rem,1fr))] gap-3">
            {lista.map((al) => (
              <li key={al.id}>
                <a
                  href={`${RUTA}album?id=${encodeURIComponent(al.id)}`}
                  class="sytue-focus-ring block rounded-sm"
                >
                  <div class="bg-bg-subtle aspect-square overflow-hidden rounded-sm">
                    {al.coverAssetId ? (
                      <img
                        src={urlDeVariante(al.coverAssetId, 'miniatura')}
                        alt=""
                        loading="lazy"
                        decoding="async"
                        class="h-full w-full object-cover"
                      />
                    ) : null}
                  </div>
                  <div class="mt-1 truncate text-sm font-medium">{al.title}</div>
                  <div class="text-text-muted text-xs">
                    {t('albums.count', { count: al.count })}
                  </div>
                </a>
              </li>
            ))}
          </ul>
        )}
        {dialogoPreguntar}
      </div>
    </Marco>
  );
}
