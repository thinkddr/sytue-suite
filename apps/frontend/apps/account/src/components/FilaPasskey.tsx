/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Una passkey de la lista de Seguridad: su nombre, CUÁNDO se añadió y cuándo se usó
 * por última vez, y las dos acciones —cambiarle el nombre y quitarla—.
 *
 * Las fechas ya llegaban del servicio (`created_at`, `last_used`) y no se pintaban:
 * con tres llaves llamadas «Passkey», la única forma de saber cuál quitar el día que
 * se pierde un portátil era adivinar. Ahora se ve cuál no se usa desde hace meses y
 * se le puede poner un nombre que diga de qué aparato es.
 *
 * El cambio de nombre es en línea: el nombre se vuelve un campo con «Guardar» y
 * «Cancelar», Intro guarda y Escape cancela, y el foco vuelve al botón que lo abrió.
 * Si el servicio lo rechaza se dice aquí, junto a la fila, y el campo se queda con lo
 * escrito.
 */
import type { JSX } from 'preact';
import { useEffect, useRef, useState } from 'preact/hooks';
import { Button, Input } from '@sytue/ui';
import { formatDate, formatTimeAgo, type Locale } from '@sytue/i18n';
import type { T } from '../i18n/index.js';

export interface Passkey {
  id: string;
  name: string;
  created_at?: string | null;
  last_used?: string | null;
}

/** «Añadida el 3 oct 2026 · Último uso hace 2 horas», o lo que se sepa de las dos. */
export function detallePasskey(t: T, locale: Locale, k: Passkey): string {
  const fecha = (iso?: string | null): Date | null => {
    if (!iso) return null;
    const d = new Date(iso);
    return Number.isNaN(d.getTime()) ? null : d;
  };
  const alta = fecha(k.created_at);
  const uso = fecha(k.last_used);
  return [
    alta ? t('security.passkeys.added_on', { date: formatDate(locale, alta) }) : '',
    uso
      ? t('security.passkeys.last_used', { time: formatTimeAgo(locale, uso) })
      : t('security.passkeys.never_used'),
  ]
    .filter(Boolean)
    .join(' · ');
}

export function FilaPasskey({
  t,
  locale,
  llave,
  onQuitar,
  onRenombrada,
}: {
  t: T;
  locale: Locale;
  llave: Passkey;
  onQuitar: (k: Passkey) => void;
  /** Tras guardar el nombre: la tarjeta relee la lista. */
  onRenombrada: () => void;
}): JSX.Element {
  const nombre = llave.name || t('security.passkeys.unnamed');
  const [editando, setEditando] = useState(false);
  const [borrador, setBorrador] = useState(llave.name);
  const [guardando, setGuardando] = useState(false);
  const [fallo, setFallo] = useState(false);
  const boton = useRef<HTMLDivElement>(null);
  const campoId = `pk-renombrar-${llave.id}`;

  // Al abrir, el foco al campo; al cerrar, de vuelta al botón que lo abrió. Por id y
  // por contenedor, no por `ref` del control: `Input` y `Button` no la reenvían. El
  // «devolver» es estado y no un ref que se muta en el efecto: el compilador de React
  // se salta un componente que lo hace.
  const [devolverFoco, setDevolverFoco] = useState(false);
  useEffect(() => {
    if (editando) {
      const campo = document.getElementById(campoId) as HTMLInputElement | null;
      campo?.focus();
      campo?.select();
    } else if (devolverFoco) {
      boton.current?.querySelector('button')?.focus();
      setDevolverFoco(false);
    }
  }, [editando, devolverFoco, campoId]);

  function cerrar(): void {
    setEditando(false);
    setDevolverFoco(true);
  }

  function abrir(): void {
    setBorrador(llave.name);
    setFallo(false);
    setEditando(true);
  }

  async function guardar(e: Event): Promise<void> {
    e.preventDefault();
    const limpio = borrador.trim();
    if (!limpio || limpio === llave.name) {
      cerrar();
      return;
    }
    setGuardando(true);
    setFallo(false);
    try {
      const res = await fetch(`/api/passkeys/${encodeURIComponent(llave.id)}`, {
        method: 'PATCH',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ name: limpio }),
      });
      if (!res.ok) throw new Error(String(res.status));
      cerrar();
      onRenombrada();
    } catch {
      setFallo(true);
    } finally {
      setGuardando(false);
    }
  }

  if (editando) {
    return (
      <li class="flex flex-col gap-2">
        <form class="flex flex-wrap items-center gap-2" onSubmit={(e) => void guardar(e)}>
          <label class="sr-only" for={campoId}>
            {t('security.passkeys.rename_label')}
          </label>
          <div class="min-w-0 flex-1">
            <Input
              id={campoId}
              value={borrador}
              maxLength={60}
              onInput={(e: Event) => setBorrador((e.target as HTMLInputElement).value)}
              onKeyDown={(e: KeyboardEvent) => {
                if (e.key === 'Escape') {
                  e.preventDefault();
                  cerrar();
                }
              }}
            />
          </div>
          <Button type="submit" size="sm" disabled={guardando || !borrador.trim()}>
            {t('common.save')}
          </Button>
          <Button type="button" size="sm" variant="ghost" onClick={cerrar}>
            {t('common.cancel')}
          </Button>
        </form>
        {fallo && (
          <p role="alert" class="text-danger-text text-xs">
            {t('security.passkeys.rename_failed')}
          </p>
        )}
      </li>
    );
  }

  return (
    <li class="flex items-center justify-between gap-3">
      <div class="min-w-0">
        <p class="truncate text-sm">{nombre}</p>
        <p class="text-text-muted text-xs">{detallePasskey(t, locale, llave)}</p>
      </div>
      <div class="flex shrink-0 gap-1" ref={boton}>
        {/* El texto visible es el verbo; el nombre accesible dice CUÁL. */}
        <Button
          variant="ghost"
          aria-label={t('security.passkeys.rename_named', { name: nombre })}
          onClick={abrir}
        >
          {t('security.passkeys.rename')}
        </Button>
        <Button
          variant="ghost"
          aria-label={t('security.passkeys.remove_named', { name: nombre })}
          onClick={() => onQuitar(llave)}
        >
          {t('security.passkeys.remove')}
        </Button>
      </div>
    </li>
  );
}

export default FilaPasskey;
