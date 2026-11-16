/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El cliente del BFF de esta app. Una función, porque la API del servicio es una
 * sola: `/api/v1/{modelo}` sirve a los treinta y seis modelos.
 *
 * El prefijo `/erp` lo pone el shim de `basePath` (@sytue/ui), así que aquí las
 * rutas se escriben desde `/api/` y nunca con el prefijo a mano.
 */
import type { ErpMessageKey } from '../i18n/index.js';

/** Error del servicio con su estado, para que la interfaz pueda distinguirlos. */
export class ErrorApi extends Error {
  readonly estado: number;
  /**
   * Qué campo falló, cuando el servicio lo dice. El 422 de validación trae
   * `details.fields` —el `validation.Errors` de la plataforma— y sin recogerlo
   * aquí la ficha solo puede pintar una frase única arriba del formulario.
   */
  readonly porCampo: Record<string, string>;
  /** El `code` del error de la plataforma (`sin_permiso`…): lo que se traduce. */
  readonly codigo: string;

  constructor(mensaje: string, estado: number, porCampo: Record<string, string> = {}, codigo = '') {
    super(mensaje);
    this.name = 'ErrorApi';
    this.estado = estado;
    this.porCampo = porCampo;
    this.codigo = codigo;
  }
}

/** El módulo existe, pero la administración no le ha dado acceso a esta persona. */
export function esSinPermiso(err: unknown): boolean {
  return err instanceof ErrorApi && err.codigo === 'sin_permiso';
}

/**
 * Códigos del servicio —los de `services/syerp/internal/http` y los canónicos de
 * `platform/pkg/httpx`— traducidos a lo que le pasa a quien está delante.
 *
 * La plataforma manda `code` y un RESPALDO EN INGLÉS, y «el cliente traduce por
 * Code». Solo se traducían tres y el resto salía en inglés en mitad de una pantalla
 * en español: «That accounting period is closed…». Un código que falte aquí ya no
 * enseña esa frase, dice «algo ha ido mal»; por eso el test recorre la lista del
 * servicio y exige que cada código tenga su fila.
 *
 * La tabla de Fyne (`apps/fyne/apps/erp/acciones.go`) es esta misma, y su test la
 * compara con esta fila a fila.
 *
 * Fuera a propósito: `internal` y `method_not_allowed` (no hay nada que contarle a
 * la persona: «algo ha ido mal» es lo cierto), y `sin_verbo_<verbo>`, que es una
 * familia y se resuelve por prefijo en `claveDeError`.
 */
export const ERRORES: Record<string, ErpMessageKey> = {
  // Acceso.
  sin_permiso: 'error.sinPermiso',
  cuatro_ojos: 'error.code.fourEyes',
  // VERI*FACTU.
  certificado_invalido: 'vf.error.invalidCertificate',
  certificado_nif_no_coincide: 'vf.error.certificateTaxIdMismatch',
  certificado_contrasena: 'vf.error.wrongPassword',
  verifactu_no_disponible: 'error.code.verifactuUnavailable',
  factura_emitida: 'error.code.invoiceIssued',
  verifactu_pendiente: 'error.code.verifactuPending',
  nada_que_subsanar: 'error.code.nothingToCorrect',
  nif_con_verifactu: 'error.code.taxIdWithVerifactu',
  // El libro y los documentos.
  periodo_cerrado: 'error.code.periodClosed',
  cierre_hacia_atras: 'error.code.closeBackwards',
  requiere_aprobacion: 'error.code.approvalRequired',
  aprobacion_resuelta: 'error.code.approvalResolved',
  modelo_desconocido: 'error.code.unknownModel',
  estado_invalido: 'error.code.invalidStatus',
  no_contable: 'error.code.notPostable',
  ya_asentado: 'error.code.alreadyPosted',
  referencia_duplicada: 'error.code.duplicateReference',
  accion_invalida: 'error.code.invalidAction',
  sin_ids: 'error.code.noIds',
  ids_invalidos: 'error.code.invalidIds',
  factura_ilegible: 'error.code.unreadableInvoice',
  modules_disabled: 'error.code.modulesDisabled',
  organizacion_no_configurada: 'error.code.orgNotConfigured',
  no_calculable: 'error.code.notComputable',
  // Adjuntos, importación y conectores.
  adjunto_tipo: 'error.code.attachmentType',
  adjunto_grande: 'error.code.attachmentTooLarge',
  csv_invalido: 'error.code.invalidCsv',
  csv_sin_columnas: 'error.code.csvNoColumns',
  importacion_no_simulable: 'error.code.importNotSimulable',
  ya_revertida: 'error.code.alreadyReverted',
  conector_desconocido: 'error.code.unknownConnector',
  conexion_bases_invalida: 'error.code.invalidBasesConnection',
  // Banco.
  ya_justificado: 'error.code.alreadyJustified',
  no_justificado: 'error.code.notJustified',
  factura_no_abierta: 'error.code.invoiceNotOpen',
  sentido_factura: 'error.code.invoiceDirection',
  motivo_desconocido: 'error.code.unknownReason',
  // Sytue AI.
  ia_no_disponible: 'error.code.aiUnavailable',
  ia_limite: 'error.code.aiLimit',
  ia_sin_informe: 'error.code.aiNoReport',
  ia_sin_plan: 'error.code.aiNoPlan',
  informe_invalido: 'error.code.invalidReport',
  informe_ocupado: 'error.code.reportBusy',
  pdf_sin_texto: 'error.code.pdfNoText',
  // Enviar la factura por correo.
  correo_no_configurado: 'send.unavailable',
  factura_no_enviable: 'send.notIssued',
  destinatario_invalido: 'send.invalidTo',
  correo_no_enviado: 'send.failed',
  // Los canónicos de la plataforma. Su respaldo en inglés a veces es concreto
  // («hasta: expected a date…»), pero sigue siendo inglés: se dice lo que pasa.
  bad_request: 'error.code.badRequest',
  empty_body: 'error.code.badRequest',
  invalid_json: 'error.code.badRequest',
  multiple_json_objects: 'error.code.badRequest',
  unsupported_media_type: 'error.code.badRequest',
  body_too_large: 'error.code.tooLarge',
  unauthorized: 'error.code.sessionExpired',
  // El BFF contesta así cuando la cookie de sesión ya no vale.
  unauthenticated: 'error.code.sessionExpired',
  forbidden: 'error.code.forbidden',
  no_active_organization: 'error.code.noActiveOrg',
  not_found: 'error.code.notFound',
  unprocessable: 'error.code.unprocessable',
  too_many_requests: 'error.code.tooManyRequests',
  unavailable: 'error.offline',
  // El BFF, cuando el servicio no contesta (502).
  upstream_unavailable: 'error.offline',
};

/** El rol alcanza el módulo pero no el verbo: `sin_verbo_asentar`, `sin_verbo_conciliar`… */
const PREFIJO_SIN_VERBO = 'sin_verbo_';

/**
 * La clave de catálogo de un error del servicio.
 *
 * Un 422 de validación trae los fallos campo a campo (`porCampo`), que la ficha
 * pinta debajo de cada uno: arriba basta con mandar a mirarlos, y repetir allí la
 * frase única —en inglés— era lo que se hacía antes de tener los detalles.
 */
export function claveDeError(err: ErrorApi): ErpMessageKey {
  if (err.codigo.startsWith(PREFIJO_SIN_VERBO)) return 'error.code.actionNotAllowed';
  if (err.codigo === 'unprocessable' && Object.keys(err.porCampo).length > 0) {
    return 'error.code.checkFields';
  }
  const clave = ERRORES[err.codigo];
  if (clave) return clave;
  // Sin código —el 401 que el BFF corta antes de llegar al servicio, un `HTTP 502`
  // del proxy—, el estado es lo único que hay.
  if (!err.codigo && err.estado === 401) return 'error.code.sessionExpired';
  return 'error.generic';
}

/**
 * Lo que se le dice a la persona cuando una llamada falla.
 *
 * El mensaje del servicio es un respaldo en inglés y no se enseña NUNCA: quien no
 * tenía acceso a un módulo leía «You do not have access to this module…» en mitad
 * de la interfaz en español (medido en producción, 2026-09-21). Lo que el ERP sabe
 * traducir se traduce aquí, una vez, para los treinta sitios que avisan.
 *
 * Un `Error` que no viene del servicio sí conserva su texto: lo lanzan las propias
 * pantallas, ya traducido (`throw new Error(t('pos.resume'))`). El corte de red, no.
 */
export function mensajeDeError(err: unknown, t: (clave: ErpMessageKey) => string): string {
  if (err instanceof ErrorApi) return t(claveDeError(err));
  // Un `fetch` que no llega a ninguna parte rechaza con un `TypeError` cuyo texto
  // escribe el navegador, en inglés («Failed to fetch»): eso tampoco se enseña.
  if (err instanceof TypeError) return t('error.generic');
  return err instanceof Error && err.message ? err.message : t('error.generic');
}

/** El error de `platform/pkg/httpx`, dentro de su sobre `{ error: … }`. */
interface CuerpoDeError {
  code?: string;
  message?: string;
  details?: { fields?: { field: string; message: string }[] };
}

export async function api<T>(ruta: string, init?: RequestInit): Promise<T> {
  // `Headers` y no un objeto con spread: `HeadersInit` admite también un array de
  // pares, y esparcirlo produciría claves numéricas en vez de cabeceras.
  const headers = new Headers(init?.headers);
  // JSON salvo que quien llama diga otra cosa: la importación manda `text/csv`.
  if (!headers.has('content-type')) headers.set('content-type', 'application/json');
  const resp = await fetch(`/api/${ruta}`, { ...init, headers });
  if (!resp.ok) {
    const cuerpo = (await resp.json().catch(() => null)) as {
      error?: string | CuerpoDeError;
    } | null;
    // El servicio manda un objeto; el BFF, cuando corta antes, el código a secas
    // (`{ error: 'unauthenticated' }`). Sin leer los dos, ese 401 llegaba sin código.
    const error: CuerpoDeError | undefined =
      typeof cuerpo?.error === 'string' ? { code: cuerpo.error } : cuerpo?.error;
    const campos = Object.fromEntries(
      (error?.details?.fields ?? []).map((f) => [f.field, f.message]),
    );
    throw new ErrorApi(
      error?.message ?? `HTTP ${resp.status}`,
      resp.status,
      campos,
      error?.code ?? '',
    );
  }
  if (resp.status === 204) return undefined as T;
  return (await resp.json()) as T;
}

/** Construye `?a=b&c=d` saltándose lo vacío. */
export function consulta(params: Record<string, string | number | undefined>): string {
  const q = new URLSearchParams();
  for (const [clave, valor] of Object.entries(params)) {
    if (valor !== undefined && valor !== '') q.set(clave, String(valor));
  }
  const s = q.toString();
  return s ? `?${s}` : '';
}

/**
 * El prefijo bajo el que se sirve esta app, tal y como lo publicó el servidor.
 * `basePathScript` lo deja en `window.__sytueBase`; el respaldo es el del build.
 */
export function baseDeLaApp(): string {
  return (globalThis as { __sytueBase?: string }).__sytueBase ?? '/erp';
}
