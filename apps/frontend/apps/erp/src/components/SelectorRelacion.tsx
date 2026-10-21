/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Elegir el registro al que apunta una relación.
 *
 * ES LA PIEZA QUE FALTABA PARA QUE EL PRODUCTO SE PUEDA USAR. Hasta ahora el
 * campo era una caja de texto donde había que PEGAR un identificador, así que
 * crear una factura exigía ir a la lista de contactos, copiar un UUID y volver.
 * No es una carencia de acabado: es que el flujo principal del ERP no se podía
 * completar desde la interfaz.
 *
 * Busca contra `/api/{modelo}?q=`, que es la misma consulta que sirve las listas
 * —índice trigram sobre el nombre legible—, así que no hace falta traerse el
 * catálogo entero de clientes al navegador para poder elegir uno.
 */
import { useCallback, useContext, useEffect, useRef, useState } from 'preact/hooks';
import type { JSX } from 'preact';
import { Spinner, useMobileBack } from '@sytue/ui';
import { api, consulta } from '../lib/api.js';
import type { Pagina, Registro } from './tipos.js';
import { nombreDe } from './formato.js';
import { AltasContext } from './AltasContext.js';
import { Button } from '@sytue/ui';
import type { T } from '../i18n/index.js';

export interface SelectorRelacionProps {
  /** Modelo al que apunta la relación (viene del catálogo). */
  modelo: string;
  /** Identificador elegido, o cadena vacía. */
  valor: string;
  /** Nombre legible del elegido, si se conoce. */
  etiqueta: string;
  id: string;
  requerido?: boolean;
  onCambio: (valor: string, etiqueta: string) => void;
  t: T;
}

export function SelectorRelacion({
  modelo,
  valor,
  etiqueta,
  id,
  requerido,
  onCambio,
  t,
}: SelectorRelacionProps): JSX.Element {
  const altas = useContext(AltasContext);
  const [abierto, setAbierto] = useState(false);
  const [q, setQ] = useState('');
  const [opciones, setOpciones] = useState<Registro[]>([]);
  const [cargando, setCargando] = useState(false);
  const [cursor, setCursor] = useState(0);
  const entrada = useRef<HTMLInputElement | null>(null);
  const caja = useRef<HTMLDivElement | null>(null);

  useMobileBack(abierto, () => setAbierto(false));

  // Buscar, con espera: una consulta por pulsación es una consulta por pulsación
  // contra la base de la organización.
  useEffect(() => {
    if (!abierto) return;
    let vivo = true;
    const temporizador = setTimeout(
      () => {
        setCargando(true);
        api<Pagina>(`${modelo}${consulta({ q, limite: 8 })}`)
          .then((p) => {
            if (!vivo) return;
            setOpciones(p.registros ?? []);
            setCursor(0);
          })
          .catch(() => vivo && setOpciones([]))
          .finally(() => vivo && setCargando(false));
      },
      q ? 180 : 0,
    );
    return () => {
      vivo = false;
      clearTimeout(temporizador);
    };
  }, [abierto, modelo, q]);

  useEffect(() => {
    if (abierto) entrada.current?.focus();
  }, [abierto]);

  // Cerrar al salir del control: sin esto el desplegable se queda abierto encima
  // del resto del formulario.
  useEffect(() => {
    if (!abierto) return;
    const alTocarFuera = (ev: MouseEvent): void => {
      if (caja.current && !caja.current.contains(ev.target as Node)) setAbierto(false);
    };
    document.addEventListener('mousedown', alTocarFuera);
    return () => document.removeEventListener('mousedown', alTocarFuera);
  }, [abierto]);

  const elegir = useCallback(
    (r: Registro) => {
      onCambio(r.id, nombreDe(r));
      setAbierto(false);
      setQ('');
    },
    [onCambio],
  );

  const rotulo = etiqueta || (valor ? valor.slice(0, 8) : '');

  return (
    <div ref={caja} class="relative">
      <button
        type="button"
        id={id}
        onClick={() => setAbierto((v) => !v)}
        aria-haspopup="listbox"
        aria-expanded={abierto}
        class="sytue-focus-ring border-border bg-bg text-text flex w-full items-center justify-between gap-2 rounded-md border px-3 py-2 text-left text-sm"
      >
        <span class={rotulo ? 'truncate' : 'text-text-subtle truncate'}>
          {rotulo || t('relation.choose')}
        </span>
        {valor ? (
          // `<span role="button">` y no un `<button>`: uno dentro de otro no es
          // HTML válido y el navegador lo desanida, sacándolo del control.
          <span
            role="button"
            tabIndex={0}
            aria-label={t('relation.clear')}
            onClick={(e) => {
              e.stopPropagation();
              onCambio('', '');
            }}
            onKeyDown={(e) => {
              if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault();
                e.stopPropagation();
                onCambio('', '');
              }
            }}
            class="sytue-focus-ring text-text-subtle hover:text-text shrink-0 rounded px-1"
          >
            ×
          </span>
        ) : null}
      </button>

      {/* El valor de verdad, para que el formulario sepa si falta un obligatorio
          sin que haya que reimplementar la validación del navegador. */}
      <input type="hidden" value={valor} required={requerido} />

      {abierto ? (
        <div class="sytue-menu-surface border-border rounded-card absolute z-20 mt-1 w-full overflow-hidden border shadow-lg">
          <input
            ref={entrada}
            type="text"
            value={q}
            placeholder={t('relation.search')}
            aria-label={t('relation.search')}
            onInput={(e) => setQ((e.target as HTMLInputElement).value)}
            onKeyDown={(e) => {
              if (e.key === 'ArrowDown') {
                e.preventDefault();
                setCursor((c) => Math.min(c + 1, opciones.length - 1));
              } else if (e.key === 'ArrowUp') {
                e.preventDefault();
                setCursor((c) => Math.max(c - 1, 0));
              } else if (e.key === 'Enter') {
                e.preventDefault();
                const elegida = opciones[cursor];
                if (elegida) elegir(elegida);
              } else if (e.key === 'Escape') {
                e.preventDefault();
                setAbierto(false);
              }
            }}
            class="border-border text-text w-full border-b bg-transparent px-3 py-2 text-sm outline-none"
          />
          <ul role="listbox" class="max-h-56 overflow-auto py-1">
            {cargando && opciones.length === 0 ? (
              <li class="flex justify-center py-4">
                <Spinner size={16} />
              </li>
            ) : opciones.length === 0 ? (
              <li class="text-text-subtle px-3 py-3 text-sm">{t('relation.empty')}</li>
            ) : (
              opciones.map((r, i) => (
                <li key={r.id} role="option" aria-selected={i === cursor}>
                  <button
                    type="button"
                    onClick={() => elegir(r)}
                    onMouseEnter={() => setCursor(i)}
                    class={
                      i === cursor
                        ? 'bg-selected text-text w-full truncate px-3 py-2 text-left text-sm'
                        : 'text-text-muted w-full truncate px-3 py-2 text-left text-sm'
                    }
                  >
                    {nombreDe(r)}
                  </button>
                </li>
              ))
            )}
          </ul>
        </div>
      ) : null}
      {altas?.puedeCrear(modelo) ? (
        <Button
          type="button"
          variant="ghost"
          size="sm"
          onClick={() => {
            setAbierto(false);
            altas.crear(modelo, (r) => onCambio(r.id, nombreDe(r)));
          }}
        >
          {t('action.new')}
        </Button>
      ) : null}
    </div>
  );
}
