/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Catálogo de FIRMAS de funciones (paridad Google Sheets/Excel) para el hint de
 * argumentos y la descripción del autocompletado del editor de fórmulas. El
 * motor @sytue/formula NO lleva metadatos por función (solo nombres): este mapa
 * los aporta desde el carril FORMULAS-UX. Clave = nombre EN INGLÉS tal y como lo
 * registra el motor (lo que el usuario teclea, p. ej. «SUM»); las etiquetas de
 * argumento y la descripción van en español, separador «;» (convención es-ES).
 *
 * Cobertura RICA de las ~130 funciones más usadas. Las que no estén aquí siguen
 * autocompletándose por nombre (registro en vivo) pero sin hint de firma.
 * Puro (datos + 2 helpers). Self-test: packages/office/test/formula-edit.test.ts.
 */

/** Firma de una función para el hint de argumentos. */
export interface FnSignature {
  /** Firma completa, p. ej. «SUM(número1; [número2]; …)». `[x]` opcional, `…` repite. */
  args: string;
  /** Descripción breve de una línea. */
  summary: string;
}

/* Mapa NOMBRE → firma. Orden por familias (solo para leerlo; el acceso es por clave). */
export const FORMULA_CATALOG: Record<string, FnSignature> = {
  /* ---- Matemáticas y agregados ---- */
  SUM: {
    args: 'SUM(número1; [número2]; …)',
    summary: 'Suma todos los números de un conjunto de valores o rangos.',
  },
  SUBTOTAL: {
    args: 'SUBTOTAL(función; ref1; [ref2]; …)',
    summary:
      'Resume un rango (1 media, 2 cuenta, 9 suma…) sin contar otros subtotales ni las filas filtradas; con 101-111, tampoco las ocultas.',
  },
  AGGREGATE: {
    args: 'AGGREGATE(función; opciones; ref1; …)',
    summary:
      'Como SUBTOTAL, con 19 funciones y opciones para saltar errores, filas ocultas y subtotales anidados.',
  },
  SUMIF: {
    args: 'SUMIF(rango; criterio; [rango_suma])',
    summary: 'Suma las celdas que cumplen un criterio.',
  },
  SUMIFS: {
    args: 'SUMIFS(rango_suma; rango_criterios1; criterio1; …)',
    summary: 'Suma las celdas que cumplen varios criterios.',
  },
  SUMPRODUCT: {
    args: 'SUMPRODUCT(matriz1; [matriz2]; …)',
    summary: 'Multiplica los componentes de las matrices y suma los productos.',
  },
  PRODUCT: {
    args: 'PRODUCT(número1; [número2]; …)',
    summary: 'Multiplica todos los números indicados.',
  },
  SUMSQ: {
    args: 'SUMSQ(número1; [número2]; …)',
    summary: 'Suma de los cuadrados de los argumentos.',
  },
  MOD: { args: 'MOD(dividendo; divisor)', summary: 'Resto de una división.' },
  QUOTIENT: { args: 'QUOTIENT(dividendo; divisor)', summary: 'Parte entera de una división.' },
  POWER: { args: 'POWER(base; exponente)', summary: 'Eleva un número a una potencia.' },
  SQRT: { args: 'SQRT(número)', summary: 'Raíz cuadrada de un número.' },
  ABS: { args: 'ABS(número)', summary: 'Valor absoluto de un número.' },
  SIGN: { args: 'SIGN(número)', summary: 'Signo de un número: 1, 0 o -1.' },
  INT: { args: 'INT(número)', summary: 'Redondea hacia abajo al entero más cercano.' },
  TRUNC: {
    args: 'TRUNC(número; [decimales])',
    summary: 'Trunca un número a los decimales indicados.',
  },
  ROUND: {
    args: 'ROUND(número; decimales)',
    summary: 'Redondea un número a los decimales indicados.',
  },
  ROUNDUP: { args: 'ROUNDUP(número; decimales)', summary: 'Redondea alejándose de cero.' },
  ROUNDDOWN: { args: 'ROUNDDOWN(número; decimales)', summary: 'Redondea acercándose a cero.' },
  MROUND: { args: 'MROUND(número; múltiplo)', summary: 'Redondea al múltiplo más cercano.' },
  CEILING: {
    args: 'CEILING(número; múltiplo)',
    summary: 'Redondea hacia arriba al múltiplo indicado.',
  },
  FLOOR: { args: 'FLOOR(número; múltiplo)', summary: 'Redondea hacia abajo al múltiplo indicado.' },
  GCD: { args: 'GCD(número1; [número2]; …)', summary: 'Máximo común divisor.' },
  LCM: { args: 'LCM(número1; [número2]; …)', summary: 'Mínimo común múltiplo.' },
  EXP: { args: 'EXP(número)', summary: 'e elevado a la potencia indicada.' },
  LN: { args: 'LN(número)', summary: 'Logaritmo natural (base e).' },
  LOG: { args: 'LOG(número; [base])', summary: 'Logaritmo en la base indicada (10 por defecto).' },
  LOG10: { args: 'LOG10(número)', summary: 'Logaritmo en base 10.' },
  PI: { args: 'PI()', summary: 'Valor de π.' },
  RAND: { args: 'RAND()', summary: 'Número aleatorio entre 0 y 1.' },
  RANDBETWEEN: {
    args: 'RANDBETWEEN(inferior; superior)',
    summary: 'Entero aleatorio entre dos límites.',
  },
  FACT: { args: 'FACT(número)', summary: 'Factorial de un número.' },
  COMBIN: { args: 'COMBIN(total; elegidos)', summary: 'Número de combinaciones sin repetición.' },
  PERMUT: { args: 'PERMUT(total; elegidos)', summary: 'Número de permutaciones sin repetición.' },
  DEGREES: { args: 'DEGREES(radianes)', summary: 'Convierte radianes en grados.' },
  RADIANS: { args: 'RADIANS(grados)', summary: 'Convierte grados en radianes.' },
  SIN: { args: 'SIN(ángulo)', summary: 'Seno de un ángulo en radianes.' },
  COS: { args: 'COS(ángulo)', summary: 'Coseno de un ángulo en radianes.' },
  TAN: { args: 'TAN(ángulo)', summary: 'Tangente de un ángulo en radianes.' },
  ATAN2: { args: 'ATAN2(x; y)', summary: 'Arcotangente de las coordenadas x e y.' },

  /* ---- Estadística ---- */
  AVERAGE: {
    args: 'AVERAGE(número1; [número2]; …)',
    summary: 'Promedio (media aritmética) de los argumentos.',
  },
  AVERAGEA: {
    args: 'AVERAGEA(valor1; [valor2]; …)',
    summary: 'Promedio incluyendo texto (0) y lógicos.',
  },
  AVERAGEIF: {
    args: 'AVERAGEIF(rango; criterio; [rango_promedio])',
    summary: 'Promedio de las celdas que cumplen un criterio.',
  },
  AVERAGEIFS: {
    args: 'AVERAGEIFS(rango_promedio; rango_criterios1; criterio1; …)',
    summary: 'Promedio de las celdas que cumplen varios criterios.',
  },
  COUNT: {
    args: 'COUNT(valor1; [valor2]; …)',
    summary: 'Cuenta cuántas celdas contienen números.',
  },
  COUNTA: {
    args: 'COUNTA(valor1; [valor2]; …)',
    summary: 'Cuenta cuántas celdas no están vacías.',
  },
  COUNTBLANK: { args: 'COUNTBLANK(rango)', summary: 'Cuenta las celdas vacías de un rango.' },
  COUNTIF: {
    args: 'COUNTIF(rango; criterio)',
    summary: 'Cuenta las celdas que cumplen un criterio.',
  },
  COUNTIFS: {
    args: 'COUNTIFS(rango_criterios1; criterio1; …)',
    summary: 'Cuenta las celdas que cumplen varios criterios.',
  },
  MAX: { args: 'MAX(número1; [número2]; …)', summary: 'Mayor valor de un conjunto.' },
  MAXIFS: {
    args: 'MAXIFS(rango_máx; rango_criterios1; criterio1; …)',
    summary: 'Mayor valor que cumple varios criterios.',
  },
  MIN: { args: 'MIN(número1; [número2]; …)', summary: 'Menor valor de un conjunto.' },
  MINIFS: {
    args: 'MINIFS(rango_mín; rango_criterios1; criterio1; …)',
    summary: 'Menor valor que cumple varios criterios.',
  },
  MEDIAN: {
    args: 'MEDIAN(número1; [número2]; …)',
    summary: 'Mediana (valor central) de los argumentos.',
  },
  MODE: { args: 'MODE(número1; [número2]; …)', summary: 'Valor más frecuente de un conjunto.' },
  LARGE: { args: 'LARGE(rango; k)', summary: 'k-ésimo valor mayor de un rango.' },
  SMALL: { args: 'SMALL(rango; k)', summary: 'k-ésimo valor menor de un rango.' },
  RANK: {
    args: 'RANK(número; rango; [orden])',
    summary: 'Posición de un número dentro de una lista.',
  },
  STDEV: { args: 'STDEV(número1; [número2]; …)', summary: 'Desviación estándar de una muestra.' },
  'STDEV.P': {
    args: 'STDEV.P(número1; [número2]; …)',
    summary: 'Desviación estándar de una población.',
  },
  'STDEV.S': {
    args: 'STDEV.S(número1; [número2]; …)',
    summary: 'Desviación estándar de una muestra.',
  },
  VAR: { args: 'VAR(número1; [número2]; …)', summary: 'Varianza de una muestra.' },
  'VAR.P': { args: 'VAR.P(número1; [número2]; …)', summary: 'Varianza de una población.' },
  'VAR.S': { args: 'VAR.S(número1; [número2]; …)', summary: 'Varianza de una muestra.' },
  PERCENTILE: { args: 'PERCENTILE(rango; k)', summary: 'Percentil k (0..1) de un rango.' },
  QUARTILE: { args: 'QUARTILE(rango; cuartil)', summary: 'Cuartil (0..4) de un rango.' },
  CORREL: {
    args: 'CORREL(rango1; rango2)',
    summary: 'Coeficiente de correlación entre dos rangos.',
  },
  FORECAST: {
    args: 'FORECAST(x; y_conocidos; x_conocidos)',
    summary: 'Predice un valor por regresión lineal.',
  },
  SLOPE: {
    args: 'SLOPE(y_conocidos; x_conocidos)',
    summary: 'Pendiente de la recta de regresión.',
  },
  INTERCEPT: {
    args: 'INTERCEPT(y_conocidos; x_conocidos)',
    summary: 'Ordenada en el origen de la regresión.',
  },
  TREND: {
    args: 'TREND(y_conocidos; [x_conocidos]; [x_nuevos])',
    summary: 'Valores ajustados por regresión lineal.',
  },

  /* ---- Lógicas ---- */
  IF: {
    args: 'IF(condición; valor_si_verdadero; [valor_si_falso])',
    summary: 'Devuelve un valor u otro según una condición.',
  },
  IFS: {
    args: 'IFS(condición1; valor1; [condición2; valor2]; …)',
    summary: 'Comprueba varias condiciones y devuelve el primer valor que cumple.',
  },
  IFERROR: {
    args: 'IFERROR(valor; valor_si_error)',
    summary: 'Devuelve otro valor si el primero es un error.',
  },
  IFNA: { args: 'IFNA(valor; valor_si_na)', summary: 'Devuelve otro valor si el primero es #N/A.' },
  AND: { args: 'AND(lógico1; [lógico2]; …)', summary: 'VERDADERO si todos los argumentos lo son.' },
  OR: {
    args: 'OR(lógico1; [lógico2]; …)',
    summary: 'VERDADERO si alguno de los argumentos lo es.',
  },
  NOT: { args: 'NOT(lógico)', summary: 'Invierte el valor lógico del argumento.' },
  XOR: { args: 'XOR(lógico1; [lógico2]; …)', summary: 'O exclusivo de los argumentos lógicos.' },
  SWITCH: {
    args: 'SWITCH(expresión; caso1; valor1; [caso2; valor2]; …; [predeterminado])',
    summary: 'Compara una expresión con varios casos.',
  },
  TRUE: { args: 'TRUE()', summary: 'Valor lógico VERDADERO.' },
  FALSE: { args: 'FALSE()', summary: 'Valor lógico FALSO.' },

  /* ---- Texto ---- */
  CONCAT: {
    args: 'CONCAT(texto1; [texto2]; …)',
    summary: 'Une el texto de varios valores o rangos.',
  },
  CONCATENATE: {
    args: 'CONCATENATE(texto1; [texto2]; …)',
    summary: 'Une varios textos en uno solo.',
  },
  TEXTJOIN: {
    args: 'TEXTJOIN(delimitador; ignorar_vacías; texto1; …)',
    summary: 'Une textos con un delimitador entre ellos.',
  },
  TEXT: {
    args: 'TEXT(valor; formato)',
    summary: 'Convierte un número en texto con un formato dado.',
  },
  LEFT: { args: 'LEFT(texto; [nº_caracteres])', summary: 'Primeros caracteres de un texto.' },
  RIGHT: { args: 'RIGHT(texto; [nº_caracteres])', summary: 'Últimos caracteres de un texto.' },
  MID: {
    args: 'MID(texto; inicio; nº_caracteres)',
    summary: 'Extrae caracteres desde una posición.',
  },
  LEN: { args: 'LEN(texto)', summary: 'Número de caracteres de un texto.' },
  FIND: {
    args: 'FIND(buscar; dentro_de; [inicio])',
    summary: 'Posición de un texto dentro de otro (distingue mayúsculas).',
  },
  SEARCH: {
    args: 'SEARCH(buscar; dentro_de; [inicio])',
    summary: 'Posición de un texto dentro de otro (sin distinguir mayúsculas).',
  },
  SUBSTITUTE: {
    args: 'SUBSTITUTE(texto; buscar; reemplazar; [nº_aparición])',
    summary: 'Sustituye un texto por otro.',
  },
  REPLACE: {
    args: 'REPLACE(texto; inicio; nº_caracteres; nuevo)',
    summary: 'Reemplaza parte de un texto por posición.',
  },
  REPT: { args: 'REPT(texto; veces)', summary: 'Repite un texto un número de veces.' },
  TRIM: { args: 'TRIM(texto)', summary: 'Quita los espacios sobrantes de un texto.' },
  UPPER: { args: 'UPPER(texto)', summary: 'Convierte el texto a mayúsculas.' },
  LOWER: { args: 'LOWER(texto)', summary: 'Convierte el texto a minúsculas.' },
  PROPER: { args: 'PROPER(texto)', summary: 'Pone en mayúscula la inicial de cada palabra.' },
  EXACT: { args: 'EXACT(texto1; texto2)', summary: 'Comprueba si dos textos son idénticos.' },
  VALUE: { args: 'VALUE(texto)', summary: 'Convierte un texto en número.' },
  NUMBERVALUE: {
    args: 'NUMBERVALUE(texto; [separador_decimal]; [separador_grupo])',
    summary: 'Convierte texto en número según los separadores indicados.',
  },
  CHAR: { args: 'CHAR(número)', summary: 'Carácter correspondiente a un código.' },
  CODE: { args: 'CODE(texto)', summary: 'Código numérico del primer carácter.' },
  TEXTBEFORE: {
    args: 'TEXTBEFORE(texto; delimitador; [instancia])',
    summary: 'Texto anterior a un delimitador.',
  },
  TEXTAFTER: {
    args: 'TEXTAFTER(texto; delimitador; [instancia])',
    summary: 'Texto posterior a un delimitador.',
  },
  TEXTSPLIT: {
    args: 'TEXTSPLIT(texto; delimitador_col; [delimitador_fila])',
    summary: 'Divide un texto en varias celdas.',
  },
  REGEXEXTRACT: {
    args: 'REGEXEXTRACT(texto; patrón)',
    summary: 'Extrae la primera coincidencia de una expresión regular.',
  },
  REGEXREPLACE: {
    args: 'REGEXREPLACE(texto; patrón; reemplazo)',
    summary: 'Reemplaza las coincidencias de una expresión regular.',
  },
  REGEXTEST: {
    args: 'REGEXTEST(texto; patrón)',
    summary: 'Comprueba si un texto casa con una expresión regular.',
  },

  /* ---- Búsqueda y referencia ---- */
  VLOOKUP: {
    args: 'VLOOKUP(valor_buscado; rango; índice_columna; [ordenado])',
    summary: 'Busca en la 1.ª columna y devuelve un valor de la misma fila.',
  },
  HLOOKUP: {
    args: 'HLOOKUP(valor_buscado; rango; índice_fila; [ordenado])',
    summary: 'Busca en la 1.ª fila y devuelve un valor de la misma columna.',
  },
  XLOOKUP: {
    args: 'XLOOKUP(buscado; rango_búsqueda; rango_resultado; [si_no_hay]; [modo_coincidencia]; [modo_búsqueda])',
    summary: 'Busca un valor y devuelve el correspondiente de otro rango.',
  },
  LOOKUP: {
    args: 'LOOKUP(valor_buscado; vector_búsqueda; [vector_resultado])',
    summary: 'Busca un valor en un vector ordenado.',
  },
  INDEX: {
    args: 'INDEX(rango; fila; [columna])',
    summary: 'Valor en una posición (fila/columna) de un rango.',
  },
  MATCH: {
    args: 'MATCH(valor_buscado; rango; [tipo_coincidencia])',
    summary: 'Posición de un valor dentro de un rango.',
  },
  XMATCH: {
    args: 'XMATCH(buscado; rango_búsqueda; [modo_coincidencia]; [modo_búsqueda])',
    summary: 'Posición de un valor con modos de coincidencia avanzados.',
  },
  CHOOSE: {
    args: 'CHOOSE(índice; valor1; [valor2]; …)',
    summary: 'Elige un valor de una lista por su número de orden.',
  },
  OFFSET: {
    args: 'OFFSET(referencia; filas; columnas; [alto]; [ancho])',
    summary: 'Referencia desplazada respecto de otra.',
  },
  INDIRECT: {
    args: 'INDIRECT(texto_ref; [estilo_a1])',
    summary: 'Referencia indicada por un texto.',
  },
  ADDRESS: {
    args: 'ADDRESS(fila; columna; [tipo_abs]; [estilo_a1]; [hoja])',
    summary: 'Dirección de celda en forma de texto.',
  },
  ROW: { args: 'ROW([referencia])', summary: 'Número de fila de una referencia.' },
  ROWS: { args: 'ROWS(rango)', summary: 'Número de filas de un rango.' },
  COLUMN: { args: 'COLUMN([referencia])', summary: 'Número de columna de una referencia.' },
  COLUMNS: { args: 'COLUMNS(rango)', summary: 'Número de columnas de un rango.' },
  HYPERLINK: { args: 'HYPERLINK(url; [texto])', summary: 'Crea un enlace a una dirección.' },
  TRANSPOSE: { args: 'TRANSPOSE(rango)', summary: 'Intercambia filas por columnas de un rango.' },
  FILTER: {
    args: 'FILTER(rango; condición; [si_vacío])',
    summary: 'Filtra un rango según una condición.',
  },
  SORT: {
    args: 'SORT(rango; [índice_orden]; [ascendente])',
    summary: 'Ordena las filas de un rango.',
  },
  SORTBY: {
    args: 'SORTBY(rango; por_rango1; [orden1]; …)',
    summary: 'Ordena un rango según otros rangos.',
  },
  UNIQUE: {
    args: 'UNIQUE(rango; [por_columna]; [una_sola_vez])',
    summary: 'Valores únicos de un rango.',
  },
  SEQUENCE: {
    args: 'SEQUENCE(filas; [columnas]; [inicio]; [paso])',
    summary: 'Genera una secuencia de números.',
  },

  /* ---- Fecha y hora ---- */
  DATE: { args: 'DATE(año; mes; día)', summary: 'Construye una fecha a partir de año, mes y día.' },
  TODAY: { args: 'TODAY()', summary: 'Fecha de hoy.' },
  NOW: { args: 'NOW()', summary: 'Fecha y hora actuales.' },
  TIME: { args: 'TIME(hora; minuto; segundo)', summary: 'Construye una hora.' },
  YEAR: { args: 'YEAR(fecha)', summary: 'Año de una fecha.' },
  MONTH: { args: 'MONTH(fecha)', summary: 'Mes de una fecha (1..12).' },
  DAY: { args: 'DAY(fecha)', summary: 'Día del mes de una fecha.' },
  HOUR: { args: 'HOUR(hora)', summary: 'Hora de un valor de tiempo.' },
  MINUTE: { args: 'MINUTE(hora)', summary: 'Minuto de un valor de tiempo.' },
  SECOND: { args: 'SECOND(hora)', summary: 'Segundo de un valor de tiempo.' },
  WEEKDAY: { args: 'WEEKDAY(fecha; [tipo])', summary: 'Día de la semana de una fecha.' },
  WEEKNUM: { args: 'WEEKNUM(fecha; [tipo])', summary: 'Número de semana del año.' },
  EOMONTH: { args: 'EOMONTH(fecha_inicial; meses)', summary: 'Último día del mes desplazado.' },
  EDATE: { args: 'EDATE(fecha_inicial; meses)', summary: 'Fecha desplazada un número de meses.' },
  DATEDIF: {
    args: 'DATEDIF(fecha_inicial; fecha_final; unidad)',
    summary: 'Diferencia entre fechas en la unidad indicada.',
  },
  DAYS: { args: 'DAYS(fecha_final; fecha_inicial)', summary: 'Número de días entre dos fechas.' },
  NETWORKDAYS: {
    args: 'NETWORKDAYS(fecha_inicial; fecha_final; [festivos])',
    summary: 'Días laborables entre dos fechas.',
  },
  WORKDAY: {
    args: 'WORKDAY(fecha_inicial; días; [festivos])',
    summary: 'Fecha tras un número de días laborables.',
  },
  DATEVALUE: {
    args: 'DATEVALUE(texto_fecha)',
    summary: 'Convierte un texto de fecha en número de serie.',
  },
  YEARFRAC: {
    args: 'YEARFRAC(fecha_inicial; fecha_final; [base])',
    summary: 'Fracción de año entre dos fechas.',
  },

  /* ---- Información ---- */
  ISBLANK: { args: 'ISBLANK(valor)', summary: 'VERDADERO si la celda está vacía.' },
  ISNUMBER: { args: 'ISNUMBER(valor)', summary: 'VERDADERO si el valor es un número.' },
  ISTEXT: { args: 'ISTEXT(valor)', summary: 'VERDADERO si el valor es texto.' },
  ISERROR: { args: 'ISERROR(valor)', summary: 'VERDADERO si el valor es un error.' },
  ISNA: { args: 'ISNA(valor)', summary: 'VERDADERO si el valor es #N/A.' },
  ISLOGICAL: { args: 'ISLOGICAL(valor)', summary: 'VERDADERO si el valor es lógico.' },
  ISFORMULA: {
    args: 'ISFORMULA(referencia)',
    summary: 'VERDADERO si la celda contiene una fórmula.',
  },
  ISEVEN: { args: 'ISEVEN(número)', summary: 'VERDADERO si el número es par.' },
  ISODD: { args: 'ISODD(número)', summary: 'VERDADERO si el número es impar.' },
  NA: { args: 'NA()', summary: 'Devuelve el error #N/A.' },
  N: { args: 'N(valor)', summary: 'Convierte un valor en número.' },
  TYPE: { args: 'TYPE(valor)', summary: 'Tipo de un valor en forma de código.' },

  /* ---- Financieras ---- */
  PMT: { args: 'PMT(tasa; nper; va; [vf]; [tipo])', summary: 'Pago periódico de un préstamo.' },
  FV: { args: 'FV(tasa; nper; pago; [va]; [tipo])', summary: 'Valor futuro de una inversión.' },
  PV: { args: 'PV(tasa; nper; pago; [vf]; [tipo])', summary: 'Valor actual de una inversión.' },
  NPER: {
    args: 'NPER(tasa; pago; va; [vf]; [tipo])',
    summary: 'Número de periodos de una inversión.',
  },
  RATE: {
    args: 'RATE(nper; pago; va; [vf]; [tipo]; [estimación])',
    summary: 'Tasa de interés por periodo.',
  },
  NPV: { args: 'NPV(tasa; valor1; [valor2]; …)', summary: 'Valor actual neto de unos flujos.' },
  IRR: { args: 'IRR(valores; [estimación])', summary: 'Tasa interna de retorno de unos flujos.' },
  XIRR: {
    args: 'XIRR(valores; fechas; [estimación])',
    summary: 'TIR de flujos en fechas no periódicas.',
  },
  XNPV: { args: 'XNPV(tasa; valores; fechas)', summary: 'VAN de flujos en fechas no periódicas.' },

  /* ---- LAMBDA, orden superior y agrupaciones (las despacha el motor, no el registro) ---- */
  LET: {
    args: 'LET(nombre1; valor1; [nombre2; valor2]; …; cálculo)',
    summary: 'Da nombre a unos valores dentro de la fórmula y calcula con ellos.',
  },
  LAMBDA: {
    args: 'LAMBDA(parámetro1; [parámetro2]; …; cálculo)',
    summary:
      'Crea una función propia. Con [corchetes] un parámetro es opcional (ISOMITTED). Guardada como nombre definido se llama como cualquier función.',
  },
  ISOMITTED: {
    args: 'ISOMITTED(parámetro)',
    summary: 'Dentro de un LAMBDA: VERDADERO si ese parámetro opcional no recibió argumento.',
  },
  MAP: {
    args: 'MAP(matriz; lambda)',
    summary: 'Aplica una función a cada elemento y devuelve una matriz de la misma forma.',
  },
  REDUCE: {
    args: 'REDUCE(inicial; matriz; lambda(acumulado; valor))',
    summary: 'Recorre la matriz acumulando un valor y devuelve solo el acumulado final.',
  },
  SCAN: {
    args: 'SCAN(inicial; matriz; lambda(acumulado; valor))',
    summary: 'Como REDUCE pero devuelve todos los acumulados intermedios.',
  },
  MAKEARRAY: {
    args: 'MAKEARRAY(filas; columnas; lambda(fila; columna))',
    summary: 'Construye una matriz aplicando una función a cada posición (índices desde 1).',
  },
  BYROW: {
    args: 'BYROW(matriz; lambda(fila))',
    summary: 'Aplica una función a cada fila y devuelve una columna con los resultados.',
  },
  BYCOL: {
    args: 'BYCOL(matriz; lambda(columna))',
    summary: 'Aplica una función a cada columna y devuelve una fila con los resultados.',
  },
  GROUPBY: {
    args: 'GROUPBY(campos_fila; valores; función; [cabeceras]; [total]; [orden]; [filtro])',
    summary:
      'Agrupa por uno o varios campos y resume cada grupo con una función (SUM, un LAMBDA, PERCENTOF…). Total: 0 ninguno, 1 general, 2 con subtotales, negativo arriba.',
  },
  PIVOTBY: {
    args: 'PIVOTBY(campos_fila; campos_columna; valores; función; [cabeceras]; [total_filas]; [orden_filas]; [total_columnas]; [orden_columnas]; [filtro])',
    summary: 'Cruza filas y columnas como una tabla dinámica y resume cada cruce con una función.',
  },
  PERCENTOF: {
    args: 'PERCENTOF(subconjunto; todos)',
    summary: 'Qué parte del total es el subconjunto, como razón (0,25 = 25 %).',
  },
  TRIMRANGE: {
    args: 'TRIMRANGE(rango; [recortar_filas]; [recortar_columnas])',
    summary:
      'Quita las filas y columnas en blanco de los bordes del rango (0 nada, 1 principio, 2 final, 3 ambos).',
  },
  STDEVPA: {
    args: 'STDEVPA(valor1; [valor2]; …)',
    summary: 'Desviación estándar de la población, contando texto y lógicos.',
  },
  VARPA: {
    args: 'VARPA(valor1; [valor2]; …)',
    summary: 'Varianza de la población, contando texto y lógicos.',
  },
  'BINOM.DIST.RANGE': {
    args: 'BINOM.DIST.RANGE(ensayos; prob_éxito; éxitos; [éxitos2])',
    summary: 'Probabilidad binomial de un número de éxitos o de un rango.',
  },
  FVSCHEDULE: {
    args: 'FVSCHEDULE(capital; tasas)',
    summary: 'Valor futuro de un capital con una serie de tasas de interés compuesto.',
  },
  TBILLPRICE: {
    args: 'TBILLPRICE(liquidación; vencimiento; descuento)',
    summary: 'Precio por 100 de una letra del Tesoro.',
  },
  TBILLYIELD: {
    args: 'TBILLYIELD(liquidación; vencimiento; precio)',
    summary: 'Rendimiento de una letra del Tesoro.',
  },
  TBILLEQ: {
    args: 'TBILLEQ(liquidación; vencimiento; descuento)',
    summary: 'Rendimiento equivalente a un bono de una letra del Tesoro.',
  },
  PRICEMAT: {
    args: 'PRICEMAT(liquidación; vencimiento; emisión; tasa; rendimiento; [base])',
    summary: 'Precio por 100 de un valor que paga interés al vencimiento.',
  },
  YIELDMAT: {
    args: 'YIELDMAT(liquidación; vencimiento; emisión; tasa; precio; [base])',
    summary: 'Rendimiento anual de un valor que paga interés al vencimiento.',
  },
  ODDFPRICE: {
    args: 'ODDFPRICE(liquidación; vencimiento; emisión; primer_cupón; tasa; rendimiento; rescate; frecuencia; [base])',
    summary: 'Precio por 100 de un valor con primer periodo de cupón irregular.',
  },
  ODDFYIELD: {
    args: 'ODDFYIELD(liquidación; vencimiento; emisión; primer_cupón; tasa; precio; rescate; frecuencia; [base])',
    summary: 'Rendimiento de un valor con primer periodo de cupón irregular.',
  },
  ODDLPRICE: {
    args: 'ODDLPRICE(liquidación; vencimiento; último_interés; tasa; rendimiento; rescate; frecuencia; [base])',
    summary: 'Precio por 100 de un valor con último periodo de cupón irregular.',
  },
  ODDLYIELD: {
    args: 'ODDLYIELD(liquidación; vencimiento; último_interés; tasa; precio; rescate; frecuencia; [base])',
    summary: 'Rendimiento de un valor con último periodo de cupón irregular.',
  },
  'FORECAST.ETS': {
    args: 'FORECAST.ETS(objetivo; valores; línea_de_tiempo; [estacionalidad]; [completar]; [agregación])',
    summary: 'Pronostica un valor por suavizado exponencial triple (estacionalidad aditiva).',
  },
  'FORECAST.ETS.ADD': {
    args: 'FORECAST.ETS.ADD(objetivo; valores; línea_de_tiempo; [estacionalidad]; [completar]; [agregación])',
    summary: 'Pronostica un valor por suavizado exponencial triple con estacionalidad aditiva.',
  },
  'FORECAST.ETS.MULT': {
    args: 'FORECAST.ETS.MULT(objetivo; valores; línea_de_tiempo; [estacionalidad]; [completar]; [agregación])',
    summary:
      'Pronostica un valor por suavizado exponencial triple con estacionalidad multiplicativa.',
  },
  'FORECAST.ETS.CONFINT': {
    args: 'FORECAST.ETS.CONFINT(objetivo; valores; línea_de_tiempo; [confianza]; [estacionalidad]; [completar]; [agregación])',
    summary: 'Semianchura del intervalo de confianza del pronóstico (0,95 por defecto).',
  },
  'FORECAST.ETS.PI.ADD': {
    args: 'FORECAST.ETS.PI.ADD(objetivo; valores; línea_de_tiempo; [confianza]; [estacionalidad]; [completar]; [agregación])',
    summary: 'Semianchura del intervalo de predicción del modelo aditivo.',
  },
  'FORECAST.ETS.PI.MULT': {
    args: 'FORECAST.ETS.PI.MULT(objetivo; valores; línea_de_tiempo; [confianza]; [estacionalidad]; [completar]; [agregación])',
    summary: 'Semianchura del intervalo de predicción del modelo multiplicativo.',
  },
  'FORECAST.ETS.SEASONALITY': {
    args: 'FORECAST.ETS.SEASONALITY(valores; línea_de_tiempo; [completar]; [agregación])',
    summary: 'Longitud del patrón estacional detectado en la serie (1 si no hay).',
  },
  'FORECAST.ETS.STAT': {
    args: 'FORECAST.ETS.STAT(valores; línea_de_tiempo; tipo; [estacionalidad]; [completar]; [agregación])',
    summary:
      'Estadística del pronóstico: 1 alfa, 2 beta, 3 gamma, 4 MASE, 5 SMAPE, 6 MAE, 7 RMSE, 8 paso.',
  },
  'FORECAST.ETS.STAT.ADD': {
    args: 'FORECAST.ETS.STAT.ADD(valores; línea_de_tiempo; tipo; [estacionalidad]; [completar]; [agregación])',
    summary:
      'Estadística del modelo aditivo: 1 alfa, 2 beta, 3 gamma, 4 MASE, 5 SMAPE, 6 MAE, 7 RMSE, 8 paso.',
  },
  'FORECAST.ETS.STAT.MULT': {
    args: 'FORECAST.ETS.STAT.MULT(valores; línea_de_tiempo; tipo; [estacionalidad]; [completar]; [agregación])',
    summary:
      'Estadística del modelo multiplicativo: 1 alfa, 2 beta, 3 gamma, 4 MASE, 5 SMAPE, 6 MAE, 7 RMSE, 8 paso.',
  },
  AMORLINC: {
    args: 'AMORLINC(coste; compra; primer_periodo; residual; periodo; tasa; [base])',
    summary: 'Depreciación lineal de cada periodo contable.',
  },
  AMORDEGRC: {
    args: 'AMORDEGRC(coste; compra; primer_periodo; residual; periodo; tasa; [base])',
    summary: 'Depreciación degresiva de cada periodo contable.',
  },
  LEFTB: {
    args: 'LEFTB(texto; [nº_bytes])',
    summary: 'Como LEFT: primeros caracteres de un texto.',
  },
  RIGHTB: {
    args: 'RIGHTB(texto; [nº_bytes])',
    summary: 'Como RIGHT: últimos caracteres de un texto.',
  },
  MIDB: {
    args: 'MIDB(texto; posición_inicial; nº_bytes)',
    summary: 'Como MID: caracteres de un texto desde una posición.',
  },
  LENB: {
    args: 'LENB(texto)',
    summary: 'Como LEN: longitud de un texto.',
  },
  FINDB: {
    args: 'FINDB(buscado; en_texto; [inicio])',
    summary: 'Como FIND: posición de un texto dentro de otro, sensible a mayúsculas.',
  },
  SEARCHB: {
    args: 'SEARCHB(buscado; en_texto; [inicio])',
    summary: 'Como SEARCH: posición de un texto, sin distinguir mayúsculas y con comodines.',
  },
  REPLACEB: {
    args: 'REPLACEB(texto; inicio; nº_bytes; nuevo)',
    summary: 'Como REPLACE: sustituye parte de un texto.',
  },
  ASC: {
    args: 'ASC(texto)',
    summary: 'Pasa los caracteres de ancho completo a ancho medio.',
  },
  DBCS: {
    args: 'DBCS(texto)',
    summary: 'Pasa los caracteres a ancho completo.',
  },
  JIS: {
    args: 'JIS(texto)',
    summary: 'Pasa los caracteres a ancho completo.',
  },
  PHONETIC: {
    args: 'PHONETIC(referencia)',
    summary: 'Lectura fonética de un texto; aquí, el propio texto.',
  },
  ENCODEURL: {
    args: 'ENCODEURL(texto)',
    summary: 'Codifica un texto para usarlo en una dirección web.',
  },
  BAHTTEXT: {
    args: 'BAHTTEXT(número)',
    summary: 'Escribe un importe en tailandés, en baht y satang.',
  },
  AREAS: {
    args: 'AREAS(referencia)',
    summary: 'Cuántas áreas tiene una referencia.',
  },
  FORMULATEXT: {
    args: 'FORMULATEXT(referencia)',
    summary: 'La fórmula de una celda, escrita como texto.',
  },
  SHEET: {
    args: 'SHEET([valor])',
    summary: 'Número de la hoja de una referencia o de un nombre de hoja.',
  },
  SHEETS: {
    args: 'SHEETS([referencia])',
    summary: 'Cuántas hojas tiene el libro.',
  },
  CELL: {
    args: 'CELL(tipo; [referencia])',
    summary: 'Información de una celda: address, col, row, contents, type o filename.',
  },
};

/** Firma de una función por nombre (insensible a mayúsculas); undefined si no
 *  está en el catálogo. */
export function lookupSignature(name: string): FnSignature | undefined {
  return FORMULA_CATALOG[name.toUpperCase()];
}

/**
 * Divide una firma en el nombre y sus parámetros, para resaltar el argumento
 * actual en el hint: «SUM(número1; [número2]; …)» →
 * { name: 'SUM', params: ['número1', '[número2]', '…'] }.
 */
export function splitSignature(args: string): { name: string; params: string[] } {
  const open = args.indexOf('(');
  if (open < 0) return { name: args, params: [] };
  const name = args.slice(0, open);
  const inside = args.slice(open + 1, args.lastIndexOf(')'));
  const params = inside
    .split(';')
    .map((p) => p.trim())
    .filter((p) => p !== '');
  return { name, params };
}
