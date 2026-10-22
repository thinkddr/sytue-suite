#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
"""Carga la base de rangos IP → país/ciudad que usa «Accesos recientes».

La tabla `geoip_networks` nace VACÍA en la migración 0037 y este script la
rellena. Sin ella la interfaz sigue funcionando: simplemente no enseña la
ubicación de cada acceso.

    cargar.py --dsn "$SYTUE_ID_DATABASE_DSN" --fichero dbip-city-lite.csv.gz
    cargar.py --dsn ... --descargar          # baja el mes en curso de DB-IP

FUENTE Y LICENCIA
    Por defecto usa **DB-IP City Lite**, publicada por db-ip.com bajo
    **CC BY 4.0**: se puede usar en un producto comercial siempre que se
    atribuya. La atribución exigida está en deploy/geoip/README.md y hay que
    mantenerla visible donde corresponda.

    NO se usa GeoLite2 de MaxMind: desde 2019 exige cuenta, aceptar su EULA y
    una clave de licencia para descargarla, lo que la hace inservible para un
    despliegue que un cliente deba poder reproducir por su cuenta.

    Los datos NO se versionan en el repositorio: son ~100 MB que cambian cada
    mes y que tienen su propia licencia. Se descargan al desplegar.

PRECISIÓN
    El país acierta casi siempre; la ciudad, bastante menos. Una VPN, un móvil o
    el proxy de una operadora desplazan la ubicación cientos de kilómetros. Por
    eso la interfaz la presenta como pista para reconocer un acceso y nunca como
    prueba de dónde estuvo alguien.
"""

import argparse
import csv
import gzip
import io
import os
import subprocess
import sys
import urllib.request

# El mes en curso; DB-IP publica una base nueva cada mes en esta ruta.
URL_PLANTILLA = "https://download.db-ip.com/free/dbip-city-lite-{aaaa_mm}.csv.gz"


def descargar(aaaa_mm):
    url = URL_PLANTILLA.format(aaaa_mm=aaaa_mm)
    print(f"  descargando {url}")
    with urllib.request.urlopen(url, timeout=300) as r:  # noqa: S310 (URL fija, https)
        datos = r.read()
    print(f"  {len(datos) / 1e6:.0f} MB")
    return datos


def filas(bruto):
    """Genera (ip_inicio, ip_fin, código_país, país, ciudad) del CSV de DB-IP.

    El formato es sin cabecera: inicio, fin, continente, país, región, ciudad,
    latitud, longitud. Se descarta todo lo que no se muestra —coordenadas y
    región— para no guardar datos de ubicación que nadie va a enseñar.
    """
    texto = io.TextIOWrapper(io.BytesIO(gzip.decompress(bruto)), encoding="utf-8", errors="replace")
    for f in csv.reader(texto):
        if len(f) < 6:
            continue
        inicio, fin, _cont, pais, _region, ciudad = f[0], f[1], f[2], f[3], f[4], f[5]
        if not inicio or not fin or not pais:
            continue
        yield inicio, fin, pais.upper()[:2], NOMBRES.get(pais.upper(), pais), ciudad


# Nombres de país en español para los europeos más frecuentes; el resto se queda
# con el código ISO, que es mejor que una traducción inventada. La tabla completa
# vendría de un catálogo de países, que es otra base de datos y otro problema.
NOMBRES = {
    "ES": "España", "FR": "Francia", "DE": "Alemania", "IT": "Italia",
    "PT": "Portugal", "GB": "Reino Unido", "IE": "Irlanda", "NL": "Países Bajos",
    "BE": "Bélgica", "LU": "Luxemburgo", "CH": "Suiza", "AT": "Austria",
    "PL": "Polonia", "SE": "Suecia", "NO": "Noruega", "DK": "Dinamarca",
    "FI": "Finlandia", "GR": "Grecia", "CZ": "Chequia", "RO": "Rumanía",
    "US": "Estados Unidos", "CA": "Canadá", "MX": "México", "BR": "Brasil",
    "AR": "Argentina", "MA": "Marruecos", "AD": "Andorra",
}


def main():
    ap = argparse.ArgumentParser(description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--dsn", default=os.environ.get("SYTUE_ID_DATABASE_DSN"),
                    help="DSN de la base de datos de Sytue ID")
    ap.add_argument("--fichero", help="CSV .gz ya descargado")
    ap.add_argument("--descargar", metavar="AAAA-MM", nargs="?", const="",
                    help="descargar la base de ese mes (por defecto, el que se indique)")
    ap.add_argument("--limite", type=int, default=0,
                    help="cargar solo las N primeras filas (para probar)")
    args = ap.parse_args()

    if not args.dsn:
        sys.exit("falta --dsn (o SYTUE_ID_DATABASE_DSN en el entorno)")
    if not args.fichero and args.descargar is None:
        sys.exit("indica --fichero o --descargar AAAA-MM")

    if args.fichero:
        bruto = open(args.fichero, "rb").read()
    else:
        if not args.descargar:
            sys.exit("--descargar necesita el mes, como 2026-08")
        bruto = descargar(args.descargar)

    # Se carga en una tabla temporal y se cambia al final: mientras dura la carga
    # —millones de filas, varios minutos— la tabla en uso sigue respondiendo. Con
    # un TRUNCATE + INSERT, la interfaz se quedaría sin ubicaciones justo durante
    # ese rato, y un fallo a mitad dejaría la base a medias.
    sql = io.StringIO()
    sql.write("BEGIN;\n")
    sql.write("CREATE TEMP TABLE geoip_carga (LIKE geoip_networks INCLUDING ALL) ON COMMIT DROP;\n")
    sql.write("COPY geoip_carga (ip_start, ip_end, country_code, country, city) FROM STDIN WITH (FORMAT csv);\n")

    n = 0
    salida = csv.writer(sql)
    for inicio, fin, cc, pais, ciudad in filas(bruto):
        salida.writerow([inicio, fin, cc, pais, ciudad])
        n += 1
        if args.limite and n >= args.limite:
            break
    sql.write("\\.\n")
    # El intercambio va dentro de la MISMA transacción que la carga: si algo falla,
    # la tabla anterior sigue intacta.
    sql.write("DELETE FROM geoip_networks;\n")
    sql.write("INSERT INTO geoip_networks SELECT * FROM geoip_carga;\n")
    sql.write("COMMIT;\n")

    print(f"  {n} rangos → base de datos")
    r = subprocess.run(["psql", args.dsn, "-v", "ON_ERROR_STOP=1", "-q"],
                       input=sql.getvalue(), text=True)
    if r.returncode:
        sys.exit("la carga falló; la tabla anterior queda intacta")
    print("  ✓ cargado")


if __name__ == "__main__":
    main()
