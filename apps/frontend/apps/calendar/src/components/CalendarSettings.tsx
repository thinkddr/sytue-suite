/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
/**
 * CalendarSettings — la página de AJUSTES de Sytue Calendar (design.md §8.4).
 *
 * Mismo marco que la de Mail (`SettingsShell` + `SettingsSection`): tres temas que se
 * explican solos —cómo nace un evento, la jornada y lo que enseña la vista—. No hay
 * botón de guardar: cada control es un valor de pocos y se guarda al elegirlo, como la
 * ventana de deshacer de Mail; el calendario lo aplica en cuanto se vuelve a él.
 *
 * Fyne pinta la misma página (apps/fyne/calendario/ajustes.go) con las mismas claves.
 */
import type { JSX } from 'preact';
import { Button, Icon, Select, SettingsSection, SettingsShell, Switch } from '@sytue/ui';
import type { T } from '../i18n/index.js';
import { DURACIONES, RECORDATORIOS, type AjustesCalendario } from '../lib/preferencias.js';
import { reminderLabel } from './EventModal.js';

interface Props {
  t: T;
  ajustes: AjustesCalendario;
  onChange: (ajustes: AjustesCalendario) => void;
  onClose: () => void;
}

/** «1 hora», «30 min»: la duración en la unidad en la que se dice. */
export function etiquetaDuracion(t: T, minutos: number): string {
  return minutos % 60 === 0
    ? t('settings.durationHours', { count: minutos / 60 })
    : t('settings.durationMinutes', { count: minutos });
}

/** «09:00»: la hora en punto en el formato de 24 h del eje de la rejilla. */
function etiquetaHora(h: number): string {
  return `${String(h).padStart(2, '0')}:00`;
}

const HORAS = Array.from({ length: 25 }, (_, h) => h);

export function CalendarSettings({ t, ajustes, onChange, onClose }: Props): JSX.Element {
  const poner = (cambio: Partial<AjustesCalendario>): void => onChange({ ...ajustes, ...cambio });
  const valor = (e: Event): string => (e.currentTarget as HTMLSelectElement).value;

  return (
    <SettingsShell
      level={2}
      title={t('settings.title')}
      description={t('settings.description')}
      actions={
        <Button
          variant="secondary"
          size="sm"
          leadingIcon={<Icon name="chevronLeft" size={16} />}
          onClick={onClose}
        >
          {t('settings.back')}
        </Button>
      }
    >
      <SettingsSection
        title={t('settings.eventsTitle')}
        description={t('settings.eventsDescription')}
      >
        <div class="grid gap-4 sm:grid-cols-2">
          <label class="flex flex-col gap-1.5">
            <span class="text-text text-sm font-semibold">{t('settings.duration')}</span>
            <Select
              data-ajuste="duracion"
              value={String(ajustes.duracion)}
              onChange={(e: Event) => poner({ duracion: Number(valor(e)) })}
            >
              {DURACIONES.map((m) => (
                <option key={m} value={String(m)}>
                  {etiquetaDuracion(t, m)}
                </option>
              ))}
            </Select>
          </label>
          <label class="flex flex-col gap-1.5">
            <span class="text-text text-sm font-semibold">{t('settings.reminder')}</span>
            <Select
              data-ajuste="recordatorio"
              value={ajustes.recordatorio === null ? '' : String(ajustes.recordatorio)}
              onChange={(e: Event) => {
                const v = valor(e);
                poner({ recordatorio: v === '' ? null : Number(v) });
              }}
            >
              <option value="">{t('settings.reminderNone')}</option>
              {RECORDATORIOS.map((m) => (
                <option key={m} value={String(m)}>
                  {reminderLabel(t, m)}
                </option>
              ))}
            </Select>
          </label>
        </div>
      </SettingsSection>

      <SettingsSection title={t('settings.workTitle')} description={t('settings.workDescription')}>
        <div class="grid gap-4 sm:grid-cols-2">
          <label class="flex flex-col gap-1.5">
            <span class="text-text text-sm font-semibold">{t('settings.workStart')}</span>
            <Select
              data-ajuste="inicioLaboral"
              value={String(ajustes.inicioLaboral)}
              onChange={(e: Event) => poner({ inicioLaboral: Number(valor(e)) })}
            >
              {/* Solo las horas antes del fin: una jornada al revés no se puede elegir. */}
              {HORAS.filter((h) => h < ajustes.finLaboral).map((h) => (
                <option key={h} value={String(h)}>
                  {etiquetaHora(h)}
                </option>
              ))}
            </Select>
          </label>
          <label class="flex flex-col gap-1.5">
            <span class="text-text text-sm font-semibold">{t('settings.workEnd')}</span>
            <Select
              data-ajuste="finLaboral"
              value={String(ajustes.finLaboral)}
              onChange={(e: Event) => poner({ finLaboral: Number(valor(e)) })}
            >
              {HORAS.filter((h) => h > ajustes.inicioLaboral).map((h) => (
                <option key={h} value={String(h)}>
                  {etiquetaHora(h)}
                </option>
              ))}
            </Select>
          </label>
        </div>
      </SettingsSection>

      <SettingsSection title={t('settings.viewTitle')} description={t('settings.viewDescription')}>
        <div class="flex flex-col gap-3">
          <Switch
            data-ajuste="finesDeSemana"
            label={t('settings.weekends')}
            checked={ajustes.finesDeSemana}
            onChange={(e: Event) =>
              poner({ finesDeSemana: (e.currentTarget as HTMLInputElement).checked })
            }
          />
          <Switch
            data-ajuste="numeroSemana"
            label={t('settings.weekNumbers')}
            checked={ajustes.numeroSemana}
            onChange={(e: Event) =>
              poner({ numeroSemana: (e.currentTarget as HTMLInputElement).checked })
            }
          />
          <Switch
            data-ajuste="rechazados"
            label={t('settings.declined')}
            checked={ajustes.rechazados}
            onChange={(e: Event) =>
              poner({ rechazados: (e.currentTarget as HTMLInputElement).checked })
            }
          />
        </div>
      </SettingsSection>
    </SettingsShell>
  );
}
