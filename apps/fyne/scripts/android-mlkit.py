#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
"""ML Kit GenAI (Gemini Nano) en la APK de Sytue Office, sin Gradle.

  android-mlkit.py resolver [--salida <dependencias.tsv>]   (red: solo quien sube la versión)
  android-mlkit.py bajar --cache <dir>                       (red o caché; sha256 siempre)
  android-mlkit.py preparar --cache <dir> --salida <dir>     (sin red)

POR QUÉ EXISTE. La IA del sistema en Android (`android/java/eu/sytue/nativo/IASistema.java`)
llama por reflexión a la Prompt API de ML Kit GenAI (`com.google.mlkit:genai-prompt`), que
habla con AICore. La APK la arma `empaquetar.sh` con javac contra `android.jar` y d8, sin
Gradle: nadie resuelve el árbol de dependencias de Maven, ni dexa los AAR, ni junta sus
recursos y manifiestos. Este guion hace esas tres cosas con lo MÍNIMO:

- `resolver` lee los POM desde `com.google.mlkit:genai-prompt` hacia abajo (alcance compile
  y runtime, sin opcionales) y, si una biblioteca aparece con dos versiones, se queda con la
  más alta, que es lo que hace Gradle, sin bajar de lo que piden las restricciones de sus
  metadatos `.module` y sus BOM. Escribe `android/mlkit/dependencias.tsv` con la versión,
  el repositorio, la licencia que declara el POM y la SUMA de cada fichero. Ese fichero se
  revisa y se versiona: nada se resuelve al empaquetar.
- `bajar` trae cada fichero de su repositorio (o de la caché) y comprueba su sha256 contra
  el fijado; uno que no case PARA el empaquetado.
- `preparar` deja en `<salida>`: `jars/` (el `classes.jar` y los `libs/*.jar` de cada AAR, y
  los JAR tal cual) para d8; `res/<n>-<artefacto>/` con los recursos de cada AAR que los
  trae y `rtxt/<paquete>.txt` con los símbolos de su R (las genera `android-widgets.py
  --edicion local` con los identificadores de cada app); `servicios/META-INF/services`, los
  de ServiceLoader de los JAR; y `manifiesto.xml`, lo que la Prompt API necesita declarado
  (ver `COMPONENTES`).

LOS COMPONENTES NO SE MEZCLAN A CIEGAS. Gradle fundiría el manifiesto de cada AAR en el de
la app, con sus permisos, servicios y receptores. Aquí entra SOLO lo que la Prompt API usa
para arrancar en el proceso de la app —el proveedor que inicializa ML Kit y el servicio que
descubre sus componentes— y la versión de Play Services que pide `play-services-basement`.
Lo demás se deja fuera A PROPÓSITO: en particular, el transporte de registros de Google
(`com.google.android.datatransport`, que ML Kit usa para mandar métricas de uso a Google)
no tiene declarado su servicio de descubrimiento de canales, así que no encuentra a quién
enviar nada (ver «ML Kit GenAI» en docs/operations.md). `preparar` falla si un AAR del
árbol declara un componente que no está en `COMPONENTES` ni en `DESCARTADOS`: así una
versión nueva no cuela uno sin que alguien lo mire.
"""
import argparse
import hashlib
import io
import json
from pathlib import Path
import re
import shutil
import sys
import time
import urllib.error
import urllib.request
import xml.etree.ElementTree as ET
import zipfile

FYNE = Path(__file__).resolve().parents[1]
FIJADO = FYNE / "android/mlkit/dependencias.tsv"
RAIZ = ("com.google.mlkit", "genai-prompt", "1.0.0-beta5")
REPOSITORIOS = {
    "google": "https://dl.google.com/dl/android/maven2",
    "central": "https://repo1.maven.org/maven2",
}
ANDROID = "http://schemas.android.com/apk/res/android"
ATTR = "{" + ANDROID + "}"
ET.register_namespace("android", ANDROID)

# Lo que entra en el manifiesto de la app, de lo que declaran los AAR: (etiqueta, nombre).
# Las meta-datos `com.google.firebase.components:*` del servicio de descubrimiento son los
# registradores de componentes de ML Kit: van todos los que declaren los AAR del árbol.
COMPONENTES = {
    # ML Kit se inicializa con este proveedor (MlKitContext) antes que nada de la app.
    ("provider", "com.google.mlkit.common.internal.MlKitInitProvider"),
    # Y encuentra sus componentes por las meta-datos de este servicio (no se arranca nunca).
    ("service", "com.google.mlkit.common.internal.MlKitComponentDiscoveryService"),
    # La versión de Play Services con la que se compiló basement, que él mismo comprueba.
    ("meta-data", "com.google.android.gms.version"),
    # La Prompt API se enlaza con el servicio de AICore: su permiso y verlo (Android 11+).
    ("uses-permission", "com.google.android.apps.aicore.service.BIND_SERVICE"),
    ("queries/package", "com.google.android.aicore"),
}
# Lo que se deja fuera a sabiendas, con su porqué en la cabecera y en docs/operations.md.
DESCARTADOS = {
    # El transporte de registros de Google (métricas de uso de ML Kit): sin su servicio de
    # descubrimiento no hay canal, y sin sus servicios de programación no se sube nada. Y
    # sin ellos sobran el estado de la red que pedía.
    ("service", "com.google.android.datatransport.runtime.backends.TransportBackendDiscovery"),
    ("service", "com.google.android.datatransport.runtime.scheduling.jobscheduling.JobInfoSchedulerService"),
    ("receiver", "com.google.android.datatransport.runtime.scheduling.jobscheduling.AlarmManagerSchedulerBroadcastReceiver"),
    ("uses-permission", "android.permission.ACCESS_NETWORK_STATE"),
    # INTERNET ya lo pide la app (manifiesto de empaquetar.sh): no se duplica.
    ("uses-permission", "android.permission.INTERNET"),
    # El diálogo de «actualiza Play Services» de play-services-base: la app no lo abre
    # nunca (Ajustes dice en su lugar qué falta).
    ("activity", "com.google.android.gms.common.api.GoogleApiActivity"),
    # androidx.startup arranca EmojiCompat (vistas de appcompat) y ProcessLifecycleOwner:
    # nada del camino de la Prompt API los usa, y la ventana es la de Fyne.
    ("provider", "androidx.startup.InitializationProvider"),
    # androidx.core: el permiso propio de ContextCompat.registerReceiver sin exportar en
    # Android < 13 (ML Kit GenAI no registra receptores) y su fábrica de componentes.
    ("permission", "${applicationId}.DYNAMIC_RECEIVER_NOT_EXPORTED_PERMISSION"),
    ("uses-permission", "${applicationId}.DYNAMIC_RECEIVER_NOT_EXPORTED_PERMISSION"),
    ("application@appComponentFactory", "androidx.core.app.CoreComponentFactory"),
}


# --- Versiones -------------------------------------------------------------------------

PREVIAS = {"alpha": 0, "a": 0, "beta": 1, "b": 1, "milestone": 2, "m": 2, "rc": 3, "cr": 3}


def _trozos(version):
    """Orden de versiones de Maven lo bastante fiel para las de este árbol: los números (1.0 =
    1.0.0) y después alpha < beta < milestone < rc < la final < cualquier otro sufijo
    (`-jre`, `-android`, `-empty-to-avoid-conflict-with-guava`)."""
    m = re.match(r"(\d+(?:\.\d+)*)(.*)$", version)
    numeros, resto = ([int(x) for x in m.group(1).split(".")], m.group(2)) if m else ([], version)
    while numeros and numeros[-1] == 0:
        numeros.pop()
    resto = resto.lstrip(".-")
    previa = re.fullmatch(r"([a-zA-Z]+)[.-]?(\d*)", resto)
    if not resto:
        sufijo = (4, 0, "")
    elif previa and previa.group(1).lower() in PREVIAS:
        sufijo = (PREVIAS[previa.group(1).lower()], int(previa.group(2) or 0), "")
    else:
        sufijo = (5, 0, resto)
    return tuple(numeros), sufijo


def mayor(a, b):
    return a if _trozos(a) >= _trozos(b) else b


# --- POM -------------------------------------------------------------------------------

def _texto(nodo, nombre, ns):
    e = nodo.find(ns + nombre)
    return e.text.strip() if e is not None and e.text else ""


def _bajar_bytes(url, intentos=6):
    """GET con reintentos: Maven Central contesta 429 si se le piden muchos POM seguidos."""
    for i in range(intentos):
        try:
            with urllib.request.urlopen(url, timeout=60) as r:
                return r.read()
        except urllib.error.HTTPError as e:
            if e.code not in (429, 500, 502, 503, 504) or i == intentos - 1:
                raise
            time.sleep(2 ** i)
    raise AssertionError("inalcanzable")


def _repositorios(grupo):
    """El repositorio de Google primero para lo suyo; Maven Central primero para lo demás."""
    propio = grupo.startswith(("com.google.android", "com.google.mlkit", "com.google.firebase", "androidx."))
    return sorted(REPOSITORIOS, key=lambda r: (r == "google") != propio)


def _ruta(grupo, artefacto, version, ext):
    return "%s/%s/%s/%s-%s.%s" % (grupo.replace(".", "/"), artefacto, version, artefacto, version, ext)


class Resolutor:
    def __init__(self):
        self.poms = {}

    def pom(self, grupo, artefacto, version):
        clave = (grupo, artefacto, version)
        if clave in self.poms:
            return self.poms[clave]
        ultimo = None
        for repo in _repositorios(grupo):
            try:
                datos = _bajar_bytes(REPOSITORIOS[repo] + "/" + _ruta(grupo, artefacto, version, "pom"))
            except Exception as e:  # noqa: BLE001 - se prueba el siguiente repositorio
                ultimo = e
                continue
            self.poms[clave] = (repo, ET.fromstring(datos))
            return self.poms[clave]
        raise SystemExit("no encuentro el POM de %s:%s:%s (%s)" % (grupo, artefacto, version, ultimo))

    def propiedades(self, raiz, ns, grupo, version):
        props = {"project.version": version, "project.groupId": grupo, "pom.version": version}
        padre = raiz.find(ns + "parent")
        if padre is not None:
            pg, pa, pv = (_texto(padre, k, ns) for k in ("groupId", "artifactId", "version"))
            _, praiz = self.pom(pg, pa, pv)
            pns = "{" + praiz.tag[1:].split("}")[0] + "}" if praiz.tag.startswith("{") else ""
            props.update(self.propiedades(praiz, pns, pg, pv))
            props["project.parent.version"] = pv
        p = raiz.find(ns + "properties")
        if p is not None:
            for e in p:
                props[e.tag.replace(ns, "")] = (e.text or "").strip()
        return props

    def gestionadas(self, raiz, ns, props):
        """Las versiones de <dependencyManagement> del POM y de sus padres (Guava las pone ahí)."""
        salida = {}
        padre = raiz.find(ns + "parent")
        if padre is not None:
            pg, pa, pv = (_texto(padre, k, ns) for k in ("groupId", "artifactId", "version"))
            _, praiz = self.pom(pg, pa, pv)
            pns = "{" + praiz.tag[1:].split("}")[0] + "}" if praiz.tag.startswith("{") else ""
            salida.update(self.gestionadas(praiz, pns, self.propiedades(praiz, pns, pg, pv)))
        for d in raiz.findall(ns + "dependencyManagement/" + ns + "dependencies/" + ns + "dependency"):
            v = re.sub(r"\$\{([^}]+)\}", lambda m: props.get(m.group(1), m.group(0)), _texto(d, "version", ns))
            salida[(_texto(d, "groupId", ns), _texto(d, "artifactId", ns))] = v
        return salida

    def modulo(self, grupo, artefacto, version):
        """Los metadatos de Gradle (`.module`) de una versión, o None si no se publicaron."""
        clave = (grupo, artefacto, version, "module")
        if clave not in self.poms:
            repo, _ = self.pom(grupo, artefacto, version)
            try:
                datos = _bajar_bytes(REPOSITORIOS[repo] + "/" + _ruta(grupo, artefacto, version, "module"))
                self.poms[clave] = json.loads(datos)
            except urllib.error.HTTPError as e:
                if e.code != 404:
                    raise
                self.poms[clave] = None
        return self.poms[clave]

    def restricciones(self, grupo, artefacto, version):
        """Las `dependencyConstraints` de sus variantes de ejecución en la JVM o Android, y las
        de las plataformas (BOM) de las que depende, como (grupo, artefacto, versión)."""
        datos = self.modulo(grupo, artefacto, version)
        if not datos:
            return []
        salida = []
        for var in datos.get("variants", []):
            at = var.get("attributes", {})
            if (at.get("org.gradle.usage") != "java-runtime" or "available-at" in var
                    or at.get("org.gradle.category") == "documentation"
                    or at.get("org.jetbrains.kotlin.platform.type", "jvm") not in ("jvm", "androidJvm")):
                continue
            for c in var.get("dependencyConstraints", []):
                v = c.get("version", {})
                v = v.get("strictly") or v.get("requires") or v.get("prefers")
                if v:
                    salida.append((c["group"], c["module"], v))
            for d in var.get("dependencies", []):
                if d.get("attributes", {}).get("org.gradle.category") in ("platform", "enforced-platform"):
                    v = d.get("version", {})
                    v = v.get("strictly") or v.get("requires") or v.get("prefers")
                    if v and (d["group"], d["module"], v) != (grupo, artefacto, version):
                        salida += self.plataforma(d["group"], d["module"], v)
        return salida

    def plataforma(self, grupo, artefacto, version):
        """Las versiones que fija un BOM: las de su `.module` o, si no lo publica (el de
        kotlinx-coroutines), las de <dependencyManagement> de su POM."""
        if self.modulo(grupo, artefacto, version):
            return self.restricciones(grupo, artefacto, version)
        _, raiz = self.pom(grupo, artefacto, version)
        ns = "{" + raiz.tag[1:].split("}")[0] + "}" if raiz.tag.startswith("{") else ""
        gestionadas = self.gestionadas(raiz, ns, self.propiedades(raiz, ns, grupo, version))
        return [(g, a, v) for (g, a), v in sorted(gestionadas.items()) if v and "$" not in v]

    def licencias(self, raiz, ns):
        """Las del POM o, si no declara ninguna, las de su padre (Guava las pone en guava-parent)."""
        propias = [_texto(l, "name", ns) for l in raiz.findall(ns + "licenses/" + ns + "license")]
        padre = raiz.find(ns + "parent")
        if propias or padre is None:
            return propias
        pg, pa, pv = (_texto(padre, k, ns) for k in ("groupId", "artifactId", "version"))
        _, praiz = self.pom(pg, pa, pv)
        pns = "{" + praiz.tag[1:].split("}")[0] + "}" if praiz.tag.startswith("{") else ""
        return self.licencias(praiz, pns)

    def dependencias(self, grupo, artefacto, version):
        repo, raiz = self.pom(grupo, artefacto, version)
        ns = "{" + raiz.tag[1:].split("}")[0] + "}" if raiz.tag.startswith("{") else ""
        props = self.propiedades(raiz, ns, grupo, version)
        gestionadas = self.gestionadas(raiz, ns, props)

        def sustituir(v):
            return re.sub(r"\$\{([^}]+)\}", lambda m: props.get(m.group(1), m.group(0)), v)

        empaquetado = _texto(raiz, "packaging", ns) or "jar"
        licencias = self.licencias(raiz, ns)
        deps = []
        for d in raiz.findall(ns + "dependencies/" + ns + "dependency"):
            alcance = _texto(d, "scope", ns) or "compile"
            if alcance not in ("compile", "runtime") or _texto(d, "optional", ns) == "true":
                continue
            g, a, v = (sustituir(_texto(d, k, ns)) for k in ("groupId", "artifactId", "version"))
            v = v or gestionadas.get((g, a), "")
            # Los POM viejos de Play Services fijan con «[16.0.1]»: es una versión exacta.
            v = v[1:-1] if re.fullmatch(r"\[[^,\]]+\]", v) else v
            if not v or "$" in v or re.search(r"[\[\]\(\),]", v):
                raise SystemExit("%s:%s:%s pide %s:%s con versión «%s», que no sé resolver" % (grupo, artefacto, version, g, a, v))
            deps.append((g, a, v, _texto(d, "type", ns) or ""))
        return repo, empaquetado, " / ".join(l for l in licencias if l) or "-", deps


def _grafo(r, forzadas):
    """Versiones elegidas (la más alta, sin bajar de las restricciones) y lo alcanzable."""
    elegida, pendientes, visto = {}, [RAIZ], set()
    # Punto fijo: cada vez que sube la versión de una biblioteca se recorren SUS dependencias.
    while pendientes:
        g, a, v = pendientes.pop()
        v = mayor(v, forzadas.get((g, a), v))
        if (g, a, v) in visto:
            continue
        visto.add((g, a, v))
        anterior = elegida.get((g, a))
        if anterior is not None and mayor(anterior, v) == anterior:
            continue
        elegida[(g, a)] = v
        for dg, da, dv, _ in r.dependencias(g, a, v)[3]:
            pendientes.append((dg, da, dv))
    # Lo alcanzable con las versiones elegidas (una versión vieja descartada no aporta hijos).
    alcanzable, cola = set(), [RAIZ[:2]]
    while cola:
        ga = cola.pop()
        if ga in alcanzable:
            continue
        alcanzable.add(ga)
        cola.extend((dg, da) for dg, da, _, _ in r.dependencias(ga[0], ga[1], elegida[ga])[3])
    return elegida, alcanzable


def resolver(salida):
    r = Resolutor()
    # Las restricciones de los metadatos de Gradle (`.module`) también suben versiones: así
    # kotlin-stdlib 2.x lleva a stdlib-jdk7/jdk8 a las vacías de la 1.8 (si no, sus clases
    # estarían dos veces) y el BOM de kotlinx-coroutines alinea los módulos de coroutines.
    forzadas = {}
    while True:
        elegida, alcanzable = _grafo(r, forzadas)
        nuevas = dict(forzadas)
        for g, a in alcanzable:
            for cg, ca, cv in r.restricciones(g, a, elegida[(g, a)]):
                nuevas[(cg, ca)] = mayor(cv, nuevas.get((cg, ca), cv))
        if nuevas == forzadas:
            break
        forzadas = nuevas
    filas = []
    for g, a in sorted(alcanzable):
        v = elegida[(g, a)]
        repo, empaquetado, licencia, _ = r.dependencias(g, a, v)
        if empaquetado == "pom":
            filas.append((g, a, v, "pom", repo, "-", licencia))
            continue
        ext = "aar" if empaquetado == "aar" else "jar"
        datos = _bajar_bytes(REPOSITORIOS[repo] + "/" + _ruta(g, a, v, ext))
        filas.append((g, a, v, ext, repo, hashlib.sha256(datos).hexdigest(), licencia))
    cabecera = (
        "# Árbol de com.google.mlkit:genai-prompt para la APK de Sytue Office, sin Gradle.\n"
        "# Lo escribe `android-mlkit.py resolver` (la más alta si hay dos versiones, como\n"
        "# Gradle) y lo lee `android-mlkit.py bajar`, que comprueba cada suma. Se revisa a mano:\n"
        "# las licencias son las que declara cada POM (ver «ML Kit GenAI» en docs/operations.md).\n"
        "# grupo:artefacto:versión\ttipo\trepositorio\tsha256\tlicencia\n")
    texto = cabecera + "".join("%s:%s:%s\t%s\t%s\t%s\t%s\n" % f for f in filas)
    Path(salida).write_text(texto, encoding="utf-8")
    print("%d bibliotecas en %s" % (len(filas), salida))


# --- Fijado, bajar y preparar -----------------------------------------------------------

def leer_fijado(ruta=FIJADO):
    filas = []
    for linea in Path(ruta).read_text(encoding="utf-8").splitlines():
        if not linea or linea.startswith("#"):
            continue
        coord, tipo, repo, suma, licencia = linea.split("\t")
        g, a, v = coord.split(":")
        if tipo not in ("aar", "jar", "pom") or repo not in REPOSITORIOS:
            raise SystemExit("fila inválida en %s: %s" % (ruta, linea))
        if tipo != "pom" and not re.fullmatch(r"[0-9a-f]{64}", suma):
            raise SystemExit("suma inválida en %s: %s" % (ruta, linea))
        filas.append((g, a, v, tipo, repo, suma, licencia))
    if not any((g, a, v) == RAIZ for g, a, v, *_ in filas):
        raise SystemExit("%s no contiene %s:%s:%s" % (ruta, *RAIZ))
    return filas


def _suma(ruta):
    h = hashlib.sha256()
    with open(ruta, "rb") as f:
        for trozo in iter(lambda: f.read(1 << 20), b""):
            h.update(trozo)
    return h.hexdigest()


def bajar(cache):
    cache = Path(cache)
    cache.mkdir(parents=True, exist_ok=True)
    traidos = 0
    for g, a, v, tipo, repo, suma, _ in leer_fijado():
        if tipo == "pom":
            continue
        destino = cache / ("%s-%s.%s" % (suma, a, tipo))
        if destino.exists() and _suma(destino) == suma:
            continue
        url = REPOSITORIOS[repo] + "/" + _ruta(g, a, v, tipo)
        temporal = destino.with_suffix(".parcial")
        with urllib.request.urlopen(url, timeout=120) as r, open(temporal, "wb") as f:
            shutil.copyfileobj(r, f)
        if _suma(temporal) != suma:
            temporal.unlink()
            raise SystemExit("%s:%s:%s no casa con su sha256 fijado (%s): no se usa." % (g, a, v, url))
        temporal.rename(destino)
        traidos += 1
    print("ML Kit: %d bibliotecas bajadas, el resto de la caché %s" % (traidos, cache))


TOOLS = "{http://schemas.android.com/tools}"


def _declaraciones(manifiesto, origen):
    """Todo lo que un AAR pide al manifiesto de la app, como (clave, nombre, elemento)."""
    raiz = ET.fromstring(manifiesto)
    paquete = raiz.get("package", "")
    salida = []
    for e in raiz:
        if e.tag == "uses-sdk":
            continue  # el mínimo y el objetivo los fija empaquetar.sh
        if e.tag in ("uses-permission", "permission"):
            salida.append((e.tag, e.get(ATTR + "name", ""), e))
        elif e.tag == "queries":
            for q in e:
                salida.append(("queries/" + q.tag, q.get(ATTR + "name", q.get(ATTR + "authorities", "")), q))
        elif e.tag == "application":
            for clave, valor in e.attrib.items():
                if not clave.startswith(TOOLS):
                    salida.append(("application@" + clave.replace(ATTR, ""), valor, e))
            for c in e:
                nombre = c.get(ATTR + "name", "")
                salida.append((c.tag, paquete + nombre if nombre.startswith(".") else nombre, c))
        else:
            salida.append((e.tag, "", e))
    for clave, nombre, _ in salida:
        if (clave, nombre) not in COMPONENTES and (clave, nombre) not in DESCARTADOS:
            raise SystemExit("%s declara %s «%s», que no está revisado (COMPONENTES o DESCARTADOS de "
                             "android-mlkit.py): mira qué hace antes de meterlo o dejarlo fuera." % (origen, clave, nombre))
    return paquete, salida


def _sin_tools(e):
    """Copia de un elemento sin los atributos del espacio `tools` (los usa solo Gradle)."""
    copia = ET.Element(e.tag, {k: v for k, v in e.attrib.items() if not k.startswith(TOOLS)})
    for hijo in e:
        copia.append(_sin_tools(hijo))
    return copia


def _servicios(jar, servicios):
    """Los META-INF/services de un JAR (ServiceLoader), juntados por nombre."""
    with zipfile.ZipFile(io.BytesIO(jar)) as z:
        for nombre in z.namelist():
            if nombre.startswith("META-INF/services/") and not nombre.endswith("/"):
                lineas = servicios.setdefault(nombre[len("META-INF/services/"):], [])
                for l in z.read(nombre).decode("utf-8").splitlines():
                    l = l.split("#", 1)[0].strip()
                    if l and l not in lineas:
                        lineas.append(l)


def preparar(cache, salida):
    cache, salida = Path(cache), Path(salida)
    if salida.exists():
        shutil.rmtree(salida)
    for d in ("jars", "res", "rtxt"):
        (salida / d).mkdir(parents=True)
    permisos, consultas, aplicacion, servicio, registradores = [], [], [], None, {}
    servicios, n = {}, 0
    for g, a, v, tipo, repo, suma, _ in leer_fijado():
        if tipo == "pom":
            continue
        fichero = cache / ("%s-%s.%s" % (suma, a, tipo))
        if not fichero.exists() or _suma(fichero) != suma:
            raise SystemExit("falta %s:%s:%s en la caché %s (o no casa su suma): `bajar` primero." % (g, a, v, cache))
        if tipo == "jar":
            datos = fichero.read_bytes()
            (salida / "jars" / ("%s-%s.jar" % (a, v))).write_bytes(datos)
            _servicios(datos, servicios)
            continue
        with zipfile.ZipFile(fichero) as z:
            nombres = set(z.namelist())
            for nombre in sorted(nombres):
                if nombre.startswith("jni/"):
                    raise SystemExit("%s:%s:%s trae código nativo (%s): no está previsto." % (g, a, v, nombre))
                if nombre == "classes.jar" or (nombre.startswith("libs/") and nombre.endswith(".jar")):
                    datos = z.read(nombre)
                    sufijo = "" if nombre == "classes.jar" else "-" + Path(nombre).stem
                    (salida / "jars" / ("%s-%s%s.jar" % (a, v, sufijo))).write_bytes(datos)
                    _servicios(datos, servicios)
            paquete, declaraciones = _declaraciones(z.read("AndroidManifest.xml"), "%s:%s:%s" % (g, a, v))
            res = [x for x in nombres if x.startswith("res/") and not x.endswith("/")]
            if res:
                n += 1
                destino = salida / "res" / ("%02d-%s" % (n, a))
                for x in res:
                    (destino / x[4:]).parent.mkdir(parents=True, exist_ok=True)
                    (destino / x[4:]).write_bytes(z.read(x))
            # La R de la biblioteca: los símbolos que su código ve (los de su R.txt), con
            # los identificadores de la tabla final, que pone android-widgets.py al enlazar.
            if "R.txt" in nombres and z.read("R.txt").strip():
                (salida / "rtxt" / (paquete + ".txt")).write_bytes(z.read("R.txt"))
            for clave, nombre, e in declaraciones:
                if (clave, nombre) not in COMPONENTES:
                    continue
                if clave == "uses-permission":
                    permisos += [] if nombre in permisos else [nombre]
                elif clave == "queries/package":
                    consultas += [] if nombre in consultas else [nombre]
                elif clave == "service":
                    servicio = servicio if servicio is not None else _sin_tools(e)
                    for m in e.findall("meta-data"):
                        registradores[m.get(ATTR + "name")] = m.get(ATTR + "value")
                elif not any(x.get(ATTR + "name") == nombre for x in aplicacion):
                    aplicacion.append(_sin_tools(e))
    claves = {(clave, nombre) for clave, nombre in COMPONENTES}
    hallado = set((("uses-permission", p) for p in permisos)) | set(("queries/package", q) for q in consultas)
    hallado |= {(x.tag, x.get(ATTR + "name")) for x in aplicacion}
    if servicio is not None:
        hallado.add(("service", servicio.get(ATTR + "name")))
    if hallado != claves:
        raise SystemExit("el árbol ya no declara %s (o declara %s): ¿ha cambiado el arranque de ML Kit?" % (
            sorted(claves - hallado), sorted(hallado - claves)))
    if not any(k.startswith("com.google.firebase.components:") for k in registradores):
        raise SystemExit("ningún AAR del árbol registra componentes de ML Kit: ¿ha cambiado el arranque?")
    for m in list(servicio):
        servicio.remove(m)
    for clave in sorted(registradores):
        ET.SubElement(servicio, "meta-data", {ATTR + "name": clave, ATTR + "value": registradores[clave]})
    manifiesto = ET.Element("manifest")
    for p in permisos:
        ET.SubElement(manifiesto, "uses-permission", {ATTR + "name": p})
    q = ET.SubElement(manifiesto, "queries")
    for c in consultas:
        ET.SubElement(q, "package", {ATTR + "name": c})
    app = ET.SubElement(manifiesto, "application")
    for e in aplicacion + [servicio]:
        app.append(e)
    ET.ElementTree(manifiesto).write(salida / "manifiesto.xml", encoding="utf-8", xml_declaration=True)
    for nombre, lineas in servicios.items():
        destino = salida / "servicios/META-INF/services" / nombre
        destino.parent.mkdir(parents=True, exist_ok=True)
        destino.write_text("".join(l + "\n" for l in lineas), encoding="utf-8")
    print("ML Kit: %d JAR, %d bibliotecas con recursos, %d con R, %d registradores, permisos %s" % (
        len(list((salida / "jars").glob("*.jar"))), n, len(list((salida / "rtxt").glob("*.txt"))),
        len(registradores), " ".join(permisos)))


def main():
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = p.add_subparsers(dest="orden", required=True)
    r = sub.add_parser("resolver")
    r.add_argument("--salida", default=str(FIJADO))
    b = sub.add_parser("bajar")
    b.add_argument("--cache", required=True)
    pr = sub.add_parser("preparar")
    pr.add_argument("--cache", required=True)
    pr.add_argument("--salida", required=True)
    a = p.parse_args()
    if a.orden == "resolver":
        resolver(a.salida)
    elif a.orden == "bajar":
        bajar(a.cache)
    else:
        preparar(a.cache, a.salida)


if __name__ == "__main__":
    main()
