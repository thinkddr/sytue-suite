/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La galería cifrada en la web (ADR 0086): las fotos que viven cifradas en «Fotos»
 * del Drive de la bóveda, con su índice cifrado (`client/galeriaCifrada.ts`).
 *
 * - `useDriveCifrado` abre el Drive de la bóveda (solo si la persona lo pide o al
 *   subir) y `useGaleriaCifrada` lee su índice, reconstruye las entradas que falten y
 *   descifra las miniaturas en memoria.
 * - Las fotos cifradas entran en la línea de tiempo por fecha (`MiniaturaCifrada`),
 *   en favoritas, en la búsqueda (en el navegador) y en sus álbumes cifrados; se
 *   abren descifradas en memoria (`VisorCifrado`) y se editan guardando una versión
 *   nueva cifrada (`EditorCifrado`). Todo se escribe SOLO en la bóveda.
 * - Lo que necesita al servidor (Sytue AI, parecidas, cerca, recuerdos) no las
 *   incluye, y se dice (`gallery.encrypted.note`), no se calla.
 */
import type { JSX } from 'preact';
import { useCallback, useEffect, useMemo, useRef, useState } from 'preact/hooks';
import {
  Button,
  EmptyState,
  Icon,
  IconButton,
  Modal,
  Spinner,
  cx,
  usePrompt,
  useToast,
  withBase,
} from '@sytue/ui';
import { formatDateTime, translate, type Locale } from '@sytue/i18n';
import { bridgeHandoffKey } from '@sytue/offline/vault';
import {
  abrirDriveCifrado,
  AperturaDriveCifrado,
  driveEnBoveda,
  MAXIMO_A_LA_VISTA,
  MiniaturasCifradas,
  type EstadoDriveCifrado,
  type MotivoDriveCifrado,
} from '../client/driveCifrado.js';
import {
  GaleriaCifrada,
  type AlbumCifrado,
  type EntradaFoto,
  type FotoDeGaleria,
  type IndiceCifrado,
} from '../client/galeriaCifrada.js';
import { aplicarColorCPU, aplicarGeometria } from '../editor/motor.js';
import { recetaVacia, type Paso, type Receta } from '../editor/receta.js';
import { estiloDeTesela } from '../lib/filas.js';
import { useT } from '../i18n/index.js';

/** Cuántas entradas que faltan se reconstruyen por apertura (cada una descifra el original). */
const RECONSTRUIR_POR_VEZ = 30;
/** Lado del lienzo de la vista previa del editor; guardar usa la foto entera. */
const LADO_PREVIA = 1600;

/**
 * El Drive de la bóveda de la persona para la galería: si está en la bóveda (sin
 * claves) y, cuando se pide, abierto. Una sola apertura a la vez
 * (`AperturaDriveCifrado`), que se cierra al desmontar o al cambiar de cuenta.
 */
export function useDriveCifrado(actor: string, tenant: string | undefined) {
  const [enBoveda, setEnBoveda] = useState(false);
  const [estado, setEstado] = useState<EstadoDriveCifrado>({ fase: 'cerrado' });
  const [version, setVersion] = useState(0);
  const apertura = useMemo(
    () => new AperturaDriveCifrado(() => abrirDriveCifrado({ actor, tenant }), setEstado),
    [actor, tenant],
  );

  useEffect(() => {
    const vida = new AbortController();
    setEstado({ fase: 'cerrado' });
    void driveEnBoveda(actor, tenant, undefined, vida.signal).then((si) => {
      if (!vida.signal.aborted) setEnBoveda(si);
    });
    return () => {
      vida.abort();
      void apertura.cerrar().catch(() => {});
    };
  }, [actor, tenant, apertura]);

  return {
    // Si se abrió es que está: también tras un 409 que llegó antes que el estado.
    enBoveda: enBoveda || estado.fase === 'abierto',
    estado,
    abrir: useCallback(() => apertura.abrir(), [apertura]),
    /** Vuelve a leer el índice (tras subir). */
    version,
    tocar: useCallback(() => setVersion((v) => v + 1), []),
  };
}

export type DriveCifradoVivo = ReturnType<typeof useDriveCifrado>;

/** Una foto con la entrada nueva que acaba de escribirse. */
function conEntrada(foto: FotoDeGaleria, entrada: EntradaFoto): FotoDeGaleria {
  return {
    ...foto,
    entrada,
    conEntrada: true,
    pendiente: !entrada.miniatura || (foto.suma !== '' && entrada.suma !== foto.suma),
  };
}

/**
 * Las miniaturas que se piden, descifradas de una en una y sin repetir. Vive fuera
 * de los hooks para no mutar refs; una por galería abierta.
 */
class ColaMiniaturas {
  private readonly pedidas = new Set<string>();
  private readonly cola: FotoDeGaleria[] = [];
  private corriendo = false;
  constructor(
    private readonly galeria: GaleriaCifrada,
    private readonly miniaturas: MiniaturasCifradas,
    private readonly alCambiar: (urls: Record<string, string>) => void,
  ) {}

  pedir(fotos: readonly FotoDeGaleria[]): void {
    for (const f of fotos) {
      const clave = `${f.id}:${f.entrada.miniatura}`;
      if (this.pedidas.has(clave)) continue;
      this.pedidas.add(clave);
      this.cola.push(f);
    }
    if (this.corriendo) return;
    this.corriendo = true;
    void this.correr().finally(() => {
      this.corriendo = false;
    });
  }

  private async correr(): Promise<void> {
    for (let f = this.cola.shift(); f; f = this.cola.shift()) {
      try {
        const blob = f.entrada.miniatura
          ? await this.galeria.miniatura(f)
          : f.bytes <= MAXIMO_A_LA_VISTA
            ? await this.galeria.original(f)
            : null;
        if (blob) {
          this.miniaturas.poner(`${f.id}:${f.entrada.miniatura}`, blob);
          this.alCambiar(this.miniaturas.todas());
        }
      } catch {
        // Se queda el candado.
      }
    }
  }
}

/**
 * El índice de la galería cifrada del Drive abierto: lo lee, reconstruye en segundo
 * plano las entradas que faltan (unas pocas por apertura) y descifra en memoria las
 * miniaturas que se piden. Las URL se sueltan al cerrar el Drive o desmontar.
 */
export function useGaleriaCifrada(cifrado: DriveCifradoVivo) {
  const drive = cifrado.estado.fase === 'abierto' ? cifrado.estado.drive : null;
  const galeria = useMemo(
    () => (drive ? new GaleriaCifrada(drive.repositorio, { signal: drive.signal }) : null),
    [drive],
  );
  const [indice, setIndice] = useState<IndiceCifrado | null>(null);
  const [fallo, setFallo] = useState(false);
  const [urls, setUrls] = useState<Record<string, string>>({});
  const miniaturas = useMemo(() => new MiniaturasCifradas(drive?.signal), [drive]);
  useEffect(() => () => miniaturas.soltar(), [miniaturas]);

  useEffect(() => {
    if (!galeria) {
      setIndice(null);
      return;
    }
    let vivo = true;
    setFallo(false);
    void (async () => {
      let leido: IndiceCifrado;
      try {
        leido = await galeria.leer();
      } catch {
        if (vivo) setFallo(true);
        return;
      }
      if (!vivo) return;
      setIndice(leido);
      // Lo que falta o es de otro contenido se rehace desde el original: nunca se
      // pierde nada, solo se completa el índice.
      const pendientes = leido.fotos
        .filter((f) => f.pendiente && f.bytes <= MAXIMO_A_LA_VISTA)
        .slice(0, RECONSTRUIR_POR_VEZ);
      for (const foto of pendientes) {
        if (!vivo) return;
        try {
          const entrada = await galeria.reconstruir(foto);
          if (!vivo) return;
          setIndice((i) =>
            i
              ? { ...i, fotos: i.fotos.map((f) => (f.id === foto.id ? conEntrada(f, entrada) : f)) }
              : i,
          );
        } catch {
          // Esa foto sigue saliendo con lo que dice Drive.
        }
      }
    })();
    return () => {
      vivo = false;
    };
  }, [galeria, cifrado.version]);

  const cola = useMemo(
    () => (galeria ? new ColaMiniaturas(galeria, miniaturas, setUrls) : null),
    [galeria, miniaturas],
  );
  const pedir = useCallback((fotos: readonly FotoDeGaleria[]) => cola?.pedir(fotos), [cola]);

  return {
    abierta: drive !== null,
    galeria,
    indice,
    fallo,
    /** La URL en memoria de la miniatura de una foto, si ya se descifró. */
    url: (f: FotoDeGaleria) => urls[`${f.id}:${f.entrada.miniatura}`],
    pedir,
    cambiar: useCallback((foto: FotoDeGaleria, entrada: EntradaFoto) => {
      setIndice((i) =>
        i
          ? { ...i, fotos: i.fotos.map((f) => (f.id === foto.id ? conEntrada(f, entrada) : f)) }
          : i,
      );
    }, []),
    anyadirAlbum: useCallback((album: AlbumCifrado) => {
      setIndice((i) => (i ? { ...i, albumes: [...i.albumes, album] } : i));
    }, []),
  };
}

export type GaleriaCifradaViva = ReturnType<typeof useGaleriaCifrada>;

/** Volver a entrar para desbloquear la bóveda, con la vuelta del puente si la hay. */
function urlDeEntrar(): string {
  const params = new URLSearchParams({
    unlock: '1',
    returnTo: window.location.pathname + window.location.search,
  });
  const clave = bridgeHandoffKey();
  if (clave) params.set('key', clave);
  // `withBase(` junto a `/auth/login`, sin alias: lo busca el gate de packages/auth.
  return withBase(import.meta.env.BASE_URL, `/auth/login?${params.toString()}`);
}

export function MotivoDeDriveCifrado({
  locale,
  motivo,
  alReintentar,
}: {
  locale: Locale;
  motivo: MotivoDriveCifrado;
  alReintentar: () => void;
}): JSX.Element {
  const t = useT(locale);
  return (
    <div class="flex flex-wrap items-center gap-2 text-sm" role="alert">
      <span class="text-text-muted">
        {motivo === 'entrar'
          ? t('gallery.uploadVaultSignIn')
          : motivo === 'pendiente'
            ? t('gallery.uploadVaultNotReady')
            : t('gallery.encrypted.failed')}
      </span>
      {motivo === 'entrar' ? (
        <Button size="sm" onClick={() => window.location.assign(urlDeEntrar())}>
          {t('gallery.encrypted.signIn')}
        </Button>
      ) : (
        <Button size="sm" variant="secondary" onClick={alReintentar}>
          {translate(locale, 'action.retry')}
        </Button>
      )}
    </div>
  );
}

/**
 * La franja de la galería cifrada: dice que existe, la abre cuando se pide, cuenta
 * por qué no abre y avisa de lo que no la incluye. Las fotos van en la línea de
 * tiempo, no aquí.
 */
export function FotosCifradas({
  locale,
  cifrado,
  viva,
}: {
  locale: Locale;
  cifrado: DriveCifradoVivo;
  viva: GaleriaCifradaViva;
}): JSX.Element | null {
  const t = useT(locale);
  const { enBoveda, estado, abrir } = cifrado;
  if (!enBoveda && estado.fase === 'cerrado') return null;
  return (
    <section
      class="border-border mb-6 rounded-md border px-4 py-3"
      aria-label={t('gallery.encrypted.title')}
    >
      <header class="flex flex-wrap items-center gap-2">
        <Icon name="lock" size={16} />
        <h2 class="text-sm font-semibold">{t('gallery.encrypted.title')}</h2>
        {estado.fase === 'cerrado' ? (
          <Button
            variant="ghost"
            size="xs"
            class="ml-auto"
            onClick={() => void abrir().catch(() => {})}
          >
            {t('gallery.encrypted.show')}
          </Button>
        ) : null}
      </header>
      <p class="text-text-muted mt-1 text-xs">{t('gallery.encrypted.note')}</p>
      {estado.fase === 'abriendo' || (viva.abierta && viva.indice === null && !viva.fallo) ? (
        <p class="mt-2 flex items-center gap-2 text-sm" role="status">
          <Spinner size={16} />
          {t('gallery.encrypted.opening')}
        </p>
      ) : null}
      {estado.fase === 'fallo' ? (
        <MotivoDeDriveCifrado
          locale={locale}
          motivo={estado.motivo}
          alReintentar={() => void abrir().catch(() => {})}
        />
      ) : null}
      {viva.abierta && viva.fallo ? (
        <MotivoDeDriveCifrado locale={locale} motivo="fallo" alReintentar={() => cifrado.tocar()} />
      ) : null}
      {viva.indice && viva.indice.fotos.length === 0 ? (
        <EmptyState compact class="mt-2" title={t('gallery.encrypted.empty')} />
      ) : null}
    </section>
  );
}

/** Una foto cifrada en las filas de la galería: su miniatura descifrada y un candado. */
export function MiniaturaCifrada({
  foto,
  url,
  locale,
  seleccionando = false,
  onOpen,
  onFavorite,
}: {
  foto: FotoDeGaleria;
  url: string | undefined;
  locale: Locale;
  seleccionando?: boolean;
  onOpen: () => void;
  onFavorite?: (() => void) | undefined;
}): JSX.Element {
  const t = useT(locale);
  const nombre = foto.entrada.nombre || foto.nombre;
  const favorita = foto.entrada.favorita;
  return (
    <li
      style={estiloDeTesela({ width: foto.entrada.ancho, height: foto.entrada.alto })}
      class="group bg-bg-subtle relative rounded-sm"
      data-cifrada=""
    >
      <button
        type="button"
        data-foto-tesela=""
        class="sytue-focus-ring-inset block h-full w-full scroll-mt-24 overflow-hidden rounded-sm disabled:cursor-not-allowed disabled:opacity-60"
        // En el modo selección no se marcan: las acciones en lote son del servidor.
        disabled={seleccionando}
        title={seleccionando ? t('gallery.encrypted.notSelectable') : nombre}
        aria-label={t('gallery.encrypted.open', { name: nombre })}
        aria-haspopup="dialog"
        onClick={onOpen}
      >
        {url ? (
          <img
            src={url}
            alt={nombre}
            loading="lazy"
            decoding="async"
            class="h-full w-full object-cover"
          />
        ) : (
          <span class="text-text-muted absolute inset-0 flex items-center justify-center">
            <Icon name="lock" size={18} />
          </span>
        )}
      </button>
      <span
        class="pointer-events-none absolute top-1 right-1 rounded-full bg-black/60 p-1 text-white"
        title={t('gallery.encrypted.badge')}
      >
        <Icon name="lock" size={12} label={t('gallery.encrypted.badge')} />
      </span>
      {onFavorite && !seleccionando ? (
        <button
          type="button"
          onClick={onFavorite}
          aria-label={t(favorita ? 'gallery.unfavorite' : 'gallery.favorite')}
          aria-pressed={favorita}
          class={cx(
            'sytue-focus-ring rounded-control absolute bottom-0 left-0 p-1',
            // Con el dedo, solo el de las favoritas: como en la galería del servidor
            // (PhotosApp) y en las apps nativas; se marca desde el visor o la selección.
            !favorita &&
              'opacity-0 group-hover:opacity-100 focus-visible:opacity-100 pointer-coarse:hidden',
          )}
        >
          <span class="rounded-control block bg-black/60 p-1 text-white">
            <Icon name="corazon" size={16} {...(favorita ? { fill: 'currentColor' } : {})} />
          </span>
        </button>
      ) : null}
    </li>
  );
}

/**
 * Marca o desmarca favorita una foto cifrada, solo en la bóveda, y avisa si falla.
 * Lo comparten la línea de tiempo, el visor y los álbumes cifrados.
 */
export function useAccionesCifradas(viva: GaleriaCifradaViva, locale: Locale) {
  const t = useT(locale);
  const toast = useToast();
  const fallo = useCallback(
    () => toast.notify({ tone: 'danger', title: t('gallery.encrypted.actionFailed') }),
    [toast, t],
  );
  return {
    favorita: async (foto: FotoDeGaleria, valor: boolean) => {
      if (!viva.galeria) return;
      try {
        viva.cambiar(foto, await viva.galeria.marcarFavorita(foto, valor));
      } catch {
        fallo();
      }
    },
    album: async (foto: FotoDeGaleria, album: string, dentro: boolean) => {
      if (!viva.galeria) return;
      try {
        viva.cambiar(foto, await viva.galeria.ponerEnAlbum(foto, album, dentro));
      } catch {
        fallo();
      }
    },
    crearAlbum: async (titulo: string): Promise<AlbumCifrado | null> => {
      if (!viva.galeria) return null;
      try {
        const album = await viva.galeria.crearAlbum(titulo);
        viva.anyadirAlbum(album);
        return album;
      } catch {
        fallo();
        return null;
      }
    },
  };
}

/** El visor de una foto cifrada: descifrada en memoria, con favorita, álbumes y edición. */
export function VisorCifrado({
  locale,
  viva,
  fotos,
  inicial,
  alCerrar,
}: {
  locale: Locale;
  viva: GaleriaCifradaViva;
  /** Las fotos por las que se pasa con anterior/siguiente. */
  fotos: readonly FotoDeGaleria[];
  inicial: string;
  alCerrar: () => void;
}): JSX.Element | null {
  const t = useT(locale);
  const toast = useToast();
  const acciones = useAccionesCifradas(viva, locale);
  const { preguntar, dialogo } = usePrompt();
  const [id, setId] = useState(inicial);
  const [original, setOriginal] = useState<{ id: string; blob: Blob; url: string } | null>(null);
  const [editando, setEditando] = useState(false);
  const posicion = fotos.findIndex((f) => f.id === id);
  // La foto viva del índice (favorita y álbumes al día), no la que se abrió.
  const foto = viva.indice?.fotos.find((f) => f.id === id) ?? fotos[posicion];

  useEffect(() => {
    if (!foto || !viva.galeria || foto.bytes > MAXIMO_A_LA_VISTA) return;
    let vivo = true;
    let url = '';
    viva.galeria.original(foto).then(
      (blob) => {
        if (!vivo) return;
        url = URL.createObjectURL(blob);
        setOriginal({ id: foto.id, blob, url });
      },
      () => {},
    );
    return () => {
      vivo = false;
      if (url) URL.revokeObjectURL(url);
    };
    // Por id y versión del contenido: favorita o álbumes no vuelven a descifrar.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [foto?.id, foto?.entrada.suma, viva.galeria]);

  if (!foto) return null;
  const nombre = foto.entrada.nombre || foto.nombre;
  const vista = original?.id === foto.id ? original.url : viva.url(foto);
  const ir = (paso: number) => {
    const otra = fotos[posicion + paso];
    if (otra) setId(otra.id);
  };
  const albumes = viva.indice?.albumes ?? [];

  if (editando && original?.id === foto.id && viva.galeria) {
    const galeria = viva.galeria;
    return (
      <EditorCifrado
        locale={locale}
        nombre={nombre}
        original={original.blob}
        alCancelar={() => setEditando(false)}
        alGuardar={async (editada) => {
          try {
            viva.cambiar(foto, await galeria.guardarEdicion(foto, editada));
            setOriginal(null);
            setEditando(false);
            toast.notify({ tone: 'success', title: t('gallery.encrypted.saved') });
          } catch {
            toast.notify({ tone: 'danger', title: t('editor.saveFailed') });
          }
        }}
      />
    );
  }

  return (
    <Modal open onClose={alCerrar} title={nombre} size="xl">
      <div class="flex flex-col gap-3">
        <div class="relative flex min-h-48 items-center justify-center">
          {vista ? (
            <img src={vista} alt={nombre} class="mx-auto max-h-[65dvh] object-contain" />
          ) : (
            <p class="text-text-muted flex items-center gap-2 text-sm" role="status">
              <Icon name="lock" size={16} />
              {t('gallery.encrypted.opening')}
            </p>
          )}
        </div>
        <p class="text-text-muted text-xs">
          {formatDateTime(locale, new Date(foto.entrada.tomada))}
          {foto.entrada.exif ? '' : ` · ${t('gallery.noExifDate')}`}
        </p>
        <div class="flex flex-wrap items-center gap-2">
          <IconButton label={t('viewer.prev')} disabled={posicion <= 0} onClick={() => ir(-1)}>
            <Icon name="chevronLeft" />
          </IconButton>
          <IconButton
            label={t('viewer.next')}
            disabled={posicion < 0 || posicion >= fotos.length - 1}
            onClick={() => ir(1)}
          >
            <Icon name="chevronRight" />
          </IconButton>
          <Button
            variant="ghost"
            size="sm"
            leadingIcon={
              <Icon name="corazon" {...(foto.entrada.favorita ? { fill: 'currentColor' } : {})} />
            }
            aria-pressed={foto.entrada.favorita}
            onClick={() => void acciones.favorita(foto, !foto.entrada.favorita)}
          >
            {t(foto.entrada.favorita ? 'gallery.unfavorite' : 'gallery.favorite')}
          </Button>
          <Button
            variant="ghost"
            size="sm"
            leadingIcon={<Icon name="pen" />}
            disabled={original?.id !== foto.id}
            onClick={() => setEditando(true)}
          >
            {t('viewer.edit')}
          </Button>
          {vista && original?.id === foto.id ? (
            <a
              class="text-primary-text inline-flex items-center gap-1 text-sm"
              href={original.url}
              download={nombre}
            >
              <Icon name="download" size={16} />
              {t('viewer.download')}
            </a>
          ) : null}
        </div>
        <fieldset class="flex flex-wrap items-center gap-2">
          <legend class="mb-1 text-xs font-semibold">{t('gallery.encrypted.albums')}</legend>
          {albumes.map((a) => {
            const dentro = foto.entrada.albumes.includes(a.id);
            return (
              <Button
                key={a.id}
                size="xs"
                variant={dentro ? 'secondary' : 'ghost'}
                aria-pressed={dentro}
                leadingIcon={<Icon name={dentro ? 'check' : 'album'} size={14} />}
                onClick={() => void acciones.album(foto, a.id, !dentro)}
              >
                {a.titulo}
              </Button>
            );
          })}
          <Button
            size="xs"
            variant="ghost"
            leadingIcon={<Icon name="plus" size={14} />}
            onClick={() =>
              void preguntar({
                titulo: t('gallery.encrypted.newAlbum'),
                mensaje: t('albums.title'),
                confirmLabel: t('gallery.newAlbumCreate'),
              }).then(async (titulo) => {
                if (!titulo?.trim()) return;
                const album = await acciones.crearAlbum(titulo);
                if (album) await acciones.album(foto, album.id, true);
              })
            }
          >
            {t('gallery.encrypted.newAlbum')}
          </Button>
        </fieldset>
        <p class="text-text-muted flex items-start gap-1 text-xs">
          <Icon name="lock" size={12} />
          {t('gallery.encrypted.note')}
        </p>
      </div>
      {dialogo}
    </Modal>
  );
}

/** El valor vigente de un deslizador: el último paso de su tipo, o 0. */
function valorDe(r: Receta, tipo: Paso['tipo']): number {
  for (let i = r.pasos.length - 1; i >= 0; i--)
    if (r.pasos[i]!.tipo === tipo) return r.pasos[i]!.params?.valor ?? 0;
  return 0;
}

const DESLIZADORES = [
  ['exposicion', 'editor.exposure', -5, 5],
  ['contraste', 'editor.contrast', -1, 1],
  ['saturacion', 'editor.saturation', -1, 1],
  ['temperatura', 'editor.temperature', -1, 1],
] as const;

/** Pinta la receta sobre una imagen: geometría y luego color, con el motor del editor. */
export function pintarReceta(bm: ImageBitmap, receta: Receta, lado = Infinity): HTMLCanvasElement {
  const escala = Math.min(1, lado / Math.max(bm.width, bm.height));
  const fuente = aplicarGeometria(
    bm,
    Math.max(1, Math.round(bm.width * escala)),
    Math.max(1, Math.round(bm.height * escala)),
    receta,
  );
  const ctx = fuente.getContext('2d')!;
  const img = ctx.getImageData(0, 0, fuente.width, fuente.height);
  aplicarColorCPU(img, receta);
  ctx.putImageData(img, 0, 0);
  return fuente;
}

/**
 * El editor de una foto cifrada: giro, espejo y los cuatro ajustes del editor, todo
 * en el navegador. Guardar pinta la foto entera y la sube como versión nueva del
 * mismo fichero (el original sigue en el historial de versiones de Drive).
 */
export function EditorCifrado({
  locale,
  nombre,
  original,
  alGuardar,
  alCancelar,
}: {
  locale: Locale;
  nombre: string;
  original: Blob;
  alGuardar: (editada: Blob) => Promise<void>;
  alCancelar: () => void;
}): JSX.Element {
  const t = useT(locale);
  const [receta, setReceta] = useState<Receta>(recetaVacia());
  const [bitmap, setBitmap] = useState<ImageBitmap | null>(null);
  const [guardando, setGuardando] = useState(false);
  const contenedor = useRef<HTMLDivElement>(null);

  useEffect(() => {
    let vivo = true;
    let bm: ImageBitmap | null = null;
    void createImageBitmap(original, { imageOrientation: 'from-image' }).then(
      (b) => {
        bm = b;
        if (vivo) setBitmap(b);
        else b.close();
      },
      () => {},
    );
    return () => {
      vivo = false;
      bm?.close();
    };
  }, [original]);

  useEffect(() => {
    const caja = contenedor.current;
    if (!bitmap || !caja) return;
    const lienzo = pintarReceta(bitmap, receta, LADO_PREVIA);
    lienzo.className = 'mx-auto max-h-[55dvh] max-w-full';
    lienzo.setAttribute('role', 'img');
    lienzo.setAttribute('aria-label', nombre);
    caja.replaceChildren(lienzo);
  }, [bitmap, receta, nombre]);

  const anyadir = (p: Paso) => setReceta((r) => ({ ...r, pasos: [...r.pasos, p] }));
  const poner = (tipo: Paso['tipo'], valor: number) =>
    setReceta((r) => {
      const pasos = r.pasos.filter((p) => p.tipo !== tipo);
      if (valor !== 0) pasos.push({ tipo, params: { valor } });
      return { ...r, pasos };
    });

  const guardar = async () => {
    if (!bitmap) return;
    setGuardando(true);
    try {
      const lienzo = pintarReceta(bitmap, receta);
      const blob = await new Promise<Blob | null>((ok) => lienzo.toBlob(ok, 'image/jpeg', 0.92));
      if (blob) await alGuardar(blob);
    } finally {
      setGuardando(false);
    }
  };

  return (
    <Modal
      open
      onClose={alCancelar}
      title={nombre}
      size="xl"
      footer={
        <>
          <Button variant="secondary" onClick={alCancelar}>
            {t('viewer.close')}
          </Button>
          <Button
            variant="primary"
            leadingIcon={<Icon name="save" />}
            disabled={!bitmap || guardando || receta.pasos.length === 0}
            onClick={() => void guardar()}
          >
            {t('gallery.encrypted.saveVersion')}
          </Button>
        </>
      }
    >
      <div class="flex flex-col gap-3">
        <div ref={contenedor} class="flex min-h-48 items-center justify-center">
          {bitmap ? null : <Spinner />}
        </div>
        <div class="flex flex-wrap gap-1">
          <IconButton
            label={t('editor.rotate')}
            onClick={() => anyadir({ tipo: 'giro', params: { grados: 90 } })}
          >
            <Icon name="rotate" />
          </IconButton>
          <IconButton
            label={t('editor.flipH')}
            onClick={() => anyadir({ tipo: 'espejo', eje: 'h' })}
          >
            <Icon name="flipH" />
          </IconButton>
          <IconButton
            label={t('editor.flipV')}
            onClick={() => anyadir({ tipo: 'espejo', eje: 'v' })}
          >
            <Icon name="flipV" />
          </IconButton>
          <IconButton
            label={t('editor.undo')}
            disabled={receta.pasos.length === 0}
            onClick={() => setReceta((r) => ({ ...r, pasos: r.pasos.slice(0, -1) }))}
          >
            <Icon name="undo" />
          </IconButton>
        </div>
        <div class="grid gap-2 sm:grid-cols-2">
          {DESLIZADORES.map(([tipo, clave, min, max]) => (
            <label key={tipo} class="flex flex-col gap-1 text-xs">
              {t(clave)}
              <input
                type="range"
                min={min}
                max={max}
                step={(max - min) / 100}
                value={valorDe(receta, tipo)}
                onInput={(e) => poner(tipo, Number((e.target as HTMLInputElement).value))}
              />
            </label>
          ))}
        </div>
        <p class="text-text-muted text-xs">{t('gallery.encrypted.editHint')}</p>
      </div>
    </Modal>
  );
}
