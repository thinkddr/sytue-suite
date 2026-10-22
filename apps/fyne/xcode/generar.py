#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
"""EL PROYECTO DE XCODE DE LAS APPS DE apps/fyne, PARA XCODE CLOUD.

    python3 apps/fyne/xcode/generar.py              # reescribe el proyecto, plists y lo de cada app
    python3 apps/fyne/xcode/generar.py --comprobar  # rojo si lo versionado no es lo que saldría

Xcode Cloud solo construye proyectos de Xcode versionados («a consistent Xcode project or
workspace that's continuously present»), con esquemas compartidos y firma automática. Las
apps son de Go (Fyne), así que el proyecto es un ENVOLTORIO: un target por app, para iOS y
macOS a la vez (mismo App ID, un producto de Xcode Cloud por app), sin fuentes. Su fase
«Compilar el Go» llama a `empaquetar.sh xcode`, que compila con la misma cadena que
Codemagic y pone el esquema y los permisos en el Info.plist; Xcode pone el icono, firma,
archiva y sube.

QUÉ APPS HAY LO DICE EL ÁRBOL, como en empaquetar.sh: cada `apps/fyne/cmd/<app>/FyneApp.toml`
menos la demo. Lo que sale de aquí no se edita a mano: se cambia el FyneApp.toml (o este
guion) y se vuelve a generar. Los derechos de macOS son los de `empaquetar.sh
derechos-tienda`, la tabla que vigila empaquetar-test.sh. Lo común va en Sytue.xcconfig.
"""
import copy
import hashlib
import json
import os
import plistlib
import re
import shutil
import subprocess
import sys
import tempfile
import xml.etree.ElementTree as ET

AQUI = os.path.dirname(os.path.abspath(__file__))
FYNE = os.path.dirname(AQUI)
RAIZ = os.path.dirname(os.path.dirname(FYNE))
SVG = "http://www.w3.org/2000/svg"
ET.register_namespace("", SVG)
EQUIPO = "VHC5QW7BZ3"
# El dominio de la suite que se publica en las tiendas (empaquetar.sh: SYTUE_DOMINIO).
DOMINIO = "sytue.eu"
PROYECTO = "Sytue.xcodeproj"
# LA EXTENSIÓN FILE PROVIDER (Drive en el Finder y en Archivos): su Swift, escrito a mano,
# vive aquí al lado y no lo genera este guion; su target, Info.plist y derechos, sí, en
# la carpeta de cada app que declara `archivos = "si"`. File Provider replicado pide
# macOS 13 (el mínimo de todas) e iOS 16.
PROVEEDOR = "proveedor-archivos"
IOS_ARCHIVOS = "16.0"
WIDGETS = "widgets"
PUENTE_WIDGETS = "SytueWidgetBridge"
# La versión de Xcode con la que se escribió: sin ella, Xcode ofrece «actualizar» el
# proyecto al abrirlo y lo reescribe.
XCODE = "2700"


def metadato(toml, clave):
    m = re.search(r'^\s*%s\s*=\s*"([^"]*)"' % re.escape(clave), toml, re.M)
    return m.group(1) if m else ""


def apps():
    for d in sorted(os.listdir(os.path.join(FYNE, "cmd"))):
        ruta = os.path.join(FYNE, "cmd", d, "FyneApp.toml")
        if d == "demo" or not os.path.isfile(ruta):
            continue
        with open(ruta, encoding="utf-8") as f:
            toml = f.read()
        app = {"clave": d, "nombre": metadato(toml, "Name"), "id": metadato(toml, "ID"),
               "icono": metadato(toml, "Icon"), "enlaces": metadato(toml, "enlaces"),
               "archivos": metadato(toml, "archivos") == "si",
               "marca": metadato(toml, "esquema").removeprefix("sytue-")}
        if not (app["nombre"] and app["id"] and app["icono"] and app["marca"]):
            sys.exit("apps/fyne/cmd/%s/FyneApp.toml no declara Name, ID, Icon o esquema" % d)
        acciones = metadato(toml, "widget_acciones")
        app["widget_acciones"] = acciones.split(",") if acciones else []
        permitidas = {"open", "search"}
        if app["marca"] == "mail":
            permitidas.add("compose")
        if app["marca"] == "calendar":
            permitidas.add("create")
        if (len(set(app["widget_acciones"])) != len(app["widget_acciones"])
                or not set(app["widget_acciones"]).issubset(permitidas)
                or (acciones and "open" not in app["widget_acciones"])):
            sys.exit("apps/fyne/cmd/%s/FyneApp.toml declara acciones de widget no admitidas" % d)
        yield app


def widgets_apple():
    """El interruptor de WidgetKit y del App Group (SYTUE_WIDGETS_APPLE de Sytue.xcconfig).

    Apagado, los targets de los widgets siguen en el proyecto (widgetkit-apple.py y el CI
    los compilan), pero ningún host los lleva dentro ni pide el App Group: es la firma de
    antes de los widgets. La variable de entorno (1/0) es la de empaquetar.sh, para tests.
    """
    valor = os.environ.get("SYTUE_WIDGETS_APPLE", "")
    if not valor:
        with open(os.path.join(AQUI, "Sytue.xcconfig"), encoding="utf-8") as f:
            valores = re.findall(r"^SYTUE_WIDGETS_APPLE = ([A-Z]+)$", f.read(), re.M)
        if len(valores) > 1:
            sys.exit("Sytue.xcconfig declara SYTUE_WIDGETS_APPLE más de una vez")
        valor = valores[0] if valores else "NO"
    if valor not in ("1", "YES", "0", "NO"):
        sys.exit("SYTUE_WIDGETS_APPLE vale «%s»: solo YES/NO (o 1/0 en el entorno)" % valor)
    return valor in ("1", "YES")


def entorno_empaquetar():
    """El mismo interruptor para empaquetar.sh, resuelto aquí una sola vez."""
    return dict(os.environ, SYTUE_WIDGETS_APPLE="1" if widgets_apple() else "0")


# Manifiesto de privacidad de las extensiones: no rastrean, no recogen datos y no usan
# ninguna API de «motivo requerido» (ni UserDefaults, ni fechas de ficheros, ni
# systemUptime). Leen un JSON del App Group con FileHandle. Si eso cambia, se declara aquí.
PRIVACIDAD_WIDGETS = {
    "NSPrivacyTracking": False, "NSPrivacyTrackingDomains": [],
    "NSPrivacyCollectedDataTypes": [], "NSPrivacyAccessedAPITypes": [],
}


def grupo_widgets():
    """El contenedor, nunca el grupo de acceso al Keychain, tiene una sola fuente."""
    ruta = os.path.join(RAIZ, "apps", "nucleo", "ajustes", "grupo_apple.go")
    with open(ruta, encoding="utf-8") as f:
        grupos = re.findall(r'^const GrupoContenedorSuite = "(group\.[a-z0-9.]+)"$', f.read(), re.M)
    if len(grupos) != 1:
        sys.exit("ajustes/grupo_apple.go debe declarar un único GrupoContenedorSuite")
    return grupos[0]


def plist_widgets(datos):
    return plistlib.dumps(datos, fmt=plistlib.FMT_XML, sort_keys=True).decode()


def info_widgets(a):
    return plist_widgets({
        "CFBundleDevelopmentRegion": "en", "CFBundleDisplayName": a["nombre"],
        "CFBundleExecutable": "$(EXECUTABLE_NAME)", "CFBundleIdentifier": "$(PRODUCT_BUNDLE_IDENTIFIER)",
        "CFBundleInfoDictionaryVersion": "6.0", "CFBundleName": "$(PRODUCT_NAME)",
        "CFBundlePackageType": "XPC!", "CFBundleShortVersionString": "$(MARKETING_VERSION)",
        "CFBundleVersion": "$(CURRENT_PROJECT_VERSION)", "ITSAppUsesNonExemptEncryption": False,
        "NSExtension": {"NSExtensionPointIdentifier": "com.apple.widgetkit-extension"},
        "SytueWidgetBundle": a["id"], "SytueWidgetProduct": a["marca"],
        "SytueWidgetGroup": grupo_widgets(), "SytueWidgetScheme": "sytue-" + a["marca"],
        "SytueWidgetActions": a["widget_acciones"],
    })


def derechos_widgets(a, plataforma):
    datos = {"com.apple.developer.team-identifier": EQUIPO,
             "com.apple.security.application-groups": [grupo_widgets()]}
    identificador = EQUIPO + "." + a["id"] + ".widgets"
    if plataforma == "ios":
        datos.update({"application-identifier": identificador,
                      "com.apple.developer.default-data-protection": "NSFileProtectionComplete"})
    else:
        datos.update({"com.apple.application-identifier": identificador,
                      "com.apple.security.app-sandbox": True})
    return plist_widgets(datos)


def idiomas_widgets():
    """Las mismas traducciones del cliente Go; ninguna segunda tabla por idioma."""
    carpeta = os.path.join(RAIZ, "apps", "nucleo", "i18n", "locales")
    claves = (["widgets." + c for c in ("open", "compose", "create", "notes", "rows", "pending", "channels", "photos", "episodes", "apps", "calendars", "address_books")]
              + ["search.label"]
              + ["dataflow.item." + c for c in ("files", "events", "documents", "contacts")])
    for nombre in sorted(os.listdir(carpeta)):
        if not nombre.endswith(".json"):
            continue
        with open(os.path.join(carpeta, nombre), encoding="utf-8") as f:
            mensajes = json.load(f)
        for clave in claves:
            if not isinstance(mensajes.get(clave), str) or not mensajes[clave]:
                sys.exit("i18n/locales/%s no traduce %s" % (nombre, clave))
        lineas = "".join("%s = %s;\n" % (json.dumps(k), json.dumps(mensajes[k], ensure_ascii=False)) for k in claves)
        plural = mensajes.get("notifications.unread")
        if not isinstance(plural, dict) or not plural.get("other"):
            sys.exit("i18n/locales/%s no traduce notifications.unread" % nombre)
        reglas = {"NSStringFormatSpecTypeKey": "NSStringPluralRuleType", "NSStringFormatValueTypeKey": "lld"}
        reglas.update({k: v.replace("{count}", "%lld") for k, v in plural.items()})
        diccionario = {"notifications.unread": {"NSStringLocalizedFormatKey": "%#@value@", "value": reglas}}
        yield nombre[:-5], lineas, plist_widgets(diccionario)


def icono_apple(a):
    """La marca compartida, en capas SVG que Apple compone con Liquid Glass.

    El descriptor sigue la plantilla creada en Icon Composer 27 (Effects activado,
    sombra neutral 0,5 y translucencia 0,3); las capas siguen también el descriptor
    del ejemplo Landmarks de Apple. No se rasteriza ni se pinta el material:
    lo compila actool en Xcode Cloud, también para los sistemas anteriores.
    https://developer.apple.com/documentation/xcode/creating-your-app-icon-using-icon-composer
    https://developer.apple.com/documentation/swiftui/landmarks-building-an-app-with-liquid-glass
    """
    ruta = os.path.join(RAIZ, "branding", "marks", a["marca"] + "-maskable.svg")
    marca = ET.parse(ruta).getroot()
    if len(marca) != 3:
        sys.exit("%s: la marca no separa fondo, cuña y glifo" % ruta)
    fondo, cuna, glifo = list(marca)
    color = fondo.attrib["fill"]
    if not re.fullmatch(r"#[0-9a-fA-F]{6}", color) or glifo.tag != "{%s}g" % SVG:
        sys.exit("%s: la marca no tiene el fondo y glifo canónicos" % ruta)

    def capa(elemento):
        dibujo = ET.Element("{%s}svg" % SVG, {"viewBox": "0 0 512 512", "width": "1024", "height": "1024"})
        dibujo.append(copy.deepcopy(elemento))
        return ET.tostring(dibujo, encoding="unicode") + "\n"

    # El mismo anillo que scripts/gen-iconos-fyne.sh, sobre el mismo grid: conservar
    # la identidad al separar las capas, sin aplicar la máscara de la plataforma.
    with open(os.path.join(RAIZ, "branding", "tokens", "colors.css"), encoding="utf-8") as f:
        css = f.read()
    tinta = re.findall(r"--sytue-ink-rgb:\s*([0-9, ]+)", css)[1].replace(" ", "")
    alfa = re.search(r"--sytue-product-tile-ring:\s*rgba\(var\(--sytue-ink-rgb\),\s*([0-9.]+)\)", css).group(1)
    contorno = ET.Element("{%s}g" % SVG, {"transform": glifo.attrib["transform"]})
    ET.SubElement(contorno, "{%s}rect" % SVG, {
        "x": "3.5", "y": "3.5", "width": "33", "height": "33", "rx": "9.5", "ry": "9.5",
        "fill": "none", "stroke": "rgb(%s)" % tinta, "stroke-opacity": alfa, "stroke-width": "1"})
    capas = {"glifo.svg": capa(glifo), "contorno.svg": capa(contorno), "cuna.svg": capa(cuna)}

    def grupo(nombre, vidrio):
        capa = {"image-name": nombre + ".svg", "name": nombre}
        if not vidrio:
            capa["glass"] = False
        return {"layers": [capa], "shadow": {"kind": "neutral", "opacity": 0.5},
                "translucency": {"enabled": vidrio, "value": 0.3}}

    rgb = [int(color[i:i + 2], 16) / 255 for i in (1, 3, 5)]
    descriptor = {"fill": {"automatic-gradient": "extended-srgb:%.5f,%.5f,%.5f,1.00000" % tuple(rgb)},
                  "groups": [grupo("glifo", True), grupo("contorno", True), grupo("cuna", False)],
                  "supported-platforms": {"circles": ["watchOS"], "squares": "shared"}}
    return descriptor, capas


def ident(*partes):
    """Identificadores de objeto estables: los mismos en cada generación."""
    return hashlib.md5("/".join(partes).encode()).hexdigest()[:24].upper()


def valor(v, sangria):
    if isinstance(v, dict):
        dentro = sangria + "\t"
        return "{\n" + "".join("%s%s = %s;\n" % (dentro, valor(k, dentro), valor(v[k], dentro))
                               for k in sorted(v)) + sangria + "}"
    if isinstance(v, list):
        dentro = sangria + "\t"
        return "(\n" + "".join("%s%s,\n" % (dentro, valor(x, dentro)) for x in v) + sangria + ")"
    v = str(v)
    if re.fullmatch(r"[A-Za-z0-9_./]+", v):
        return v
    return '"' + v.replace("\\", "\\\\").replace('"', '\\"').replace("\n", "\\n").replace("\t", "\\t") + '"'


FASE = """# La fase de apps/fyne/scripts/empaquetar.sh: el Go de la app (SYTUE_APP) y su Info.plist.
# Xcode arranca las fases con un PATH mínimo: Go y la CLI de Fyne donde los deja
# ci_scripts/ci_post_clone.sh en Xcode Cloud, o donde estén en un Mac de desarrollo.
export PATH="$HOME/.sytue-toolchain/go/bin:/usr/local/go/bin:$HOME/go/bin:/opt/homebrew/bin:/usr/local/bin:$PATH"
export GOPRIVATE='github.com/thinkddr/*'
exec bash "$SRCROOT/../scripts/empaquetar.sh" xcode
"""


def pbxproj(lista):
    o = {}
    raiz = ident("proyecto")

    def fichero(clave, ruta, tipo):
        o[ident("ref", clave)] = {"isa": "PBXFileReference", "lastKnownFileType": tipo, "path": ruta,
                                  "sourceTree": "<group>"}
        return ident("ref", clave)

    def configuraciones(clave, ajustes, base=None):
        ids = []
        for nombre in ("Debug", "Release"):
            c = {"isa": "XCBuildConfiguration", "buildSettings": dict(ajustes.get(nombre, ajustes)),
                 "name": nombre}
            if base:
                c["baseConfigurationReference"] = base
            o[ident("config", clave, nombre)] = c
            ids.append(ident("config", clave, nombre))
        o[ident("configs", clave)] = {"isa": "XCConfigurationList", "buildConfigurations": ids,
                                      "defaultConfigurationIsVisible": 0, "defaultConfigurationName": "Release"}
        return ident("configs", clave)

    def extension(a, hijos):
        """El target de la extensión File Provider de `a`. Devuelve su clave."""
        c = a["clave"]
        e = c + "-archivos"
        hijos.extend([fichero(e + "/info", "archivos/Info.plist", "text.plist.xml"),
                      fichero(e + "/macos", "archivos/macos.entitlements", "text.plist.entitlements"),
                      fichero(e + "/ios", "archivos/ios.entitlements", "text.plist.entitlements")])
        o[ident("producto", e)] = {"isa": "PBXFileReference", "explicitFileType": "wrapper.app-extension",
                                   "includeInIndex": 0, "path": "Archivos.appex", "sourceTree": "BUILT_PRODUCTS_DIR"}
        productos.append(ident("producto", e))
        o[ident("bf", e)] = {"isa": "PBXBuildFile", "fileRef": ident("ref", PROVEEDOR + "/swift")}
        o[ident("fuentes", e)] = {"isa": "PBXSourcesBuildPhase", "buildActionMask": 2147483647,
                                  "files": [ident("bf", e)], "runOnlyForDeploymentPostprocessing": 0}
        ajustes = {"PRODUCT_NAME": "Archivos", "PRODUCT_BUNDLE_IDENTIFIER": a["id"] + ".archivos",
                   "INFOPLIST_FILE": c + "/archivos/Info.plist",
                   "INFOPLIST_FILE[sdk=macosx*]": c + "/archivos/Info.plist",
                   "CODE_SIGN_ENTITLEMENTS[sdk=macosx*]": c + "/archivos/macos.entitlements",
                   "CODE_SIGN_ENTITLEMENTS[sdk=iphoneos*]": c + "/archivos/ios.entitlements",
                   # El NSExtensionFileProviderDocumentGroup del Info.plist: el grupo de apps
                   # de los derechos de cada plataforma (empaquetar.sh derechos-archivos).
                   "SYTUE_GRUPO_ARCHIVOS": "group." + a["id"],
                   "SYTUE_GRUPO_ARCHIVOS[sdk=macosx*]": EQUIPO + "." + a["id"],
                   "IPHONEOS_DEPLOYMENT_TARGET": IOS_ARCHIVOS, "SWIFT_VERSION": "5.0",
                   "APPLICATION_EXTENSION_API_ONLY": "YES", "SKIP_INSTALL": "YES",
                   "LD_RUNPATH_SEARCH_PATHS": "$(inherited) @executable_path/../Frameworks @executable_path/../../Frameworks",
                   "LD_RUNPATH_SEARCH_PATHS[sdk=macosx*]": "$(inherited) @executable_path/../Frameworks @executable_path/../../../../Frameworks"}
        o[ident("target", e)] = {"isa": "PBXNativeTarget", "buildConfigurationList": configuraciones(e, ajustes),
                                 "buildPhases": [ident("fuentes", e)], "buildRules": [], "dependencies": [],
                                 "name": e, "productName": e, "productReference": ident("producto", e),
                                 "productType": "com.apple.product-type.app-extension"}
        objetivos.append(ident("target", e))
        return e

    def incluir(c, e, fases, dependencias, clave, carpeta, nombre, firmar=False):
        atributos = ["RemoveHeadersOnCopy"]
        if firmar:
            atributos.append("CodeSignOnCopy")
        o[ident("bf-embebida", clave)] = {"isa": "PBXBuildFile", "fileRef": ident("producto", e),
                                          "settings": {"ATTRIBUTES": atributos}}
        o[ident("embeber", clave)] = {"isa": "PBXCopyFilesBuildPhase", "buildActionMask": 2147483647,
                                      "dstPath": "", "dstSubfolderSpec": carpeta,
                                      "files": [ident("bf-embebida", clave)], "name": nombre,
                                      "runOnlyForDeploymentPostprocessing": 0}
        fases.append(ident("embeber", clave))
        o[ident("proxy", e)] = {"isa": "PBXContainerItemProxy", "containerPortal": raiz, "proxyType": 1,
                                "remoteGlobalIDString": ident("target", e), "remoteInfo": e}
        o[ident("dependencia", e)] = {"isa": "PBXTargetDependency", "target": ident("target", e),
                                      "targetProxy": ident("proxy", e)}
        dependencias.append(ident("dependencia", e))

    def widgets(a, hijos):
        c = a["clave"]
        e = c + "-widgets"
        producto = "Sytue-" + c + "-Widgets"
        hijos.extend([fichero(e + "/info", "widgets/Info.plist", "text.plist.xml"),
                      fichero(e + "/macos", "widgets/macos.entitlements", "text.plist.entitlements"),
                      fichero(e + "/ios", "widgets/ios.entitlements", "text.plist.entitlements")])
        imagen = fichero(e + "/icono", "../../cmd/%s/%s" % (c, a["icono"]), "image.png")
        hijos.append(imagen)
        o[ident("producto", e)] = {"isa": "PBXFileReference", "explicitFileType": "wrapper.app-extension",
                                   "includeInIndex": 0, "path": producto + ".appex", "sourceTree": "BUILT_PRODUCTS_DIR"}
        productos.append(ident("producto", e))
        fuentes = []
        for nombre in ("snapshot", "vista"):
            bf = ident("bf", e, nombre)
            o[bf] = {"isa": "PBXBuildFile", "fileRef": ident("ref", WIDGETS + "/" + nombre)}
            fuentes.append(bf)
        o[ident("fuentes", e)] = {"isa": "PBXSourcesBuildPhase", "buildActionMask": 2147483647,
                                  "files": fuentes, "runOnlyForDeploymentPostprocessing": 0}
        recursos = []
        for nombre, referencia in ([("icono", imagen), ("privacidad", ident("ref", WIDGETS + "/privacidad"))]
                                   + [(n, ident("idioma", n)) for n in ("Localizable.strings", "Localizable.stringsdict")]):
            bf = ident("bf", e, nombre)
            o[bf] = {"isa": "PBXBuildFile", "fileRef": referencia}
            recursos.append(bf)
        o[ident("recursos", e)] = {"isa": "PBXResourcesBuildPhase", "buildActionMask": 2147483647,
                                   "files": recursos, "runOnlyForDeploymentPostprocessing": 0}
        ajustes = {"PRODUCT_NAME": producto, "PRODUCT_BUNDLE_IDENTIFIER": a["id"] + ".widgets",
                   "INFOPLIST_FILE": c + "/widgets/Info.plist", "INFOPLIST_FILE[sdk=macosx*]": c + "/widgets/Info.plist",
                   "CODE_SIGN_ENTITLEMENTS[sdk=macosx*]": c + "/widgets/macos.entitlements",
                   "CODE_SIGN_ENTITLEMENTS[sdk=iphoneos*]": c + "/widgets/ios.entitlements",
                   "SWIFT_VERSION": "5.0", "APPLICATION_EXTENSION_API_ONLY": "YES", "SKIP_INSTALL": "YES",
                   "ASSETCATALOG_COMPILER_APPICON_NAME": "",
                   "LD_RUNPATH_SEARCH_PATHS": "$(inherited) @executable_path/../../Frameworks /usr/lib/swift",
                   "LD_RUNPATH_SEARCH_PATHS[sdk=macosx*]": "$(inherited) @executable_path/../../../../Frameworks /usr/lib/swift"}
        o[ident("target", e)] = {"isa": "PBXNativeTarget", "buildConfigurationList": configuraciones(e, ajustes),
                                 "buildPhases": [ident("fuentes", e), ident("recursos", e)], "buildRules": [],
                                 "dependencies": [], "name": e, "productName": producto,
                                 "productReference": ident("producto", e), "productType": "com.apple.product-type.app-extension"}
        objetivos.append(ident("target", e))
        return e

    comunes = [fichero("xcconfig", "Sytue.xcconfig", "text.xcconfig"),
               fichero("info-ios", "Info-ios.plist", "text.plist.xml"),
               fichero("info-macos", "Info-macos.plist", "text.plist.xml")]
    grupos, productos, objetivos, idiomas = [], [], [], []
    if any(a["archivos"] for a in lista):
        o[ident("grupo", PROVEEDOR)] = {"isa": "PBXGroup", "path": PROVEEDOR, "sourceTree": "<group>",
                                        "children": [fichero(PROVEEDOR + "/swift", "Proveedor.swift", "sourcecode.swift")]}
        comunes.append(ident("grupo", PROVEEDOR))
    if any(a["widget_acciones"] for a in lista):
        hijos_widgets = [fichero(WIDGETS + "/snapshot", "Snapshot.swift", "sourcecode.swift"),
                         fichero(WIDGETS + "/vista", "Widgets.swift", "sourcecode.swift"),
                         fichero(WIDGETS + "/puente", "Puente.swift", "sourcecode.swift"),
                         fichero(WIDGETS + "/info-puente", "Puente-Info.plist", "text.plist.xml"),
                         fichero(WIDGETS + "/privacidad", "PrivacyInfo.xcprivacy", "text.xml")]
        idiomas = [idioma for idioma, _, _ in idiomas_widgets()]
        for nombre in ("Localizable.strings", "Localizable.stringsdict"):
            referencias = []
            for idioma in idiomas:
                clave = WIDGETS + "/" + idioma + "/" + nombre
                referencia = fichero(clave, "locales/%s.lproj/%s" % (idioma, nombre), "text.plist.strings" if nombre.endswith(".strings") else "text.plist.xml")
                o[referencia]["name"] = idioma
                referencias.append(referencia)
            o[ident("idioma", nombre)] = {"isa": "PBXVariantGroup", "children": referencias, "name": nombre, "sourceTree": "<group>"}
            hijos_widgets.append(ident("idioma", nombre))
        o[ident("grupo", WIDGETS)] = {"isa": "PBXGroup", "path": WIDGETS, "sourceTree": "<group>", "children": hijos_widgets}
        comunes.append(ident("grupo", WIDGETS))
        e = PUENTE_WIDGETS
        o[ident("producto", e)] = {"isa": "PBXFileReference", "explicitFileType": "wrapper.framework",
                                   "includeInIndex": 0, "path": e + ".framework", "sourceTree": "BUILT_PRODUCTS_DIR"}
        productos.append(ident("producto", e))
        o[ident("bf", e)] = {"isa": "PBXBuildFile", "fileRef": ident("ref", WIDGETS + "/puente")}
        o[ident("fuentes", e)] = {"isa": "PBXSourcesBuildPhase", "buildActionMask": 2147483647,
                                  "files": [ident("bf", e)], "runOnlyForDeploymentPostprocessing": 0}
        ajustes = {"PRODUCT_NAME": e, "PRODUCT_BUNDLE_IDENTIFIER": "eu.sytue.widgetbridge",
                   "INFOPLIST_FILE": "widgets/Puente-Info.plist", "INFOPLIST_FILE[sdk=macosx*]": "widgets/Puente-Info.plist",
                   "SWIFT_VERSION": "5.0", "APPLICATION_EXTENSION_API_ONLY": "YES", "SKIP_INSTALL": "YES",
                   "DEFINES_MODULE": "YES", "ASSETCATALOG_COMPILER_APPICON_NAME": "",
                   "LD_DYLIB_INSTALL_NAME": "@rpath/$(EXECUTABLE_PATH)",
                   "LD_RUNPATH_SEARCH_PATHS": "$(inherited) /usr/lib/swift"}
        o[ident("target", e)] = {"isa": "PBXNativeTarget", "buildConfigurationList": configuraciones(e, ajustes),
                                 "buildPhases": [ident("fuentes", e)], "buildRules": [], "dependencies": [],
                                 "name": e, "productName": e, "productReference": ident("producto", e),
                                 "productType": "com.apple.product-type.framework"}
        objetivos.append(ident("target", e))
    for a in lista:
        c = a["clave"]
        hijos = [fichero(c + "/assets", "AppIcon.icon", "folder.iconcomposer.icon"),
                 fichero(c + "/tienda", "tienda.entitlements", "text.plist.entitlements")]
        ajustes = {"PRODUCT_NAME": a["nombre"], "EXECUTABLE_NAME": "sytue-" + c,
                   "PRODUCT_BUNDLE_IDENTIFIER": a["id"], "SYTUE_APP": c,
                   "CODE_SIGN_ENTITLEMENTS[sdk=macosx*]": c + "/tienda.entitlements"}
        hijos.append(fichero(c + "/ios", "ios.entitlements", "text.plist.entitlements"))
        ajustes["CODE_SIGN_ENTITLEMENTS[sdk=iphoneos*]"] = c + "/ios.entitlements"
        fases, dependencias = [ident("recursos", c), ident("fase", c)], []
        if a["archivos"]:
            e = extension(a, hijos)
            incluir(c, e, fases, dependencias, c, 13, "Embed Foundation Extensions")
        if a["widget_acciones"]:
            e = widgets(a, hijos)
            # El target existe siempre; el host lo lleva dentro solo con el interruptor.
            if widgets_apple():
                ajustes["ALWAYS_EMBED_SWIFT_STANDARD_LIBRARIES"] = "YES"
                incluir(c, e, fases, dependencias, c + "-widgets", 13, "Embed WidgetKit", True)
                incluir(c, PUENTE_WIDGETS, fases, dependencias, c + "-widgetbridge", 10, "Embed WidgetKit Bridge", True)
        o[ident("grupo", c)] = {"isa": "PBXGroup", "children": hijos, "path": c, "sourceTree": "<group>"}
        grupos.append(ident("grupo", c))
        o[ident("producto", c)] = {"isa": "PBXFileReference", "explicitFileType": "wrapper.application",
                                   "includeInIndex": 0, "path": a["nombre"] + ".app",
                                   "sourceTree": "BUILT_PRODUCTS_DIR"}
        productos.append(ident("producto", c))
        o[ident("bf", c)] = {"isa": "PBXBuildFile", "fileRef": hijos[0]}
        o[ident("recursos", c)] = {"isa": "PBXResourcesBuildPhase", "buildActionMask": 2147483647,
                                   "files": [ident("bf", c)], "runOnlyForDeploymentPostprocessing": 0}
        # Siempre: Go lleva su propia caché y Xcode no sabe qué fuentes lee. La entrada es el
        # Info.plist que Xcode acaba de escribir (la fase lo retoca después) y la salida, el
        # ejecutable que Xcode firma al final.
        o[ident("fase", c)] = {"isa": "PBXShellScriptBuildPhase", "alwaysOutOfDate": 1,
                               "buildActionMask": 2147483647, "files": [], "inputFileListPaths": [],
                               "inputPaths": ["$(TARGET_BUILD_DIR)/$(INFOPLIST_PATH)"],
                               "name": "Compilar el Go", "outputFileListPaths": [],
                               "outputPaths": ["$(TARGET_BUILD_DIR)/$(EXECUTABLE_PATH)"],
                               "runOnlyForDeploymentPostprocessing": 0, "shellPath": "/bin/bash",
                               "shellScript": FASE}
        o[ident("target", c)] = {"isa": "PBXNativeTarget", "buildConfigurationList": configuraciones(c, ajustes),
                                 "buildPhases": fases, "buildRules": [],
                                 "dependencies": dependencias, "name": c, "productName": c,
                                 "productReference": ident("producto", c),
                                 "productType": "com.apple.product-type.application"}
        objetivos.append(ident("target", c))
    o[ident("grupo", "productos")] = {"isa": "PBXGroup", "children": productos, "name": "Products",
                                      "sourceTree": "<group>"}
    o[ident("grupo", "principal")] = {"isa": "PBXGroup", "children": comunes + grupos + [ident("grupo", "productos")],
                                      "sourceTree": "<group>"}
    o[raiz] = {"isa": "PBXProject",
               "attributes": {"BuildIndependentTargetsInParallel": 1, "LastUpgradeCheck": XCODE},
               "buildConfigurationList": configuraciones(
                   "proyecto", {"Debug": {"ONLY_ACTIVE_ARCH": "YES"}, "Release": {}}, comunes[0]),
               "compatibilityVersion": "Xcode 14.0", "developmentRegion": "en", "hasScannedForEncodings": 0,
               "knownRegions": sorted(set(["en", "Base"] + idiomas)), "mainGroup": ident("grupo", "principal"),
               "productRefGroup": ident("grupo", "productos"), "projectDirPath": "", "projectRoot": "",
               "targets": objetivos}
    # Los objetos por tipo y, dentro, por identificador: el orden en que los escribe Xcode.
    cuerpo = "".join("\t\t%s = %s;\n" % (k, valor(o[k], "\t\t"))
                     for k in sorted(o, key=lambda k: (o[k]["isa"], k)))
    return ("// !$*UTF8*$!\n{\n\tarchiveVersion = 1;\n\tclasses = {\n\t};\n\tobjectVersion = 56;\n"
            "\tobjects = {\n%s\t};\n\trootObject = %s;\n}\n" % (cuerpo, raiz))


def esquema(a):
    ref = ('<BuildableReference BuildableIdentifier = "primary" BlueprintIdentifier = "%s" '
           'BuildableName = "%s.app" BlueprintName = "%s" ReferencedContainer = "container:%s">'
           '</BuildableReference>') % (ident("target", a["clave"]), a["nombre"], a["clave"], PROYECTO)
    return """<?xml version="1.0" encoding="UTF-8"?>
<Scheme LastUpgradeVersion = "%(x)s" version = "1.7">
   <BuildAction parallelizeBuildables = "YES" buildImplicitDependencies = "YES">
      <BuildActionEntries>
         <BuildActionEntry buildForTesting = "YES" buildForRunning = "YES" buildForProfiling = "YES" buildForArchiving = "YES" buildForAnalyzing = "YES">
            %(r)s
         </BuildActionEntry>
      </BuildActionEntries>
   </BuildAction>
   <TestAction buildConfiguration = "Debug" selectedDebuggerIdentifier = "Xcode.DebuggerFoundation.Debugger.LLDB" selectedLauncherIdentifier = "Xcode.DebuggerFoundation.Launcher.LLDB" shouldUseLaunchSchemeArgsEnv = "YES">
   </TestAction>
   <LaunchAction buildConfiguration = "Debug" selectedDebuggerIdentifier = "Xcode.DebuggerFoundation.Debugger.LLDB" selectedLauncherIdentifier = "Xcode.DebuggerFoundation.Launcher.LLDB" launchStyle = "0" useCustomWorkingDirectory = "NO" ignoresPersistentStateOnLaunch = "NO" debugDocumentVersioning = "YES" debugServiceExtension = "internal" allowLocationSimulation = "YES">
      <BuildableProductRunnable runnableDebuggingMode = "0">
         %(r)s
      </BuildableProductRunnable>
   </LaunchAction>
   <ProfileAction buildConfiguration = "Release" shouldUseLaunchSchemeArgsEnv = "YES" savedToolIdentifier = "" useCustomWorkingDirectory = "NO" debugDocumentVersioning = "YES">
      <BuildableProductRunnable runnableDebuggingMode = "0">
         %(r)s
      </BuildableProductRunnable>
   </ProfileAction>
   <AnalyzeAction buildConfiguration = "Debug">
   </AnalyzeAction>
   <ArchiveAction buildConfiguration = "Release" revealArchiveInOrganizer = "YES">
   </ArchiveAction>
</Scheme>
""" % {"x": XCODE, "r": ref}


def plist(cuerpo):
    return ('<?xml version="1.0" encoding="UTF-8"?>\n<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" '
            '"http://www.apple.com/DTDs/PropertyList-1.0.dtd">\n<plist version="1.0"><dict>\n%s</dict></plist>\n'
            % cuerpo)


def info_plist(plataforma):
    """Los dos Info.plist compartidos de los quince targets.

    Algoritmos estándar y distribución sin Francia: FALSE indica exención de
    documentación, no ausencia de cifrado. Revisar antes de habilitar Francia.
    ci_post_clone.sh admite TRUE + código si el workflow de la app aporta el que
    Apple aprobó; TRUE sin código bloquea la subida (error 90592).
    """
    comunes = """\t<key>CFBundleDevelopmentRegion</key><string>en</string>
\t<key>CFBundleExecutable</key><string>$(EXECUTABLE_NAME)</string>
\t<key>CFBundleIdentifier</key><string>$(PRODUCT_BUNDLE_IDENTIFIER)</string>
\t<key>CFBundleInfoDictionaryVersion</key><string>6.0</string>
\t<key>CFBundleName</key><string>$(PRODUCT_NAME)</string>
\t<key>CFBundlePackageType</key><string>$(PRODUCT_BUNDLE_PACKAGE_TYPE)</string>
\t<key>CFBundleShortVersionString</key><string>$(MARKETING_VERSION)</string>
\t<key>CFBundleVersion</key><string>$(CURRENT_PROJECT_VERSION)</string>
\t<key>ITSAppUsesNonExemptEncryption</key><false/>
"""
    extras = {
        "ios": """\t<key>CFBundleDisplayName</key><string>$(PRODUCT_NAME)</string>
\t<key>LSRequiresIPhoneOS</key><true/>
\t<key>LSSupportsOpeningDocumentsInPlace</key><true/>
\t<key>UIFileSharingEnabled</key><true/>
\t<key>UILaunchScreen</key><dict/>
\t<key>UIRequiredDeviceCapabilities</key><array><string>arm64</string></array>
\t<key>UIRequiresFullScreen</key><true/>
\t<key>UISupportedInterfaceOrientations</key><array>
\t\t<string>UIInterfaceOrientationPortrait</string>
\t\t<string>UIInterfaceOrientationLandscapeLeft</string>
\t\t<string>UIInterfaceOrientationLandscapeRight</string>
\t</array>
\t<key>UISupportedInterfaceOrientations~ipad</key><array>
\t\t<string>UIInterfaceOrientationPortrait</string>
\t\t<string>UIInterfaceOrientationPortraitUpsideDown</string>
\t\t<string>UIInterfaceOrientationLandscapeLeft</string>
\t\t<string>UIInterfaceOrientationLandscapeRight</string>
\t</array>
""",
        "macos": """\t<key>LSApplicationCategoryType</key><string>public.app-category.productivity</string>
\t<key>LSMinimumSystemVersion</key><string>$(MACOSX_DEPLOYMENT_TARGET)</string>
\t<key>NSHighResolutionCapable</key><true/>
\t<key>NSSupportsAutomaticGraphicsSwitching</key><true/>
"""}
    aviso = ("<!--\n  SPDX-License-Identifier: Apache-2.0\n"
             "  Copyright (c) 2026 Daniel Delgado Ramos\n\n"
             "  GENERADO por generar.py para las quince apps de Sytue.xcodeproj.\n"
             "  plist_apple añade los esquemas, documentos y permisos de cada app.\n"
             "  Cifrado estándar sin distribución en Francia: exento de documentación (FALSE).\n"
             "  Ver apps/fyne/appstore/README.md, «Cifrado».\n-->\n")
    return plist(comunes + extras[plataforma]).replace('<plist version=', aviso + '<plist version=', 1)


def derechos_archivos(clave, plataforma):
    """Los derechos de la extensión de archivos, los de empaquetar.sh y su guardián."""
    return subprocess.run(["bash", os.path.join(FYNE, "scripts", "empaquetar.sh"), "derechos-archivos",
                           os.path.join(FYNE, "cmd", clave), EQUIPO, plataforma],
                          check=True, capture_output=True, text=True).stdout


def cuerpo_plist(texto):
    """Lo de dentro del <dict> de un plist de una línea por clave."""
    return texto.split("<dict>\n", 1)[1].rsplit("</dict>", 1)[0]


def info_archivos():
    """El Info.plist de la extensión File Provider (NSFileProviderReplicatedExtension)."""
    return plist("""\t<key>CFBundleDevelopmentRegion</key><string>en</string>
\t<key>CFBundleDisplayName</key><string>Sytue Drive</string>
\t<key>CFBundleExecutable</key><string>$(EXECUTABLE_NAME)</string>
\t<key>CFBundleIdentifier</key><string>$(PRODUCT_BUNDLE_IDENTIFIER)</string>
\t<key>CFBundleInfoDictionaryVersion</key><string>6.0</string>
\t<key>CFBundleName</key><string>$(PRODUCT_NAME)</string>
\t<key>CFBundlePackageType</key><string>$(PRODUCT_BUNDLE_PACKAGE_TYPE)</string>
\t<key>CFBundleShortVersionString</key><string>$(MARKETING_VERSION)</string>
\t<key>CFBundleVersion</key><string>$(CURRENT_PROJECT_VERSION)</string>
\t<key>NSExtension</key><dict>
\t\t<key>NSExtensionFileProviderDocumentGroup</key><string>$(SYTUE_GRUPO_ARCHIVOS)</string>
\t\t<key>NSExtensionFileProviderSupportsEnumeration</key><true/>
\t\t<key>NSExtensionPointIdentifier</key><string>com.apple.fileprovider-nonui</string>
\t\t<key>NSExtensionPrincipalClass</key><string>$(PRODUCT_MODULE_NAME).Proveedor</string>
\t</dict>
""")


def generar(destino):
    """Escribe en `destino` todo lo generado. Devuelve las rutas, relativas a él."""
    lista = list(apps())
    hechos = []

    def escribir(ruta, contenido):
        completa = os.path.join(destino, ruta)
        os.makedirs(os.path.dirname(completa), exist_ok=True)
        with open(completa, "w", encoding="utf-8") as f:
            f.write(contenido)
        hechos.append(ruta)

    for plataforma in ("ios", "macos"):
        escribir("Info-%s.plist" % plataforma, info_plist(plataforma))
    if any(a["widget_acciones"] for a in lista):
        escribir("widgets/Puente-Info.plist", plist_widgets({
            "CFBundleDevelopmentRegion": "en", "CFBundleExecutable": "$(EXECUTABLE_NAME)",
            "CFBundleIdentifier": "$(PRODUCT_BUNDLE_IDENTIFIER)", "CFBundleInfoDictionaryVersion": "6.0",
            "CFBundleName": "$(PRODUCT_NAME)", "CFBundlePackageType": "FMWK",
            "CFBundleShortVersionString": "$(MARKETING_VERSION)", "CFBundleVersion": "$(CURRENT_PROJECT_VERSION)",
        }))
        escribir("widgets/PrivacyInfo.xcprivacy", plist_widgets(PRIVACIDAD_WIDGETS))
        for idioma, cadenas, plurales in idiomas_widgets():
            escribir("widgets/locales/%s.lproj/Localizable.strings" % idioma, cadenas)
            escribir("widgets/locales/%s.lproj/Localizable.stringsdict" % idioma, plurales)
    escribir(PROYECTO + "/project.pbxproj", pbxproj(lista))
    escribir(PROYECTO + "/project.xcworkspace/contents.xcworkspacedata",
             '<?xml version="1.0" encoding="UTF-8"?>\n<Workspace\n   version = "1.0">\n'
             '   <FileRef\n      location = "self:">\n   </FileRef>\n</Workspace>\n')
    for a in lista:
        c = a["clave"]
        escribir("%s/xcshareddata/xcschemes/%s.xcscheme" % (PROYECTO, c), esquema(a))
        # Una composición nativa común para las quince, con sus vectores y colores
        # de branding/marks. Los Icon.png de cmd siguen sirviendo a los otros sistemas.
        descriptor, capas = icono_apple(a)
        escribir(c + "/AppIcon.icon/icon.json", json.dumps(descriptor, ensure_ascii=False, indent=2) + "\n")
        for nombre, dibujo in capas.items():
            escribir(c + "/AppIcon.icon/Assets/" + nombre, dibujo)
        # LOS DERECHOS DE LA MAC APP STORE, los de empaquetar.sh (y su guardián), no otros.
        derechos = subprocess.run(["bash", os.path.join(FYNE, "scripts", "empaquetar.sh"), "derechos-tienda",
                                   os.path.join(FYNE, "cmd", c), EQUIPO], check=True, capture_output=True, text=True,
                                  env=entorno_empaquetar()).stdout
        escribir(c + "/tienda.entitlements", derechos)
        # La misma sesión de la suite y el contenedor que usa el empaquetador,
        # fusionados con los enlaces y extensiones propios de cada producto.
        sesion = subprocess.run(["bash", os.path.join(FYNE, "scripts", "empaquetar.sh"), "derechos-sesion",
                                os.path.join(FYNE, "cmd", c), EQUIPO, "ios"],
                               check=True, capture_output=True, text=True, env=entorno_empaquetar()).stdout
        ios = cuerpo_plist(sesion)
        if a["enlaces"]:
            ios += ("\t<key>com.apple.developer.associated-domains</key><array><string>applinks:%s.%s</string></array>\n"
                    % (a["enlaces"], DOMINIO))
        if a["archivos"]:
            for plataforma in ("macos", "ios"):
                escribir("%s/archivos/%s.entitlements" % (c, plataforma), derechos_archivos(c, plataforma))
            escribir(c + "/archivos/Info.plist", info_archivos())
        escribir(c + "/ios.entitlements", plist(ios))
        if a["widget_acciones"]:
            escribir(c + "/widgets/Info.plist", info_widgets(a))
            for plataforma in ("ios", "macos"):
                escribir(c + "/widgets/%s.entitlements" % plataforma, derechos_widgets(a, plataforma))
    return hechos, [a["clave"] for a in lista]


def contenido(ruta):
    if os.path.islink(ruta):
        return "enlace:" + os.readlink(ruta)
    with open(ruta, "rb") as f:
        return f.read()


def main():
    comprobar = sys.argv[1:] == ["--comprobar"]
    if sys.argv[1:] not in ([], ["--comprobar"]):
        sys.exit("uso: generar.py [--comprobar]")
    tmp = tempfile.mkdtemp()
    try:
        hechos, claves = generar(tmp)
        # Lo que es de este guion: los plists compartidos, el proyecto (menos lo de cada
        # persona y el manifest.json de Xcode Cloud) y una carpeta por app. Los plists
        # se comparan directamente en `distintos`; `hay` recorre los directorios.
        mios = [PROYECTO, "widgets/locales"] + claves + [d for d in os.listdir(AQUI)
                                      if os.path.isfile(os.path.join(AQUI, d, "tienda.entitlements"))]
        hay = set()
        for m in set(mios):
            for base, dirs, ficheros in os.walk(os.path.join(AQUI, m)):
                dirs[:] = [d for d in dirs if d not in ("xcuserdata", "xcodecloud")]
                hay.update(os.path.relpath(os.path.join(base, f), AQUI) for f in ficheros)
                hay.update(os.path.relpath(os.path.join(base, d), AQUI) for d in dirs
                           if os.path.islink(os.path.join(base, d)))
        distintos = sorted(r for r in hechos
                           if not os.path.lexists(os.path.join(AQUI, r))
                           or contenido(os.path.join(AQUI, r)) != contenido(os.path.join(tmp, r)))
        sobran = sorted(hay - set(hechos))
        if comprobar:
            if distintos or sobran:
                for r in distintos:
                    print("not ok - apps/fyne/xcode/%s no es lo que genera generar.py" % r, file=sys.stderr)
                for r in sobran:
                    print("not ok - apps/fyne/xcode/%s sobra: ya no lo genera generar.py" % r, file=sys.stderr)
                print("  python3 apps/fyne/xcode/generar.py, y versiona el resultado", file=sys.stderr)
                sys.exit(1)
            print("ok - Sytue.xcodeproj es el de las %d apps de apps/fyne/cmd" % len(claves))
            return
        for r in sobran:
            os.remove(os.path.join(AQUI, r))
        for r in distintos:
            final = os.path.join(AQUI, r)
            os.makedirs(os.path.dirname(final), exist_ok=True)
            if os.path.lexists(final):
                os.remove(final)
            shutil.move(os.path.join(tmp, r), final)
        print("%d ficheros escritos, %d borrados" % (len(distintos), len(sobran)))
    finally:
        shutil.rmtree(tmp)


if __name__ == "__main__":
    main()
