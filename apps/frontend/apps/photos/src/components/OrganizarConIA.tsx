/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Organizar en álbumes con IA», en la página de álbumes.
 *
 * Parte de las fotos que no están en ninguno de los álbumes de la persona (como
 * mucho `TOPE_ORGANIZAR` por pasada), describe con Sytue AI las que no tienen nada
 * escrito (como mucho `TOPE_DESCRIBIR`, porque cada una se cobra), y con lo visto
 * le pide en UNA llamada de texto una propuesta de álbumes. Antes de tocar nada la
 * ENSEÑA: cada álbum con su título editable, cuántas fotos lleva, unas miniaturas y
 * su casilla. «Crear álbumes» crea lo marcado —o lo añade al álbum que ya se llama
 * igual— y el aviso trae «Deshacer» durante sus 10 s.
 *
 * LA GALERÍA CIFRADA NO ENTRA, como en describir (ADR 0086): de esas fotos el
 * servidor no tiene ni la miniatura, y lo que se organiza sale de la línea de
 * tiempo del servidor. Si la persona tiene el Drive en la bóveda se le dice con el
 * mismo aviso que en la galería (`gallery.encrypted.note`).
 *
 * El espejo nativo es `apps/fyne/apps/photos/organizar.go`.
 */
import type { JSX } from 'preact';
import { useRef, useState } from 'preact/hooks';
import {
  Button,
  Checkbox,
  DURACION_DESHACER,
  EmptyState,
  Icon,
  Input,
  Spinner,
  useToast,
} from '@sytue/ui';
import { plegar, type Locale } from '@sytue/i18n';
import type { SytueUser } from '@sytue/auth';
import {
  agruparFotos,
  albumes as pedirAlbumes,
  anyadirAlAlbum,
  borrarAlbum,
  crearAlbum,
  describirFoto,
  ErrorDeFotos,
  fotosDelAlbum,
  guardarDescripcion,
  lineaDeTiempo,
  quitarDelAlbum,
  textoAlternativo,
  urlDeVariante,
  type Foto,
} from '../client/photosApi.js';
import { driveEnBoveda } from '../client/driveCifrado.js';
import { TOPE_ORGANIZAR, type AlbumPropuesto } from '../lib/agruparIA.js';
import {
  crearAlbumes,
  deshacerOrganizar,
  fotosSinAlbum,
  planDeAlbumes,
  POR_PAGINA_ORGANIZAR,
  type AlbumMio,
  type HechoOrganizar,
} from '../lib/organizarIA.js';
import { TOPE_DESCRIBIR } from '../lib/respuestaIA.js';
import { textoDeFallo } from '../lib/fallo.js';
import { CAMPO_TACTIL } from '../lib/tactil.js';
import { useT } from '../i18n/index.js';

/** Lo que se le pide al servicio. Va por props para que la prueba no necesite red. */
export interface ServicioOrganizar {
  albumes: () => Promise<AlbumMio[]>;
  fotosDelAlbum: (id: string) => Promise<{ id: string }[]>;
  pagina: (cursor: string | undefined) => Promise<{ items: Foto[]; nextCursor?: string }>;
  describir: (id: string) => Promise<{ description: string; tags: string[] }>;
  guardarDescripcion: (id: string, descripcion: string, etiquetas: string[]) => Promise<unknown>;
  agrupar: (fotos: Foto[], signal: AbortSignal) => Promise<AlbumPropuesto[]>;
  crearAlbum: (titulo: string) => Promise<AlbumMio>;
  anyadirAlAlbum: (id: string, ids: string[]) => Promise<unknown>;
  borrarAlbum: (id: string) => Promise<unknown>;
  quitarDelAlbum: (id: string, foto: string) => Promise<unknown>;
  /** ¿Tiene la persona el Drive en la bóveda? Entonces hay fotos cifradas que no entran. */
  enBoveda: () => Promise<boolean>;
}

/** El de verdad: el BFF de Fotos con la sesión de quien mira. */
export function servicioDeFotos(user: Pick<SytueUser, 'id' | 'tenant'>): ServicioOrganizar {
  return {
    albumes: pedirAlbumes,
    fotosDelAlbum,
    pagina: (cursor) => lineaDeTiempo(cursor, POR_PAGINA_ORGANIZAR),
    describir: describirFoto,
    guardarDescripcion,
    agrupar: agruparFotos,
    crearAlbum,
    anyadirAlAlbum,
    borrarAlbum,
    quitarDelAlbum,
    enBoveda: () => driveEnBoveda(user.id, user.tenant),
  };
}

/** Un álbum de la propuesta en la vista previa. */
interface Propuesto {
  clave: string;
  titulo: string;
  fotos: Foto[];
  incluido: boolean;
}

type Estado =
  | { fase: 'inactiva' }
  | { fase: 'buscando' }
  | { fase: 'describiendo'; hechas: number; total: number }
  | { fase: 'agrupando' }
  | {
      fase: 'propuesta';
      albumes: Propuesto[];
      existentes: AlbumMio[];
      /** Quedan fotos sin mirar o sin describir: la pasada siguiente sigue por ahí. */
      topado: boolean;
      describirTopado: boolean;
      creando: boolean;
    }
  | { fase: 'vacia'; clave: 'albums.organize.nothing' | 'albums.organize.noGroups' }
  | { fase: 'error'; texto: string };

/** Las miniaturas que se enseñan de cada álbum propuesto. */
const MINIATURAS = 6;

const noConfigurada = (e: unknown): boolean => String(e).includes('ai_not_configured');

export function OrganizarConIA({
  locale,
  servicio,
  alCambiar,
}: {
  locale: Locale;
  servicio: ServicioOrganizar;
  /** Lo que había en «Álbumes» ha cambiado: hay que volver a pedir la lista. */
  alCambiar: () => void;
}): JSX.Element {
  const t = useT(locale);
  const toast = useToast();
  const [estado, setEstado] = useState<Estado>({ fase: 'inactiva' });
  const [enBoveda, setEnBoveda] = useState(false);
  /**
   * La pasada en curso. «Parar» la cambia, y lo que vuelva de una pasada que ya no es
   * la de ahora se tira: tras parar se puede volver a empezar con la anterior aún en
   * vuelo, y sin esto seguirían las dos a la vez.
   */
  const vezRef = useRef(0);
  const abortarRef = useRef<AbortController | null>(null);

  const trabajando =
    estado.fase === 'buscando' || estado.fase === 'describiendo' || estado.fase === 'agrupando';

  const organizar = async () => {
    if (trabajando) return;
    const vez = ++vezRef.current;
    const otra = () => vezRef.current !== vez;
    const control = new AbortController();
    abortarRef.current = control;
    setEstado({ fase: 'buscando' });
    void servicio
      .enBoveda()
      .then(setEnBoveda)
      .catch(() => {});
    try {
      const { albumes, fotos, topado } = await fotosSinAlbum({
        albumes: servicio.albumes,
        fotosDelAlbum: servicio.fotosDelAlbum,
        pagina: servicio.pagina,
      });
      if (otra()) return;
      if (fotos.length === 0) {
        setEstado({ fase: 'vacia', clave: 'albums.organize.nothing' });
        return;
      }
      // Se usa lo que Sytue AI ya vio; solo se describen las que no tienen nada, y
      // de una en una: cada una es una llamada de visión, y se cobra.
      const vistas = new Map(fotos.map((f) => [f.id, f]));
      const sinNada = fotos.filter((f) => !f.description?.trim() && !f.tags?.length);
      const lote = sinNada.slice(0, TOPE_DESCRIBIR);
      let hechas = 0;
      for (const f of lote) {
        if (otra()) return;
        setEstado({ fase: 'describiendo', hechas, total: lote.length });
        try {
          const { description, tags } = await servicio.describir(f.id);
          await servicio.guardarDescripcion(f.id, description, tags);
          vistas.set(f.id, { ...f, description, tags });
        } catch (e) {
          // Sin Sytue AI no se sigue: las demás fallarían igual, una a una.
          if (noConfigurada(e)) throw e;
          // Una que falla no para el resto: se queda fuera de esta pasada.
        }
        hechas++;
      }
      if (otra()) return;
      setEstado({ fase: 'agrupando' });
      const propuesta = await servicio.agrupar([...vistas.values()], control.signal);
      if (otra()) return;
      const albumesPropuestos = propuesta
        .map((a, i) => ({
          clave: String(i),
          titulo: a.titulo,
          fotos: a.ids.flatMap((id) => vistas.get(id) ?? []),
          incluido: true,
        }))
        .filter((a) => a.fotos.length > 0);
      if (albumesPropuestos.length === 0) {
        setEstado({ fase: 'vacia', clave: 'albums.organize.noGroups' });
        return;
      }
      setEstado({
        fase: 'propuesta',
        albumes: albumesPropuestos,
        existentes: albumes,
        topado,
        describirTopado: sinNada.length > TOPE_DESCRIBIR,
        creando: false,
      });
    } catch (e) {
      if (otra()) return;
      setEstado({
        fase: 'error',
        texto: noConfigurada(e)
          ? t('viewer.describeUnavailable')
          : e instanceof ErrorDeFotos
            ? textoDeFallo(locale, e)
            : t('albums.organize.failed'),
      });
    }
  };

  /** «Parar»: lo descrito hasta ahí ya está guardado; lo demás no se hace. */
  const pararYa = () => {
    vezRef.current++;
    abortarRef.current?.abort();
    setEstado({ fase: 'inactiva' });
  };

  const cambiar = (clave: string, cambio: Partial<Propuesto>) =>
    setEstado((e) =>
      e.fase === 'propuesta'
        ? { ...e, albumes: e.albumes.map((a) => (a.clave === clave ? { ...a, ...cambio } : a)) }
        : e,
    );

  const deshacer = async (hechos: HechoOrganizar[]) => {
    const { fallidas } = await deshacerOrganizar(hechos, servicio);
    alCambiar();
    toast.notify(
      fallidas > 0
        ? { tone: 'danger', title: t('albums.organize.undoFailed', { count: fallidas }) }
        : { tone: 'success', title: t('albums.organize.undone') },
    );
  };

  const crear = async () => {
    if (estado.fase !== 'propuesta' || estado.creando) return;
    const plan = planDeAlbumes(
      estado.albumes.map((a) => ({ ...a, ids: a.fotos.map((f) => f.id) })),
      estado.existentes,
    );
    if (plan.length === 0) return;
    setEstado({ ...estado, creando: true });
    const { hechos, fallidos } = await crearAlbumes(plan, servicio);
    setEstado({ fase: 'inactiva' });
    if (hechos.length > 0) alCambiar();
    const fotos = hechos.reduce((n, h) => n + h.ids.length, 0);
    if (hechos.length > 0) {
      toast.notify({
        tone: 'success',
        title: t('albums.organize.done', { count: hechos.length }),
        description: t('albums.organize.donePhotos', { count: fotos }),
        duration: DURACION_DESHACER,
        action: { label: t('gallery.undo'), onClick: () => void deshacer(hechos) },
      });
    }
    if (fallidos > 0) {
      toast.notify({
        tone: 'danger',
        title: t('albums.organize.createFailed', { count: fallidos }),
      });
    }
  };

  const progreso =
    estado.fase === 'buscando'
      ? t('albums.organize.looking')
      : estado.fase === 'describiendo'
        ? t('albums.organize.describing', { done: estado.hechas, total: estado.total })
        : estado.fase === 'agrupando'
          ? t('albums.organize.grouping')
          : null;

  return (
    <section class="mb-6" aria-label={t('albums.organize.title')}>
      <div class="mb-2 flex flex-wrap items-center gap-2">
        <h2 class="text-sm font-semibold">{t('albums.organize.title')}</h2>
        {/* De fantasma, como «Buscar montones»: lo principal de la página es crear. */}
        <Button
          variant="ghost"
          size="sm"
          leadingIcon={<Icon name="ai" />}
          disabled={trabajando || (estado.fase === 'propuesta' && estado.creando)}
          onClick={() => void organizar()}
        >
          {t('albums.organize.start')}
        </Button>
      </div>
      <p class="text-text-muted mb-2 text-xs">{t('albums.organize.hint')}</p>
      {enBoveda ? <p class="text-text-muted mb-2 text-xs">{t('gallery.encrypted.note')}</p> : null}

      {progreso ? (
        // El progreso se lee siempre: es un botón que gasta dinero por foto, y
        // «Parar» tiene que estar a mano MIENTRAS corre.
        <p class="flex flex-wrap items-center gap-2 text-sm" role="status">
          <Spinner size={16} />
          {progreso}
          <Button variant="ghost" size="sm" onClick={pararYa}>
            {t('gallery.describeStop')}
          </Button>
        </p>
      ) : null}

      {estado.fase === 'error' ? (
        <p class="text-danger-text text-sm" role="alert">
          {estado.texto}
        </p>
      ) : null}

      {estado.fase === 'vacia' ? <EmptyState compact title={t(estado.clave)} icon="album" /> : null}

      {estado.fase === 'propuesta' ? (
        <VistaPrevia
          locale={locale}
          estado={estado}
          alCambiar={cambiar}
          alCrear={() => void crear()}
          alDescartar={() => setEstado({ fase: 'inactiva' })}
        />
      ) : null}
    </section>
  );
}

function VistaPrevia({
  locale,
  estado,
  alCambiar,
  alCrear,
  alDescartar,
}: {
  locale: Locale;
  estado: Extract<Estado, { fase: 'propuesta' }>;
  alCambiar: (clave: string, cambio: Partial<Propuesto>) => void;
  alCrear: () => void;
  alDescartar: () => void;
}): JSX.Element {
  const t = useT(locale);
  const existe = (titulo: string) =>
    titulo.trim() !== '' &&
    estado.existentes.some((a) => plegar(a.title.trim()) === plegar(titulo.trim()));
  const algo = estado.albumes.some((a) => a.incluido && a.titulo.trim() !== '');
  return (
    <>
      <h3 class="text-text-muted mb-2 text-xs font-semibold">
        {t('albums.organize.proposal', { count: estado.albumes.length })}
      </h3>
      <ul class="flex flex-col gap-3">
        {estado.albumes.map((a) => (
          <li
            key={a.clave}
            class="border-border flex flex-wrap items-center gap-2 rounded-md border p-2"
          >
            <Checkbox
              checked={a.incluido}
              aria-label={t('albums.organize.include', { title: a.titulo })}
              onChange={(e) => alCambiar(a.clave, { incluido: e.currentTarget.checked })}
            />
            <div class="w-64 max-w-full min-w-0">
              <Input
                type="text"
                value={a.titulo}
                aria-label={t('albums.title')}
                maxLength={200}
                class={CAMPO_TACTIL}
                onInput={(e) => alCambiar(a.clave, { titulo: e.currentTarget.value })}
              />
            </div>
            {a.fotos.slice(0, MINIATURAS).map((f) => (
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
              {t('albums.count', { count: a.fotos.length })}
            </span>
            {existe(a.titulo) ? (
              <span class="text-text-muted text-xs">{t('albums.organize.existing')}</span>
            ) : null}
          </li>
        ))}
      </ul>
      {/* Lo que queda para la pasada siguiente: fotos sin mirar, o sin describir. */}
      {estado.topado ? (
        <p class="text-text-muted mt-2 text-xs">
          {t('albums.organize.capped', { count: TOPE_ORGANIZAR })}
        </p>
      ) : null}
      {estado.describirTopado ? (
        <p class="text-text-muted mt-2 text-xs">
          {t('gallery.describeCapped', { count: TOPE_DESCRIBIR })}
        </p>
      ) : null}
      <div class="mt-3 flex flex-wrap items-center gap-2">
        <Button
          variant="primary"
          size="sm"
          leadingIcon={<Icon name="album" />}
          disabled={!algo || estado.creando}
          loading={estado.creando}
          onClick={alCrear}
        >
          {t('albums.organize.create')}
        </Button>
        <Button variant="ghost" size="sm" disabled={estado.creando} onClick={alDescartar}>
          {t('albums.organize.discard')}
        </Button>
      </div>
    </>
  );
}
