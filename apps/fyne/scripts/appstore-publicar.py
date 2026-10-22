#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
"""Sube a App Store Connect (API v1) la ficha de cada app de apps/fyne/appstore/ y, con
una confirmación escrita a mano, envía a revisión la versión con su build. Ver «App
Store: ficha y envío por la API» en docs/operations.md y apps/fyne/appstore/README.md.

  appstore-publicar.py --seco [app...]      valida textos, capturas y ficha.json; no llama a nadie
  appstore-publicar.py --sondear [--version X.Y.Z] [app...]
                                            versiones y, con --version, estados de builds. Solo GET
  appstore-publicar.py --preparacion [--version X.Y.Z] [app...]
                                            metadatos reutilizables y ausencias; solo GET, sin datos personales
  appstore-publicar.py --ficha [--version X.Y.Z] [app...]
                                            textos, capturas, categoría, edades, precio y países
  appstore-publicar.py --revision --version X.Y.Z [app...]
                                            ata el build de esa versión y la ENVÍA A REVISIÓN
  appstore-publicar.py --cifrado-exento --version X.Y.Z [app...]
                                            completa solo el cifrado pendiente, sin Francia ni revisión

Python y no bash, al revés que play-publicar.sh: la API de Apple es JSON:API (cada dato
son dos o tres peticiones encadenadas) y aquí el token no existe fuera del proceso: ni
argv, ni fichero de cabecera. Solo biblioteca estándar y `openssl`, que firma el JWT.

CREDENCIALES: APP_STORE_CONNECT_ISSUER_ID, _KEY_IDENTIFIER y _PRIVATE_KEY, secretos de
GitHub que .github/workflows/appstore-publicar.yml pasa a este guion (desde el 19/10; antes
las ponía la integración de Codemagic). Nunca van al repositorio. La clave pasa a openssl por un fichero 600 que se borra al firmar.
Se admite el PEM completo del .p8, sus saltos escapados o una copia en base64,
y las referencias @file:/@env: de codemagic-cli-tools. Un valor ilegible se rechaza antes de
llamar a Apple, sin imprimirlo.

`--revision` NO se dispara por accidente: exige APPSTORE_CONFIRMAR_REVISION con la frase
exacta `ENVIAR_<n>_APPS_A_REVISION`, donde <n> es el número de apps que se envían (15 si
no se nombra ninguna), y no cancela ni pisa una revisión que ya esté en curso. Tampoco
declara el cifrado: si el build no tiene contestado el cuestionario, se niega.

`--plataformas "IOS MAC_OS"` (por defecto las dos) acota a una.
"""
import base64
import hashlib
import json
import os
import plistlib
import re
import struct
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.request

from apple_clave import normalizar_clave

AQUI = os.path.dirname(os.path.abspath(__file__))
# APPSTORE_DIR solo lo cambia el guardián, para verlo en rojo.
FICHAS = os.environ.get("APPSTORE_DIR") or os.path.join(AQUI, "..", "appstore")
CMD = os.path.join(AQUI, "..", "cmd")
API = os.environ.get("APPSTORE_API", "https://api.appstoreconnect.apple.com")  # otro solo en las pruebas

# Límites de App Store Connect («App information» y «Platform version information»), en caracteres.
LIMITES = {"name": 30, "subtitle": 30, "promotional_text": 170, "description": 4000, "keywords": 100}
OPCIONALES = {"subtitle"}
# Los locales que la API admite de los nuestros (apps/fyne/play/generar.ts, APPLE).
LOCALES = "cs da de-DE el en-US es-ES fi fr-FR hr hu it nl-NL no pl pt-PT ro sk sl-SI sv".split()
# Capturas: fichero → (tipo de la API, plataforma, medida). Las medidas son las que la
# referencia «Screenshot specifications» da por OBLIGATORIAS a 2026-10-06: iPhone con Dynamic
# Island de pantalla mediana, iPad de 13" (las apps declaran UIDeviceFamily 1 y 2) y Mac 16:10.
CAPTURAS = {
    "iphone": ("APP_IPHONE_61", "IOS", (1179, 2556)),
    "ipad": ("APP_IPAD_PRO_3GEN_129", "IOS", (2048, 2732)),
    "mac": ("APP_DESKTOP", "MAC_OS", (2880, 1800)),
}
CATEGORIAS = {"PRODUCTIVITY", "BUSINESS", "PHOTO_AND_VIDEO", "FINANCE", "UTILITIES", "SOCIAL_NETWORKING"}
# UE + EEE + Suiza, sin Francia por decisión del dueño el 2026-10-14 mientras no se
# tramite su declaración de cifrado. Liechtenstein no tiene App Store propia: son 29.
# Es una política de Apple; los países de Google Play se conservan en su propio guion.
PAISES = (
    "AUT BEL BGR HRV CYP CZE DNK EST FIN DEU GRC HUN IRL ITA LVA LTU LUX MLT NLD POL PRT ROU SVK SVN ESP SWE "
    "ISL NOR CHE"
).split()
# Estados en los que una versión se puede editar, y en los que una revisión está viva.
EDITABLE = {"PREPARE_FOR_SUBMISSION", "DEVELOPER_REJECTED", "REJECTED", "METADATA_REJECTED", "INVALID_BINARY"}
EN_REVISION = {"WAITING_FOR_REVIEW", "IN_REVIEW", "UNRESOLVED_ISSUES", "CANCELING"}
DATOS_REVISION = (
    ("contactFirstName", "APPSTORE_CONTACTO_NOMBRE"),
    ("contactLastName", "APPSTORE_CONTACTO_APELLIDOS"),
    ("contactPhone", "APPSTORE_CONTACTO_TELEFONO"),
    ("contactEmail", "APPSTORE_CONTACTO_CORREO"),
    ("demoAccountName", "APPSTORE_DEMO_USUARIO"),
    ("demoAccountPassword", "APPSTORE_DEMO_CLAVE"),
)

fallos = 0


def error(texto):
    global fallos
    print(f"::error::{texto}", file=sys.stderr)
    fallos += 1


def leer(ruta):
    with open(ruta, encoding="utf-8") as f:
        return f.read().rstrip("\n")


def bundle_de(app):
    try:
        m = re.search(r'^\s*ID\s*=\s*"([^"]+)"', leer(os.path.join(CMD, app, "FyneApp.toml")), re.M)
    except OSError:
        return ""
    return m.group(1) if m else ""


def las_quince():
    return sorted(d for d in os.listdir(CMD) if d != "demo" and os.path.isfile(os.path.join(CMD, d, "FyneApp.toml")))


def png(ruta):
    """(ancho, alto, tipo de color) del IHDR. El tipo 2 es RGB sin alfa, que es lo que pide Apple."""
    with open(ruta, "rb") as f:
        cabecera = f.read(26)
    if cabecera[:8] != b"\x89PNG\r\n\x1a\n":
        return (0, 0, -1)
    ancho, alto, _, color = struct.unpack(">IIBB", cabecera[16:26])
    return (ancho, alto, color)


def capturas_de(app, clase):
    d = os.path.join(FICHAS, app, "capturas")
    if not os.path.isdir(d):
        return []
    n = lambda f: int(re.search(r"-(\d+)\.png$", f).group(1))
    return [os.path.join(d, f) for f in sorted((f for f in os.listdir(d) if re.fullmatch(rf"{clase}-\d+\.png", f)), key=n)]


# ---------------------------------------------------------------- validación (--seco)


def validar(app, ficha):
    datos = ficha["apps"].get(app)
    if not datos:
        return error(f"{app}: no está en ficha.json")
    if not bundle_de(app):
        error(f"{app}: cmd/{app}/FyneApp.toml sin ID")
    if not re.fullmatch(r"\d{9,12}", datos.get("id", "")):
        error(f"{app}: el Apple ID «{datos.get('id')}» no es un número de App Store Connect")
    for campo in ("categoria", "secundaria"):
        if datos.get(campo) not in CATEGORIAS:
            error(f"{app}: {campo} «{datos.get(campo)}» no es una categoría de la App Store que usemos")
    if datos.get("categoria") == datos.get("secundaria"):
        error(f"{app}: la categoría secundaria repite la principal")
    d = os.path.join(FICHAS, app)
    if not os.path.isdir(os.path.join(d, ficha["idioma_principal"])):
        error(f"{app}: falta la ficha en {ficha['idioma_principal']}, el idioma principal")
    for loc in sorted(os.listdir(d)) if os.path.isdir(d) else []:
        if loc == "capturas" or not os.path.isdir(os.path.join(d, loc)):
            continue
        if loc not in LOCALES:
            error(f"{app}/{loc}: no es un locale de App Store Connect de los nuestros")
            continue
        for campo, tope in LIMITES.items():
            ruta = os.path.join(d, loc, campo + ".txt")
            if not os.path.isfile(ruta):
                if campo not in OPCIONALES:
                    error(f"{app}/{loc}: falta {campo}.txt")
                continue
            n = len(leer(ruta))
            if n == 0 or n > tope:
                error(f"{app}/{loc}/{campo}.txt: {n} caracteres (máximo {tope})")
    for clase, (_, _, medida) in CAPTURAS.items():
        ficheros = capturas_de(app, clase)
        if not 1 <= len(ficheros) <= 10:
            error(f"{app}: {len(ficheros)} capturas de {clase} (Apple pide de 1 a 10)")
        for f in ficheros:
            ancho, alto, color = png(f)
            if (ancho, alto) != medida:
                error(f"{app}: {os.path.basename(f)} es {ancho}×{alto} y tiene que ser {medida[0]}×{medida[1]}")
            if color != 2:
                error(f"{app}: {os.path.basename(f)} lleva canal alfa o paleta (tipo {color}): Apple pide RGB sin transparencia")
    notas = ficha["revision"]["notas"] + "\n\n" + datos.get("notas", "")
    if len(notas) > 4000:
        error(f"{app}: las notas para la revisión pasan de 4000 caracteres")


def validar_comun(ficha):
    for campo in ("soporte", "marketing", "privacidad"):
        if not str(ficha.get(campo, "")).startswith("https://"):
            error(f"ficha.json: {campo} tiene que ser una URL https")
    if not re.fullmatch(r"[A-Z0-9]{10}", ficha.get("equipo", "")):
        error("ficha.json: equipo no es un Team ID de Apple")
    ids = [a.get("id") for a in ficha["apps"].values()]
    if len(set(ids)) != len(ids):
        error("ficha.json: dos apps comparten el mismo Apple ID")
    # La cuenta de prueba del revisor la pone el dueño en los secretos de GitHub Actions: aquí, jamás.
    if re.search(r"(?i)password\s*[:=]|contraseña\s*[:=]", json.dumps(ficha, ensure_ascii=False)):
        error("ficha.json: lleva algo que parece una credencial; la cuenta de prueba no va en el repositorio")
    if ficha.get("ITSAppUsesNonExemptEncryption") is not None or "usesNonExemptEncryption" in json.dumps(ficha):
        error("ficha.json: el cifrado no se declara desde aquí (apps/fyne/appstore/README.md, «Cifrado»)")


# ---------------------------------------------------------------- la API


class Apple:
    def __init__(self):
        self.emisor = os.environ["APP_STORE_CONNECT_ISSUER_ID"]
        self.clave_id = os.environ["APP_STORE_CONNECT_KEY_IDENTIFIER"]
        self.clave = normalizar_clave(os.environ["APP_STORE_CONNECT_PRIVATE_KEY"])
        self.token, self.caduca = "", 0

    def _b64(self, b):
        return base64.urlsafe_b64encode(b).rstrip(b"=").decode()

    def _token(self):
        if time.time() < self.caduca - 120:
            return self.token
        ahora = int(time.time())
        cabecera = self._b64(json.dumps({"alg": "ES256", "kid": self.clave_id, "typ": "JWT"}).encode())
        # Apple admite como mucho veinte minutos; se renueva sola antes.
        cuerpo = self._b64(
            json.dumps({"iss": self.emisor, "iat": ahora, "exp": ahora + 900, "aud": "appstoreconnect-v1"}).encode()
        )
        firmar = f"{cabecera}.{cuerpo}".encode()
        # openssl solo lee la clave de un FICHERO: en un temporal 600 que se borra aquí mismo.
        fd, ruta = tempfile.mkstemp(prefix="asc-", suffix=".p8")
        try:
            os.fchmod(fd, 0o600)
            with os.fdopen(fd, "w") as f:
                f.write(self.clave if self.clave.endswith("\n") else self.clave + "\n")
            # ES256 exige EC P-256. Solo se inspecciona la parte PÚBLICA, capturada;
            # una clave de otro algoritmo tampoco debe acabar en un traceback.
            publica = subprocess.run(
                ["openssl", "pkey", "-in", ruta, "-text_pub", "-noout"], capture_output=True, check=True
            ).stdout
            if not re.search(rb"^\s*ASN1 OID: prime256v1\s*$", publica, re.M):
                raise ValueError("la clave no es EC P-256")
            der = subprocess.run(
                ["openssl", "dgst", "-sha256", "-sign", ruta, "-binary"], input=firmar, capture_output=True, check=True
            ).stdout
        except (subprocess.CalledProcessError, ValueError):
            # No se vuelca stderr: una credencial mal formada tampoco debe acabar en el log.
            sys.exit("::error::APP_STORE_CONNECT_PRIVATE_KEY no contiene una clave privada EC P-256 legible por openssl: revisa el .p8 completo del secreto de GitHub Actions APP_STORE_CONNECT_PRIVATE_KEY")
        finally:
            os.unlink(ruta)
        # openssl da la firma ECDSA en DER (SEQUENCE de dos INTEGER); el JWT la quiere como r‖s de 32 bytes.
        i = 4 if der[1] < 0x80 else 5
        lr = der[i - 1]
        r, s = der[i : i + lr], der[i + lr + 2 :]
        firma = b"".join(int.from_bytes(x, "big").to_bytes(32, "big") for x in (r, s))
        self.token, self.caduca = f"{cabecera}.{cuerpo}.{self._b64(firma)}", ahora + 900
        return self.token

    def pedir(self, metodo, ruta, cuerpo=None, tolerar=()):
        """Devuelve el JSON; None si el código está en `tolerar` (p. ej. 404); si no, lanza."""
        url = ruta if ruta.startswith("http") else API + ruta
        datos = json.dumps(cuerpo).encode() if cuerpo is not None else None
        for intento in range(4):
            pet = urllib.request.Request(url, data=datos, method=metodo)
            pet.add_header("Authorization", "Bearer " + self._token())
            if datos is not None:
                pet.add_header("Content-Type", "application/json")
            try:
                with urllib.request.urlopen(pet, timeout=120) as r:
                    texto = r.read()
                    return json.loads(texto) if texto else {}
            except urllib.error.HTTPError as e:
                if e.code in tolerar:
                    return None
                if e.code in (429, 500, 502, 503, 504) and intento < 3:
                    time.sleep(5 * (intento + 1))
                    continue
                try:
                    errores = [f"{x.get('code')}: {x.get('detail')}" for x in json.loads(e.read()).get("errors", [])]
                except ValueError:
                    errores = []
                raise RuntimeError(f"HTTP {e.code} {metodo} {ruta.split('?')[0]}: {'; '.join(errores) or 'sin detalle'}")
            except urllib.error.URLError as e:
                if intento < 3:
                    time.sleep(5 * (intento + 1))
                    continue
                raise RuntimeError(f"{metodo} {ruta.split('?')[0]}: {e.reason}")

    def lista(self, ruta):
        """Todas las páginas de una colección."""
        todo, r = [], self.pedir("GET", ruta)
        while r:
            todo += r.get("data", [])
            sig = r.get("links", {}).get("next")
            r = self.pedir("GET", sig) if sig else None
        return todo

    def escribir(self, tipo, attrs=None, rels=None, id=None, ruta=None):
        """POST (sin id) o PATCH (con id) de un recurso JSON:API."""
        data = {"type": tipo}
        if id:
            data["id"] = id
        if attrs:
            data["attributes"] = attrs
        if rels:
            data["relationships"] = {k: {"data": v} for k, v in rels.items()}
        base = ruta or f"/v1/{tipo}"
        return self.pedir("PATCH" if id else "POST", f"{base}/{id}" if id else base, {"data": data})


def rel(tipo, id):
    return {"type": tipo, "id": id}


def estado(v):
    a = v["attributes"]
    return a.get("appVersionState") or a.get("appStoreState") or a.get("state") or "?"


def comprobar_app(asc, app, ficha):
    """La app de ese Apple ID tiene que ser la NUESTRA: mismo bundle id que FyneApp.toml."""
    id = ficha["apps"][app]["id"]
    r = asc.pedir("GET", f"/v1/apps/{id}?fields[apps]=bundleId,name,primaryLocale,contentRightsDeclaration")
    visto = r["data"]["attributes"]
    if visto["bundleId"] != bundle_de(app):
        raise RuntimeError(f"el Apple ID {id} es de «{visto['bundleId']}», no de {bundle_de(app)}: ficha.json está mal")
    return id, visto


def versiones(asc, id, plataforma):
    return asc.lista(f"/v1/apps/{id}/appStoreVersions?filter[platform]={plataforma}&limit=50")


def builds_de(asc, id, plataforma, version, solo_validos=False):
    # El marketing version pertenece a preReleaseVersion; build.version es el
    # entero de CI_BUILD_NUMBER, distinto por producto en Xcode Cloud.
    ruta = f"/v1/builds?filter[app]={id}&filter[preReleaseVersion.version]={version}&filter[preReleaseVersion.platform]={plataforma}"
    if solo_validos:
        ruta += "&filter[processingState]=VALID&filter[expired]=false"
    return asc.lista(ruta + "&sort=-uploadedDate&limit=" + ("1" if solo_validos else "200"))


def revision_de(asc, id_version):
    r = asc.pedir("GET", f"/v1/appStoreVersions/{id_version}/appStoreReviewDetail", tolerar=(404,))
    return (r or {}).get("data") or {}


def comprobar_revision(asc, id, plataforma):
    vivas = asc.lista(f"/v1/reviewSubmissions?filter[app]={id}&filter[platform]={plataforma}&filter[state]=READY_FOR_REVIEW,{','.join(sorted(EN_REVISION))}")
    if any(s["attributes"]["state"] in EN_REVISION for s in vivas):
        raise RuntimeError(f"{plataforma}: ya hay una revisión en curso; no se cancela ni se envía otra")
    return vivas


def testflight_de(asc, build):
    # Solo metadatos de distribución: no se leen nombres, correos ni enlaces de
    # probadores. VALID por sí solo no significa que alguien pueda instalarlo.
    r = asc.pedir("GET", f"/v1/builds/{build}?include=betaGroups,buildBetaDetail&fields[builds]=betaGroups,buildBetaDetail&fields[betaGroups]=isInternalGroup,hasAccessToAllBuilds&fields[buildBetaDetails]=internalBuildState")
    relaciones = r.get("data", {}).get("relationships", {})
    incluidos = {(x["type"], x["id"]): x.get("attributes", {}) for x in r.get("included", [])}
    refs = relaciones.get("betaGroups", {}).get("data")
    grupos = None
    if refs is not None and all((x["type"], x["id"]) in incluidos for x in refs):
        grupos = [(x["id"], incluidos[(x["type"], x["id"])]) for x in refs]
    internos = None if grupos is None else [(id, a) for id, a in grupos if a.get("isInternalGroup") is True]
    probadores = None if internos is None else set()
    for id, _ in internos or []:
        # Solo identificadores para contar personas únicas, nunca sus atributos.
        for p in asc.lista(f"/v1/betaGroups/{id}/relationships/betaTesters?limit=200"):
            probadores.add((p["type"], p["id"]))
    detalle = relaciones.get("buildBetaDetail", {}).get("data")
    atributos = incluidos.get((detalle["type"], detalle["id"]), {}) if detalle else {}
    return {
        "internalBuildState": atributos.get("internalBuildState"),
        "internalGroups": None if internos is None else len(internos),
        "automaticInternalGroups": None if internos is None else sum(a.get("hasAccessToAllBuilds") is True for _, a in internos),
        "internalTesters": None if probadores is None else len(probadores),
    }


def errores_procesamiento(asc, id, plataforma, version, build):
    # Apple puede aceptar el transporte y rechazar después el binario. El motivo
    # vive en buildUploads.state, no en builds.processingState. Solo GET y campos
    # de diagnóstico: nunca URLs de carga, tokens ni datos de los probadores.
    ruta = (
        f"/v1/apps/{id}/buildUploads?filter[cfBundleShortVersionString]={version}"
        f"&filter[cfBundleVersion]={build}&filter[platform]={plataforma}"
        "&fields[buildUploads]=state&sort=-uploadedDate&limit=1"
    )
    estados = []
    for subida in asc.lista(ruta):
        datos = subida.get("attributes", {}).get("state") or {}
        estados.append({
            "state": datos.get("state"),
            "errors": [{"code": e.get("code"), "description": e.get("description")} for e in datos.get("errors", [])],
        })
    return estados


def sondear(asc, app, ficha, plataformas, version=""):
    id, visto = comprobar_app(asc, app, ficha)
    partes = []
    for p in plataformas:
        vs = versiones(asc, id, p)
        partes.append(f"{p}: " + (", ".join(f"{v['attributes']['versionString']} {estado(v)}" for v in vs) or "SIN VERSIÓN"))
        if not any(estado(v) in EDITABLE for v in vs):
            partes[-1] += " (ninguna editable)"
        if version:
            estados = []
            for b in builds_de(asc, id, p, version):
                a = b["attributes"]
                estados.append(
                    f"build {a.get('version', '?')}: {a.get('processingState', '?')} "
                    f"(expired={json.dumps(a.get('expired'))}, buildAudienceType={a.get('buildAudienceType', '?')}, "
                    f"usesNonExemptEncryption={json.dumps(a.get('usesNonExemptEncryption'))}, "
                    f"testFlight={json.dumps(testflight_de(asc, b['id']), sort_keys=True)})"
                )
                if a.get("processingState") in {"INVALID", "FAILED"}:
                    diagnostico = errores_procesamiento(asc, id, p, version, a["version"])
                    estados[-1] += "; processingDetails=" + json.dumps(diagnostico, sort_keys=True)
            partes[-1] += f"; builds {version}: " + (", ".join(estados) or "SIN BUILDS")
            objetivo = next((v for v in vs if v["attributes"]["versionString"] == version), None)
            if objetivo:
                da = revision_de(asc, objetivo["id"]).get("attributes", {})
                presencia = {campo: bool(da.get(campo)) for campo, _ in DATOS_REVISION}
                partes[-1] += "; reviewDetails=" + json.dumps(presencia, sort_keys=True)
                partes[-1] += "; releaseType=" + objetivo["attributes"].get("releaseType", "?")
            else:
                partes[-1] += f"; reviewDetails: SIN VERSIÓN {version}"
    print(f"✓ {app} ({visto['bundleId']}, {id}): existe, «{visto['name']}», {visto['primaryLocale']} · " + " · ".join(partes))


def revisiones_pendientes(asc, app_id, plataforma, versiones_app):
    """Sólo estados/refs de versión y build. Nunca actores, contacto ni feedback privado."""
    estados = ",".join(sorted(EN_REVISION | {"READY_FOR_REVIEW", "COMPLETING"}))
    envios = asc.lista(f"/v1/reviewSubmissions?filter[app]={app_id}&filter[platform]={plataforma}&filter[state]={estados}&fields[reviewSubmissions]=state&limit=200")
    versiones_por_id = {v["id"]: v for v in versiones_app}
    salida = []
    for envio in envios:
        detalle = {"state": estado(envio), "items": []}
        items = asc.lista(f"/v1/reviewSubmissions/{envio['id']}/items?fields[reviewSubmissionItems]=state,appStoreVersion&include=appStoreVersion&fields[appStoreVersions]=versionString,appStoreState,platform&limit=200")
        for item in items:
            resumen = {"state": estado(item)}
            ref = item.get("relationships", {}).get("appStoreVersion", {}).get("data")
            if ref:
                ident = ref["id"]
                version_app = versiones_por_id.get(ident)
                if version_app is None:
                    version_app = (asc.pedir("GET", f"/v1/appStoreVersions/{ident}?fields[appStoreVersions]=versionString,appStoreState,platform") or {}).get("data", {})
                attrs = version_app.get("attributes", {})
                resumen.update({"version": attrs.get("versionString"), "versionState": estado(version_app)})
                # Este endpoint de relación sólo admite fields[builds]. El
                # include de /builds/{id} produce HTTP 400 en esta ruta.
                build = asc.pedir("GET", f"/v1/appStoreVersions/{ident}/build?fields[builds]=version,processingState", tolerar=(404,)) or {}
                datos_build = build.get("data") or {}
                attrs_build = datos_build.get("attributes", {})
                resumen.update({"build": attrs_build.get("version"), "buildState": attrs_build.get("processingState"), "buildVersion": None})
                if datos_build.get("id"):
                    version_build = (asc.pedir("GET", f"/v1/builds/{datos_build['id']}/preReleaseVersion?fields[preReleaseVersions]=version,platform", tolerar=(404,)) or {}).get("data") or {}
                    resumen["buildVersion"] = version_build.get("attributes", {}).get("version")
            detalle["items"].append(resumen)
        salida.append(detalle)
    return salida


def preparacion(asc, app, ficha, plataformas, version=""):
    """Inventario previo a crear una versión. Nunca imprime textos ni datos del revisor."""
    id, visto = comprobar_app(asc, app, ficha)
    infos = asc.lista(f"/v1/apps/{id}/appInfos?limit=200")
    info = next((i for i in infos if estado(i) not in {"READY_FOR_DISTRIBUTION", "READY_FOR_SALE"}), infos[0] if infos else None)
    comunes = {"contentRights": bool(visto.get("contentRightsDeclaration")), "appInfo": bool(info)}
    if info:
        ident = info["id"]
        locs = asc.lista(f"/v1/appInfos/{ident}/appInfoLocalizations?limit=200")
        comunes["localizations"] = len(locs)
        primario = next((l.get("attributes", {}) for l in locs if l.get("attributes", {}).get("locale") == visto["primaryLocale"]), {})
        comunes["primaryName"] = bool(primario.get("name"))
        comunes["privacyPolicyUrl"] = bool(primario.get("privacyPolicyUrl"))
        categoria = asc.pedir("GET", f"/v1/appInfos/{ident}/primaryCategory", tolerar=(404,))
        comunes["primaryCategory"] = bool(categoria and categoria.get("data"))
        edad = asc.pedir("GET", f"/v1/appInfos/{ident}/ageRatingDeclaration", tolerar=(404,))
        comunes["ageRating"] = bool(edad and edad.get("data"))
    precio = asc.pedir("GET", f"/v1/appPriceSchedules/{id}/manualPrices?limit=1", tolerar=(404,))
    comunes["manualPrice"] = bool(precio and precio.get("data"))
    disponibilidad = asc.pedir("GET", f"/v1/apps/{id}/appAvailabilityV2", tolerar=(404,))
    comunes["availability"] = bool(disponibilidad and disponibilidad.get("data"))
    for plataforma in plataformas:
        vs = versiones(asc, id, plataforma)
        objetivo = next((v for v in vs if v["attributes"]["versionString"] == version), None)
        candidatas = [v for v in vs if estado(v) in EDITABLE] or vs
        origen = objetivo or max(candidatas, key=lambda v: tuple(map(int, re.findall(r"\d+", v["attributes"]["versionString"]))), default=None)
        datos = {"app": app, "platform": plataforma, "requestedVersion": version or None,
                 "targetVersionExists": bool(objetivo), "common": comunes,
                 "privacyLabels": "CHECK_IN_APP_STORE_CONNECT", "sourceVersion": None,
                 "reviewFeedback": "CHECK_IN_APP_STORE_CONNECT",
                 "reviewSubmissions": revisiones_pendientes(asc, id, plataforma, vs)}
        if origen:
            ident = origen["id"]
            a = origen["attributes"]
            datos["sourceVersion"] = a["versionString"]
            datos["sourceState"] = estado(origen)
            datos["copyright"] = bool(a.get("copyright"))
            locs = asc.lista(f"/v1/appStoreVersions/{ident}/appStoreVersionLocalizations?limit=200")
            datos["versionLocalizations"] = len(locs)
            primario = next((l for l in locs if l.get("attributes", {}).get("locale") == visto["primaryLocale"]), None)
            attrs = primario.get("attributes", {}) if primario else {}
            datos["primaryText"] = {k: bool(attrs.get(k)) for k in ("description", "keywords", "supportUrl")}
            juegos = asc.lista(f"/v1/appStoreVersionLocalizations/{primario['id']}/appScreenshotSets?limit=200") if primario else []
            capturas = {}
            for juego in juegos:
                tipo = juego["attributes"]["screenshotDisplayType"]
                imagenes = asc.lista(f"/v1/appScreenshotSets/{juego['id']}/appScreenshots?fields[appScreenshots]=assetDeliveryState&limit=200")
                capturas[tipo] = {"total": len(imagenes), "complete": sum((i.get("attributes", {}).get("assetDeliveryState") or {}).get("state") == "COMPLETE" for i in imagenes)}
            datos["screenshots"] = capturas
            datos["canonicalScreenshotsComplete"] = all(capturas.get(tipo, {}).get("complete", 0) > 0 for tipo, p, _ in CAPTURAS.values() if p == plataforma)
            detalle = revision_de(asc, ident).get("attributes", {})
            datos["reviewDetails"] = {campo: bool(detalle.get(campo)) for campo, _ in DATOS_REVISION}
            datos["demoAccountRequired"] = detalle.get("demoAccountRequired")
            datos["reviewNotes"] = bool(detalle.get("notes"))
        print(json.dumps(datos, sort_keys=True))


def version_editable(asc, id, plataforma, version):
    vs = [v for v in versiones(asc, id, plataforma) if estado(v) in EDITABLE]
    if vs:
        return vs[0]
    if any(estado(v) in EN_REVISION | {"PENDING_DEVELOPER_RELEASE"} for v in versiones(asc, id, plataforma)):
        raise RuntimeError(f"{plataforma}: hay una versión en revisión o pendiente de publicar; no se toca")
    if not version:
        raise RuntimeError(f"{plataforma}: no hay ninguna versión editable; pasa --version X.Y.Z para crearla")
    return asc.escribir("appStoreVersions", {"platform": plataforma, "versionString": version}, {"app": rel("apps", id)})[
        "data"
    ]


def subir_capturas(asc, loc_id, app, plataforma):
    juegos = {
        j["attributes"]["screenshotDisplayType"]: j
        for j in asc.lista(f"/v1/appStoreVersionLocalizations/{loc_id}/appScreenshotSets?limit=50")
    }
    for clase, (tipo, plat, _) in CAPTURAS.items():
        if plat != plataforma:
            continue
        ficheros = capturas_de(app, clase)
        md5 = [hashlib.md5(open(f, "rb").read()).hexdigest() for f in ficheros]  # noqa: S324 — lo pide la API
        juego = juegos.get(tipo) or asc.escribir(
            "appScreenshotSets", {"screenshotDisplayType": tipo}, {"appStoreVersionLocalization": rel("appStoreVersionLocalizations", loc_id)}
        )["data"]
        ya = asc.lista(f"/v1/appScreenshotSets/{juego['id']}/appScreenshots?limit=50")
        if [c["attributes"].get("sourceFileChecksum") for c in ya] == md5:
            continue  # las mismas, en el mismo orden: no se vuelven a subir
        for c in ya:
            asc.pedir("DELETE", f"/v1/appScreenshots/{c['id']}")
        for f, suma in zip(ficheros, md5):
            contenido = open(f, "rb").read()
            c = asc.escribir(
                "appScreenshots",
                {"fileName": os.path.basename(f), "fileSize": len(contenido)},
                {"appScreenshotSet": rel("appScreenshotSets", juego["id"])},
            )["data"]
            # Apple da las URL a las que mandar cada trozo; no llevan nuestro token.
            for op in c["attributes"]["uploadOperations"]:
                pet = urllib.request.Request(
                    op["url"], data=contenido[op["offset"] : op["offset"] + op["length"]], method=op["method"]
                )
                for h in op.get("requestHeaders", []):
                    pet.add_header(h["name"], h["value"])
                urllib.request.urlopen(pet, timeout=300).read()
            asc.escribir("appScreenshots", {"uploaded": True, "sourceFileChecksum": suma}, id=c["id"])


def publicar_ficha(asc, app, ficha, plataformas, version):
    datos = ficha["apps"][app]
    id, visto = comprobar_app(asc, app, ficha)
    d = os.path.join(FICHAS, app)
    locales = [l for l in LOCALES if os.path.isdir(os.path.join(d, l))]
    texto = lambda loc, campo: leer(os.path.join(d, loc, campo + ".txt")) if os.path.isfile(os.path.join(d, loc, campo + ".txt")) else None

    if visto.get("contentRightsDeclaration") != ficha["derechos_de_contenido"]:
        asc.escribir("apps", {"contentRightsDeclaration": ficha["derechos_de_contenido"]}, id=id)

    # --- Información de la app: nombre, subtítulo, privacidad, categorías y edades.
    infos = asc.lista(f"/v1/apps/{id}/appInfos")
    info = next((i for i in infos if estado(i) not in ("READY_FOR_DISTRIBUTION", "READY_FOR_SALE")), infos[0])
    asc.escribir(
        "appInfos",
        rels={
            "primaryCategory": rel("appCategories", datos["categoria"]),
            "secondaryCategory": rel("appCategories", datos["secundaria"]),
        },
        id=info["id"],
    )
    hechas = {l["attributes"]["locale"]: l["id"] for l in asc.lista(f"/v1/appInfos/{info['id']}/appInfoLocalizations?limit=200")}
    for loc in locales:
        attrs = {"name": texto(loc, "name"), "subtitle": texto(loc, "subtitle"), "privacyPolicyUrl": ficha["privacidad"]}
        if loc in hechas:
            asc.escribir("appInfoLocalizations", attrs, id=hechas[loc])
        else:
            asc.escribir("appInfoLocalizations", {**attrs, "locale": loc}, {"appInfo": rel("appInfos", info["id"])})
    edad = asc.pedir("GET", f"/v1/appInfos/{info['id']}/ageRatingDeclaration")["data"]
    asc.escribir("ageRatingDeclarations", {**ficha["edades"], **datos.get("edades", {})}, id=edad["id"])

    # --- Cada plataforma: su versión editable, con textos y capturas.
    pendientes_revision = []
    actualizadas = []
    omitidas = []
    for p in plataformas:
        try:
            v = version_editable(asc, id, p, version)
        except RuntimeError as e:
            if "no se toca" in str(e):
                print(f"  {app} {p}: {e}")
                omitidas.append(p)
                continue
            raise
        attrs = {"copyright": ficha["copyright"], "releaseType": ficha.get("publicacion", "AFTER_APPROVAL")}
        if version:
            attrs["versionString"] = version
        asc.escribir("appStoreVersions", attrs, id=v["id"])
        hechas = {
            l["attributes"]["locale"]: l["id"]
            for l in asc.lista(f"/v1/appStoreVersions/{v['id']}/appStoreVersionLocalizations?limit=200")
        }
        for loc in locales:
            attrs = {
                "description": texto(loc, "description"),
                "keywords": texto(loc, "keywords"),
                "promotionalText": texto(loc, "promotional_text"),
                "supportUrl": ficha["soporte"],
                "marketingUrl": ficha["marketing"],
            }
            if loc in hechas:
                asc.escribir("appStoreVersionLocalizations", attrs, id=hechas[loc])
            else:
                hechas[loc] = asc.escribir(
                    "appStoreVersionLocalizations", {**attrs, "locale": loc}, {"appStoreVersion": rel("appStoreVersions", v["id"])}
                )["data"]["id"]
        # Las capturas, solo al idioma principal: la App Store las enseña en los demás.
        subir_capturas(asc, hechas[ficha["idioma_principal"]], app, p)
        pendientes_revision.append((p, v["id"]))
        actualizadas.append(p)

    if not actualizadas and omitidas and len(plataformas) == 1:
        raise RuntimeError(f"{omitidas[0]}: hay una versión en revisión o pendiente de publicar; no se toca")

    # --- Precio (gratis) y países: SOLO si no están puestos. Lo que el dueño haya elegido
    # en la web no se pisa.
    hechos = []
    precios = asc.pedir("GET", f"/v1/appPriceSchedules/{id}/manualPrices?limit=1", tolerar=(404,))
    if not (precios and precios.get("data")):
        puntos = asc.lista(f"/v1/apps/{id}/appPricePoints?filter[territory]=ESP&limit=200")
        gratis = next(p for p in puntos if float(p["attributes"]["customerPrice"]) == 0)
        asc.pedir(
            "POST",
            "/v1/appPriceSchedules",
            {
                "data": {
                    "type": "appPriceSchedules",
                    "relationships": {
                        "app": {"data": rel("apps", id)},
                        "baseTerritory": {"data": rel("territories", "ESP")},
                        "manualPrices": {"data": [rel("appPrices", "${gratis}")]},
                    },
                },
                "included": [
                    {
                        "type": "appPrices",
                        "id": "${gratis}",
                        "attributes": {"startDate": None},
                        "relationships": {"appPricePoint": {"data": rel("appPricePoints", gratis["id"])}},
                    }
                ],
            },
        )
        hechos.append("precio gratis")
    if not asc.pedir("GET", f"/v1/apps/{id}/appAvailabilityV2", tolerar=(404,)):
        asc.pedir(
            "POST",
            "/v2/appAvailabilities",
            {
                "data": {
                    "type": "appAvailabilities",
                    "attributes": {"availableInNewTerritories": False},
                    "relationships": {
                        "app": {"data": rel("apps", id)},
                        "territoryAvailabilities": {"data": [rel("territoryAvailabilities", "${" + t + "}") for t in PAISES]},
                    },
                },
                "included": [
                    {
                        "type": "territoryAvailabilities",
                        "id": "${" + t + "}",
                        "attributes": {"available": True},
                        "relationships": {"territory": {"data": rel("territories", t)}},
                    }
                    for t in PAISES
                ],
            },
        )
        hechos.append(f"{len(PAISES)} países")
    # Un contacto incompleto no impide terminar las dos plataformas, el precio y
    # los países. Los errores siguen siendo rojos y se identifican por plataforma.
    notas = (ficha["revision"]["notas"] + "\n\n" + datos.get("notas", "")).strip()
    attrs = {"notes": notas, "demoAccountRequired": bool(ficha["revision"]["cuenta_de_prueba"])}
    # Los valores reales solo proceden de las variables secretas del dueño; si
    # faltan, se conserva lo que exista en App Store Connect.
    for campo, var in DATOS_REVISION:
        if os.environ.get(var):
            attrs[campo] = os.environ[var]
    errores_revision = []
    for p, id_version in pendientes_revision:
        try:
            detalle = revision_de(asc, id_version)
            if detalle:
                asc.escribir("appStoreReviewDetails", attrs, id=detalle["id"])
            else:
                asc.escribir("appStoreReviewDetails", attrs, {"appStoreVersion": rel("appStoreVersions", id_version)})
        except RuntimeError as e:
            errores_revision.append(f"{p}: {e}")
    resumen_plat = []
    if actualizadas:
        resumen_plat.append(" y ".join(actualizadas))
    if omitidas:
        resumen_plat.append("no se toca: " + " y ".join(omitidas))
    plat_str = ", ".join(resumen_plat) if resumen_plat else " y ".join(plataformas)
    if errores_revision:
        raise RuntimeError("textos, capturas, precio y países completados; reviewDetails pendiente: " + "; ".join(errores_revision))
    print(f"✓ {app}: ficha en {len(locales)} idiomas, capturas, categoría y edades ({plat_str})" + "".join(f", {h}" for h in hechos))


def comprobar_sin_francia(asc, id):
    disponibilidad = asc.pedir("GET", f"/v1/apps/{id}/appAvailabilityV2")["data"]
    if disponibilidad["attributes"].get("availableInNewTerritories") is not False:
        raise RuntimeError("no está desactivada la disponibilidad automática en territorios nuevos")
    territorios = asc.lista(f"/v2/appAvailabilities/{disponibilidad['id']}/territoryAvailabilities?include=territory&fields[territoryAvailabilities]=available,territory&limit=200")
    francia = [t for t in territorios if (t.get("relationships", {}).get("territory", {}).get("data") or {}).get("id") == "FRA"]
    if len(francia) != 1 or francia[0].get("attributes", {}).get("available") is not False:
        raise RuntimeError("Francia no consta inequívocamente excluida en App Store Connect")


def comprobar_build_exento(build):
    a = build["attributes"]
    if a.get("processingState") != "VALID" or a.get("expired") is not False or a.get("buildAudienceType") != "APP_STORE_ELIGIBLE":
        raise RuntimeError("el build no es válido, vigente y apto para App Store")
    valor = a.get("usesNonExemptEncryption")
    if "usesNonExemptEncryption" not in a or (valor is not None and valor is not False):
        raise RuntimeError("el cifrado ya tiene otra respuesta o no se pudo leer; no se sobrescribe")
    return valor is False


def completar_exencion(asc, apps, ficha, plataformas, version):
    """Solo null→false: política revisada, territorio real, todas las lecturas antes del primer PATCH."""
    plan, malas = [], []
    for app in apps:
        try:
            id, _ = comprobar_app(asc, app, ficha)
            comprobar_sin_francia(asc, id)
            for p in plataformas:
                comprobar_revision(asc, id, p)
                builds = builds_de(asc, id, p, version, solo_validos=True)
                if not builds:
                    raise RuntimeError(f"{p}: no hay build válido de {version}")
                b = builds[0]
                respondido = comprobar_build_exento(b)
                if not respondido:
                    declaracion = asc.pedir("GET", f"/v1/builds/{b['id']}/appEncryptionDeclaration", tolerar=(404,))
                    if declaracion and declaracion.get("data"):
                        raise RuntimeError(f"{p}: el build ya tiene una declaración de cifrado; no se modifica")
                plan.append((app, id, p, b["id"], respondido))
        except (RuntimeError, KeyError, TypeError, urllib.error.URLError) as e:
            print(f"✗ {app}: {e}", file=sys.stderr)
            malas.append(app)
    if malas:
        print(f"::error::preflight sin terminar: {' '.join(malas)}; ningún build modificado", file=sys.stderr)
        return 1

    for app, id, p, build, respondido in plan:
        try:
            if not respondido:
                # Detectar cambios desde el preflight sin pisar otra respuesta o una revisión.
                comprobar_sin_francia(asc, id)
                comprobar_revision(asc, id, p)
                respondido = comprobar_build_exento(asc.pedir("GET", f"/v1/builds/{build}")["data"])
                if not respondido:
                    declaracion = asc.pedir("GET", f"/v1/builds/{build}/appEncryptionDeclaration", tolerar=(404,))
                    if declaracion and declaracion.get("data"):
                        raise RuntimeError("apareció una declaración de cifrado; no se modifica")
                    asc.escribir("builds", {"usesNonExemptEncryption": False}, id=build)
                if not comprobar_build_exento(asc.pedir("GET", f"/v1/builds/{build}")["data"]):
                    raise RuntimeError("Apple no confirma usesNonExemptEncryption=false tras la operación")
            print(f"✓ {app} {p}: {version}, cifrado estándar exento confirmado; sin envío a revisión")
        except (RuntimeError, KeyError, TypeError, urllib.error.URLError) as e:
            print(f"✗ {app} {p}: {e}; no se reintenta con otra respuesta ni se revierten builds anteriores", file=sys.stderr)
            return 1
    return 0


def enviar_a_revision(asc, app, ficha, plataformas, version):
    id, _ = comprobar_app(asc, app, ficha)
    for p in plataformas:
        # Antes de tocar nada: si ya hay una revisión viva, se deja como está.
        vivas = comprobar_revision(asc, id, p)
        v = version_editable(asc, id, p, None)
        if v["attributes"]["versionString"] != version:
            raise RuntimeError(f"{p}: la versión editable es {v['attributes']['versionString']}, no {version}: pasa antes --ficha --version {version}")
        builds = builds_de(asc, id, p, version, solo_validos=True)
        if not builds:
            raise RuntimeError(f"{p}: no hay ningún build válido de la {version} en App Store Connect")
        build = builds[0]
        # Este emisor de fichas solo lee el cifrado: Xcode Cloud lo declara en el
        # Info.plist y la documentación se gestiona en App Store Connect con autorización.
        if build["attributes"].get("usesNonExemptEncryption") is None:
            raise RuntimeError(f"{p}: el build {build['attributes'].get('version')} no tiene contestado el cuestionario de cifrado: se responde en App Store Connect (apps/fyne/appstore/README.md, «Cifrado»), no aquí")
        da = revision_de(asc, v["id"]).get("attributes", {})
        faltan = [
            campo for campo, _ in DATOS_REVISION
            if (ficha["revision"]["cuenta_de_prueba"] or not campo.startswith("demoAccount")) and not da.get(campo)
        ]
        if faltan:
            raise RuntimeError(f"{p}: faltan campos para el revisor: {', '.join(faltan)}; se completan en App Store Connect o con los secretos de GitHub Actions APPSTORE_CONTACTO_*/APPSTORE_DEMO_*")
        asc.pedir("PATCH", f"/v1/appStoreVersions/{v['id']}/relationships/build", {"data": rel("builds", build["id"])})
        envio = next((s for s in vivas if s["attributes"]["state"] == "READY_FOR_REVIEW"), None) or asc.escribir(
            "reviewSubmissions", {"platform": p}, {"app": rel("apps", id)}
        )["data"]
        asc.escribir(
            "reviewSubmissionItems",
            rels={"reviewSubmission": rel("reviewSubmissions", envio["id"]), "appStoreVersion": rel("appStoreVersions", v["id"])},
        )
        asc.escribir("reviewSubmissions", {"submitted": True}, id=envio["id"])
        print(f"✓ {app} {p}: versión {version} (build {build['attributes'].get('version')}) ENVIADA A REVISIÓN")


# ---------------------------------------------------------------- principal


def main(argv):
    modo, version, plataformas, apps = "", "", ["IOS", "MAC_OS"], []
    i = 0
    while i < len(argv):
        a = argv[i]
        if a in ("--seco", "--sondear", "--preparacion", "--ficha", "--revision", "--cifrado-exento"):
            if modo:
                sys.exit("appstore-publicar: un solo modo cada vez")
            modo = a[2:]
        elif a == "--version" and i + 1 < len(argv):
            i += 1
            version = argv[i]
        elif a == "--plataformas" and i + 1 < len(argv):
            i += 1
            plataformas = argv[i].split()
        elif a.startswith("-"):
            modo = ""
            break
        else:
            apps.append(a)
        i += 1
    if not modo or not plataformas or set(plataformas) - {"IOS", "MAC_OS"}:
        print(f"uso: {sys.argv[0]} --seco|--sondear|--preparacion|--ficha|--revision|--cifrado-exento [--version X.Y.Z] [--plataformas \"IOS MAC_OS\"] [app...]", file=sys.stderr)
        return 2
    if version and not re.fullmatch(r"\d+\.\d+\.\d+", version):
        print(f"appstore-publicar: la versión tiene que ser X.Y.Z y es «{version}»", file=sys.stderr)
        return 2
    apps = apps or las_quince()

    if modo == "cifrado-exento":
        if os.environ.get("APPSTORE_CONFIRMAR_CIFRADO") != "ESTANDAR_EXENTO_SIN_FRANCIA" or not version:
            print("appstore-publicar: cifrado-exento requiere --version X.Y.Z y APPSTORE_CONFIRMAR_CIFRADO=ESTANDAR_EXENTO_SIN_FRANCIA", file=sys.stderr)
            return 2
        # Esta política es la ya revisada para la suite, no un clasificador automático.
        if "FRA" in PAISES:
            print("appstore-publicar: Francia está en la política de distribución; no se declara exención", file=sys.stderr)
            return 2
        for p in ("ios", "macos"):
            with open(os.path.join(AQUI, "..", "xcode", f"Info-{p}.plist"), "rb") as f:
                if plistlib.load(f).get("ITSAppUsesNonExemptEncryption") is not False:
                    print("appstore-publicar: la política de Xcode no declara exención estándar", file=sys.stderr)
                    return 2

    # El envío a revisión no admite atajos: la frase exacta, con el número de apps, o nada.
    # Se comprueba ANTES de leer una sola credencial.
    if modo == "revision":
        frase = f"ENVIAR_{len(apps)}_APPS_A_REVISION"
        if os.environ.get("APPSTORE_CONFIRMAR_REVISION", "") != frase:
            print(f"appstore-publicar: la revisión requiere APPSTORE_CONFIRMAR_REVISION={frase}", file=sys.stderr)
            return 2
        if not version:
            print("appstore-publicar: la revisión requiere --version X.Y.Z, la del build que se envía", file=sys.stderr)
            return 2

    with open(os.path.join(FICHAS, "ficha.json"), encoding="utf-8") as f:
        ficha = json.load(f)
    if modo not in ("sondear", "preparacion", "cifrado-exento"):
        validar_comun(ficha)
        for app in apps:
            validar(app, ficha)
    else:
        for app in apps:
            if app not in ficha["apps"]:
                error(f"{app}: no está en ficha.json")
    if fallos:
        print(f"appstore-publicar: {fallos} errores de validación", file=sys.stderr)
        return 1
    if modo == "seco":
        print(f"appstore-publicar --seco: {len(apps)} apps válidas ({' '.join(apps)})")
        return 0

    for v in ("APP_STORE_CONNECT_ISSUER_ID", "APP_STORE_CONNECT_KEY_IDENTIFIER", "APP_STORE_CONNECT_PRIVATE_KEY"):
        if not os.environ.get(v):
            print(f"::error::falta {v}: es un secreto de GitHub Actions (APP_STORE_CONNECT_*, CERTIFICATE_PRIVATE_KEY) que pasa appstore-publicar.yml", file=sys.stderr)
            return 1
    try:
        asc = Apple()
    except ValueError as e:
        entrada = os.environ["APP_STORE_CONNECT_PRIVATE_KEY"].strip().lstrip("\ufeff")
        tipo = next((p for p in ("@file:", "@env:", "-----BEGIN ") if entrada.startswith(p)), "contenido")
        if tipo == "contenido" and entrada.startswith(("{", "[", '"')):
            tipo = "JSON"
        # Solo categorías constantes: no se imprime la referencia, el nombre de
        # otra variable, una ruta, el PEM ni el error de una biblioteca externa.
        print(f"::error::APP_STORE_CONNECT_PRIVATE_KEY: {e} (entrada {tipo}); usa PEM del .p8, saltos escapados, base64 PEM/DER o referencias @file:/@env: de codemagic-cli-tools", file=sys.stderr)
        return 1
    if modo == "cifrado-exento":
        return completar_exencion(asc, apps, ficha, plataformas, version)
    hacer = {"sondear": sondear, "preparacion": preparacion, "ficha": publicar_ficha, "revision": enviar_a_revision}[modo]
    malas = []
    for app in apps:
        try:
            hacer(asc, app, ficha, plataformas, version)
        except (RuntimeError, KeyError, StopIteration, urllib.error.URLError) as e:
            print(f"✗ {app}: {e!r}" if not isinstance(e, RuntimeError) else f"✗ {app}: {e}", file=sys.stderr)
            malas.append(app)
    if malas:
        print(f"::error::sin terminar: {' '.join(malas)}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
