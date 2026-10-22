/**
 * PaletaDeComandos — buscar y saltar (⌘K): un campo y la lista de lo que casa.
 *
 * La tenían dos apps, cada una la suya: Sytue Cloud con el patrón ARIA completo
 * (combobox + listbox con `aria-activedescendant`, Tab dentro, foco de vuelta) y el ERP
 * con un velo `bg-black/30`, `shadow-2xl`, sin foco atrapado y sin Escape propio. El
 * mismo componente con dos cuerpos que divergían; aquí queda uno, y cada app solo dice
 * QUÉ comandos hay y qué pasa al elegir uno.
 *
 * Es la misma capa que un diálogo (`Modal`): el mismo velo, la superficie del panel, el
 * radio de tarjeta de la forma y la sombra `xl`. No usa `Modal` porque su cabecera es el
 * CAMPO —no un título con aspa— y el foco no sale de él: las flechas mueven la opción
 * activa y Tab no tiene adónde ir.
 *
 * - **Escritorio** (≥ 768): tarjeta de 560 alineada arriba (a 12 dvh), como la de Cloud.
 * - **Teléfono**: la HOJA de los diálogos a toda la altura disponible —con el aire de
 *   arriba y la zona segura de abajo—, apoyada en el viewport VISUAL para que el teclado
 *   no tape la lista. Lleva «Cancelar» al lado del campo, porque con el dedo no hay
 *   Escape, y quita los atajos de teclado del pie, que ahí no existen. Las opciones miden
 *   44 de alto con el dedo.
 */
import type { ComponentChildren, JSX } from 'preact';
import { Fragment } from 'preact';
import { createPortal } from 'preact/compat';
import { useEffect, useId, useLayoutEffect, useRef, useState } from 'preact/hooks';
import { useT } from '@sytue/i18n/preact';
import { cx } from '../lib/cx.js';
import { useMobileBack } from '../lib/useMobileBack.js';
import { Button } from './Button.js';
import { Icon, type IconName } from './Icon.js';

export interface OpcionDePaleta {
  id: string;
  /** Lo que se lee. */
  rotulo: string;
  /** Encabezado del grupo, ya traducido; las opciones llegan ya agrupadas. */
  grupo?: string;
  icono?: IconName;
  /** Contexto a la derecha (el slug, el módulo). */
  pista?: string;
}

export interface PaletaDeComandosProps<O extends OpcionDePaleta> {
  open: boolean;
  onClose: () => void;
  /** Nombre accesible del diálogo. */
  etiqueta: string;
  /** Nombre accesible del campo. */
  rotuloCampo: string;
  placeholder: string;
  /** Nombre accesible de la lista. */
  rotuloResultados: string;
  /** Lo que se lee cuando no casa nada, con la consulta ya puesta. */
  sinResultados: (consulta: string) => string;
  consulta: string;
  onConsulta: (consulta: string) => void;
  /** Las opciones que casan, en su orden (ya filtradas por quien llama). */
  opciones: O[];
  onElegir: (opcion: O) => void;
  /** El pie de escritorio (los atajos). En el teléfono no se pinta. */
  pie?: ComponentChildren;
}

export function PaletaDeComandos<O extends OpcionDePaleta>({
  open,
  onClose,
  etiqueta,
  rotuloCampo,
  placeholder,
  rotuloResultados,
  sinResultados,
  consulta,
  onConsulta,
  opciones,
  onElegir,
  pie,
}: PaletaDeComandosProps<O>): JSX.Element | null {
  const t = useT();
  const campo = useRef<HTMLInputElement>(null);
  const previoRef = useRef<HTMLElement | null>(null);
  const base = useId();
  const lista = `${base}-lista`;
  const [activa, setActiva] = useState(0);
  // El «atrás» del teléfono la cierra, como a un diálogo.
  useMobileBack(open, onClose, { nivel: 2 });

  // Al abrir: foco al campo, sin desplazar el fondo, y al cerrar, el foco donde estaba
  // (el mismo patrón que `Modal`). Síncrono: quien ya teclea no pierde letras.
  useLayoutEffect(() => {
    if (!open) return;
    previoRef.current = document.activeElement as HTMLElement | null;
    campo.current?.focus();
    const antes = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    return () => {
      document.body.style.overflow = antes;
      previoRef.current?.focus();
    };
  }, [open]);

  // Una consulta nueva vuelve a la primera opción.
  const previoOpenRef = useRef(open);
  const previoConsultaRef = useRef(consulta);
  useEffect(() => {
    if (previoOpenRef.current !== open || previoConsultaRef.current !== consulta) {
      previoOpenRef.current = open;
      previoConsultaRef.current = consulta;
      setActiva(0);
    }
  }, [consulta, open]);

  const i = opciones.length > 0 ? Math.min(Math.max(activa, 0), opciones.length - 1) : -1;
  const idActiva = i >= 0 ? `${base}-op-${i}` : undefined;

  // La opción activa, siempre a la vista al moverse con el teclado.
  useEffect(() => {
    if (idActiva) document.getElementById(idActiva)?.scrollIntoView({ block: 'nearest' });
  }, [idActiva]);

  if (!open || typeof document === 'undefined') return null;

  const mover = (d: number) =>
    opciones.length > 0 &&
    setActiva((((i + d) % opciones.length) + opciones.length) % opciones.length);

  const teclas = (e: KeyboardEvent): void => {
    switch (e.key) {
      case 'ArrowDown':
        e.preventDefault();
        mover(1);
        break;
      case 'ArrowUp':
        e.preventDefault();
        mover(-1);
        break;
      case 'Home':
        e.preventDefault();
        setActiva(0);
        break;
      case 'End':
        e.preventDefault();
        setActiva(opciones.length - 1);
        break;
      case 'Enter': {
        e.preventDefault();
        const o = opciones[i];
        if (o) onElegir(o);
        break;
      }
      case 'Escape':
        e.preventDefault();
        e.stopPropagation();
        onClose();
        break;
      // Solo el campo es enfocable: Tab se queda en el diálogo.
      case 'Tab':
        e.preventDefault();
        break;
    }
  };

  return createPortal(
    <div
      // Alto y origen = viewport VISUAL, como el `Modal`: con el teclado fuera, la hoja
      // acaba encima de él y la lista sigue a la vista.
      class={cx(
        'fixed inset-x-0 top-[var(--sytue-vvy,0px)] flex h-[var(--sytue-vvh,100dvh)]',
        'max-md:items-stretch max-md:pt-[max(2.5rem,calc(env(safe-area-inset-top)_+_1rem))]',
        'md:items-start md:justify-center md:p-4 md:pt-[12dvh]',
      )}
      style={{ zIndex: 'var(--z-modal)' }}
    >
      <div
        class="sytue-fade-in absolute inset-0 bg-[var(--sytue-overlay)]"
        onClick={onClose}
        aria-hidden="true"
      />
      <div
        role="dialog"
        aria-modal="true"
        aria-label={etiqueta}
        class={cx(
          'sytue-glass-surface sytue-modal-in sytue-hoja-movil relative flex w-full flex-col overflow-hidden',
          'border-border border bg-[var(--sytue-panel-surface,var(--sytue-surface))]',
          'md:rounded-card md:max-h-[70dvh] md:max-w-[560px] md:shadow-[var(--shadow-xl)]',
          'max-md:rounded-t-card max-md:rounded-b-none max-md:border-x-0 max-md:border-b-0',
        )}
      >
        <div class="border-border-subtle flex shrink-0 items-center gap-2.5 border-b px-4 max-md:pt-1">
          <Icon name="search" size={16} class="text-text-subtle shrink-0" />
          <input
            ref={campo}
            type="text"
            role="combobox"
            aria-expanded
            aria-controls={lista}
            aria-autocomplete="list"
            aria-label={rotuloCampo}
            {...(idActiva ? { 'aria-activedescendant': idActiva } : {})}
            value={consulta}
            placeholder={placeholder}
            spellcheck={false}
            autocomplete="off"
            class="sytue-campo-paleta text-text placeholder:text-text-subtle h-12 min-w-0 flex-1 bg-transparent text-base focus-visible:outline-none max-md:h-14"
            onInput={(e) => onConsulta(e.currentTarget.value)}
            onKeyDown={teclas}
          />
          {/* Con el dedo no hay Escape: la salida va donde está el pulgar al escribir. */}
          <Button variant="ghost" onClick={onClose} class="-mr-2 shrink-0 md:hidden">
            {t('ui.cancel')}
          </Button>
        </div>
        <ul
          id={lista}
          role="listbox"
          aria-label={rotuloResultados}
          class="min-h-0 flex-1 overflow-y-auto overscroll-contain p-1.5 max-md:pb-[max(0.375rem,env(safe-area-inset-bottom))]"
        >
          {opciones.length === 0 ? (
            <li role="presentation" class="text-text-muted px-3 py-6 text-center text-sm">
              {sinResultados(consulta.trim())}
            </li>
          ) : (
            opciones.map((o, n) => {
              const empieza =
                o.grupo !== undefined && (n === 0 || opciones[n - 1]?.grupo !== o.grupo);
              const es = n === i;
              return (
                <Fragment key={o.id}>
                  {empieza ? (
                    <li
                      role="presentation"
                      class="text-text-subtle sytue-overline px-2.5 pt-3 pb-1 first:pt-1"
                    >
                      {o.grupo}
                    </li>
                  ) : null}
                  {/* eslint-disable-next-line jsx-a11y/click-events-have-key-events -- patrón APG combobox: el teclado lo lleva el campo (activedescendant); la opción no es enfocable */}
                  <li
                    id={`${base}-op-${n}`}
                    role="option"
                    aria-selected={es}
                    onClick={() => onElegir(o)}
                    onMouseEnter={() => setActiva(n)}
                    class={cx(
                      'text-text flex cursor-pointer items-center gap-2.5 rounded-md px-2.5 py-2 text-base pointer-coarse:min-h-[max(44px,2.75rem)]',
                      es && 'bg-selected',
                    )}
                  >
                    {o.icono ? (
                      <Icon
                        name={o.icono}
                        size={16}
                        class={es ? 'text-primary-text' : 'text-text-subtle'}
                      />
                    ) : null}
                    <span class="min-w-0 flex-1 truncate">{o.rotulo}</span>
                    {o.pista ? (
                      <span class="sytue-meta max-w-[40%] shrink-0 truncate">{o.pista}</span>
                    ) : null}
                  </li>
                </Fragment>
              );
            })
          )}
        </ul>
        {pie ? (
          <div class="border-border-subtle sytue-meta flex shrink-0 items-center gap-3 border-t px-4 py-2 max-md:hidden">
            {pie}
          </div>
        ) : null}
      </div>
    </div>,
    document.body,
  );
}
