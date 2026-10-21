<!-- SPDX-License-Identifier: Apache-2.0 -->
# Ubicación aproximada de los accesos

«Accesos recientes» y «Sesiones activas» de Sytue ID pueden mostrar de dónde vino
cada acceso. Una IP no le dice a nadie si un acceso fue suyo; «Valencia, España»
sí, y esa es justo la pregunta de quien entra a revisar su cuenta.

**Es opcional.** La tabla nace vacía y, vacía, la interfaz omite la ubicación sin
errores ni huecos.

## Cargar los datos

```bash
# Descargar y cargar el mes en curso
deploy/geoip/cargar.py --dsn "$SYTUE_ID_DATABASE_DSN" --descargar 2026-08

# O desde un fichero ya bajado
deploy/geoip/cargar.py --dsn "$SYTUE_ID_DATABASE_DSN" --fichero dbip-city-lite.csv.gz
```

Tarda unos minutos (varios millones de rangos). La carga va a una tabla temporal
y se intercambia al final **dentro de la misma transacción**: mientras dura, la
tabla en uso sigue respondiendo, y si algo falla la anterior queda intacta.

Conviene repetirlo cada pocos meses: los rangos se reasignan y una base vieja
empieza a situar accesos donde no son.

## Atribución obligatoria

Los datos son de **[DB-IP](https://db-ip.com)**, base *IP to City Lite*,
publicada bajo **[CC BY 4.0](https://creativecommons.org/licenses/by/4.0/)**.

> IP Geolocation by DB-IP — https://db-ip.com

Esa atribución es **condición de la licencia**, no una cortesía: hay que
mantenerla visible allí donde se publique el producto que la usa. Si algún día se
cambia de fuente, hay que revisar qué exige la nueva.

**No se usa GeoLite2 de MaxMind**: desde 2019 exige cuenta, aceptar su EULA y una
clave de licencia para descargarla, lo que la hace inservible para un despliegue
que un cliente deba poder reproducir por su cuenta.

## Por qué en PostgreSQL y no con una librería

Leer un fichero MMDB significa añadir una dependencia al binario del IdP —que hoy
solo usa la stdlib y pgx— y montar el fichero en cada réplica. PostgreSQL ya está
desplegado, ya sabe comparar direcciones con el tipo `inet`, y un índice resuelve
la búsqueda en microsegundos. Los datos quedan además dentro de nuestra
infraestructura, que es lo que se le promete a un cliente europeo.

## Qué no hace

- **No es prueba de dónde estuvo alguien.** El país acierta casi siempre; la
  ciudad, bastante menos. Una VPN, un móvil o el proxy de una operadora la
  desplazan cientos de kilómetros.
- **No se guardan coordenadas ni región.** El cargador las descarta: no se
  muestran, así que no se almacenan.
- **No sale del servidor.** La resolución ocurre contra nuestra propia base de
  datos; no se consulta ningún servicio externo con las IP de los usuarios.
