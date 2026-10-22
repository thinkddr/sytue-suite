<!-- SPDX-License-Identifier: Apache-2.0 -->
<!-- Copyright (c) 2026 Daniel Delgado Ramos -->

# SyERP (`:8098`)

El ERP de Sytue One: contactos, ventas, compras, inventario, fabricación,
facturación, contabilidad, proyectos, RRHH, mantenimiento, soporte y once áreas
más. **24 módulos, 36 modelos**, un servicio, una base de datos.

## La decisión que ordena todo lo demás

Un ERP «con todos los módulos» escrito módulo a módulo son doscientas tablas,
doscientos CRUD que solo se diferencian en los nombres de las columnas y una
migración cada vez que alguien quiere un campo nuevo. Aquí no.

**El catálogo de modelos es DATO.** Vive en
[`internal/catalogo/modulos/*.json`](internal/catalogo/modulos): cada fichero
declara modelos con sus campos, sus estados y sus columnas de lista. El núcleo no
tiene una sola línea de código específica de Ventas, de Flota ni de Calidad.

```jsonc
{
  "clave": "pedido_venta", "modulo": "ventas", "secuencia": "PV",
  "estados": ["borrador", "confirmado", "entregado", "facturado", "cancelado"],
  "campos": [
    { "clave": "cliente", "tipo": "relacion", "modelo": "contacto", "requerido": true },
    { "clave": "fecha", "tipo": "fecha", "requerido": true },
    { "clave": "total", "tipo": "dinero" }
  ],
  "lista": ["ref", "cliente", "fecha", "total", "estado"]
}
```

Consecuencias, que son el producto:

- **Añadir un módulo es un fichero JSON.** Sin DDL, sin migración, sin bloqueo de
  tabla, sin ruta HTTP nueva, sin pantalla nueva, sin tocar el frontend.
- **Una sola API.** `/api/v1/{modelo}` sirve a los 36 modelos.
- **Una sola interfaz.** La app (`apps/frontend/apps/erp`) genera lista y ficha
  del catálogo que le sirve este servicio.
- **Los cinco idiomas, una vez.** Los rótulos comunes (fecha, importe, cliente…)
  viven en [`terminos.json`](internal/catalogo/terminos.json) con sus cinco
  traducciones y los comparten los 24 módulos; un campo sin rótulo **impide el
  arranque**, no sale en inglés. Lo vigila `TestTodoTraducidoEnLosCincoIdiomas`.

## Lo que NO va en JSONB: el dinero

La maestra vive en `records.data` (jsonb). La contabilidad tiene tablas propias,
columnas tipadas y restricciones de verdad:

- **Importes en céntimos enteros** (`bigint`). Ni un `float` en ninguna columna
  de dinero: 0,1 + 0,2 no es 0,3 y una factura no puede depender de eso. Las
  cantidades no monetarias viajan como cadena decimal y Postgres las castea a
  `numeric` para sumar.
- **El cuadre lo impone Postgres**, no el código: `move_lines_cuadre` es un
  `CONSTRAINT TRIGGER … DEFERRABLE` que comprueba `sum(debe) = sum(haber)` al
  confirmar la transacción.
- **Lo asentado no se toca.** `moves_inmutables` y `move_lines_inmutables`
  rechazan modificar, borrar **y añadir** sobre un asiento asentado; la única
  transición permitida es anularlo. Corregir se hace con otro asiento, que es lo
  que separa un libro contable de una hoja de cálculo.

  El `INSERT` de esa lista llegó tarde y conviene que quede escrito: el trigger
  cubría solo `UPDATE`/`DELETE`, así que **añadir** apuntes a un asiento ya
  cerrado estaba permitido. Un apunte suelto lo paraba el cuadre por accidente,
  pero un par equilibrado —un cargo y su abono— entraba sin que nada se quejara.
  Lo encontró `TestAsientoAsentadoEsInmutable/colar_un_par_equilibrado` al
  exigir que el fallo viniera del trigger y no de cualquier error. De ahí que un
  asiento se escriba en borrador, se le pongan los apuntes y se cierre al final:
  ese orden ya no es una convención del código, lo impone la base.
- **Un documento, un asiento.** Índice único parcial `moves_un_asiento_por_documento`:
  un reintento, dos pestañas o una carrera entre réplicas se estrellan contra el
  índice en vez de duplicar ingresos.

Un abono no es un caso aparte: es la misma factura en negativo, y cada importe se
coloca en su lado natural invirtiéndolo si viene negativo. Cubierto por
`TestAsientoRectificativo`.

## Rendimiento, medido

Números reales, no estimaciones: 20.000 filas por modelo en un PostgreSQL con
estadísticas al día. Se reproducen con
`go test -tags integration -run "TestElListadoUsaSuIndice|TestTiempoDeListado"`
y con `go test -bench Vista ./internal/catalogo`.

| | Antes | Ahora |
|---|---|---|
| Primera página de una lista | 9,4 ms · coste 2114 · **20.000 filas leídas** | **0,56 ms** · coste 8,88 · 51 filas |
| Lista filtrada por estado | 5,5 ms | **0,54 ms** |
| Catálogo (cada carga de página) | 144 µs · **115 KB reservados** | **7,9 ns · 0 B** |
| Catálogo por el cable, al volver | 32 KB | **304, sin cuerpo** |

Las tres causas, y ninguna era la que parecía:

1. **`tenant` faltaba en el `WHERE`.** Lo impone la RLS, sí, pero comparándolo
   con `current_setting`, que es una expresión estable y **no sirve como
   condición de índice ORDENADA**. `tenant` es la primera columna de
   `records_listado`, así que sin la igualdad explícita PostgreSQL no podía
   recorrer el índice en orden: leía el modelo entero y hacía un top-N para
   servir cincuenta filas. Con ella, *Index Scan* y se para en la página.
   La política sigue aplicándose como `Filter`: esto no la sustituye, le da al
   planificador el dato que ella no puede darle.

2. **El catálogo se traducía y serializaba en cada petición.** Es inmutable —va
   horneado en la imagen—, así que ahora se calcula una vez por idioma al
   arrancar. Cuesta 160 KB de memoria en total y deja el manejador en una copia
   de bytes. Con ETag encima, volver a una pantalla no baja ni un byte de él.

3. **Y un parpadeo que no era de rendimiento sino de corrección:** al cambiar de
   modelo se quedaban en pantalla las filas del anterior pintadas con las
   columnas del nuevo —una tabla de guiones que parecía un error de datos—.

En el navegador, las listas ya vistas se pintan **al instante** desde una caché
en memoria y se refrescan por detrás; escribir invalida la de ese modelo, porque
servir una lista vieja justo después de crear algo es la forma más rápida de que
alguien crea que no se ha guardado.

`TestElListadoUsaSuIndice` es un **gate sobre el plan**, no una medida: quien
quite el `tenant` del `WHERE` no rompe nada visible —la lista sigue saliendo
bien, solo que diecisiete veces más lenta— y por eso hay un test mirando el
`EXPLAIN`.

**Lo que se midió y se decidió NO tocar:** la búsqueda por texto tarda 9,8 ms y
no usa `records_etiqueta_trgm` — a este tamaño PostgreSQL acierta al preferir el
escaneo, y lo mismo con un patrón literal o con un GIN compuesto sobre
`btree_gin`. El índice se queda para cuando la tabla crezca; la alternativa era
añadir una extensión nueva sin ninguna mejora medible.

## Conectores

| Conector | Sentido | Cómo |
|---|---|---|
| **Salesforce** | entrada | `POST /api/v1/importar/salesforce`. Sale por el **Sytue Integration Gateway** (`:8093`) reenviando la autorización de quien importa: SyERP no guarda credenciales de Salesforce ni tiene cuenta de servicio con acceso a todas las organizaciones. Idempotente por `externo_id`: importar dos veces actualiza, no duplica. Mapea `Account`, `Contact` y `Opportunity`. |
| **Stripe** | entrada | `POST /webhooks/stripe/{org}`. Sin sesión —Stripe no presenta un token de Sytue ID—: su autenticación es la **firma HMAC-SHA256** del cuerpo crudo, con ventana de 5 minutos contra repeticiones y comparación en tiempo constante. `invoice.paid` marca la factura pagada y escribe el asiento de cobro. La repetición de eventos la corta la clave primaria de `eventos_entrantes`. |

Lo que **no** hay todavía, dicho claro: **cobrar desde SyERP** (crear un enlace
de pago en Stripe desde una factura). El plano de datos de la pasarela es de
**solo lectura** (`TabularSource`), así que un conector de escritura necesita
primero ese plano en `services/integrations`. Mientras tanto la factura se crea
en Stripe y se casa por `externo_id` o por `metadata.syerp_ref`.

## Licencias

Modelo propio, escrito desde el dominio funcional. Que un ERP tenga
oportunidades, albaranes, partes de horas o asientos por partida doble son ideas
del oficio —y contabilidad de 1494—, no código con licencia: **no se ha
consultado ni portado fuente de terceros** (CONVENTIONS §12). Sin dependencias
LGPL/AGPL arrastradas: el servicio usa la misma pila que el resto de la suite
(chi, pgx, `platform/pkg/*`).

## Lo que NO reimplementa

La suite ya tiene correo, documentos, hoja de cálculo, calendario, chat, vídeo,
almacenamiento y firma. SyERP **no** los vuelve a construir: donde otro ERP mete
su propio módulo de mensajería o de documentos, aquí se usa Sytue Mail, Docus,
Drive y Rooms. Es la mitad del código que no existe.

Las cuatro garantías tienen su prueba contra un PostgreSQL real en
`internal/store/libro_integration_test.go` (`-tags integration`), y los negativos
están atados al mensaje del trigger: un test que solo exija «que falle» seguiría
en verde el día que falle por otra cosa.

## Uso

```bash
# Validar los manifiestos y ver qué hay. Sin base de datos y sin red.
go run ./services/syerp/cmd/syerp catalogo

# Las garantías de la base, contra un Postgres efímero (testcontainers).
go test -tags integration ./services/syerp/internal/store/...

# Levantar en dev (perfil propio: no arrastra los treinta servicios).
docker compose -p sytue-one-dev -f deploy/compose/docker-compose.yml \
  --profile erp up -d postgres id syerp erp-web
```

| Variable | Para qué |
|---|---|
| `DATABASE_URL` | PostgreSQL (única fuente de verdad) |
| `OIDC_ISSUER` / `OIDC_JWKS_URL` | Validación de tokens de Sytue ID |
| `INTEGRATIONS_BASE_URL` | Pasarela de egress (por defecto `http://integrations:8093`) |
| `SECRET_ENC_KEY` | AES-256 en base64: sella el secreto de firma de los webhooks. **Obligatoria en prod** |

## Rutas

```
GET    /api/v1/catalogo                    catálogo traducido + módulos activos
POST   /api/v1/modulos/{modulo}            encender/apagar un módulo        (admin)
GET    /api/v1/{modelo}                    lista (q, estado, campo/valor, cursor)
POST   /api/v1/{modelo}                    crear
GET    /api/v1/{modelo}/{id}               leer
PUT    /api/v1/{modelo}/{id}               actualizar
DELETE /api/v1/{modelo}/{id}               a la papelera
POST   /api/v1/{modelo}/{id}/estado        mover en el flujo
POST   /api/v1/{modelo}/{id}/asentar       llevar al libro por partida doble
GET    /api/v1/{modelo}/{id}/asientos      asientos de un documento
GET    /api/v1/libro                       libro diario (desde, hasta)
GET    /api/v1/balance                     sumas y saldos
POST   /api/v1/libro/{id}/anular           anular un asiento               (admin)
GET    /api/v1/conexiones                  conectores configurados         (admin)
POST   /api/v1/conexiones/{proveedor}      configurar Stripe/Salesforce    (admin)
POST   /api/v1/importar/salesforce         importar                        (admin)
POST   /webhooks/stripe/{org}              webhook de Stripe          (sin sesión)
```
