/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * LinkTool — la herramienta de enlaces de la barra: UNA caja para todo.
 *
 * Se pega una URL y se enlaza. Se escribe el nombre de una nota —o el de uno de
 * los títulos que hay DENTRO de una nota— y sale un desplegable con lo que casa:
 * primero lo de este cuaderno, al instante y sin red; después, lo de los demás
 * cuadernos, que viene de la búsqueda del servicio. Es el «insertar referencia»
 * de BasKet Note Pads, sin el diálogo de tres pasos.
 *
 * Los títulos de este cuaderno se sacan del contenido que ya está en memoria;
 * los de otros cuadernos los trae la búsqueda en `headings`.
 */
import type { JSX } from 'preact';
import { useEffect, useRef, useState } from 'preact/hooks';
import { Spinner, cx } from '@sytue/ui';
import type { Note, NoteHit } from '@sytue/api-clients';
import type { T } from '../i18n/index.js';
import { esURLWeb } from '../client/captura.js';
import { slugDe, titulosDe, urlDeNota, type DestinoInterno } from '../client/enlaces.js';
import type { VistaPrevia } from './VistaPreviaEnlace.js';
import { listarDeLaSuite, type Elegible } from '../client/notesApi.js';
import type { ClaroPorNota } from './NoteCanvas.js';
import { Glyph } from './glyphs.js';
import { plegar } from '@sytue/i18n';

/** Lo que el editor necesita para ofrecer notas y títulos. */
export interface FuentesDeEnlace {
  notas: Note[];
  notebookId: string;
  publicBase: string;
  buscar: (q: string) => Promise<NoteHit[]>;
  /** Título y contenido en claro de las notas cifradas, si los hay. */
  claro?: ClaroPorNota;
  /** La ficha de un enlace a otra nota, para la lectura (VistaPreviaEnlace.tsx). */
  previa?: (d: DestinoInterno) => Promise<VistaPrevia | null>;
}

export interface Eleccion {
  url: string;
  label: string;
}

interface Opcion {
  clave: string;
  url: string;
  /** Texto que se inserta si no había selección. */
  label: string;
  /** Lo que se pinta: nota, y título si lo hay. */
  nota: string;
  titulo?: string;
  cuaderno?: string;
  tipo: 'url' | 'local' | 'remota' | 'drive';
}

interface Props {
  t: T;
  fuentes?: FuentesDeEnlace;
  onPick: (e: Eleccion) => void;
  onClose: () => void;
}

/** Comparación sin acentos ni mayúsculas: «presupuesto» casa con «Presupuesto». */
function normaliza(s: string): string {
  return plegar(s);
}

function opcionesLocales(q: string, f: FuentesDeEnlace): Opcion[] {
  const nq = normaliza(q);
  const out: Opcion[] = [];
  for (const n of f.notas) {
    const titulo = (f.claro?.[n.id]?.title ?? n.title ?? '').trim();
    const contenido = f.claro?.[n.id]?.content ?? n.content ?? '';
    const nombre = titulo || '…';
    if (titulo && normaliza(titulo).includes(nq)) {
      out.push({
        clave: n.id,
        url: urlDeNota(f.publicBase, f.notebookId, n.id),
        label: titulo,
        nota: nombre,
        tipo: 'local',
      });
    }
    for (const h of titulosDe(contenido)) {
      if (normaliza(h.texto).includes(nq)) {
        out.push({
          clave: `${n.id}#${h.slug}`,
          url: urlDeNota(f.publicBase, f.notebookId, n.id, h.slug),
          label: h.texto,
          nota: nombre,
          titulo: h.texto,
          tipo: 'local',
        });
      }
    }
    if (out.length >= 8) break;
  }
  return out.slice(0, 8);
}

function opcionesRemotas(q: string, hits: NoteHit[], f: FuentesDeEnlace): Opcion[] {
  const nq = normaliza(q);
  const out: Opcion[] = [];
  for (const h of hits) {
    if (h.notebook_id === f.notebookId) continue; // lo local ya está arriba
    const nombre = h.title || h.notebook;
    out.push({
      clave: h.id,
      url: urlDeNota(f.publicBase, h.notebook_id, h.id),
      label: nombre,
      nota: nombre,
      cuaderno: h.notebook,
      tipo: 'remota',
    });
    for (const titulo of h.headings ?? []) {
      if (normaliza(titulo).includes(nq)) {
        out.push({
          clave: `${h.id}#${slugDe(titulo)}`,
          url: urlDeNota(f.publicBase, h.notebook_id, h.id, slugDe(titulo)),
          label: titulo,
          nota: nombre,
          titulo,
          cuaderno: h.notebook,
          tipo: 'remota',
        });
      }
    }
    if (out.length >= 8) break;
  }
  return out.slice(0, 8);
}

/**
 * Los ficheros de Drive que casan con lo escrito, hasta ocho. Las carpetas no se
 * enlazan (se navegan), y sin el enlace que compone el BFF no hay qué enlazar: el
 * respaldo que había, `/drive?file=`, era relativo a ESTE host y abría un 404 de
 * Notepads. Es el `opcionesDrive` de la app nativa.
 */
export function opcionesDrive(items: Elegible[]): Opcion[] {
  return items
    .filter((it): it is Elegible & { url: string } => !it.isDirectory && !!it.url)
    .slice(0, 8)
    .map((it) => ({
      clave: `drive:${it.id}`,
      url: it.url,
      label: it.name,
      nota: it.name,
      cuaderno: 'Sytue Drive',
      tipo: 'drive' as const,
    }));
}

export function LinkTool({ t, fuentes, onPick, onClose }: Props): JSX.Element {
  const [q, setQ] = useState('');
  const [remotas, setRemotas] = useState<Opcion[]>([]);
  const [driveOpciones, setDriveOpciones] = useState<Opcion[]>([]);
  const [buscando, setBuscando] = useState(false);
  const [activa, setActiva] = useState(0);
  const input = useRef<HTMLInputElement>(null);
  const debounceRef = useRef<ReturnType<typeof setTimeout>>();

  useEffect(() => {
    input.current?.focus();
  }, []);

  const consulta = q.trim();
  const esURL = esURLWeb(consulta);
  const locales = fuentes && consulta && !esURL ? opcionesLocales(consulta, fuentes) : [];

  // La búsqueda remota y de Drive va con retardo: escribir «informe» son siete pulsaciones
  // y siete viajes para seis resultados que nadie llegó a leer.
  useEffect(() => {
    clearTimeout(debounceRef.current);
    if (!consulta || consulta.length < 2 || esURL) {
      setRemotas([]);
      setDriveOpciones([]);
      setBuscando(false);
      return;
    }
    setBuscando(true);
    let vivo = true;
    debounceRef.current = setTimeout(() => {
      const promesas: Promise<unknown>[] = [];
      if (fuentes) {
        promesas.push(
          fuentes
            .buscar(consulta)
            .then((hits) => vivo && setRemotas(opcionesRemotas(consulta, hits, fuentes)))
            .catch(() => vivo && setRemotas([])),
        );
      }
      promesas.push(
        listarDeLaSuite('drive', { q: consulta })
          .then((res) => vivo && setDriveOpciones(opcionesDrive(res.items ?? [])))
          .catch(() => vivo && setDriveOpciones([])),
      );
      void Promise.allSettled(promesas).finally(() => vivo && setBuscando(false));
    }, 250);
    return () => {
      vivo = false;
    };
  }, [consulta, esURL, fuentes]);

  const opciones: Opcion[] = esURL
    ? [{ clave: 'url', url: consulta, label: consulta, nota: consulta, tipo: 'url' }]
    : [...locales, ...remotas, ...driveOpciones];
  const elegir = (o: Opcion) => onPick({ url: o.url, label: o.label });

  return (
    <div class="grid w-[min(92vw,26rem)] gap-2 p-2">
      <input
        ref={input}
        type="text"
        value={q}
        aria-label={t('editor.linkTool')}
        placeholder={t('editor.linkPlaceholder')}
        class="border-border bg-surface text-text sytue-focus-ring w-full rounded-md border px-2.5 py-1.5 text-base"
        onInput={(e) => {
          setQ((e.target as HTMLInputElement).value);
          setActiva(0);
        }}
        onKeyDown={(e) => {
          if (e.key === 'ArrowDown') {
            e.preventDefault();
            setActiva((a) => Math.min(a + 1, opciones.length - 1));
          } else if (e.key === 'ArrowUp') {
            e.preventDefault();
            setActiva((a) => Math.max(a - 1, 0));
          } else if (e.key === 'Enter') {
            e.preventDefault();
            const o = opciones[activa];
            if (o) elegir(o);
          } else if (e.key === 'Escape') {
            e.stopPropagation();
            onClose();
          }
        }}
      />
      {consulta ? (
        <ul class="max-h-72 overflow-auto" aria-label={t('editor.linkTool')}>
          {opciones.map((o, i) => (
            <li key={o.clave}>
              {/* Los grupos se rotulan por el primer elemento de cada tipo, no
                  con cabeceras sueltas que se lleven el foco del teclado. */}
              {i === 0 && o.tipo === 'local' ? (
                <p class="sytue-meta px-2 pt-1">{t('editor.linkThisNotebook')}</p>
              ) : null}
              {o.tipo === 'remota' && (i === 0 || opciones[i - 1]?.tipo !== 'remota') ? (
                <p class="sytue-meta px-2 pt-1">{t('editor.linkOtherNotebooks')}</p>
              ) : null}
              {o.tipo === 'drive' && (i === 0 || opciones[i - 1]?.tipo !== 'drive') ? (
                <p class="sytue-meta px-2 pt-1">Sytue Drive</p>
              ) : null}
              <button
                type="button"
                aria-current={i === activa ? 'true' : undefined}
                onMouseDown={(e) => e.preventDefault()}
                onClick={() => elegir(o)}
                class={cx(
                  'sytue-focus-ring flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-base',
                  i === activa ? 'bg-primary/10' : 'hover:bg-bg-subtle',
                )}
              >
                <Glyph
                  name={
                    o.tipo === 'url'
                      ? 'link'
                      : o.tipo === 'drive'
                        ? 'file'
                        : o.titulo
                          ? 'heading'
                          : 'note'
                  }
                  size={14}
                  class="text-text-subtle shrink-0"
                />
                <span class="min-w-0 flex-1 truncate">
                  {o.tipo === 'url' ? (
                    t('editor.linkToUrl', { url: o.url })
                  ) : (
                    <>
                      <span class="text-text">{o.nota}</span>
                      {o.titulo ? <span class="text-text-muted"> › {o.titulo}</span> : null}
                    </>
                  )}
                </span>
                {o.cuaderno ? <span class="sytue-meta shrink-0">{o.cuaderno}</span> : null}
              </button>
            </li>
          ))}
          {opciones.length === 0 && !buscando ? (
            <li class="text-text-muted px-2 py-2 text-base">{t('editor.linkNoResults')}</li>
          ) : null}
          {buscando ? (
            <li class="sytue-meta flex items-center gap-2 px-2 py-2">
              <Spinner size={14} /> {t('editor.linkSearching')}
            </li>
          ) : null}
        </ul>
      ) : null}
    </div>
  );
}
