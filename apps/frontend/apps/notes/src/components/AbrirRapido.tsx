/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Ir a» (Ctrl/⌘+P, o Ctrl/⌘+O): escribir el nombre de un cuaderno o de una
 * nota, flechas e Intro, sin tocar el ratón. Es el «quick switcher» de Twos y de
 * cualquier editor. Los cuadernos y las notas del cuaderno abierto salen al
 * instante; las de los demás, de la búsqueda del servicio (solo las indexables,
 * las mismas que encuentra el buscador).
 *
 * Su espejo en el escritorio es `abrir_rapido.go` de apps/fyne/apps/notes.
 */
import type { JSX } from 'preact';
import { useEffect, useMemo, useState } from 'preact/hooks';
import { Input, Modal, cx } from '@sytue/ui';
import { contiene } from '@sytue/i18n';
import type { Note, NoteHit, Notebook } from '@sytue/api-clients';
import type { T } from '../i18n/index.js';
import { Glyph } from './glyphs.js';
import { Resaltado } from './Resaltado.js';
import { principioDe } from './VistaPreviaEnlace.js';

const ID_CAJA = 'notes-ir-a';
/** Cuántas filas de cada grupo: más ya no se leen de un vistazo. */
const MAX = 8;
/** La búsqueda del servicio, a partir de dos letras y tras una pausa al escribir. */
const MINIMO_REMOTO = 2;
const ESPERA = 200;

export interface OpcionIrA {
  tipo: 'cuaderno' | 'nota';
  id: string;
  notebookId: string;
  titulo: string;
  detalle: string;
}

/**
 * Las filas de «Ir a» para una consulta: primero los cuadernos, luego las notas
 * del cuaderno abierto y al final las que trae la búsqueda, sin repetir.
 */
export function opcionesIrA(
  q: string,
  cuadernos: Notebook[],
  notas: Note[],
  remotas: NoteHit[],
  sinTitulo: string,
): OpcionIrA[] {
  const nombreDe = (n: Note) => n.title || principioDe(n.content ?? '', 60) || sinTitulo;
  const deCuaderno = new Map(cuadernos.map((c) => [c.id, c.title]));
  const c = cuadernos
    .filter((x) => contiene(x.title, q))
    .slice(0, MAX)
    .map<OpcionIrA>((x) => ({
      tipo: 'cuaderno',
      id: x.id,
      notebookId: x.id,
      titulo: x.title,
      detalle: '',
    }));
  const vistas = new Set<string>();
  const n = notas
    .filter((x) => !x.content_cipher && contiene(nombreDe(x), q))
    .slice(0, MAX)
    .map<OpcionIrA>((x) => {
      vistas.add(x.id);
      return {
        tipo: 'nota',
        id: x.id,
        notebookId: x.notebook_id,
        titulo: nombreDe(x),
        detalle: deCuaderno.get(x.notebook_id) ?? '',
      };
    });
  const r = remotas
    .filter((h) => !vistas.has(h.id))
    .slice(0, MAX)
    .map<OpcionIrA>((h) => ({
      tipo: 'nota',
      id: h.id,
      notebookId: h.notebook_id,
      titulo: h.title || sinTitulo,
      detalle: h.notebook,
    }));
  return [...c, ...n, ...r];
}

export function AbrirRapido({
  open,
  t,
  cuadernos,
  notas,
  buscar,
  onClose,
  onPick,
}: {
  open: boolean;
  t: T;
  cuadernos: Notebook[];
  /** Las notas del cuaderno abierto, que salen sin esperar a la búsqueda. */
  notas: Note[];
  buscar: (q: string) => Promise<NoteHit[]>;
  onClose: () => void;
  onPick: (o: OpcionIrA) => void;
}): JSX.Element {
  const [q, setQ] = useState('');
  const [idx, setIdx] = useState(0);
  const [remotas, setRemotas] = useState<NoteHit[]>([]);

  useEffect(() => {
    if (!open) return;
    setQ('');
    setIdx(0);
    setRemotas([]);
    // El foco a la caja en cuanto el modal esté pintado (lo mismo que el
    // conmutador de Rooms: `autofocus` no vale tras el primer abrir).
    const id = window.setTimeout(() => document.getElementById(ID_CAJA)?.focus(), 30);
    return () => window.clearTimeout(id);
  }, [open]);

  useEffect(() => {
    setIdx(0);
    const consulta = q.trim();
    if (consulta.length < MINIMO_REMOTO) {
      setRemotas([]);
      return;
    }
    // Lo que llega de una consulta ya pasada no se pinta.
    let vigente = true;
    const id = window.setTimeout(() => {
      buscar(consulta)
        .then((r) => vigente && setRemotas(r))
        .catch(() => vigente && setRemotas([]));
    }, ESPERA);
    return () => {
      vigente = false;
      window.clearTimeout(id);
    };
  }, [q, buscar]);

  const filas = useMemo(
    () => opcionesIrA(q.trim(), cuadernos, notas, remotas, t('note.new')),
    [q, cuadernos, notas, remotas, t],
  );
  const actual = filas[idx];
  const elegir = (o: OpcionIrA) => {
    onPick(o);
    onClose();
  };

  return (
    <Modal open={open} onClose={onClose} title={t('quickOpen.title')} size="sm">
      <div class="flex flex-col gap-2">
        <Input
          id={ID_CAJA}
          type="search"
          value={q}
          placeholder={t('quickOpen.placeholder')}
          aria-label={t('quickOpen.placeholder')}
          role="combobox"
          aria-expanded={filas.length > 0}
          aria-controls="notes-ir-a-lista"
          aria-activedescendant={actual ? `notes-ir-a-${actual.tipo}-${actual.id}` : undefined}
          autocomplete="off"
          onInput={(e) => setQ(e.currentTarget.value)}
          onKeyDown={(e) => {
            if (e.key === 'ArrowDown') {
              e.preventDefault();
              setIdx((i) => Math.min(i + 1, filas.length - 1));
            } else if (e.key === 'ArrowUp') {
              e.preventDefault();
              setIdx((i) => Math.max(i - 1, 0));
            } else if (e.key === 'Enter' && actual) {
              e.preventDefault();
              elegir(actual);
            }
          }}
        />
        {filas.length === 0 ? (
          <p class="text-text-muted px-1 py-2 text-sm">{t('quickOpen.empty')}</p>
        ) : (
          <ul
            id="notes-ir-a-lista"
            role="listbox"
            aria-label={t('quickOpen.title')}
            class="border-border max-h-[50dvh] overflow-y-auto rounded-md border p-1"
          >
            {filas.map((o, i) => {
              const grupo = i === 0 || filas[i - 1]?.tipo !== o.tipo;
              return (
                <li
                  key={`${o.tipo}-${o.id}`}
                  id={`notes-ir-a-${o.tipo}-${o.id}`}
                  role="option"
                  aria-selected={i === idx}
                >
                  {grupo ? (
                    <span class="sytue-meta block px-2 pt-1.5 pb-0.5" aria-hidden="true">
                      {t(o.tipo === 'cuaderno' ? 'quickOpen.notebooks' : 'quickOpen.notes')}
                    </span>
                  ) : null}
                  <button
                    type="button"
                    tabIndex={-1}
                    onMouseEnter={() => setIdx(i)}
                    onClick={() => elegir(o)}
                    class={cx(
                      'text-text flex w-full items-center gap-2.5 rounded-md px-2 py-1.5 text-left text-sm pointer-coarse:min-h-[max(44px,2.75rem)]',
                      i === idx ? 'bg-selected' : 'hover:bg-bg-subtle',
                    )}
                  >
                    <Glyph
                      name={o.tipo === 'cuaderno' ? 'notebook' : 'note'}
                      size={15}
                      class="text-text-muted shrink-0"
                    />
                    <span class="min-w-0 flex-1 truncate">
                      <Resaltado texto={o.titulo} consulta={q} />
                    </span>
                    {o.detalle ? (
                      <span class="sytue-meta max-w-[40%] shrink-0 truncate">{o.detalle}</span>
                    ) : null}
                  </button>
                </li>
              );
            })}
          </ul>
        )}
        <p class="sytue-meta px-1">{t('quickOpen.hint')}</p>
      </div>
    </Modal>
  );
}
