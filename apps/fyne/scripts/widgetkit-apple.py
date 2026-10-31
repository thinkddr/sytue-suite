#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
"""Compila las extensiones Apple con el SDK real y prepara su firma anidada.

El catálogo y sus validaciones son los del generador de Xcode, no otra tabla.
Sin --insertar no firma. Nunca publica ni modifica capacidades o perfiles.
"""
import argparse
import importlib.util
import json
import os
import pathlib
import plistlib
import re
import shutil
import stat
import subprocess
import sys


AQUI = pathlib.Path(__file__).resolve().parent
PROYECTOS = AQUI.parent / "xcode"


def generador():
    spec = importlib.util.spec_from_file_location("sytue_xcode", PROYECTOS / "generar.py")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def ejecutar(argumentos):
    subprocess.run([str(a) for a in argumentos], check=True)


def capturar(argumentos):
    return subprocess.check_output([str(a) for a in argumentos], text=True)


def identificador_firma(ruta):
    salida = subprocess.run(
        ["codesign", "-dv", str(ruta)],
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        text=True,
        check=False,
    )
    if salida.returncode != 0:
        raise ValueError(ruta.name + ": sin firma en SwiftSupport (" + salida.stdout.strip() + ")")
    coincidencia = re.search(r"^Identifier=(.+)$", salida.stdout, re.M)
    identificador = coincidencia.group(1).strip() if coincidencia else ""
    if not identificador or identificador == "not set":
        raise ValueError(ruta.name + ": sin signing ID en SwiftSupport (" + salida.stdout.strip() + ")")
    return identificador


def plist_del_host(app, plataforma):
    if app.is_symlink() or not app.is_dir():
        raise ValueError("el host debe ser un bundle real")
    ruta = app / ("Info.plist" if plataforma == "ios" else "Contents/Info.plist")
    with ruta.open("rb") as archivo:
        return plistlib.load(archivo)


def hacer_legible_publico(raiz):
    for carpeta, _, archivos in os.walk(raiz, followlinks=False):
        for ruta in [pathlib.Path(carpeta)] + [pathlib.Path(carpeta) / nombre for nombre in archivos]:
            if ruta.is_symlink():
                continue
            modo = ruta.stat().st_mode
            minimo = 0o555 if stat.S_ISDIR(modo) or (modo & 0o100) else 0o444
            if stat.S_IMODE(modo) & minimo != minimo:
                os.chmod(ruta, stat.S_IMODE(modo) | minimo)


def insertar(productos, host, destino, identidad, app, generador, incluir_widgets=True):
    """Firma sólo el contenido anidado. File Provider no depende de WidgetKit."""
    plataforma = "ios" if destino == "ios" else "macos"
    info = plist_del_host(host, plataforma)
    if info.get("CFBundleIdentifier") != app["id"]:
        raise ValueError("el bundle host no corresponde a la app del catálogo")
    if plataforma == "ios" and (host.parent.name != "Payload" or host.parent.is_symlink()):
        raise ValueError("el host iOS debe estar dentro del Payload real del IPA")
    raiz = host if plataforma == "ios" else host / "Contents"
    ejecutable = raiz / ("" if plataforma == "ios" else "MacOS") / info["CFBundleExecutable"]
    if incluir_widgets:
        enlace = capturar(["xcrun", "otool", "-L", ejecutable])
        puente_cargado = (r"^\s*@rpath/" + re.escape(generador.PUENTE_WIDGETS)
                          + r"\.framework/(?:Versions/A/)?" + re.escape(generador.PUENTE_WIDGETS) + r" \(")
        if not re.search(puente_cargado, enlace, re.M):
            raise ValueError("el host no carga el puente WidgetKit; no se incorpora un widget sin actualización")
        rpath = "@executable_path/Frameworks" if plataforma == "ios" else "@executable_path/../Frameworks"
        if not re.search(r"^\s*path " + re.escape(rpath) + r" \(offset \d+\)$",
                         capturar(["xcrun", "otool", "-l", ejecutable]), re.M):
            raise ValueError("el host no puede resolver su framework WidgetKit")
    umask_previa = os.umask(0o022)
    try:
        extensiones, frameworks = raiz / "PlugIns", raiz / "Frameworks"
        for carpeta in (extensiones, frameworks):
            if carpeta.is_symlink() or not carpeta.resolve().is_relative_to(host.resolve()):
                raise ValueError("el contenido anidado debe quedar dentro del host")
            carpeta.mkdir(parents=True, exist_ok=True)
            os.chmod(carpeta, 0o755)
        planes = []
        if incluir_widgets:
            planes.append(("Sytue-" + app["clave"] + "-Widgets.appex", app["id"] + ".widgets", "widgets",
                           generador.grupo_widgets(), "NSFileProtectionComplete" if plataforma == "ios" else "", ""))
        if app.get("archivos"):
            planes.append(("Archivos.appex", app["id"] + ".archivos", "archivos", "group." + app["id"],
                           "NSFileProtectionComplete" if plataforma == "ios" else "", generador.EQUIPO + "." + app["id"] + ".shared"))
        if not planes:
            raise ValueError("la app no declara ninguna extensión seleccionada")
        anidadas = []
        for nombre, identificador, carpeta, grupo, proteccion, llavero in planes:
            perfil = pathlib.Path(capturar(["bash", AQUI / "apple-firma.sh", "perfil-ruta", identificador, destino]).strip())
            derechos = PROYECTOS / app["clave"] / carpeta / ("ios.entitlements" if plataforma == "ios" else "macos.entitlements")
            ejecutar(["bash", AQUI / "apple-firma.sh", "verificar-perfil-grupo", perfil,
                      identificador, destino, grupo, proteccion, llavero, derechos])
            bundle = extensiones / nombre
            if bundle.is_symlink():
                raise ValueError("no se sustituye contenido anidado enlazado fuera del bundle")
            if bundle.exists():
                shutil.rmtree(bundle)
            shutil.copytree(productos / nombre, bundle, symlinks=True)
            perfil_destino = bundle / ("embedded.mobileprovision" if plataforma == "ios" else "Contents/embedded.provisionprofile")
            shutil.copyfile(perfil, perfil_destino)
            # El árbol clonado del job Apple y su almacén privado son 0600; instalado
            # pertenece a root y debe poder leerlo quien ejecuta la app (90255).
            os.chmod(perfil_destino, 0o644)
            hacer_legible_publico(bundle)
            anidadas.append((bundle, derechos))
        puente = None
        if incluir_widgets:
            puente = frameworks / (generador.PUENTE_WIDGETS + ".framework")
            if puente.is_symlink():
                raise ValueError("no se sustituye contenido anidado enlazado fuera del bundle")
            if puente.exists():
                shutil.rmtree(puente)
            shutil.copytree(productos / puente.name, puente, symlinks=True)
            hacer_legible_publico(puente)
        soporte_swift = host.parent.parent / "SwiftSupport"
        soporte_iphoneos = soporte_swift / "iphoneos"
        ids_soporte = {}
        bibliotecas = ["xcrun", "swift-stdlib-tool", "--copy", "--platform", "iphoneos" if plataforma == "ios" else "macosx",
                       "--scan-executable", ejecutable, "--scan-folder", extensiones,
                       "--scan-folder", frameworks, "--destination", frameworks, "--sign", identidad]
        if plataforma == "ios":
            if soporte_swift.is_symlink() or (soporte_swift.exists() and soporte_iphoneos.is_symlink()):
                raise ValueError("SwiftSupport no puede ser un enlace simbólico")
            if (soporte_swift.exists() and not soporte_swift.is_dir()) or (soporte_iphoneos.exists() and not soporte_iphoneos.is_dir()):
                raise ValueError("SwiftSupport/iphoneos debe ser un directorio")
            # swift-stdlib-tool añade por su cuenta el subdirectorio <platform> (iphoneos)
            # bajo --unsigned-destination: pasar SwiftSupport/iphoneos creaba
            # SwiftSupport/iphoneos/iphoneos y disparaba el rechazo 90430 en Apple.
            # Tampoco se pasa --strip-bitcode (retirado en Xcode 16+).
            soporte_swift.mkdir(parents=True, exist_ok=True)
            os.chmod(soporte_swift, 0o755)
            bibliotecas += ["--unsigned-destination", soporte_swift]
        else:
            bibliotecas += ["--Xcodesign", "--options=runtime", "--Xcodesign", "--timestamp"]
        ejecutar(bibliotecas)
        # En Xcode 27 swift-stdlib-tool deja copias *.dylib.original firmadas con el
        # certificado de cadena antiguo de Apple (serie 33, caducado), que App Store
        # Connect y notarytool rechazan (90284, 41166113491 en Sytue Cast/Rooms).
        for carpeta_swift in [frameworks] + ([soporte_swift] if plataforma == "ios" else []):
            if carpeta_swift.exists():
                for residuo in sorted(carpeta_swift.rglob("*.original")):
                    if residuo.is_symlink() or not residuo.is_file():
                        raise ValueError("residuo .original inesperado en soporte Swift")
                    residuo.unlink()
        for entrada in sorted(frameworks.iterdir()):
            if entrada.is_symlink() or not ((entrada.is_file() and entrada.suffix == ".dylib")
                                            or (entrada.is_dir() and entrada.suffix == ".framework")):
                raise ValueError("contenido no admitido en Frameworks: " + entrada.name)
        dylibs = sorted(p for p in frameworks.glob("*.dylib") if p.is_file() and not p.is_symlink())
        hacer_legible_publico(extensiones)
        hacer_legible_publico(frameworks)
        if plataforma == "ios" and soporte_swift.exists():
            dylibs_soporte = []
            for entrada in sorted(soporte_swift.rglob("*")):
                if entrada.is_symlink():
                    raise ValueError("contenido no admitido en SwiftSupport: " + entrada.name)
                if entrada.is_dir():
                    if entrada != soporte_iphoneos:
                        raise ValueError("contenido no admitido en SwiftSupport: " + entrada.name)
                    continue
                if entrada.parent != soporte_iphoneos or not entrada.is_file() or entrada.suffix != ".dylib":
                    raise ValueError("contenido no admitido en SwiftSupport: " + entrada.name)
                dylibs_soporte.append(entrada)
            if [p.name for p in dylibs_soporte] != [p.name for p in dylibs]:
                raise ValueError("SwiftSupport no coincide con Frameworks: "
                                 + repr([p.name for p in dylibs_soporte]) + " != " + repr([p.name for p in dylibs]))
            if dylibs_soporte:
                hacer_legible_publico(soporte_swift)
                for dylib_soporte in dylibs_soporte:
                    ids_soporte[dylib_soporte.name] = identificador_firma(dylib_soporte)
            else:
                shutil.rmtree(soporte_swift)
        firma = ["codesign", "-f", "-s", identidad]
        if plataforma == "macos":
            firma += ["--options", "runtime", "--timestamp"]
        for dylib in dylibs:
            ejecutar(firma + [dylib])
        if puente:
            ejecutar(firma + [puente])
        for bundle, derechos in anidadas:
            ejecutar(firma + ["--entitlements", derechos, bundle])
        for bundle in dylibs + ([puente] if puente else []) + [b for b, _ in anidadas]:
            ejecutar(["codesign", "--verify", "--strict", bundle])
    finally:
        os.umask(umask_previa)
    resumen = {"host": app["id"], "extensions": [p[1] for p in planes], "distribution": destino,
               "profileVerified": True, "nestedSignaturesVerified": True}
    if ids_soporte:
        resumen["swiftSupport"] = ids_soporte
    print(json.dumps(resumen))


def minimo_apple(plataforma):
    clave = "IPHONEOS_DEPLOYMENT_TARGET" if plataforma == "ios" else "MACOSX_DEPLOYMENT_TARGET"
    valores = re.findall(r"^" + clave + r" = (\d+\.\d+(?:\.\d+)?)$",
                         (PROYECTOS / "Sytue.xcconfig").read_text(), re.M)
    if len(valores) != 1:
        raise ValueError("Sytue.xcconfig debe declarar un mínimo único de " + plataforma)
    return tuple(int(n) for n in valores[0].split("."))


def verificar(productos, plataforma, apps, puente, version=None, build=None, incluir_widgets=True):
    """Comprueba los productos reales, no sólo los ajustes declarados de Xcode."""
    productos_esperados = [("Archivos.appex", a["id"] + ".archivos", "com.apple.fileprovider-nonui")
                           for a in apps if a.get("archivos")]
    if incluir_widgets:
        productos_esperados += [("Sytue-" + a["clave"] + "-Widgets.appex", a["id"] + ".widgets", "com.apple.widgetkit-extension") for a in apps]
        productos_esperados += [(puente + ".framework", "eu.sytue.widgetbridge", "")]
    for nombre, identificador, punto in productos_esperados:
        extension = bool(punto)
        bundle = productos / nombre
        info = bundle / ("Contents/Info.plist" if plataforma == "macos" and extension else "Info.plist")
        # macOS frameworks use the versioned standard bundle layout.
        if not info.is_file() and plataforma == "macos" and not extension:
            info = bundle / "Resources/Info.plist"
        with info.open("rb") as fichero:
            metadata = plistlib.load(fichero)
        if metadata.get("CFBundleIdentifier") != identificador:
            raise ValueError(nombre + ": identificador del producto incorrecto")
        if version and metadata.get("CFBundleShortVersionString") != version:
            raise ValueError(nombre + ": versión distinta a la del host")
        if build and str(metadata.get("CFBundleVersion")) != build:
            raise ValueError(nombre + ": build distinto al del host")
        ejecutable = bundle / ("Contents/MacOS" if plataforma == "macos" and extension else "") / metadata["CFBundleExecutable"]
        if extension and metadata.get("NSExtension", {}).get("NSExtensionPointIdentifier") != punto:
            raise ValueError(nombre + ": tipo de extensión incorrecto")
        cabecera = capturar(["xcrun", "otool", "-hv", ejecutable])
        lineas = cabecera.splitlines()
        tipos = []
        for indice, linea in enumerate(lineas[:-1]):
            campos = linea.split()
            if "filetype" in campos:
                datos = lineas[indice + 1].split()
                posicion = campos.index("filetype")
                tipos.append(datos[posicion] if len(datos) > posicion else "")
        if not tipos or any(t != ("EXECUTE" if extension else "DYLIB") for t in tipos):
            raise ValueError(nombre + ": el resultado no tiene el tipo Mach-O esperado")
        arquitecturas = capturar(["xcrun", "lipo", "-archs", ejecutable]).split()
        esperadas = {"arm64"} if plataforma == "ios" else {"arm64", "x86_64"}
        if set(arquitecturas) != esperadas:
            raise ValueError(nombre + ": arquitecturas incorrectas " + " ".join(arquitecturas))
        compilacion = capturar(["xcrun", "vtool", "-show-build", ejecutable])
        esperado = (16, 0) if plataforma == "ios" and punto == "com.apple.fileprovider-nonui" else minimo_apple(plataforma)
        # LC_VERSION_MIN_* uses 'version'; TOOL entries also have a version,
        # so read only the deployment command blocks below.
        bloques = re.split(r"(?=^Load command \d+)", compilacion, flags=re.M)
        minimos = []
        for bloque in bloques:
            if re.search(r"^\s*cmd LC_(?:BUILD_VERSION|VERSION_MIN_(?:IPHONEOS|MACOSX))\s*$", bloque, re.M):
                match = re.search(r"^\s*(?:minos|version)\s+(\d+(?:\.\d+){1,2})\s*$", bloque, re.M)
                if match:
                    minimos.append(tuple(int(n) for n in match.group(1).split(".")))
        if len(minimos) != len(esperadas) or any((m + (0, 0))[:3] != (esperado + (0, 0))[:3] for m in minimos):
            raise ValueError(nombre + ": mínimo real incorrecto " + repr(minimos))
        if not extension:
            simbolos = capturar(["xcrun", "nm", "-gU", ejecutable])
            if not re.search(r"\b_sytue_widgets_reload\s*$", simbolos, re.M):
                raise ValueError(nombre + ": falta el puente público de actualización")
        print(json.dumps({"producto": nombre, "plataforma": plataforma,
                          "arquitecturas": arquitecturas, "minimos": minimos,
                          "firmado": False}, ensure_ascii=False))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--salida", required=True, type=pathlib.Path)
    parser.add_argument("--plataforma", required=True, choices=("ios", "macos"))
    parser.add_argument("--sin-widgets", action="store_true", help="sólo File Provider; independiente del interruptor WidgetKit")
    parser.add_argument("--version")
    parser.add_argument("--build")
    parser.add_argument("--insertar", type=pathlib.Path, help="incorporar y firmar el widget antes de la firma del host")
    parser.add_argument("--destino", choices=("ios", "macappstore", "developerid"))
    parser.add_argument("--identidad", help="identidad pública de firma ya importada por el workflow")
    parser.add_argument("apps", nargs="*")
    opciones = parser.parse_args()
    fuente = generador()
    catalogo = {a["clave"]: a for a in fuente.apps() if a.get("archivos") or (not opciones.sin_widgets and a["widget_acciones"])}
    nombres = opciones.apps or list(catalogo)
    if len(set(nombres)) != len(nombres) or any(a not in catalogo for a in nombres):
        parser.error("las apps deben pertenecer al catálogo WidgetKit y no repetirse")
    apps = [catalogo[n] for n in nombres]
    if opciones.insertar:
        # Compilar se puede siempre (el CI lo hace); meterlo en un host firmado, solo con
        # el interruptor SYTUE_WIDGETS_APPLE encendido (Sytue.xcconfig).
        if not opciones.sin_widgets and not fuente.widgets_apple():
            parser.error("SYTUE_WIDGETS_APPLE está apagado: ningún host lleva WidgetKit")
        if len(apps) != 1 or not opciones.destino or not opciones.identidad:
            parser.error("insertar requiere una app, --destino y --identidad")
        if (opciones.destino == "ios") != (opciones.plataforma == "ios"):
            parser.error("el destino debe coincidir con la plataforma compilada")
        info = plist_del_host(opciones.insertar, opciones.plataforma)
        if ((opciones.version and opciones.version != info["CFBundleShortVersionString"])
                or (opciones.build and opciones.build != str(info["CFBundleVersion"]))):
            parser.error("la versión y el build deben coincidir con el host")
        opciones.version = opciones.version or info["CFBundleShortVersionString"]
        opciones.build = opciones.build or str(info["CFBundleVersion"])
    elif opciones.destino or opciones.identidad:
        parser.error("--destino y --identidad sólo se usan con --insertar")
    if opciones.version and not re.fullmatch(r"\d+\.\d+\.\d+", opciones.version):
        parser.error("--version debe ser X.Y.Z")
    if opciones.build and not re.fullmatch(r"[1-9]\d*", opciones.build):
        parser.error("--build debe ser un entero positivo")
    productos = opciones.salida.resolve() / opciones.plataforma / "productos"
    objetos = opciones.salida.resolve() / opciones.plataforma / "objetos"
    argumentos = ["xcodebuild", "-project", PROYECTOS / fuente.PROYECTO,
                  "-configuration", "Release", "-sdk", "iphoneos" if opciones.plataforma == "ios" else "macosx",
                  "-jobs", "2"]
    if not opciones.sin_widgets:
        argumentos += ["-target", fuente.PUENTE_WIDGETS]
    for app in apps:
        if not opciones.sin_widgets:
            argumentos += ["-target", app["clave"] + "-widgets"]
        if app.get("archivos"):
            argumentos += ["-target", app["clave"] + "-archivos"]
    argumentos += ["build", "SYMROOT=" + str(productos.parent), "OBJROOT=" + str(objetos),
                   "CONFIGURATION_BUILD_DIR=" + str(productos), "ARCHS=" + ("arm64" if opciones.plataforma == "ios" else "arm64 x86_64"),
                   "ONLY_ACTIVE_ARCH=NO", "CODE_SIGN_STYLE=Manual", "CODE_SIGNING_ALLOWED=NO",
                   "CODE_SIGNING_REQUIRED=NO", "CODE_SIGN_IDENTITY=", "DEVELOPMENT_TEAM="]
    if opciones.version:
        argumentos += ["MARKETING_VERSION=" + opciones.version]
    if opciones.build:
        argumentos += ["CURRENT_PROJECT_VERSION=" + opciones.build]
    ejecutar(argumentos)
    verificar(productos, opciones.plataforma, apps, fuente.PUENTE_WIDGETS, opciones.version, opciones.build, not opciones.sin_widgets)
    if opciones.insertar:
        insertar(productos, opciones.insertar.resolve(), opciones.destino, opciones.identidad, apps[0], fuente, not opciones.sin_widgets)


if __name__ == "__main__":
    try:
        main()
    except (ValueError, KeyError, OSError, subprocess.CalledProcessError) as error:
        print("::error::WidgetKit: " + str(error), file=sys.stderr)
        sys.exit(1)
