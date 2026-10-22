/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El contrato con el servicio Go. Es el ÚNICO sitio del frontend que sabe algo
 * de la forma del ERP, y aun así no sabe nada de ningún módulo concreto: los
 * módulos, sus campos y sus rótulos llegan traducidos en `/api/catalogo`.
 */

/** Tipos de campo que sabe pintar la ficha. Espejo de internal/catalogo. */
export type TipoCampo =
  | 'texto'
  | 'texto_largo'
  | 'entero'
  | 'decimal'
  | 'dinero'
  | 'fecha'
  | 'fechahora'
  | 'booleano'
  | 'seleccion'
  | 'relacion'
  | 'correo'
  | 'telefono'
  | 'url';

export interface Opcion {
  valor: string;
  etiqueta: string;
}

export interface Campo {
  clave: string;
  tipo: TipoCampo;
  /** Ya traducido por el servicio al idioma de la petición. */
  etiqueta: string;
  requerido?: boolean;
  /** Modelo apuntado (solo `relacion`). */
  modelo?: string;
  opciones?: Opcion[];
}

export interface Modelo {
  clave: string;
  modulo: string;
  etiqueta: string;
  numerado: boolean;
  contable: boolean;
  /** El documento se EMITE (factura de venta): se puede sacar como UBL. */
  cobro?: boolean;
  /** El documento se RECIBE de un proveedor: entra desde su factura electrónica. */
  recibe?: boolean;
  campos: Campo[];
  lista: string[];
  estados?: Opcion[];
}

export interface Modulo {
  clave: string;
  etiqueta: string;
  dependencias?: string[];
  modelos: Modelo[];
}

export interface RespuestaActivarModulo {
  activos: string[];
  activados?: string[];
  desactivados?: string[];
}

export interface Catalogo {
  modulos: Modulo[];
  activos: string[] | null;
  idioma: string;
}

/**
 * Lo que puede haber en `data`. No es `unknown`: los tipos de campo del catálogo
 * son cerrados, y declararlo permite formatear sin castear en cada celda.
 */
export type ValorCampo = string | number | boolean | null;

export interface Registro {
  id: string;
  modelo: string;
  ref?: string;
  etiqueta: string;
  estado?: string;
  data: Record<string, ValorCampo>;
  creado_en: string;
  editado_en: string;
}

export interface Pagina {
  registros: Registro[] | null;
  /** Nombre legible de cada id referenciado en la página (lo resuelve el servicio). */
  etiquetas?: Record<string, string>;
  siguiente?: string;
}

/** Un apunte del libro. Importes en CÉNTIMOS enteros, como en el servicio. */
export interface Apunte {
  cuenta: string;
  concepto?: string;
  debe: number;
  haber: number;
  tercero?: string;
}

/** Un asiento por partida doble. */
export interface Asiento {
  id: string;
  diario: string;
  ref: string;
  fecha: string;
  concepto: string;
  moneda: string;
  estado: string;
  origen_id?: string;
  apuntes?: Apunte[] | null;
}

/** Una fila del sumas y saldos. */
export interface SaldoCuenta {
  cuenta: string;
  debe: number;
  haber: number;
  saldo: number;
}

export interface RespuestaBalance {
  cuentas: SaldoCuenta[] | null;
  total_debe: number;
  total_haber: number;
}

export interface ResumenFacturacion {
  total_centimos: number;
  total_registros: number;
  pendiente_cobro_centimos: number;
  facturas_pendientes: number;
  proximo_vencimiento?: string;
}

export interface ResumenContabilidad {
  total_asientos: number;
  total_apuntes: number;
  total_debe: number;
  total_haber: number;
  cuadra: boolean;
}

export interface ResumenOperativo {
  facturacion: ResumenFacturacion;
  contabilidad: ResumenContabilidad;
  contactos: number;
}

export interface EstadisticasModelo {
  modelo: string;
  total: number;
  por_estado: Record<string, number>;
  suma_dinero?: number;
}

// --- Perfil de la organización y obligaciones -------------------------------

export interface Organizacion {
  nombre: string;
  nif: string;
  direccion: string;
  cp: string;
  ciudad: string;
  pais: string;
  forma: string;
  condiciones: Record<string, boolean>;
  actualizado_en?: string;
}

export interface FormaJuridica {
  clave: string;
  nombre: string;
  tipo: 'persona_fisica' | 'sociedad';
}

export interface PaisInfo {
  pais: string;
  verificado: boolean;
  moneda: string;
  formas: FormaJuridica[];
  portales: Record<string, { nombre: string; url: string }>;
  obligaciones: number;
}

export interface RespuestaPaises {
  paises: PaisInfo[];
  condiciones: string[];
}

export interface Sancion {
  tipo: 'recargo' | 'fija' | 'nota';
  recargo_base_pct?: number;
  recargo_pct_mes?: number;
  recargo_max_pct?: number;
  tras_meses?: number;
  con_requerimiento_pct?: number[];
  fija_centimos?: number;
  moneda?: string;
  nota?: string;
}

export interface Obligacion {
  proceso?: { fase: number; referencia: string };
  clave: string;
  nombre: string;
  concepto: string;
  concepto_rotulo: string;
  ambito: string;
  ambito_rotulo: string;
  periodicidad: 'mensual' | 'trimestral' | 'anual';
  periodo: string;
  limite: string;
  recomendada: string;
  portal: { nombre: string; url: string };
  sancion?: Sancion;
  calculable: boolean;
  /** Último día para presentar domiciliando el pago, si se puede domiciliar. */
  limite_domiciliacion?: string;
  guia?: Guia;
  /** Lo que falta en el ERP para presentarla: movimientos, facturas, nóminas. */
  pendientes?: Partial<Record<Preparacion, number>>;
  /** El año aún no tiene publicados sus días inhábiles oficiales. */
  calendario_provisional?: boolean;
  /** Ficheros que conviene subir al presentarla, con lo que tienen que traer. */
  documentos?: { clave: string; nombre: string; requisitos: string[] }[];
}

/** Qué es una obligación, qué hace falta, cómo se hace y qué pasa si no llega. */
export interface Guia {
  que: string;
  necesitas: string[];
  pasos: string[];
  si_no: string;
}

export type Preparacion = 'banco' | 'facturas' | 'nominas';

export interface RespuestaObligaciones {
  anio: number;
  pais: string;
  verificado: boolean;
  moneda: string;
  region?: string;
  /** Aviso si la región tiene hacienda propia para algunos impuestos. */
  territorio?: { nombre: string; nota: string };
  obligaciones: Obligacion[] | null;
}

/** Una factura sin cobrar o sin pagar que podría ser la de un movimiento. */
export interface FacturaCandidata {
  id: string;
  modelo: 'factura_venta' | 'factura_compra';
  ref: string;
  estado: string;
  tercero: string;
  fecha: string;
  total: number;
  /** No hay duda: el concepto del banco la nombra y no hay otra igual. */
  segura: boolean;
}

export interface MovimientoBanco {
  id: string;
  fecha: string;
  descripcion: string;
  /** Céntimos: positivo si entra dinero, negativo si sale. */
  importe: number;
  factura_id?: string;
  factura_modelo?: string;
  factura_ref?: string;
  motivo?: string;
  /** Pista de Sytue AI: quién lo cobró y dónde conseguir su factura. */
  empresa?: string;
  web?: string;
  url_factura?: string;
  email_facturas?: string;
}

/** Nivel de una persona en un módulo, de menos a más. */
export type NivelPermiso = 'ninguno' | 'lectura' | 'usuario' | 'administrador';

export interface MisPermisos {
  verbos?: Record<string, string[]>;
  administrador: boolean;
  modulos: Record<string, NivelPermiso>;
  /** Módulos en los que ya ha pedido editar (la franja dice «Petición enviada»). */
  pedidos?: string[];
}

/** Tarea con vencimiento sobre un registro (las «actividades» de Odoo). */
export interface Actividad {
  id: string;
  modelo: string;
  registro: string;
  registro_etiqueta?: string;
  tipo: 'subir_documento' | 'pedir_documento' | 'llamada' | 'correo' | 'reunion' | 'tarea';
  resumen: string;
  notas?: string;
  vence: string;
  responsable_sub?: string;
  responsable_nombre?: string;
  estado: 'pendiente' | 'hecha' | 'cancelada';
  creada_en: string;
  hecha_en?: string;
}

export interface Adjunto {
  id: string;
  modelo: string;
  registro: string;
  nombre: string;
  tipo: string;
  tamano: number;
  creado_en: string;
}

/** Qué tiene que tener una factura para valer en el país. */
export interface RequisitosFactura {
  fuente?: string;
  completa?: string[];
  simplificada?: string[];
  extranjera?: string[];
}

/** Una línea del historial de un registro (GET /{modelo}/{id}/historial). */
export interface EntradaHistorial {
  id: string;
  tipo: 'creado' | 'cambio' | 'borrado' | 'restaurado' | 'nota';
  sub?: string;
  nombre?: string;
  /** Campo (o `estado`) con su valor de antes y el de después. */
  cambios?: Record<string, { antes: ValorCampo; despues: ValorCampo }>;
  texto?: string;
  creado_en: string;
}

/** Por qué una organización no puede activar VERI*FACTU (GET /verifactu). */
export type MotivoVerifactu =
  'sin_perfil' | 'pais' | 'foral' | 'igic_ipsi' | 'nif' | 'sin_certificado' | 'declaracion';

/** VERI*FACTU en la organización y cómo van sus envíos a la Agencia Tributaria. */
export interface EstadoVerifactu {
  activo: boolean;
  desde?: string;
  pendientes: number;
  con_errores: number;
  rechazadas: number;
  error_envio?: string;
  motivo?: MotivoVerifactu;
  entorno: 'pruebas' | 'produccion';
}

/** Un registro de facturación de una factura y lo que contestó la AEAT. */
export interface RegistroVerifactu {
  id: string;
  tipo: 'alta' | 'anulacion';
  num_serie: string;
  estado: 'pendiente' | 'correcto' | 'aceptado_con_errores' | 'incorrecto';
  csv?: string;
  codigo_error?: number;
  descripcion_error?: string;
  generado_en: string;
  enviado_en?: string;
}

/** Un paso del plan que propone Sytue AI al pedirle que cree algo. */
export interface PasoPlan {
  ref: string;
  modelo: string;
  etiqueta?: string;
  existente?: { id: string; etiqueta: string };
  data: Record<string, ValorCampo>;
  enlaces?: Record<string, string>;
  etiquetas?: Record<string, string>;
  faltan?: string[];
  avisos?: string[];
  tarea?: string;
}

export interface PlanIA {
  resumen: string;
  avisos?: string[];
  pasos: PasoPlan[];
}

export interface TareaBanco extends MovimientoBanco {
  sugerencia?: string;
  candidatas: FacturaCandidata[];
}

export interface RespuestaBanco {
  pendientes: TareaBanco[];
  total_pendientes: number;
  hechas: MovimientoBanco[];
  motivos: { clave: string; rotulo: string }[];
}

export interface ResultadoN43 {
  cuentas: string[];
  nuevos: number;
  repetidos: number;
  casados: number;
  explicados: number;
  pendientes: number;
}

export interface CalculoObligacion {
  clave: string;
  periodo: string;
  desde: string;
  hasta: string;
  tipo: string;
  porcentaje: number;
  haber_centimos: number;
  debe_centimos: number;
  resultado_centimos: number;
  asentado: boolean;
}

export interface ResultadoImportacion {
  filas: number;
  creados: number;
  actualizados: number;
  ignoradas: string[];
  fallos: { fila: number; error: string }[];
}

/** Un informe de Sytue AI: un tablero o un Excel. Espejo de internal/ia. */
export interface Informe {
  titulo: string;
  partes: ParteInforme[];
}

export interface ParteInforme {
  titulo: string;
  vista: 'cifra' | 'barras' | 'tabla';
  /** Qué sale de qué fuente. La interfaz no la lee: la ejecuta el servicio. */
  consulta: Record<string, unknown>;
}

export interface ColumnaInforme {
  clave: string;
  tipo: 'texto' | 'numero' | 'dinero' | 'fecha' | 'booleano';
  etiqueta: string;
}

/** Las filas de una parte. Los importes llegan en UNIDADES, no en céntimos. */
export interface TablaInforme {
  columnas: ColumnaInforme[] | null;
  filas: (string | number | boolean | null)[][] | null;
  recortada?: boolean;
  error?: 'sin_permiso' | 'no_disponible' | 'invalida';
  /** De dónde salen los datos cuando vienen de una vista de Sytue Bases. */
  procedencia?: {
    origen: string;
    base: string;
    tabla: string;
    vista: string;
    version: number;
    concedida_por: string;
    leida_el: string;
  };
}

export interface Tablero {
  id: string;
  titulo: string;
  informe: Informe;
  creado_por: string;
  creado_en: string;
}
