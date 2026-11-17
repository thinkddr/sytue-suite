/**
 * «Posponer» una conversación: el menú con las fechas de siempre (más tarde hoy,
 * mañana, este fin de semana, la semana que viene) y «Elegir fecha y hora», como el
 * de Gmail. Las fechas son en la hora LOCAL de quien pospone, que es la que tiene en
 * la cabeza al decir «mañana»; al servicio le llega el instante en UTC.
 *
 * Solo elige la fecha: posponer de verdad, el aviso y quitar el hilo de la lista
 * son del padre (`onPosponer`).
 */
import type { JSX } from 'preact';
import { useState } from 'preact/hooks';
import { Button, Dropdown, FieldGroup, Input, MenuItem, Modal } from '@sytue/ui';
import { formatDateTime, type Locale } from '@sytue/i18n';
import { useT } from '../i18n/index.js';
import { Glyph } from './glyphs.js';
import { toLocalInput } from './utils.js';

/** Las fechas fijas del menú. */
export type OpcionDePosponer = 'laterToday' | 'tomorrow' | 'weekend' | 'nextWeek';

/** Hora a la que «vuelve» lo pospuesto a un día: la de empezar a trabajar. */
const HORA_MANANA = 8;
/** «Más tarde hoy» es al acabar la jornada; pasada la tarde ya no se ofrece. */
const HORA_TARDE = 18;
const ULTIMA_HORA_PARA_HOY = 17;

/** `d` a las `hora`:00 en punto, en hora local. */
function aLas(d: Date, hora: number): Date {
  const r = new Date(d);
  r.setHours(hora, 0, 0, 0);
  return r;
}

/** `d` más `dias` días de calendario (no 24 h: el cambio de hora no la mueve). */
function masDias(d: Date, dias: number): Date {
  const r = new Date(d);
  r.setDate(r.getDate() + dias);
  return r;
}

/**
 * Las fechas fijas que tiene sentido ofrecer `ahora`, en orden. «Más tarde hoy»
 * solo antes de las cinco; «Este fin de semana» solo de lunes a jueves (el viernes
 * sería mañana, y en el fin de semana ya se está en él); «La semana que viene» es el
 * lunes siguiente, salvo que mañana ya sea lunes.
 */
export function opcionesDePosponer(ahora: Date): { opcion: OpcionDePosponer; fecha: Date }[] {
  const out: { opcion: OpcionDePosponer; fecha: Date }[] = [];
  if (ahora.getHours() < ULTIMA_HORA_PARA_HOY) {
    out.push({ opcion: 'laterToday', fecha: aLas(ahora, HORA_TARDE) });
  }
  const manana = aLas(masDias(ahora, 1), HORA_MANANA);
  out.push({ opcion: 'tomorrow', fecha: manana });
  const dia = ahora.getDay(); // 0 domingo … 6 sábado
  if (dia >= 1 && dia <= 4) {
    out.push({ opcion: 'weekend', fecha: aLas(masDias(ahora, 6 - dia), HORA_MANANA) });
  }
  const lunes = aLas(masDias(ahora, (8 - dia) % 7 || 7), HORA_MANANA);
  if (lunes.getTime() !== manana.getTime()) out.push({ opcion: 'nextWeek', fecha: lunes });
  return out;
}

const ROTULOS = {
  laterToday: 'snooze.laterToday',
  tomorrow: 'snooze.tomorrow',
  weekend: 'snooze.weekend',
  nextWeek: 'snooze.nextWeek',
} as const;

/** La fecha de una opción, corta: «mar, 8:00». */
function fechaCorta(locale: Locale, d: Date): string {
  return formatDateTime(locale, d, { weekday: 'short', hour: 'numeric', minute: '2-digit' });
}

/** El `min` (dentro de un minuto) y el valor inicial (mañana a las 8) del selector
 *  de fecha y hora. Solo lo llama el manejador que lo abre: el render no lee el reloj. */
function valoresDelSelector(): { minimo: string; inicial: string } {
  const ahora = new Date();
  return {
    minimo: toLocalInput(new Date(ahora.getTime() + 60_000)),
    inicial: toLocalInput(aLas(masDias(ahora, 1), HORA_MANANA)),
  };
}

interface Props {
  locale: Locale;
  /** Pospone hasta `fecha`; el padre hace la llamada y el aviso. */
  onPosponer: (fecha: Date) => void;
  /** Rótulo del botón: «Posponer» o, con el hilo ya pospuesto, «Cambiar fecha». */
  rotulo?: string;
}

export function PosponerMenu({ locale, onPosponer, rotulo }: Props): JSX.Element {
  const t = useT(locale);
  // Las opciones se calculan al ABRIR el menú (el render no lee el reloj): un lector
  // abierto desde la mañana no debe ofrecer por la noche «Más tarde hoy».
  const [opciones, setOpciones] = useState<{ opcion: OpcionDePosponer; fecha: Date }[]>([]);
  const [eligiendo, setEligiendo] = useState(false);
  const [valor, setValor] = useState('');
  const [minimo, setMinimo] = useState('');
  const [invalida, setInvalida] = useState(false);
  const titulo = rotulo ?? t('snooze.action');

  function abrirPersonalizada(): void {
    const { minimo: min, inicial } = valoresDelSelector();
    setMinimo(min);
    setValor(inicial);
    setInvalida(false);
    setEligiendo(true);
  }

  function confirmar(): void {
    const fecha = new Date(valor);
    const ahora = Date.now();
    // Lo mismo que exige el servicio: entre un minuto y un año. Se dice aquí, junto
    // al campo, en vez de esperar al rechazo.
    if (
      !valor ||
      Number.isNaN(fecha.getTime()) ||
      fecha.getTime() < ahora + 60_000 ||
      fecha.getTime() > ahora + 366 * 86_400_000
    ) {
      setInvalida(true);
      return;
    }
    setEligiendo(false);
    onPosponer(fecha);
  }

  return (
    <>
      <Dropdown
        label={titulo}
        align="end"
        trigger={(p) => (
          <Button
            size="sm"
            variant="ghost"
            iconOnly
            aria-label={titulo}
            title={titulo}
            leadingIcon={<Glyph name="clock" />}
            {...p}
            onClick={() => {
              setOpciones(opcionesDePosponer(new Date()));
              p.onClick();
            }}
          />
        )}
      >
        {(cerrar) => (
          <>
            {opciones.map(({ opcion, fecha }) => (
              <MenuItem
                key={opcion}
                onClick={() => {
                  cerrar();
                  onPosponer(fecha);
                }}
              >
                <span class="flex flex-1 items-center justify-between gap-4">
                  <span>{t(ROTULOS[opcion])}</span>
                  <span class="text-text-subtle text-sm tabular-nums">
                    {fechaCorta(locale, fecha)}
                  </span>
                </span>
              </MenuItem>
            ))}
            <MenuItem
              onClick={() => {
                cerrar();
                abrirPersonalizada();
              }}
            >
              {t('snooze.custom')}
            </MenuItem>
          </>
        )}
      </Dropdown>
      <Modal
        open={eligiendo}
        onClose={() => setEligiendo(false)}
        title={t('snooze.customTitle')}
        size="sm"
        footer={
          <>
            <Button variant="secondary" onClick={() => setEligiendo(false)}>
              {t('common.cancel')}
            </Button>
            <Button onClick={confirmar}>{t('snooze.confirm')}</Button>
          </>
        }
      >
        <FieldGroup
          label={t('snooze.dateTime')}
          htmlFor="posponer-fecha"
          {...(invalida ? { error: t('snooze.invalid') } : {})}
        >
          <Input
            id="posponer-fecha"
            type="datetime-local"
            value={valor}
            min={minimo}
            aria-invalid={invalida}
            onInput={(e) => {
              setValor(e.currentTarget.value);
              setInvalida(false);
            }}
          />
        </FieldGroup>
      </Modal>
    </>
  );
}
