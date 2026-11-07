/**
 * ArchivedChannels — pestaña «Archivados» de «Explorar canales». Lista los canales
 * archivados del usuario; cada fila ofrece «Renombrar» (el nombre pasa a ser un campo:
 * Intro guarda, Esc cancela sin cerrar el diálogo) y «Desarchivar», que lo devuelve a la
 * barra. El foco nunca cae al cuerpo: entra en el campo al renombrar, vuelve a
 * «Renombrar» al guardar o cancelar y, al desarchivar, pasa a la fila siguiente (o a la
 * pestaña si no quedan). Quien no
 * administra el canal recibe el 403 del servicio y su aviso, no un botón escondido: el
 * permiso lo decide el backend (requireChannelAdmin).
 *
 * Gemelo en Fyne: apps/fyne/apps/rooms/archivados.go.
 */
import type { JSX } from 'preact';
import { useEffect, useRef, useState } from 'preact/hooks';
import { Button, EmptyState, Illustration, Input, Spinner } from '@sytue/ui';
import {
  ChatHttpError,
  listArchivedChannels,
  updateChannel,
  type Channel,
} from '../client/chatApi.js';
import {
  claveErrorArchivado,
  filaTrasQuitar,
  nombreNuevo,
  soloArchivados,
} from '../lib/archivados.js';
import type { T } from '../i18n/index.js';
import { ArchiveGlyph } from './glyphs.js';

interface Props {
  /** Canal devuelto a la barra: el padre lo añade y se suscribe a él. */
  onUnarchived: (channel: Channel) => void;
  t: T;
}

export function ArchivedChannels({ onUnarchived, t }: Props): JSX.Element {
  const [canales, setCanales] = useState<Channel[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);
  const [tick, setTick] = useState(0);
  // Filas ocupadas (desarchivando o guardando el nombre), cada una por su id: con un solo
  // hueco, el `finally` de la primera reactivaba el botón de otra aún en curso.
  const [ocupados, setOcupados] = useState<ReadonlySet<string>>(new Set());
  const [renombrando, setRenombrando] = useState<string | null>(null);
  const [nombre, setNombre] = useState('');
  const [aviso, setAviso] = useState<string | null>(null);
  // Adónde va el foco tras el próximo pintado. `autofocus` no sirve: Preact solo lo
  // aplica al montar y el repositorio ya lo dio por no fiable (QuickSwitcher).
  const focoRef = useRef<{
    fila: string | null;
    destino: 'campo' | 'renombrar' | 'desarchivar';
  } | null>(null);
  const listaRef = useRef<HTMLUListElement>(null);
  // El diálogo que nos contiene, para llevar el foco a la pestaña si la lista se vacía.
  const dialogoRef = useRef<HTMLElement | null>(null);

  useEffect(() => {
    if (listaRef.current)
      dialogoRef.current = listaRef.current.closest<HTMLElement>('[role="dialog"]');
    const f = focoRef.current;
    if (!f) return;
    const fila = f.fila
      ? listaRef.current?.querySelector(`[data-fila="${CSS.escape(f.fila)}"]`)
      : null;
    const el =
      f.destino === 'campo'
        ? fila?.querySelector<HTMLElement>('input')
        : fila?.querySelector<HTMLElement>(`[data-accion="${f.destino}"]`);
    if (el && !(el as HTMLButtonElement).disabled) {
      focoRef.current = null;
      el.focus();
      return;
    }
    if (!f.fila || !fila) {
      focoRef.current = null;
      const tab = dialogoRef.current?.querySelector<HTMLElement>(
        '[role="tab"][aria-selected="true"]',
      );
      (tab ?? dialogoRef.current)?.focus();
    }
    // Si la fila sigue ahí pero su botón está inerte (petición en curso), se espera al
    // siguiente pintado, que llega cuando la fila deja de estar ocupada.
  });

  function ocupar(id: string, si: boolean): void {
    setOcupados((prev) => {
      const next = new Set(prev);
      if (si) next.add(id);
      else next.delete(id);
      return next;
    });
  }

  useEffect(() => {
    let live = true;
    setLoading(true);
    setError(false);
    listArchivedChannels()
      .then(({ channels }) => live && setCanales(soloArchivados(channels)))
      .catch(() => live && setError(true))
      .finally(() => live && setLoading(false));
    return () => {
      live = false;
    };
  }, [tick]);

  function fallo(err: unknown): void {
    setAviso(t(claveErrorArchivado(err instanceof ChatHttpError ? err.status : undefined)));
  }

  function desarchivar(c: Channel): void {
    ocupar(c.id, true);
    setAviso(null);
    updateChannel(c.id, { archived: false })
      .then(({ channel }) => {
        setCanales((prev) => {
          focoRef.current = {
            fila: filaTrasQuitar(
              prev.map((x) => x.id),
              c.id,
            ),
            destino: 'desarchivar',
          };
          return prev.filter((x) => x.id !== c.id);
        });
        onUnarchived({ ...c, ...channel, archived: false });
      })
      .catch((err: unknown) => {
        focoRef.current = { fila: c.id, destino: 'desarchivar' };
        fallo(err);
      })
      .finally(() => ocupar(c.id, false));
  }

  function empezarRenombrado(c: Channel): void {
    setNombre(c.name);
    setRenombrando(c.id);
    focoRef.current = { fila: c.id, destino: 'campo' };
  }

  function cancelarRenombrado(id: string): void {
    setRenombrando(null);
    focoRef.current = { fila: id, destino: 'renombrar' };
  }

  function guardarNombre(c: Channel): void {
    const nuevo = nombreNuevo(c.name, nombre);
    cancelarRenombrado(c.id);
    if (!nuevo) return;
    ocupar(c.id, true);
    setAviso(null);
    updateChannel(c.id, { name: nuevo })
      .then(({ channel }) =>
        setCanales((prev) =>
          soloArchivados(
            prev.map((x) => (x.id === c.id ? { ...x, name: channel.name || nuevo } : x)),
          ),
        ),
      )
      .catch(fallo)
      .finally(() => ocupar(c.id, false));
  }

  if (loading) {
    return (
      <div class="grid place-items-center py-10">
        <Spinner size={22} />
      </div>
    );
  }
  if (error) {
    return (
      <div class="text-text-muted flex flex-col items-center gap-2 py-8 text-sm">
        <span>{t('archived.loadFailed')}</span>
        <Button size="sm" variant="secondary" onClick={() => setTick((n) => n + 1)}>
          {t('action.retry')}
        </Button>
      </div>
    );
  }
  if (canales.length === 0) {
    return (
      <EmptyState
        class="py-4"
        title={t('archived.empty')}
        illustration={<Illustration name="emptyChat" size={96} />}
      />
    );
  }
  return (
    <div class="flex flex-col gap-2">
      {aviso ? (
        <p class="text-danger-text text-xs" role="alert">
          {aviso}
        </p>
      ) : null}
      <ul ref={listaRef} class="flex max-h-[46dvh] flex-col gap-1 overflow-y-auto">
        {canales.map((c) => (
          <li
            key={c.id}
            data-fila={c.id}
            class="hover:bg-bg-subtle flex items-center gap-3 rounded-md px-2 py-2"
          >
            <span class="text-text-muted shrink-0">
              <ArchiveGlyph size={16} />
            </span>
            {renombrando === c.id ? (
              <form
                class="flex min-w-0 flex-1 items-center gap-2"
                onSubmit={(e) => {
                  e.preventDefault();
                  guardarNombre(c);
                }}
              >
                <Input
                  class="min-w-0 flex-1"
                  value={nombre}
                  // El Modal escucha Escape en `document`, en captura: sin esta marca se
                  // comía la tecla y cerraba «Explorar canales» entero.
                  data-escape-local
                  aria-label={t('archived.renameLabel', { name: c.name })}
                  onInput={(e) => setNombre(e.currentTarget.value)}
                  onKeyDown={(e) => {
                    if (e.key === 'Escape') {
                      e.preventDefault();
                      e.stopPropagation();
                      cancelarRenombrado(c.id);
                    }
                  }}
                />
                <Button size="sm" type="submit">
                  {t('action.save')}
                </Button>
                <Button size="sm" variant="ghost" onClick={() => cancelarRenombrado(c.id)}>
                  {t('action.cancel')}
                </Button>
              </form>
            ) : (
              <>
                <div class="min-w-0 flex-1">
                  <p class="text-text truncate text-base font-medium">{c.name}</p>
                  {c.purpose || c.topic ? (
                    <p class="text-text-muted truncate text-xs">{c.purpose || c.topic}</p>
                  ) : null}
                </div>
                <Button
                  size="sm"
                  variant="ghost"
                  data-accion="renombrar"
                  disabled={ocupados.has(c.id)}
                  onClick={() => empezarRenombrado(c)}
                >
                  {t('archived.rename')}
                </Button>
                <Button
                  size="sm"
                  variant="secondary"
                  data-accion="desarchivar"
                  loading={ocupados.has(c.id)}
                  onClick={() => desarchivar(c)}
                >
                  {t('archived.unarchive')}
                </Button>
              </>
            )}
          </li>
        ))}
      </ul>
    </div>
  );
}
