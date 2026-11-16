/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Activar la verificación en dos pasos, paso a paso.
 *
 * Es un flujo con ORDEN —no se escanea sin una app, no se escribe el código sin
 * haber escaneado, no se activa sin los códigos de recuperación a salvo—, que es el
 * caso de `Wizard` (CONVENTIONS §8.3). Antes era un formulario en línea con el QR,
 * la clave y el código a la vez, y los códigos aparecían debajo sin forma de
 * copiarlos ni guardarlos.
 *
 *   1. Qué necesitas: una app de autenticación, con ejemplos.
 *   2. Escanear: el QR y la clave con «Copiar» (enrolar desde el mismo móvil).
 *   3. Escribir el código: aquí se activa de verdad (`/api/mfa/confirm`).
 *   4. Guardar los códigos: no se sigue sin marcar «Los he guardado», y no se vuelve
 *      atrás ni se cancela: la verificación ya está activa.
 *   5. Hecho, con la sugerencia de añadir también una llave de acceso.
 *
 * Cancelar antes del paso 4 no deja nada a medias: `/api/mfa/enroll` guarda el
 * secreto como NO confirmado, que ni se pide al entrar ni impide empezar otra vez
 * (el siguiente alta lo sustituye). La verificación solo se enciende al confirmar
 * el código, y de ahí en adelante ya no hay «Cancelar».
 */
import type { JSX } from 'preact';
import { useState } from 'preact/hooks';
import { Button, CopyField, FieldGroup, FieldHint, InlineHelp, Input, Wizard } from '@sytue/ui';
import type { T } from '../i18n/index.js';
import { CodigosRecuperacion } from './CodigosRecuperacion.js';

interface Enrolamiento {
  otpauth_uri: string;
  secret: string;
  /** `data:image/png;base64,…` ya montado por el servicio. Ausente si no cupo. */
  qr_png?: string;
}

/** Paso a partir del cual la verificación ya está activa en Sytue ID. */
const PASO_CODIGOS = 3;

export function DosPasosWizard({
  t,
  cuenta,
  hayWebAuthn,
  onActivada,
  onCerrar,
}: {
  t: T;
  /** Correo de la cuenta, para el `.txt` de los códigos. */
  cuenta: string;
  /** Si el navegador sabe crear llaves de acceso: sin eso no se sugiere. */
  hayWebAuthn: boolean;
  /** El servidor ya la tiene activa: la tarjeta relee `/me`. */
  onActivada: () => void;
  /** Cerrar el asistente; `aLlave` pide llevar el foco al alta de llaves. */
  onCerrar: (aLlave: boolean) => void;
}): JSX.Element {
  const [paso, setPaso] = useState(0);
  const [alta, setAlta] = useState<Enrolamiento | null>(null);
  const [codigo, setCodigo] = useState('');
  const [codigos, setCodigos] = useState<string[]>([]);
  const [guardados, setGuardados] = useState(false);
  const [ocupado, setOcupado] = useState(false);
  const [error, setError] = useState('');

  /** El mensaje del servicio dice por qué falló mejor que uno genérico. */
  async function motivo(res: Response | null): Promise<string> {
    if (!res) return t('security.failed');
    try {
      const cuerpo = (await res.json()) as { message?: string; error?: string };
      return cuerpo.message || cuerpo.error || t('security.failed');
    } catch {
      return t('security.failed');
    }
  }

  async function pedir(ruta: string, init: RequestInit): Promise<Response | null> {
    try {
      return await fetch(ruta, init);
    } catch {
      // Sin red el `fetch` rechaza: sin esto el asistente se quedaba «ocupado».
      return null;
    }
  }

  async function empezar(): Promise<void> {
    // Volver a «Qué necesitas» y seguir no pide otro secreto: el QR ya escaneado
    // dejaría de valer y la app mostraría códigos que el servidor rechaza.
    if (alta) {
      setPaso(1);
      return;
    }
    setOcupado(true);
    setError('');
    const res = await pedir('/api/mfa/enroll', { method: 'POST' });
    if (!res?.ok) {
      setError(await motivo(res));
      setOcupado(false);
      return;
    }
    setAlta((await res.json()) as Enrolamiento);
    setOcupado(false);
    setPaso(1);
  }

  async function confirmar(): Promise<void> {
    setOcupado(true);
    setError('');
    const res = await pedir('/api/mfa/confirm', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ code: codigoLimpio }),
    });
    if (!res?.ok) {
      // El error se queda en ESTE paso, junto al campo, con lo escrito intacto.
      setError(await motivo(res));
      setOcupado(false);
      return;
    }
    const datos = (await res.json()) as { recovery_codes?: string[] };
    setCodigos(datos.recovery_codes ?? []);
    setCodigo('');
    setOcupado(false);
    setPaso(PASO_CODIGOS);
    onActivada();
  }

  // Muchas apps enseñan el código partido («123 456») y así se copia o se teclea:
  // los blancos no son parte del código, y con ellos «Activar» no se habilitaba.
  const codigoLimpio = codigo.replace(/\s+/g, '');
  const codigoListo = /^\d{6,8}$/.test(codigoLimpio);
  const errorDelPaso = error ? (
    <InlineHelp tone="danger" class="mt-4">
      {error}
    </InlineHelp>
  ) : null;

  const pasos = [
    {
      id: 'necesitas',
      title: t('security.mfa.step.need'),
      heading: t('security.mfa.step.need.heading'),
      description: t('security.mfa.step.need.desc'),
      nextLabel: t('security.mfa.step.need.next'),
      onNext: () => empezar(),
      content: (
        <>
          <InlineHelp tone="info" title={t('security.mfa.step.need.apps_title')}>
            {t('security.mfa.step.need.apps')}
          </InlineHelp>
          {errorDelPaso}
        </>
      ),
    },
    {
      id: 'escanear',
      title: t('security.mfa.step.scan'),
      heading: t('security.mfa.step.scan.heading'),
      description: t('security.mfa.scan'),
      content: alta ? (
        <div class="flex flex-col gap-4">
          {/* PNG y no SVG: varios gestores de TOTP dejan «subir una imagen del
              código» y filtran el selector por PNG/JPEG. Lo pinta el servicio
              (totp/qr.go) y aquí solo se enseña, sin inyectar marcado. */}
          {alta.qr_png && (
            <img
              src={alta.qr_png}
              alt={t('security.mfa.qr_alt')}
              width={180}
              height={180}
              class="self-start rounded bg-white p-2"
            />
          )}
          {/* La clave en texto SIEMPRE, no solo cuando falta el QR: es la única vía
              si se enrola desde el mismo dispositivo que muestra la pantalla. */}
          <p class="text-text-muted text-sm">{t('security.mfa.manual')}</p>
          <CopyField label={t('security.mfa.key_label')} value={alta.secret} labelWidth="sm" />
        </div>
      ) : null,
    },
    {
      id: 'codigo',
      title: t('security.mfa.step.code'),
      heading: t('security.mfa.step.code.heading'),
      description: t('security.mfa.step.code.desc'),
      nextLabel: t('security.mfa.confirm'),
      canContinue: codigoListo,
      onNext: () => confirmar(),
      content: (
        // Intro en el campo envía: es lo que se espera tras teclear un código.
        <form
          onSubmit={(e) => {
            e.preventDefault();
            if (codigoListo && !ocupado) void confirmar();
          }}
        >
          <FieldGroup label={t('security.mfa.code')} htmlFor="mfa-codigo" required>
            <Input
              id="mfa-codigo"
              inputMode="numeric"
              autocomplete="one-time-code"
              maxLength={8}
              value={codigo}
              aria-describedby="mfa-codigo-pista"
              invalid={Boolean(error)}
              onInput={(e: Event) => setCodigo((e.target as HTMLInputElement).value)}
              required
            />
            <FieldHint id="mfa-codigo-pista" tone={error ? 'danger' : 'neutral'}>
              {error || t('security.mfa.code.hint')}
            </FieldHint>
          </FieldGroup>
        </form>
      ),
    },
    {
      id: 'codigos',
      title: t('security.codes.title'),
      heading: t('security.mfa.step.codes.heading'),
      description: t('security.codes.once'),
      canContinue: guardados,
      canGoBack: false,
      content: (
        <CodigosRecuperacion
          t={t}
          codigos={codigos}
          cuenta={cuenta}
          guardados={guardados}
          onGuardados={setGuardados}
        />
      ),
    },
    {
      id: 'hecho',
      title: t('security.mfa.step.done'),
      heading: t('security.mfa.step.done.heading'),
      description: t('security.mfa.step.done.desc'),
      done: true,
      content: (
        <div class="flex flex-col gap-4">
          {hayWebAuthn && (
            <InlineHelp
              title={t('security.mfa.passkey_tip.title')}
              action={
                <Button variant="secondary" size="sm" onClick={() => onCerrar(true)}>
                  {t('security.mfa.passkey_tip.action')}
                </Button>
              }
            >
              {t('security.mfa.passkey_tip')}
            </InlineHelp>
          )}
          <div>
            <Button onClick={() => onCerrar(false)}>{t('security.mfa.close')}</Button>
          </div>
        </div>
      ),
    },
  ];

  return (
    <Wizard
      steps={pasos}
      current={paso}
      onCurrentChange={(i) => {
        setError('');
        setPaso(i);
      }}
      busy={ocupado}
      // Con la verificación ya activa no se cancela: cerrar aquí perdería los códigos.
      {...(paso < PASO_CODIGOS ? { onCancel: () => onCerrar(false) } : {})}
    />
  );
}
