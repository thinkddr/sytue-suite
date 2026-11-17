/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Subir el certificado electrónico de la organización para VERI*FACTU.
 *
 * Antes la tarjeta decía «Todavía no está disponible» y no había forma de darle
 * el certificado al servicio sin la API y dos PEM hechos con OpenSSL. La FNMT y
 * las cámaras lo entregan como `.p12`/`.pfx` con contraseña: eso es lo que se
 * pide aquí, tal cual, y el servicio lo abre, comprueba el NIF y sella la clave.
 *
 * La contraseña solo vive en este estado mientras el asistente está abierto y
 * viaja en la misma petición (por TLS) que el fichero; no se guarda en ninguna
 * parte. Al cerrar el asistente se suelta todo.
 */
import type { JSX } from 'preact';
import { useState } from 'preact/hooks';
import { Button, Checkbox, FieldGroup, InlineHelp, Input, Modal, Wizard } from '@sytue/ui';
import { ErrorApi, api, mensajeDeError } from '../lib/api.js';
import type { useT } from '../i18n/index.js';
import type { EstadoVerifactu } from './tipos.js';

type Traducir = ReturnType<typeof useT>;

/** Lo que interesa de un fichero elegido antes de mandarlo. */
interface Elegido {
  datos: ArrayBuffer;
  /** PEM en texto; si no, se trata como PKCS#12 y pide contraseña. */
  pem: boolean;
  /** Si el PEM ya trae la clave privada dentro. */
  conClave: boolean;
}

const ACEPTA = '.p12,.pfx,.pem,.crt,.cer,application/x-pkcs12';
const ACEPTA_CLAVE = '.pem,.key';
const CAMPO_FICHERO =
  'sytue-focus-ring border-border block w-full rounded-md border p-3 text-base md:text-sm';

/** Base64 estándar, que es como Go decodifica un `[]byte` en JSON. */
export function aBase64(datos: ArrayBuffer): string {
  const bytes = new Uint8Array(datos);
  let binario = '';
  // Por trozos: `String.fromCharCode(...bytes)` con un fichero entero revienta la pila.
  for (let i = 0; i < bytes.length; i += 0x8000) {
    binario += String.fromCharCode(...bytes.subarray(i, i + 0x8000));
  }
  return btoa(binario);
}

/** Mira si el fichero es PEM (y si trae la clave) sin fiarse de la extensión. */
export async function examinar(fichero: Blob): Promise<Elegido> {
  const datos = await fichero.arrayBuffer();
  // Solo el principio importa para decidir, y un `.p12` no es UTF-8 válido.
  const texto = new TextDecoder('latin1').decode(new Uint8Array(datos));
  const pem = texto.includes('-----BEGIN ');
  return { datos, pem, conClave: pem && /-----BEGIN [A-Z ]*PRIVATE KEY-----/.test(texto) };
}

export function AsistenteCertificadoVerifactu({
  onCerrar,
  onGuardado,
  t,
}: {
  onCerrar: () => void;
  onGuardado: (estado: EstadoVerifactu) => void;
  t: Traducir;
}): JSX.Element {
  const [paso, setPaso] = useState(0);
  const [fichero, setFichero] = useState<Elegido | null>(null);
  const [clave, setClave] = useState<ArrayBuffer | null>(null);
  const [contrasena, setContrasena] = useState('');
  const [sello, setSello] = useState(false);
  const [error, setError] = useState('');
  const [errorContrasena, setErrorContrasena] = useState('');
  const [enviando, setEnviando] = useState(false);
  const [guardado, setGuardado] = useState<EstadoVerifactu | null>(null);

  const faltaClave = Boolean(fichero?.pem && !fichero.conClave);
  const listoFichero = Boolean(fichero && (!faltaClave || clave));

  async function elegir(f: File | undefined): Promise<void> {
    setError('');
    setClave(null);
    setFichero(f ? await examinar(f) : null);
  }

  async function guardar(): Promise<void> {
    if (!fichero) return;
    setEnviando(true);
    setError('');
    setErrorContrasena('');
    try {
      const estado = await api<EstadoVerifactu>('verifactu/certificado', {
        method: 'PUT',
        body: JSON.stringify({
          fichero: aBase64(fichero.datos),
          ...(faltaClave && clave ? { clave: aBase64(clave) } : {}),
          ...(fichero.pem ? {} : { contrasena }),
          sello,
        }),
      });
      setContrasena('');
      setGuardado(estado);
      onGuardado(estado);
      setPaso(2);
    } catch (err) {
      if (err instanceof ErrorApi && err.codigo === 'certificado_contrasena') {
        setErrorContrasena(mensajeDeError(err, t));
      } else {
        setError(mensajeDeError(err, t));
      }
    } finally {
      setEnviando(false);
    }
  }

  return (
    <Modal open onClose={onCerrar} dismissable={!enviando} title={t('vf.cert.title')} size="md">
      {error ? (
        <InlineHelp tone="danger" class="mb-4">
          {error}
        </InlineHelp>
      ) : null}
      <Wizard
        current={paso}
        busy={enviando}
        {...(guardado ? {} : { onCancel: onCerrar })}
        onCurrentChange={(n) => {
          if (!guardado) {
            setError('');
            setPaso(n);
          }
        }}
        steps={[
          {
            id: 'fichero',
            title: t('vf.cert.stepFile'),
            heading: t('vf.cert.fileHeading'),
            description: t('vf.cert.fileHint'),
            canContinue: listoFichero,
            content: (
              <div class="space-y-4">
                <FieldGroup label={t('vf.cert.fileLabel')} htmlFor="vf-certificado">
                  <input
                    id="vf-certificado"
                    type="file"
                    accept={ACEPTA}
                    disabled={enviando}
                    class={CAMPO_FICHERO}
                    onChange={(e) => void elegir(e.currentTarget.files?.[0])}
                  />
                </FieldGroup>
                {faltaClave ? (
                  <FieldGroup
                    label={t('vf.cert.keyLabel')}
                    htmlFor="vf-clave"
                    help={t('vf.cert.keyHint')}
                  >
                    <input
                      id="vf-clave"
                      type="file"
                      accept={ACEPTA_CLAVE}
                      disabled={enviando}
                      class={CAMPO_FICHERO}
                      onChange={(e) => {
                        const f = e.currentTarget.files?.[0];
                        if (f) void f.arrayBuffer().then(setClave);
                        else setClave(null);
                      }}
                    />
                  </FieldGroup>
                ) : null}
              </div>
            ),
          },
          {
            id: 'confirmar',
            title: t('vf.cert.stepConfirm'),
            heading: t('vf.cert.confirmHeading'),
            nextLabel: t('vf.cert.save'),
            onNext: guardar,
            canContinue: listoFichero && (fichero?.pem || contrasena !== ''),
            content: (
              <div class="space-y-4">
                {fichero && !fichero.pem ? (
                  <FieldGroup
                    label={t('vf.cert.passwordLabel')}
                    htmlFor="vf-contrasena"
                    help={t('vf.cert.passwordHint')}
                    {...(errorContrasena ? { error: errorContrasena } : {})}
                  >
                    <Input
                      id="vf-contrasena"
                      type="password"
                      autocomplete="off"
                      value={contrasena}
                      invalid={Boolean(errorContrasena)}
                      disabled={enviando}
                      onInput={(e) => {
                        setContrasena(e.currentTarget.value);
                        setErrorContrasena('');
                      }}
                    />
                  </FieldGroup>
                ) : null}
                <Checkbox
                  label={t('vf.cert.seal')}
                  checked={sello}
                  disabled={enviando}
                  onChange={(e) => setSello(e.currentTarget.checked)}
                />
                <p class="text-text-muted text-xs leading-relaxed">{t('vf.cert.privacy')}</p>
              </div>
            ),
          },
          {
            id: 'listo',
            title: t('vf.cert.stepDone'),
            heading: t('vf.cert.saved'),
            done: true,
            content: (
              <div class="space-y-4">
                <p class="text-text-muted text-sm">
                  {t('vf.cert.savedBody', { nif: guardado?.certificado_nif ?? '' })}
                </p>
                <Button onClick={onCerrar}>{t('action.close')}</Button>
              </div>
            ),
          },
        ]}
      />
    </Modal>
  );
}
