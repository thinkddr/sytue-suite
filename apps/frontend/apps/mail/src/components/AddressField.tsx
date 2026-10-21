/*
 * AddressField — destinatarios como FICHAS. Lo escrito hasta la última coma son
 * fichas (el nombre, o el correo si no hay nombre) y lo que hay después es el
 * texto que se está escribiendo, con el autocompletado de Contacts de siempre.
 * El contrato con el redactor no cambia: `value` sigue siendo la cadena separada
 * por comas que espera el envío («Nombre <correo>, otro@dominio.eu, »).
 *
 * Coma, punto y coma, Intro (sin sugerencia elegida) y salir del campo cierran
 * la ficha; Retroceso con el campo vacío reabre la última para corregirla, como
 * en Gmail y Outlook. Una ficha sin un correo con forma se marca ANTES de
 * enviar, no después.
 */
import type { JSX } from 'preact';
import { useEffect, useRef, useState } from 'preact/hooks';
import { Icon, Input, cx, useMobileBack } from '@sytue/ui';
import { searchContacts, type ContactSuggestion } from '../client/mailApi.js';

interface Props {
  id: string;
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  invalid?: boolean;
  required?: boolean;
  describedBy?: string;
  /** Sin caja propia: la fila de cabecera del redactor ya pone el pelo. */
  desnudo?: boolean;
  /** Nombre accesible del «×» de cada ficha. */
  etiquetaQuitar: (nombre: string) => string;
  /** Título de una ficha cuyo correo no tiene forma de correo. */
  avisoNoValida: string;
}

const SEPARADOR = /[,;]/;

/** Divide el valor en fichas (todo hasta el último separador) y el texto en curso. */
function partir(value: string): { fichas: string[]; actual: string } {
  const partes = value.split(SEPARADOR);
  const actual = partes.pop() ?? '';
  return {
    fichas: partes.map((p) => p.trim()).filter(Boolean),
    actual: actual.replace(/^\s+/, ''),
  };
}

function unir(fichas: string[], actual: string): string {
  return fichas.length > 0 ? `${fichas.join(', ')}, ${actual}` : actual;
}

/** «Nombre <correo>» → nombre y correo; un correo suelto → solo correo. */
function leer(ficha: string): { nombre: string; correo: string } {
  const m = /^(.*?)\s*<([^>]+)>\s*$/.exec(ficha);
  if (m) return { nombre: (m[1] ?? '').replace(/^"|"$/g, '').trim(), correo: (m[2] ?? '').trim() };
  return { nombre: '', correo: ficha.trim() };
}

const tieneFormaDeCorreo = (s: string): boolean => /^[^\s@<>]+@[^\s@<>]+\.[^\s@<>]+$/.test(s);

export function AddressField({
  id,
  value,
  onChange,
  placeholder,
  invalid,
  required,
  describedBy,
  desnudo,
  etiquetaQuitar,
  avisoNoValida,
}: Props): JSX.Element {
  const [suggestions, setSuggestions] = useState<ContactSuggestion[]>([]);
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(-1);
  const [focused, setFocused] = useState(false);
  const timerRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const seq = useRef(0);

  useMobileBack(open && suggestions.length > 0, () => setOpen(false));

  const { fichas, actual } = partir(value);
  const token = actual.trim();

  useEffect(() => {
    if (!focused || token.length < 2) {
      setOpen(false);
      setSuggestions([]);
      return;
    }
    clearTimeout(timerRef.current);
    timerRef.current = setTimeout(() => {
      const mine = ++seq.current;
      void searchContacts(token).then((list) => {
        if (mine !== seq.current) return; // descarta respuestas obsoletas
        setSuggestions(list);
        setActive(-1);
        setOpen(list.length > 0);
      });
    }, 200);
    return () => clearTimeout(timerRef.current);
  }, [token, focused]);

  /** Cierra una ficha con ese texto y deja el campo vacío para la siguiente. */
  function fijar(texto: string): void {
    const limpio = texto.trim();
    onChange(unir(limpio ? [...fichas, limpio] : fichas, ''));
    setOpen(false);
    setSuggestions([]);
    setActive(-1);
  }

  function choose(s: ContactSuggestion): void {
    fijar(s.name ? `${s.name} <${s.email}>` : s.email);
  }

  function quitar(i: number): void {
    onChange(
      unir(
        fichas.filter((_, k) => k !== i),
        actual,
      ),
    );
  }

  function onKeyDown(e: KeyboardEvent): void {
    if (open && suggestions.length > 0) {
      if (e.key === 'ArrowDown') {
        e.preventDefault();
        setActive((a) => Math.min(a + 1, suggestions.length - 1));
        return;
      }
      if (e.key === 'ArrowUp') {
        e.preventDefault();
        setActive((a) => Math.max(a - 1, 0));
        return;
      }
      if (e.key === 'Enter' && active >= 0) {
        e.preventDefault();
        const s = suggestions[active];
        if (s) choose(s);
        return;
      }
      if (e.key === 'Escape') {
        setOpen(false);
        return;
      }
    }
    if ((e.key === ',' || e.key === ';' || e.key === 'Enter') && token) {
      e.preventDefault();
      fijar(token);
      return;
    }
    if (e.key === 'Backspace' && !actual && fichas.length > 0) {
      // Reabre la última ficha para corregirla, en vez de borrarla de golpe.
      e.preventDefault();
      onChange(unir(fichas.slice(0, -1), fichas[fichas.length - 1] ?? ''));
    }
  }

  const listId = `${id}-listbox`;
  return (
    <div class="relative">
      {/* La fila entera es la zona de escribir: pulsar entre fichas enfoca el campo. */}
      {/* oxlint-disable-next-line jsx-a11y/no-static-element-interactions, jsx-a11y/click-events-have-key-events */}
      <div
        class={cx(
          'flex min-h-9 flex-wrap items-center gap-1',
          !desnudo && 'border-border bg-surface rounded-md border px-2',
          !desnudo && invalid && 'border-danger',
        )}
        onClick={(e) => {
          if (e.target === e.currentTarget) document.getElementById(id)?.focus();
        }}
      >
        {fichas.map((ficha, i) => {
          const { nombre, correo } = leer(ficha);
          const valida = tieneFormaDeCorreo(correo);
          return (
            <span
              key={`${ficha}-${i}`}
              class={cx(
                'rounded-pill inline-flex h-7 max-w-[18rem] items-center gap-1 border pr-1 pl-2.5 text-sm',
                valida
                  ? 'border-border bg-bg-subtle text-text'
                  : 'border-danger bg-danger-bg text-danger-text',
              )}
              title={valida ? correo : avisoNoValida}
            >
              <span class="truncate">{nombre || correo}</span>
              <button
                type="button"
                aria-label={etiquetaQuitar(nombre || correo)}
                // mousedown sin blur: si hay texto a medias, al pulsar el «×» el campo
                // lo cerraría como ficha antes de quitar la otra, y se cruzarían.
                onMouseDown={(e) => e.preventDefault()}
                onClick={() => quitar(i)}
                class="hover:bg-border sytue-focus-ring rounded-control grid h-5 w-5 shrink-0 place-items-center pointer-coarse:h-7 pointer-coarse:w-7"
              >
                <Icon name="close" size={12} />
              </button>
            </span>
          );
        })}
        <Input
          id={id}
          type="text"
          role="combobox"
          aria-expanded={open}
          aria-controls={listId}
          aria-autocomplete="list"
          aria-activedescendant={open && active >= 0 ? `${id}-opt-${active}` : undefined}
          autocomplete="off"
          value={actual}
          {...(required && fichas.length === 0 ? { required: true } : {})}
          {...(invalid ? { invalid: true } : {})}
          {...(fichas.length === 0 && placeholder ? { placeholder } : {})}
          // `!` en cada una porque `Input` compone su clase base ANTES que la de
          // fuera y `cx` no resuelve conflictos. Se apaga la CAJA, no el aro: el
          // foco sigue en el control, 2px ajustados, como en el resto de la suite.
          class="!h-7 min-w-[10rem] flex-1 !border-0 !bg-transparent !px-0 !shadow-none"
          aria-describedby={describedBy}
          onInput={(e) => onChange(unir(fichas, e.currentTarget.value))}
          onFocus={() => setFocused(true)}
          onBlur={() => {
            setFocused(false);
            setOpen(false);
            if (token) fijar(token);
          }}
          onKeyDown={onKeyDown}
        />
      </div>
      {open && suggestions.length > 0 ? (
        <ul
          id={listId}
          role="listbox"
          // Elevación nivel 2 (popover), como la del Dropdown de @sytue/ui.
          class="sytue-menu-surface border-border absolute mt-1 max-h-60 w-full overflow-y-auto rounded-lg border py-1 shadow-[var(--shadow-md)]"
          style={{ zIndex: 'var(--z-dropdown)' }}
        >
          {suggestions.map((s, i) => (
            <li
              key={s.email}
              id={`${id}-opt-${i}`}
              role="option"
              aria-selected={i === active}
              // 44px solo con puntero grueso (WCAG 2.5.8); `bg-selected` para la
              // activa porque conmuta con el tema (en oscuro `bg-subtle` es más
              // oscuro que el popover y la fila señalada parecía un hueco).
              class={`flex cursor-pointer flex-col justify-center px-3 py-1.5 text-sm pointer-coarse:min-h-[max(44px,2.75rem)] ${
                i === active ? 'bg-selected' : ''
              }`}
              // mousedown en vez de click: evita el blur del input antes de elegir.
              onMouseDown={(e: MouseEvent) => {
                e.preventDefault();
                choose(s);
              }}
              onMouseEnter={() => setActive(i)}
            >
              {s.name ? (
                <>
                  <span class="text-text truncate">{s.name}</span>
                  <span class="text-text-subtle truncate">{s.email}</span>
                </>
              ) : (
                <span class="text-text truncate">{s.email}</span>
              )}
            </li>
          ))}
        </ul>
      ) : null}
    </div>
  );
}
