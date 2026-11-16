/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Pedir firma»: a otra persona, por su correo o su @usuario, sobre el PDF
 * abierto; y cómo van las que ya se pidieron de este documento (ADR 0051 §7).
 *
 * Varias personas firman EN ORDEN (mig. 0034): la lista es el orden, y cada una
 * recibe el PDF cuando la anterior lo ha firmado, ya con su firma. Con caducidad y
 * recordatorios, que valen igual para una sola.
 *
 * Si es de OTRA organización (ADR 0057), el servicio no crea nada hasta que quien
 * pide lo confirma: aquí se explica que el PDF saldrá por un enlace de Drive de
 * solo lectura y, al confirmar, se comparte y se pide en el mismo paso. Eso solo con
 * una persona: en una cadena firma solo gente de la organización.
 */
import { useCallback, useEffect, useId, useState } from 'preact/hooks';
import {
  Badge,
  Button,
  FieldGroup,
  FieldHint,
  Icon,
  IconButton,
  InlineHelp,
  Input,
  Select,
  Textarea,
  describedBy,
  type BadgeTone,
} from '@sytue/ui';
import { documentLocale } from '@sytue/i18n';
import type { DocsKey, docsT } from '../i18n';
import { r } from '../lib/rutas';
import { ErrorPDF, firmas, pedidasDe, quienFirma, type PeticionFirma } from './api';
import { mensajeDeError } from './errores';
import { fechaCorta } from './verificacion';

/** Cuántas personas caben en una cadena: el tope del servicio (store.MaxFirmantesEnCadena). */
export const MAX_FIRMANTES = 10;

/** Las caducidades que se ofrecen, en días (0 = nunca), con su rótulo. */
const CADUCIDADES: [number, DocsKey][] = [
  [0, 'pdf.dialog.pedirFirma.expiry.never'],
  [7, 'pdf.dialog.pedirFirma.expiry.d7'],
  [14, 'pdf.dialog.pedirFirma.expiry.d14'],
  [30, 'pdf.dialog.pedirFirma.expiry.d30'],
];

/** Cada cuántos días se recuerda (0 = nunca), con su rótulo. */
const RECORDATORIOS: [number, DocsKey][] = [
  [0, 'pdf.dialog.pedirFirma.remind.never'],
  [1, 'pdf.dialog.pedirFirma.remind.d1'],
  [3, 'pdf.dialog.pedirFirma.remind.d3'],
  [7, 'pdf.dialog.pedirFirma.remind.d7'],
];

/** Mueve el elemento `i` de `lista` un puesto hacia `paso` (-1 arriba, +1 abajo). */
export function mover<T>(lista: T[], i: number, paso: -1 | 1): T[] {
  const j = i + paso;
  if (j < 0 || j >= lista.length) return lista;
  const out = [...lista];
  [out[i], out[j]] = [out[j] as T, out[i] as T];
  return out;
}

/**
 * El tono de la insignia de cada estado: lo que espera respuesta va en informativo, lo
 * firmado en éxito y lo rechazado en peligro; lo que ya no corre (cancelada, o en espera
 * de que firme otra antes) en neutro, y lo caducado en aviso porque pide volver a pedirla.
 */
export function tonoDeEstado(estado: PeticionFirma['status']): BadgeTone {
  switch (estado) {
    case 'pendiente':
      return 'info';
    case 'firmada':
      return 'success';
    case 'rechazada':
      return 'danger';
    case 'caducada':
      return 'warning';
    default:
      return 'neutral';
  }
}

export function PedirFirma({
  t,
  path,
  onCerrar,
}: {
  t: ReturnType<typeof docsT>;
  path: string;
  onCerrar: () => void;
}) {
  const [firmantes, setFirmantes] = useState<string[]>(['']);
  const [mensaje, setMensaje] = useState('');
  const [caduca, setCaduca] = useState(0);
  const [recordar, setRecordar] = useState(0);
  const [enviando, setEnviando] = useState(false);
  const [error, setError] = useState('');
  const [hecho, setHecho] = useState('');
  const [pedidas, setPedidas] = useState<PeticionFirma[][] | null>(null);
  // La persona es de otra organización: su nombre y a quién se confirma, mientras se
  // pregunta.
  const [fuera, setFuera] = useState<{ name: string; confirm: string } | null>(null);

  // Los ids enlazan cada control con su rótulo y con la ayuda y el error que le tocan
  // (`aria-describedby`): el lector de pantalla los lee al llegar al campo, no solo
  // quien los ve debajo.
  const id = useId();
  const idFirmante = (i: number) => `${id}-firmante-${i}`;
  const idAyuda = `${id}-ayuda`;
  const idError = `${id}-error`;
  const idFuera = `${id}-fuera`;
  const idMensaje = `${id}-mensaje`;
  const idCaduca = `${id}-caduca`;
  const idRecordar = `${id}-recordar`;

  const quienes = firmantes.map((f) => f.trim()).filter((f) => f !== '');
  const enCadena = firmantes.length > 1;
  const ayudaOrden = enCadena ? t('pdf.dialog.pedirFirma.orderHelp') : undefined;
  const describeFirmante =
    [idAyuda, describedBy(idFirmante(0), ayudaOrden), error ? idError : '']
      .filter(Boolean)
      .join(' ') || undefined;

  const recargar = useCallback(
    () =>
      firmas
        .listar()
        .then((ps) => setPedidas(pedidasDe(ps.sent, path)))
        .catch(() => setPedidas([])),
    [path],
  );
  useEffect(() => void recargar(), [recargar]);

  const cambiarFirmantes = (nuevos: string[]) => {
    setFirmantes(nuevos);
    setFuera(null);
  };

  const enviar = (confirm: string) => {
    setEnviando(true);
    setError('');
    setHecho('');
    const primero = quienes[0] ?? '';
    firmas
      .pedir(path, quienes, mensaje.trim(), {
        confirm,
        caducaDias: caduca,
        recordarDias: recordar,
      })
      .then(() => {
        setHecho(
          quienes.length > 1
            ? t('pdf.dialog.pedirFirma.sentChain', { name: primero })
            : t('pdf.dialog.pedirFirma.sent', { name: fuera?.name ?? primero }),
        );
        setFirmantes(['']);
        setMensaje('');
        setFuera(null);
        return recargar();
      })
      .catch((err: unknown) => {
        if (err instanceof ErrorPDF && err.codigo === 'pdf_signature_recipient_external') {
          setFuera({
            name: err.detalles?.['name'] || primero,
            confirm: err.detalles?.['confirm'] ?? '',
          });
          return;
        }
        setError(mensajeDeError(err, t, 'pdf.error.apply'));
      })
      .finally(() => setEnviando(false));
  };

  const accion = (fn: Promise<unknown>) =>
    void fn
      .then(recargar)
      .catch((err: unknown) => setError(mensajeDeError(err, t, 'pdf.error.apply')));

  return (
    <div class="flex flex-col gap-4">
      <form
        class="flex flex-col gap-3"
        onSubmit={(e) => {
          e.preventDefault();
          enviar(fuera?.confirm ?? '');
        }}
      >
        <p id={idAyuda} class="text-text-muted text-xs text-pretty">
          {t('pdf.dialog.pedirFirma.help')}
        </p>
        {/* El rótulo es del primer campo; en una cadena cada uno lleva además su
            «Persona n», que es lo que distingue a uno de otro al oído. */}
        <FieldGroup
          label={t('pdf.dialog.pedirFirma.email')}
          htmlFor={idFirmante(0)}
          {...(ayudaOrden ? { help: ayudaOrden } : {})}
        >
          <ol class="flex flex-col gap-1.5">
            {firmantes.map((f, i) => (
              <li key={i} class="flex items-center gap-1.5">
                {enCadena && (
                  <span aria-hidden="true" class="text-text-muted w-4 shrink-0 text-right text-xs">
                    {i + 1}
                  </span>
                )}
                <Input
                  id={idFirmante(i)}
                  type="text"
                  required={i === 0}
                  autocomplete="email"
                  inputMode="email"
                  aria-label={
                    enCadena ? t('pdf.dialog.pedirFirma.signer', { n: i + 1 }) : undefined
                  }
                  aria-describedby={describeFirmante}
                  value={f}
                  onInput={(e) =>
                    cambiarFirmantes(
                      firmantes.map((x, j) => (j === i ? (e.target as HTMLInputElement).value : x)),
                    )
                  }
                  class="min-w-0 flex-1"
                />
                {enCadena && (
                  <span class="flex shrink-0 items-center">
                    <IconButton
                      type="button"
                      size="sm"
                      label={t('pdf.dialog.pedirFirma.moveUp', { n: i + 1 })}
                      disabled={i === 0}
                      onClick={() => cambiarFirmantes(mover(firmantes, i, -1))}
                    >
                      <Icon name="chevronUp" size={14} />
                    </IconButton>
                    <IconButton
                      type="button"
                      size="sm"
                      label={t('pdf.dialog.pedirFirma.moveDown', { n: i + 1 })}
                      disabled={i === firmantes.length - 1}
                      onClick={() => cambiarFirmantes(mover(firmantes, i, 1))}
                    >
                      <Icon name="chevronDown" size={14} />
                    </IconButton>
                    <IconButton
                      type="button"
                      size="sm"
                      tone="danger"
                      label={t('pdf.dialog.pedirFirma.removeSigner', { n: i + 1 })}
                      onClick={() => cambiarFirmantes(firmantes.filter((_, j) => j !== i))}
                    >
                      <Icon name="close" size={14} />
                    </IconButton>
                  </span>
                )}
              </li>
            ))}
          </ol>
          {firmantes.length < MAX_FIRMANTES && (
            <Button
              type="button"
              variant="ghost"
              size="xs"
              class="self-start"
              leadingIcon={<Icon name="plus" size={14} />}
              onClick={() => cambiarFirmantes([...firmantes, ''])}
            >
              {t('pdf.dialog.pedirFirma.addSigner')}
            </Button>
          )}
        </FieldGroup>
        <FieldGroup label={t('pdf.dialog.pedirFirma.message')} htmlFor={idMensaje}>
          <Textarea
            id={idMensaje}
            rows={3}
            maxLength={1000}
            value={mensaje}
            onInput={(e) => setMensaje((e.target as HTMLTextAreaElement).value)}
          />
        </FieldGroup>
        <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <FieldGroup label={t('pdf.dialog.pedirFirma.expiryLabel')} htmlFor={idCaduca}>
            <Select
              id={idCaduca}
              value={String(caduca)}
              onChange={(e) => setCaduca(Number((e.target as HTMLSelectElement).value))}
            >
              {CADUCIDADES.map(([dias, clave]) => (
                <option key={dias} value={String(dias)}>
                  {t(clave)}
                </option>
              ))}
            </Select>
          </FieldGroup>
          <FieldGroup label={t('pdf.dialog.pedirFirma.remindLabel')} htmlFor={idRecordar}>
            <Select
              id={idRecordar}
              value={String(recordar)}
              onChange={(e) => setRecordar(Number((e.target as HTMLSelectElement).value))}
            >
              {RECORDATORIOS.map(([dias, clave]) => (
                <option key={dias} value={String(dias)}>
                  {t(clave)}
                </option>
              ))}
            </Select>
          </FieldGroup>
        </div>
        {error && (
          <FieldHint id={idError} tone="danger">
            {error}
          </FieldHint>
        )}
        {hecho && (
          <div role="status">
            <FieldHint tone="success">{hecho}</FieldHint>
          </div>
        )}
        {fuera !== null && (
          // `role="alert"` fuera del bloque: el aviso aparece al responder el servicio,
          // y quien no lo ve tiene que enterarse de que el botón ya no envía, comparte.
          <div id={idFuera} role="alert">
            <InlineHelp
              tone="info"
              title={t('pdf.dialog.pedirFirma.external.title', { name: fuera.name })}
            >
              <p class="text-pretty">{t('pdf.dialog.pedirFirma.external.help')}</p>
            </InlineHelp>
          </div>
        )}
        <div class="flex justify-end gap-2">
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={fuera !== null ? () => setFuera(null) : onCerrar}
          >
            {t('pdf.cancel')}
          </Button>
          <Button
            type="submit"
            size="sm"
            disabled={enviando || quienes.length === 0}
            aria-describedby={fuera !== null ? idFuera : undefined}
          >
            {enviando
              ? t('pdf.working')
              : fuera !== null
                ? t('pdf.dialog.pedirFirma.external.confirm')
                : t('pdf.dialog.pedirFirma.send')}
          </Button>
        </div>
      </form>

      <section class="border-border flex flex-col gap-2 border-t pt-3">
        <h3 class="text-text text-xs font-semibold">{t('pdf.dialog.pedirFirma.list')}</h3>
        {pedidas !== null && pedidas.length === 0 && (
          <p class="text-text-subtle text-xs">{t('pdf.dialog.pedirFirma.none')}</p>
        )}
        <ul class="flex flex-col gap-2">
          {(pedidas ?? []).map((grupo) => (
            <li key={grupo[0]?.id} class="flex flex-col gap-1">
              <ol class="flex flex-col gap-1">
                {grupo.map((p) => (
                  <Pedida key={p.id} t={t} p={p} enCadena={grupo.length > 1} accion={accion} />
                ))}
              </ol>
              {grupo[0]?.expires_at && (
                <p class="text-text-subtle text-xs">
                  {t('pdf.dialog.pedirFirma.expiresOn', {
                    date: fechaCorta(grupo[0].expires_at, documentLocale()),
                  })}
                </p>
              )}
            </li>
          ))}
        </ul>
      </section>
    </div>
  );
}

/** Una persona de las pedidas: quién, cómo va y lo que se puede hacer con ella. */
function Pedida({
  t,
  p,
  enCadena,
  accion,
}: {
  t: ReturnType<typeof docsT>;
  p: PeticionFirma;
  enCadena: boolean;
  accion: (fn: Promise<unknown>) => void;
}) {
  return (
    <li class="flex flex-wrap items-center justify-between gap-2 text-xs">
      <span class="text-text min-w-0 truncate">
        {enCadena && <span class="text-text-muted">{(p.step ?? 0) + 1}. </span>}
        {quienFirma(p, 'sent')}
      </span>
      <span class="flex items-center gap-2">
        <Badge tone={tonoDeEstado(p.status)}>{t(`pdf.signatureRequest.status.${p.status}`)}</Badge>
        {p.status === 'firmada' && p.external && !p.signed_path && (
          <Button
            type="button"
            variant="ghost"
            size="xs"
            onClick={() => accion(firmas.traer(p.id))}
          >
            {t('pdf.dialog.pedirFirma.fetchSigned')}
          </Button>
        )}
        {p.status === 'firmada' && p.signed_path && (
          <Button
            variant="ghost"
            size="xs"
            href={r(`/pdf?path=${encodeURIComponent(p.signed_path)}`)}
          >
            {t('pdf.dialog.pedirFirma.openSigned')}
          </Button>
        )}
        {/* Cancelar la del turno cancela también las que esperan. */}
        {p.status === 'pendiente' && (
          <Button
            type="button"
            variant="ghost"
            size="xs"
            onClick={() => accion(firmas.cancelar(p.id))}
          >
            {t('pdf.dialog.pedirFirma.cancel')}
          </Button>
        )}
      </span>
    </li>
  );
}
