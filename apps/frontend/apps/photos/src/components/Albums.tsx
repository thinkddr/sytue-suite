/**
 * Los álbumes de la persona: tarjetas con portada y recuento, y un formulario
 * de UNA línea para crear uno. Cada tarjeta enlaza a `album?id=` bajo el prefijo de la app.
 */
import type { JSX } from 'preact';
import { useEffect, useMemo, useState } from 'preact/hooks';
import {
  Badge,
  Button,
  Checkbox,
  EmptyState,
  Icon,
  Input,
  marcarAppLista,
  Select,
  usePrompt,
} from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import {
  albumes,
  anyadirAlAlbum,
  crearAlbum,
  guardarAjustesOrganizacion,
  obtenerAjustesOrganizacion,
  temas as pedirTemas,
  textoAlternativo,
  urlDeVariante,
  type AjustesOrganizacion,
  type Album,
  type Foto,
} from '../client/photosApi.js';
import { tituloSugerido } from '../lib/tituloAlbum.js';
import {
  guardarOrdenAlbumes,
  leerOrdenAlbumes,
  ORDENES_ALBUM,
  ordenarAlbumes,
  type OrdenAlbum,
} from '../lib/ordenAlbumes.js';
import { RUTA } from '../lib/ruta.js';
import { useT, type T } from '../i18n/index.js';
import { conAvisos, FalloDeCarga, useAvisoDeFallo } from './Fallo.js';
import { Marco } from './Marco.js';
import type { SytueUser } from '@sytue/auth';
import { Nav } from './Nav.js';
import { CAMPO_TACTIL } from '../lib/tactil.js';
import { EsqueletoAlbumes } from './Esqueletos.js';
import { AlbumesCifrados } from './AlbumesCifrados.js';

export const Albums = conAvisos(Albumes);

function Albumes({
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
  const [fallo, setFallo] = useState<unknown>(null);
  const avisarFallo = useAvisoDeFallo(locale);
  // `null` es «todavía no se han pedido» y `[]` es «se pidieron y no hay».
  // Distinguirlos es lo que deja decir «no encontramos ninguno» en vez de
  // dejar la sección en blanco como si estuviera cargando para siempre.
  const [montones, setMontones] = useState<Foto[][] | null>(null);
  const [buscando, setBuscando] = useState(false);
  const [ajustes, setAjustes] = useState<AjustesOrganizacion | null>(null);
  const [guardandoAjustes, setGuardandoAjustes] = useState(false);
  const { preguntar, dialogo: dialogoPreguntar } = usePrompt();
  // «Recientes» en el primer pintado: es el orden del servidor, así que la página no
  // salta entre el HTML del servidor y la hidratación; el guardado se lee al montar.
  const [orden, setOrden] = useState<OrdenAlbum>('recent');
  useEffect(() => setOrden(leerOrdenAlbumes()), []);
  const ordenados = useMemo(
    () => (lista === null ? null : ordenarAlbumes(lista, orden, locale)),
    [lista, orden, locale],
  );
  const ordenar = (o: OrdenAlbum) => {
    setOrden(o);
    guardarOrdenAlbumes(o);
  };

  const cambiarExclusionLote = async (excluir: boolean) => {
    setGuardandoAjustes(true);
    try {
      const nuevos = await guardarAjustesOrganizacion(excluir);
      setAjustes(nuevos);
    } catch (e) {
      avisarFallo(e);
    } finally {
      setGuardandoAjustes(false);
    }
  };

  const buscarTemas = async () => {
    if (buscando) return;
    setBuscando(true);
    try {
      setMontones(await pedirTemas());
    } catch (e) {
      avisarFallo(e);
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
      avisarFallo(e);
    }
  };

  const cargar = () => {
    setFallo(null);
    albumes().then(setLista).catch(setFallo);
  };
  useEffect(() => {
    cargar();
    obtenerAjustesOrganizacion()
      .then(setAjustes)
      .catch(() => {});
  }, []);
  // `sytue:app-ready`: la lista de álbumes (o su fallo) ya está pintada. Sin la marca,
  // la bóveda esperaba los 5 s de `cuandoAppLista` para empezar a abrirse.
  useEffect(() => {
    if (lista !== null || fallo !== null) marcarAppLista();
  }, [lista, fallo]);

  const crear = async (e: Event) => {
    e.preventDefault();
    const tit = titulo.trim();
    if (!tit) return;
    try {
      const nuevo = await crearAlbum(tit);
      setLista((l) => [nuevo, ...(l ?? [])]);
      setTitulo('');
    } catch (err) {
      avisarFallo(err);
    }
  };

  return (
    <Marco user={user} hubBaseUrl={hubBaseUrl} locale={locale} title={t('nav.albums')}>
      <div class="mx-auto flex min-h-full w-full max-w-7xl flex-col px-4 py-6">
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
            {ajustes?.canManageAiBatch ? (
              <Checkbox
                class="ml-auto text-xs"
                label={t('aiBatch.excludeOrg')}
                checked={ajustes.excludeAiBatch}
                disabled={guardandoAjustes}
                onChange={(e) => void cambiarExclusionLote(e.currentTarget.checked)}
              />
            ) : null}
          </div>
          {ajustes?.excludeAiBatch ? (
            <p class="text-text-muted mb-2 text-xs">{t('aiBatch.excludedHint')}</p>
          ) : null}
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
        {fallo !== null ? (
          <FalloDeCarga locale={locale} fallo={fallo} alReintentar={cargar} />
        ) : lista === null ? (
          <EsqueletoAlbumes />
        ) : lista.length === 0 ? (
          <EmptyState
            class="flex-1"
            title={t('albums.empty.title')}
            description={t('albums.empty.description')}
            spot="emptyPhotos"
          />
        ) : (
          <>
            {/* Con un solo álbum no hay nada que ordenar: el selector solo sería ruido. */}
            {lista.length > 1 ? (
              <div class="mb-3 flex justify-end">
                <SelectorOrdenAlbumes t={t} orden={orden} alCambiar={ordenar} />
              </div>
            ) : null}
            <ul class="grid grid-cols-[repeat(auto-fill,minmax(11rem,1fr))] gap-6">
              {(ordenados ?? lista).map((al) => (
                <li key={al.id}>
                  <TarjetaAlbum t={t} album={al} />
                </li>
              ))}
            </ul>
          </>
        )}
        {/* Los álbumes de la galería cifrada: solo en la bóveda, nunca en el servidor. */}
        <AlbumesCifrados locale={locale} user={user} />
        {dialogoPreguntar}
      </div>
    </Marco>
  );
}

/** «Ordenar por», con su rótulo visible: un `<select>` sin nombre no dice qué ordena. */
export function SelectorOrdenAlbumes({
  t,
  orden,
  alCambiar,
}: {
  t: T;
  orden: OrdenAlbum;
  alCambiar: (o: OrdenAlbum) => void;
}): JSX.Element {
  return (
    <label class="text-text-muted flex items-center gap-2 text-xs">
      {t('albums.sort.label')}
      <Select
        size="sm"
        value={orden}
        class={CAMPO_TACTIL}
        onChange={(e) => alCambiar(e.currentTarget.value as OrdenAlbum)}
      >
        {ORDENES_ALBUM.map((o) => (
          <option key={o} value={o}>
            {t(`albums.sort.${o}`)}
          </option>
        ))}
      </Select>
    </label>
  );
}

/**
 * Una tarjeta de «Álbumes»: portada 4:3, título y recuento. Si el álbum tiene enlace
 * público lo dice con la insignia «Compartido» sobre la portada: que un enlace siga
 * vivo es lo primero que hay que poder ver al repasar los álbumes, y antes solo se
 * sabía abriéndolos uno a uno. Va encima de la portada y no en el pie para no cambiar
 * la altura de la tarjeta, que es la de la rejilla y la del esqueleto.
 */
export function TarjetaAlbum({ t, album: al }: { t: T; album: Album }): JSX.Element {
  return (
    <a
      href={`${RUTA}album?id=${encodeURIComponent(al.id)}`}
      class="sytue-focus-ring block rounded-sm"
    >
      <div class="bg-bg-subtle border-border-subtle relative flex aspect-[4/3] items-center justify-center overflow-hidden rounded-sm border">
        {al.coverAssetId ? (
          <img
            src={urlDeVariante(al.coverAssetId, 'miniatura')}
            alt=""
            loading="lazy"
            decoding="async"
            class="h-full w-full object-cover"
          />
        ) : (
          <Icon name="album" size={32} class="text-text-subtle" />
        )}
        {/* Tono `info` y no `primary`: el relleno de `primary` (`selected`) es
            translúcido y sobre una portada la insignia desaparecía. */}
        {al.shared ? (
          <Badge tone="info" class="absolute start-2 top-2">
            {t('album.shared')}
          </Badge>
        ) : null}
      </div>
      <div class="text-text mt-2 truncate text-sm font-semibold">{al.title}</div>
      <div class="text-text-muted text-xs">{t('albums.count', { count: al.count })}</div>
    </a>
  );
}
