/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La hoja de atajos de la SUITE: «?» en todas las apps.
 *
 * Mail, Calendar y Rooms tenían cada una su hoja, con su estado y su propio oyente de
 * «?», y las demás no enseñaban nada, ni siquiera ⌘K, que es de toda la suite. La shell
 * está en todas las superficies, así que el «?» vive aquí: abre `ShortcutsModal` con los
 * atajos que la app declara (`useAtajosDeApp`) y, al final, el grupo de la suite.
 *
 * Lo que la app declara va a un almacén de MÓDULO y no a un contexto, porque la mitad
 * de las apps pintan `<SytueShell>` DENTRO de su propio componente (CalendarApp,
 * ContactsApp…): el hook se llama fuera del árbol de la shell, donde un contexto no
 * llega. Hay una shell por página, así que un almacén único basta.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import {
  ShortcutsModal,
  atajoDelSistema,
  esAtajoDeApp,
  esMac,
  type GrupoDeAtajos,
} from '@sytue/ui';
import { useT } from '@sytue/i18n/preact';

type Oyente = () => void;

let gruposDeLaApp: GrupoDeAtajos[] = [];
const alCambiar = new Set<Oyente>();
const alPedirAbrir = new Set<Oyente>();

function avisar(oyentes: Set<Oyente>): void {
  for (const f of oyentes) f();
}

/**
 * Los atajos de la app, para la hoja de «?» de la suite. Se llama en cada render con
 * los grupos de ahora (ya traducidos); al desmontar, la hoja se queda solo con la
 * suite. Si la app tiene su propio «?» (para un caso concreto), basta con que haga
 * `preventDefault`: la app va primero.
 */
export function useAtajosDeApp(grupos: GrupoDeAtajos[]): void {
  // Se compara por contenido: las apps rehacen los grupos en cada render y avisar a
  // la shell en cada uno la volvería a pintar sin que cambie nada.
  const firma = JSON.stringify(grupos);
  useEffect(() => {
    gruposDeLaApp = grupos;
    avisar(alCambiar);
    return () => {
      if (gruposDeLaApp !== grupos) return;
      gruposDeLaApp = [];
      avisar(alCambiar);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps -- `firma` es el contenido de `grupos`
  }, [firma]);
}

/** Abre la hoja de atajos de la suite, desde un botón o un menú de la app. */
export function abrirHojaDeAtajos(): void {
  avisar(alPedirAbrir);
}

/**
 * La hoja compuesta: los grupos de la app y, al final, el de la suite. La fila de
 * «?» que traen las apps (sus tests la exigen, porque cuando la hoja era suya la
 * contaban) se quita: «?» es ahora de la suite y saldría dos veces.
 */
export function componerHoja(app: GrupoDeAtajos[], suite: GrupoDeAtajos): GrupoDeAtajos[] {
  const propios = app
    .map((g) => ({
      ...g,
      atajos: g.atajos.filter((a) => !(a.teclas.length === 1 && a.teclas[0] === '?')),
    }))
    .filter((g) => g.atajos.length > 0);
  return [...propios, suite];
}

/** El oyente de «?» y la hoja. Lo monta `SytueShell`, dentro de su `I18nProvider`. */
export function HojaDeAtajosDeLaSuite(): JSX.Element {
  const t = useT();
  const [abierta, setAbierta] = useState(false);
  const [app, setApp] = useState<GrupoDeAtajos[]>(gruposDeLaApp);

  useEffect(() => {
    const cambiar = (): void => setApp(gruposDeLaApp);
    const abrir = (): void => setAbierta(true);
    alCambiar.add(cambiar);
    alPedirAbrir.add(abrir);
    cambiar();
    return () => {
      alCambiar.delete(cambiar);
      alPedirAbrir.delete(abrir);
    };
  }, []);

  useEffect(() => {
    const alPulsar = (e: KeyboardEvent): void => {
      // Shift incluido: en un teclado español «?» lo pide, y no es un modificador de
      // sistema. Dentro de un campo, «?» es texto; con un diálogo, es de la capa.
      if (e.key !== '?' || !esAtajoDeApp(e)) return;
      // La app va primero, como con ⌘K en GlobalSearch: la que lo atiende hace
      // `preventDefault`. Se decide en la siguiente vuelta porque los oyentes de la
      // app que cuelgan de `window` pueden ir DESPUÉS de este (sus efectos corren tras
      // los de la shell que pintan dentro), y para entonces su diálogo, si abrió uno,
      // ya está en el DOM.
      setTimeout(() => {
        if (e.defaultPrevented || document.querySelector('[role="dialog"], dialog[open]')) return;
        setAbierta(true);
      }, 0);
    };
    window.addEventListener('keydown', alPulsar);
    return () => window.removeEventListener('keydown', alPulsar);
  }, []);

  const suite: GrupoDeAtajos = {
    titulo: t('shell.shortcuts.group'),
    atajos: [
      // Se escribe al abrir, en el cliente: el servidor no sabe si hay un Mac delante.
      {
        teclas: [abierta ? atajoDelSistema('Ctrl+K') : ''],
        descripcion: t('shell.shortcuts.search'),
      },
      { teclas: ['?'], descripcion: t('shell.shortcuts.help') },
      { teclas: ['Esc'], descripcion: t('shell.shortcuts.close') },
      // En el navegador, Mayús+F10 es el clic derecho del teclado: abre el menú contextual
      // de lo que tiene el foco (el de la app si lo tiene, como en Drive o Calcus). En las
      // apps nativas solo lo atiende Calcus, y por eso allí va en su hoja y no en esta.
      {
        teclas: [abierta ? (esMac() ? '⇧F10' : `${t('shell.shortcuts.key.shift')}+F10`) : ''],
        descripcion: t('shell.shortcuts.contextMenu'),
      },
    ],
  };

  return (
    <ShortcutsModal
      open={abierta}
      title={t('shell.shortcuts.title')}
      groups={componerHoja(app, suite)}
      onClose={() => setAbierta(false)}
    />
  );
}
