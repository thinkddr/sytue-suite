// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
/*
 * VersionesNota — el historial de versiones de una nota: cómo estaba antes de cada
 * tanda de edición, para verla y volver a ella.
 *
 * El servicio guarda la versión al empezar a editar (no en cada autoguardado) y la
 * guarda tal cual: en un cuaderno cifrado, el criptograma. Por eso se descifra AQUÍ,
 * con la clave que ya está en memoria, y volver a una versión es un guardado normal
 * de la nota (`onRestaurar`): el que cifra, avisa de conflictos y se deshace con el
 * «Deshacer» del cuaderno. El servidor no ve nunca el claro de una versión.
 *
 * La lista llega sin texto; cada versión se pide al elegirla.
 */
import type { JSX } from 'preact';
import { useEffect, useRef, useState } from 'preact/hooks';
import { Button, EmptyState, Modal, Spinner } from '@sytue/ui';
import { formatDateTime, type Locale } from '@sytue/i18n';
import type { Note, NoteVersion } from '@sytue/api-clients';
import type { T } from '../i18n/index.js';
import * as api from '../client/notesApi.js';
import { abrirContenidoNota } from '../client/notesEncrypted.js';
import { textoDeNota } from '../client/extracto.js';

/** Lo que se restaura: el texto en claro y los datos (adjuntos, casillas…). */
export interface ClaroDeVersion {
  title: string;
  content: string;
  data: Record<string, unknown>;
}

/** Hasta dónde se enseña: basta para reconocerla y decidir. */
const MAX_VISTA = 2000;

async function abrir(
  note: Note,
  v: NoteVersion,
  clave: CryptoKey | null,
): Promise<ClaroDeVersion | 'cerrada'> {
  if (!v.content_cipher)
    return { title: v.title ?? '', content: v.content ?? '', data: v.data ?? {} };
  if (!clave) return 'cerrada';
  // La versión se abre como la nota: con sus datos públicos de entonces al lado.
  // Sin `data_json`: es la representación de la nota de AHORA, no de la versión.
  const { data_json: _ahora, ...base } = note;
  const { claro } = await abrirContenidoNota(clave, {
    ...base,
    title: '',
    content: '',
    content_cipher: v.content_cipher,
    data: v.data ?? {},
  });
  return { title: claro.title, content: claro.content, data: claro.data ?? {} };
}

export function VersionesNota({
  t,
  locale,
  note,
  clave,
  onRestaurar,
  onClose,
}: {
  t: T;
  locale: Locale;
  note: Note;
  /** La clave del cuaderno cifrado, si está abierto; `null` si no hace falta o no la hay. */
  clave: CryptoKey | null;
  /** Vuelve a la versión elegida con un guardado normal de la nota. */
  onRestaurar: (v: ClaroDeVersion) => Promise<void> | void;
  onClose: () => void;
}): JSX.Element {
  const [lista, setLista] = useState<NoteVersion[] | 'cargando' | 'error'>('cargando');
  const [elegida, setElegida] = useState<string | null>(null);
  const [vista, setVista] = useState<ClaroDeVersion | 'cargando' | 'cerrada' | 'error' | null>(
    null,
  );
  const [restaurando, setRestaurando] = useState(false);
  // `note` cambia de identidad con cada guardado; la versión elegida, no: se lee de
  // aquí para no volver a pedirla en cada autoguardado.
  const notaRef = useRef(note);
  useEffect(() => {
    notaRef.current = note;
  });

  useEffect(() => {
    let vivo = true;
    setLista('cargando');
    api
      .versionesDeNota(note.id)
      .then((vs) => {
        if (!vivo) return;
        setLista(vs);
        setElegida(vs[0]?.id ?? null);
      })
      .catch(() => vivo && setLista('error'));
    return () => {
      vivo = false;
    };
  }, [note.id]);

  useEffect(() => {
    if (!elegida) {
      setVista(null);
      return;
    }
    // `vivo` corta la versión que llega después de elegir otra.
    let vivo = true;
    setVista('cargando');
    api
      .versionDeNota(note.id, elegida)
      .then((v) => abrir(notaRef.current, v, clave))
      .then((c) => vivo && setVista(c))
      .catch(() => vivo && setVista('error'));
    return () => {
      vivo = false;
    };
  }, [note.id, elegida, clave]);

  const restaurar = async () => {
    if (!vista || typeof vista === 'string' || restaurando) return;
    setRestaurando(true);
    try {
      await onRestaurar(vista);
      onClose();
    } finally {
      setRestaurando(false);
    }
  };

  const fecha = (iso: string) => formatDateTime(locale, iso);
  const listaLista = Array.isArray(lista) ? lista : [];

  let cuerpo: JSX.Element;
  if (lista === 'cargando') {
    cuerpo = (
      <div class="flex justify-center py-6">
        <Spinner label={t('versions.loading')} />
      </div>
    );
  } else if (lista === 'error') {
    cuerpo = (
      <p role="alert" class="text-danger-text text-sm">
        {t('versions.loadFailed')}
      </p>
    );
  } else if (lista.length === 0) {
    cuerpo = <EmptyState title={t('versions.empty')} description={t('versions.body')} />;
  } else {
    cuerpo = (
      <div class="grid gap-3 sm:grid-cols-[minmax(0,14rem)_1fr]">
        <ul class="grid content-start gap-1" aria-label={t('versions.title')}>
          {lista.map((v) => (
            <li key={v.id}>
              <button
                type="button"
                aria-pressed={v.id === elegida}
                onClick={() => setElegida(v.id)}
                class={`hover:bg-bg-subtle sytue-focus-ring-inner min-h-11 w-full rounded-md px-3 py-2 text-left text-sm ${
                  v.id === elegida ? 'bg-bg-subtle text-text font-semibold' : 'text-text-muted'
                }`}
              >
                <time dateTime={v.created_at}>{fecha(v.created_at)}</time>
              </button>
            </li>
          ))}
        </ul>
        <section
          aria-live="polite"
          class="border-border bg-bg-subtle min-h-32 rounded-md border p-3"
        >
          {vista === 'cargando' || vista === null ? (
            <div class="flex justify-center py-6">
              <Spinner label={t('versions.loading')} />
            </div>
          ) : vista === 'cerrada' ? (
            <p class="text-text-muted text-sm">{t('versions.locked')}</p>
          ) : vista === 'error' ? (
            <p role="alert" class="text-danger-text text-sm">
              {t('versions.loadFailed')}
            </p>
          ) : (
            <>
              <h3 class="text-text mb-2 font-semibold break-words">
                {vista.title.trim() || t('note.untitled')}
              </h3>
              <p class="text-text text-sm break-words whitespace-pre-line">
                {recortar(textoDeNota(vista.content).trim()) || t('conflict.empty')}
              </p>
            </>
          )}
        </section>
      </div>
    );
  }

  return (
    <Modal
      open
      size="lg"
      title={t('versions.title')}
      onClose={onClose}
      footer={
        listaLista.length > 0 ? (
          <>
            <Button variant="ghost" onClick={onClose}>
              {t('common.close')}
            </Button>
            <Button
              disabled={restaurando || !vista || typeof vista === 'string'}
              onClick={() => void restaurar()}
            >
              {t('versions.restore')}
            </Button>
          </>
        ) : undefined
      }
    >
      {listaLista.length > 0 ? (
        <p class="text-text-muted mb-3 text-sm">{t('versions.body')}</p>
      ) : null}
      {cuerpo}
    </Modal>
  );
}

function recortar(s: string): string {
  return s.length > MAX_VISTA ? `${s.slice(0, MAX_VISTA - 1)}…` : s;
}
