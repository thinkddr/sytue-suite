/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
/**
 * MailSettings — la página de AJUSTES de Sytue Mail (design.md §8.4).
 *
 * Antes eran ocho entradas sueltas en una sección plegada del lateral, cada una
 * abriendo su diálogo sin decir antes qué cambiaba: se leían como carpetas y
 * «Certificado de firma del correo» no explicaba para qué servía hasta pulsarlo.
 * Aquí van agrupadas por TEMA en `SettingsShell` + `SettingsSection`, cada una con
 * una frase llana de qué hace y, cuando se sabe sin abrirla, su ESTADO (la firma
 * está puesta o no; la autorespuesta responde ahora, está programada o apagada).
 *
 * Los editores siguen siendo los diálogos de siempre (firma, reglas, alias…): ya
 * están guiados con `FieldHint`/`InlineHelp`, y «Reglas» también se abre desde un
 * mensaje («Crear regla»). Lo que cambia es la puerta: una página que se puede
 * recorrer y que dice qué hay detrás de cada botón.
 *
 * Las claves de `AjusteMail` son las mismas que las de Fyne (`menu.go`), para que
 * las dos superficies hablen del mismo ajuste con el mismo nombre.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { Badge, Button, Icon, Select, SettingsSection, SettingsShell } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import {
  getVacation,
  AUTO_ADVANCE_OPTIONS,
  SWIPE_OPTIONS,
  UNDO_SEND_OPTIONS,
  type AutoAdvance,
  type ReadingPrefs,
  type SwipeAction,
  type UndoSendSeconds,
  type Vacation,
} from '../client/mailApi.js';
import type { MailMessageKey, T } from '../i18n/index.js';
import { estadoVacaciones, type Status } from './VacationModal.js';

export type AjusteMail =
  | 'signature'
  | 'templates'
  | 'undo-send'
  | 'vacation'
  | 'aliases'
  | 'shared'
  | 'rules'
  | 'smime-anchor'
  | 'smime-identity'
  | 'mail-clients';

/**
 * Filas que se ajustan en la propia página (un selector, una lista), sin diálogo
 * que abrir: no pasan por `onOpen`.
 */
type AjusteEnLinea = 'auto-advance' | 'swipe-right' | 'swipe-left' | 'trusted-senders';

/** El rótulo de cada opción de auto-avance y de gesto. */
const ROTULO_AVANCE = {
  older: 'settings.autoAdvanceOlder',
  newer: 'settings.autoAdvanceNewer',
  list: 'settings.autoAdvanceList',
} as const satisfies Record<AutoAdvance, MailMessageKey>;
const ROTULO_GESTO = {
  archive: 'settings.swipeArchive',
  trash: 'settings.swipeTrash',
  read: 'settings.swipeRead',
  none: 'settings.swipeNone',
} as const satisfies Record<SwipeAction, MailMessageKey>;

interface Props {
  t: T;
  locale: Locale;
  /** La firma guardada: basta para decir si está puesta sin abrir el editor. */
  signature: string;
  /**
   * Cambia cada vez que se cierra un editor: la página vuelve a leer lo que enseña
   * (la autorespuesta), y no se queda diciendo «apagada» justo después de encenderla.
   */
  version: number;
  /** La ventana de deshacer de la cuenta; null mientras no se ha leído. */
  undoSend: UndoSendSeconds | null;
  /** Se elige aquí mismo, sin diálogo: es un valor de cuatro, no un formulario. */
  onUndoSendChange: (seconds: UndoSendSeconds) => void;
  /** Lectura (auto-avance y gestos) de la cuenta; null mientras no se ha leído. */
  lectura?: ReadingPrefs | null;
  onLecturaChange?: (cambio: Partial<ReadingPrefs>) => void;
  /** Remitentes cuyas imágenes se enseñan sin preguntar; null mientras no se ha leído. */
  confiados?: readonly string[] | null;
  onDejarDeConfiar?: (direccion: string) => void;
  onOpen: (ajuste: AjusteMail) => void;
}

/** Una fila de una sección: qué es, para qué sirve, su estado y su acción. */
interface Fila {
  ajuste: AjusteMail | AjusteEnLinea;
  titulo: MailMessageKey;
  descripcion: MailMessageKey;
  /** Sin acción, la fila lleva su `control` en el sitio del botón. */
  accion?: MailMessageKey;
  control?: JSX.Element;
  /** Lo que va debajo, a todo el ancho (la lista de remitentes de confianza). */
  extra?: JSX.Element;
  estado?: Status | null;
}

export function MailSettings({
  t,
  locale,
  signature,
  version,
  undoSend,
  onUndoSendChange,
  lectura = null,
  onLecturaChange,
  confiados = null,
  onDejarDeConfiar,
  onOpen,
}: Props): JSX.Element {
  // Se guarda la configuración y no la frase: la frase depende del idioma y del día,
  // y se escribe al pintar.
  const [vacacion, setVacacion] = useState<Vacation | null>(null);

  useEffect(() => {
    let vivo = true;
    // Si no se puede leer, la fila se queda sin distintivo: no se inventa un
    // «apagada» que podría ser falso. El diálogo ya dice el error al abrirlo.
    getVacation()
      .then((v) => vivo && setVacacion(v))
      .catch(() => vivo && setVacacion(null));
    return () => {
      vivo = false;
    };
  }, [version]);
  const vacaciones = vacacion ? estadoVacaciones(t, locale, vacacion) : null;

  const firma: Status = signature.trim()
    ? { tone: 'success', label: t('settings.signatureOn'), detail: '' }
    : { tone: 'neutral', label: t('settings.signatureOff'), detail: '' };

  // Un selector de la sección de lectura. Sin el valor guardado se queda
  // deshabilitado, como el de deshacer: enseñar el de por defecto y que al tocarlo
  // pisara el de la cuenta sería peor que esperar.
  function selectorLectura<K extends keyof ReadingPrefs>(
    clave: K,
    titulo: MailMessageKey,
    opciones: readonly ReadingPrefs[K][],
    rotulo: Record<ReadingPrefs[K], MailMessageKey>,
  ): JSX.Element {
    return (
      <Select
        size="md"
        class="w-56 max-w-full"
        aria-label={t(titulo)}
        value={lectura ? lectura[clave] : ''}
        disabled={!lectura || !onLecturaChange}
        onChange={(e: Event) =>
          onLecturaChange?.({
            [clave]: (e.currentTarget as HTMLSelectElement).value,
          })
        }
      >
        {lectura ? null : <option value="" />}
        {opciones.map((o) => (
          <option key={o} value={o}>
            {t(rotulo[o])}
          </option>
        ))}
      </Select>
    );
  }

  const listaConfianza = (
    <div class="mt-2">
      {confiados === null ? null : confiados.length === 0 ? (
        <p class="text-text-subtle text-sm">{t('settings.trustedSendersEmpty')}</p>
      ) : (
        <ul class="border-border divide-border divide-y rounded-md border">
          {confiados.map((direccion) => (
            <li key={direccion} class="flex items-center gap-3 py-1 pr-1 pl-3">
              <span class="text-text min-w-0 flex-1 truncate text-sm" title={direccion}>
                {direccion}
              </span>
              <Button
                size="sm"
                variant="ghost"
                class="shrink-0"
                aria-label={t('settings.trustedSendersRemoveFor', { address: direccion })}
                disabled={!onDejarDeConfiar}
                onClick={() => onDejarDeConfiar?.(direccion)}
              >
                {t('settings.trustedSendersRemove')}
              </Button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );

  const secciones: { titulo: MailMessageKey; descripcion?: MailMessageKey; filas: Fila[] }[] = [
    {
      titulo: 'settings.writingTitle',
      filas: [
        {
          ajuste: 'signature',
          titulo: 'nav.signature',
          descripcion: 'settings.signatureDescription',
          accion: 'settings.edit',
          estado: firma,
        },
        {
          ajuste: 'templates',
          titulo: 'settings.templatesTitle',
          descripcion: 'settings.templatesDescription',
          accion: 'settings.manage',
        },
        {
          ajuste: 'vacation',
          titulo: 'nav.vacation',
          descripcion: 'settings.vacationDescription',
          accion: 'settings.edit',
          estado: vacaciones,
        },
        {
          ajuste: 'undo-send',
          titulo: 'settings.undoSendTitle',
          descripcion: 'settings.undoSendDescription',
          // Sin el valor guardado el selector se queda deshabilitado en vez de enseñar
          // un 10 que quizá no es el de la cuenta (y que, al tocarlo, lo pisaría).
          control: (
            <Select
              size="md"
              class="w-28"
              aria-label={t('settings.undoSendTitle')}
              value={undoSend === null ? '' : String(undoSend)}
              disabled={undoSend === null}
              onChange={(e: Event) =>
                onUndoSendChange(
                  Number((e.currentTarget as HTMLSelectElement).value) as UndoSendSeconds,
                )
              }
            >
              {undoSend === null ? <option value="" /> : null}
              {UNDO_SEND_OPTIONS.map((s) => (
                <option key={s} value={String(s)}>
                  {t('settings.undoSendOption', { count: s })}
                </option>
              ))}
            </Select>
          ),
        },
      ],
    },
    {
      titulo: 'settings.readingTitle',
      filas: [
        {
          ajuste: 'auto-advance',
          titulo: 'settings.autoAdvanceTitle',
          descripcion: 'settings.autoAdvanceDescription',
          control: selectorLectura(
            'autoAdvance',
            'settings.autoAdvanceTitle',
            AUTO_ADVANCE_OPTIONS,
            ROTULO_AVANCE,
          ),
        },
        {
          ajuste: 'swipe-right',
          titulo: 'settings.swipeRightTitle',
          descripcion: 'settings.swipeDescription',
          control: selectorLectura(
            'swipeRight',
            'settings.swipeRightTitle',
            SWIPE_OPTIONS,
            ROTULO_GESTO,
          ),
        },
        {
          ajuste: 'swipe-left',
          titulo: 'settings.swipeLeftTitle',
          descripcion: 'settings.swipeDescription',
          control: selectorLectura(
            'swipeLeft',
            'settings.swipeLeftTitle',
            SWIPE_OPTIONS,
            ROTULO_GESTO,
          ),
        },
        {
          ajuste: 'trusted-senders',
          titulo: 'settings.trustedSendersTitle',
          descripcion: 'settings.trustedSendersDescription',
          extra: listaConfianza,
        },
      ],
    },
    {
      titulo: 'settings.addressesTitle',
      filas: [
        {
          ajuste: 'aliases',
          titulo: 'nav.aliases',
          descripcion: 'settings.aliasesDescription',
          accion: 'settings.manage',
        },
        {
          ajuste: 'shared',
          titulo: 'nav.sharedAndDelegation',
          descripcion: 'settings.sharedDescription',
          accion: 'settings.manage',
        },
      ],
    },
    {
      titulo: 'settings.sortingTitle',
      filas: [
        {
          ajuste: 'rules',
          titulo: 'nav.rules',
          descripcion: 'settings.rulesDescription',
          accion: 'settings.manage',
        },
      ],
    },
    {
      titulo: 'settings.securityTitle',
      descripcion: 'settings.securityDescription',
      filas: [
        {
          ajuste: 'smime-anchor',
          titulo: 'nav.smimeAnchor',
          descripcion: 'settings.anchorDescription',
          accion: 'settings.download',
        },
        {
          ajuste: 'smime-identity',
          titulo: 'nav.smimeIdentity',
          descripcion: 'settings.identityDescription',
          accion: 'settings.install',
        },
      ],
    },
    {
      titulo: 'settings.clientsTitle',
      filas: [
        {
          ajuste: 'mail-clients',
          titulo: 'nav.mailClients',
          descripcion: 'settings.mailClientsDescription',
          accion: 'settings.openId',
        },
      ],
    },
  ];

  return (
    // `level={2}`: el h1 de la ventana es el de la Sytue Shell; dentro del cuerpo
    // de Mail un segundo h1 rompería el árbol de encabezados.
    <SettingsShell
      level={2}
      title={t('settings.title')}
      description={t('settings.description')}
      // El hueco del «+» de Redactar, que flota encima: lo último de los ajustes se
      // puede bajar por encima de él.
      class="sytue-reserva-flotante h-full overflow-y-auto"
    >
      {secciones.map((s) => (
        <SettingsSection
          key={s.titulo}
          title={t(s.titulo)}
          {...(s.descripcion ? { description: t(s.descripcion) } : {})}
        >
          <ul class="divide-border -my-2 flex flex-col divide-y">
            {s.filas.map((f) => (
              <li
                key={f.ajuste}
                data-ajuste={f.ajuste}
                class="flex flex-wrap items-center gap-x-4 gap-y-2 py-3"
              >
                <div class="min-w-0 flex-1 basis-64">
                  <p class="text-text flex flex-wrap items-center gap-2 text-sm font-semibold">
                    {t(f.titulo)}
                    {f.estado ? <Badge tone={f.estado.tone}>{f.estado.label}</Badge> : null}
                  </p>
                  <p class="text-text-muted mt-0.5 text-sm leading-snug">{t(f.descripcion)}</p>
                  {/* El detalle solo cuando dice algo que la descripción no: «responde
                      hasta el 14 de marzo», «empieza el 2 de agosto». Apagada, la
                      etiqueta ya lo dice todo. */}
                  {f.estado?.detail && f.estado.tone !== 'neutral' ? (
                    <p class="text-text mt-0.5 text-sm leading-snug">{f.estado.detail}</p>
                  ) : null}
                </div>
                {f.extra ? (
                  <div class="basis-full">{f.extra}</div>
                ) : f.control ? (
                  <div class="max-w-full shrink-0">{f.control}</div>
                ) : f.accion ? (
                  <Button
                    size="sm"
                    variant="secondary"
                    class="shrink-0"
                    // El rótulo solo («Gestionar») se repite en varias filas: el nombre
                    // accesible dice QUÉ se gestiona, como lo diría quien lo ve al lado.
                    aria-label={`${t(f.accion)}: ${t(f.titulo)}`}
                    {...(f.ajuste === 'mail-clients'
                      ? { trailingIcon: <Icon name="arrowUpRight" size={14} /> }
                      : {})}
                    onClick={() => onOpen(f.ajuste as AjusteMail)}
                  >
                    {t(f.accion)}
                  </Button>
                ) : null}
              </li>
            ))}
          </ul>
        </SettingsSection>
      ))}
    </SettingsShell>
  );
}
