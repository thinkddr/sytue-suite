#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
"""Añade los recursos del widget al APK intermedio, antes de convertirlo en AAB.

Fyne no acepta res/ propios. Se enlazan juntos sus iconos y los recursos nativos,
con aapt2 y el manifiesto fuente completo; dex, bibliotecas y assets se conservan.
El catálogo y las acciones salen de FyneApp.toml, y los rótulos de i18n generado.
"""
import argparse
import json
from pathlib import Path
import re
import shutil
import subprocess
import tempfile
import tomllib
import xml.etree.ElementTree as ET
import zipfile

FYNE = Path(__file__).resolve().parents[1]
ANDROID = "http://schemas.android.com/apk/res/android"
ET.register_namespace("android", ANDROID)
ATTR = "{" + ANDROID + "}"
LABELS = {
    "open": "widgets.open", "compose": "widgets.compose", "create": "widgets.create",
    "upload": "widgets.addPhotos", "open_to_update": "widgets.openToUpdate",
    "unread": "notifications.unread", "files": "dataflow.item.files",
    "events": "dataflow.item.events", "documents": "dataflow.item.documents",
    "contacts": "dataflow.item.contacts", "notes": "widgets.notes", "rows": "widgets.rows",
    "pending": "widgets.pending", "channels": "widgets.channels", "photos": "widgets.photos",
    "episodes": "widgets.episodes", "apps": "widgets.apps",
    "calendars": "widgets.calendars", "address_books": "widgets.address_books",
    "storage_percent": "widgets.storage",
    # «Actualizado: 14:05»: en Android la hora absoluta, porque la vista se queda con el
    # texto con que se pintó y nada garantiza repintarla al desbloquear (Widgets.java).
    "updated_at": "widgets.updatedAt",
}

# El widget al día con la app cerrada (ADR 0078, enmienda del 2026-11-06, pieza 6): el
# trabajo RefrescoWidgets va siempre; con FONDO_GO carga Go sin actividad y lee
# (RefrescarSinVentana), y One releva la señal de cambios a las demás (RelevoWidgets).
# ENCENDIDO el 2026-11-07 por decisión del dueño, ANTES de la prueba del runbook en un
# teléfono («Android: el widget con la app cerrada»): si esa prueba falla, se vuelve a
# False aquí y el trabajo vuelve a solo repintar. Se cambia AQUÍ, en un commit revisado;
# nunca con una variable del runner.
FONDO_GO = True

# Las acciones rápidas de cada producto, además de «open»: abren un formulario, una
# pregunta o el selector del sistema que la persona confirma. La misma tabla en
# xcode/generar.py, Snapshot.swift y WidgetSnapshot.accionPermitida.
ACCIONES = {"mail": {"compose"}, "calendar": {"create"}, "contacts": {"create"},
            "notes": {"create"}, "photos": {"upload"}, "docs": {"create"}, "sheets": {"create"},
            "slides": {"create"}}
# El rótulo de «create» en cada producto: el de Calendar es «Crear evento».
CREAR = {"contacts": "widgets.newContact", "notes": "widgets.newNote", "docs": "widgets.newDocument",
         "sheets": "widgets.newSpreadsheet", "slides": "widgets.newPresentation"}


def metadata(appdir):
    data = tomllib.loads((appdir / "FyneApp.toml").read_text())
    details, dev = data["Details"], data["Development"]
    bundle, scheme = details["ID"], dev["esquema"]
    if not re.fullmatch(r"eu\.sytue\.[a-z][a-z0-9]*", bundle) or not re.fullmatch(r"sytue-[a-z][a-z0-9]*", scheme):
        raise ValueError("identificador o esquema de widget inválido")
    product = scheme.removeprefix("sytue-")
    # «search» ya no es una acción de los widgets (2026-11-24, ADR 0078): se ignora si un
    # FyneApp.toml aún la declara (Mail, que cambia en otra rama).
    actions = [a for a in dev.get("widget_acciones", "").split(",") if a != "search"]
    allowed = {"open"} | ACCIONES.get(product, set())
    if not actions or actions[0] != "open" or len(actions) != len(set(actions)) or not set(actions) <= allowed:
        raise ValueError("acciones del widget no permitidas")
    locales = {}
    for source in sorted((FYNE.parent / "nucleo/i18n/locales").glob("*.json")):
        catalog = json.loads(source.read_text())
        locales[source.stem] = {key: catalog[label] for key, label in LABELS.items()}
        # Sin datos (sin sesión, caducado), el widget dice qué es la app en vez de quedarse
        # en blanco, con «Abrir» debajo: la misma descripción que el de Apple.
        locales[source.stem]["description"] = catalog["app." + product + ".description"]
        locales[source.stem]["create"] = catalog[CREAR.get(product, "widgets.create")]
    if "en" not in locales or not isinstance(locales["en"]["unread"], dict):
        raise ValueError("catálogo nativo incompleto")
    return {"version": 1, "id": bundle, "name": details["Name"], "scheme": scheme,
            "product": product, "actions": actions, "locales": locales}


def fondo(root, app, appid, go=FONDO_GO):
    """El trabajo del widget y, con Go, el relevo de One y el permiso de red del trabajo."""
    for name in ("eu.sytue.nativo.RefrescoWidgets", "eu.sytue.nativo.RelevoWidgets"):
        if app.find("*[@" + ATTR + "name='" + name + "']") is not None:
            raise ValueError("componente del refresco de widgets duplicado: " + name)
    # BIND_JOB_SERVICE: solo el sistema puede arrancarlo, y JobScheduler no lo encola sin él.
    service = ET.SubElement(app, "service", {ATTR + "name": "eu.sytue.nativo.RefrescoWidgets",
        ATTR + "permission": "android.permission.BIND_JOB_SERVICE", ATTR + "exported": "false"})
    ET.SubElement(service, "meta-data", {ATTR + "name": "eu.sytue.widgets.go",
        ATTR + "value": "true" if go else "false"})
    if not go:
        return
    # Exportado sin permiso a propósito: un permiso `signature` no vale entre quince apps
    # con quince certificados. RelevoWidgets.call comprueba al llamante (One, firma fijada).
    ET.SubElement(app, "provider", {ATTR + "name": "eu.sytue.nativo.RelevoWidgets",
        ATTR + "authorities": appid + ".sytue.widgets", ATTR + "exported": "true",
        ATTR + "grantUriPermissions": "false"})
    # El trabajo con red (setRequiredNetworkType) lo exige desde Android 14.
    red = "android.permission.ACCESS_NETWORK_STATE"
    if root.find("uses-permission[@" + ATTR + "name='" + red + "']") is None:
        root.insert(list(root).index(app), ET.Element("uses-permission", {ATTR + "name": red}))


def write_xml(path, root):
    path.parent.mkdir(parents=True, exist_ok=True)
    ET.ElementTree(root).write(path, encoding="utf-8", xml_declaration=True)


def icons(original, res):
    """Sólo los iconos que genera Fyne. Nuevos recursos exigen revisar su conservación."""
    allowed = {"icon.png", "ic_launcher.png", "ic_launcher_foreground.png",
               "ic_launcher_background.png", "ic_launcher_monochrome.png",
               "ic_launcher.xml", "ic_launcher_round.xml"}
    adaptive = False
    legacy = None
    for source in sorted(original.rglob("*")):
        if not source.is_file():
            continue
        rel = source.relative_to(original)
        if len(rel.parts) != 2 or source.name not in allowed or not rel.parts[0].startswith(("mipmap-", "drawable")):
            raise ValueError("recurso Fyne nuevo que no se puede descartar: " + str(rel))
        target = res / rel
        target.parent.mkdir(parents=True, exist_ok=True)
        if source.suffix == ".png":
            shutil.copyfile(source, target)
            if source.name in ("icon.png", "ic_launcher.png"):
                legacy = "@" + rel.parts[0].split("-")[0] + "/" + source.stem
            continue
        adaptive = True
        # El XML del APK es binario; se reconstruye por sus capas PNG existentes.
        root = ET.Element("adaptive-icon")
        for layer in ("background", "foreground", "monochrome"):
            if layer == "monochrome" and "v33" not in rel.parts[0]:
                continue
            name = "ic_launcher_" + layer
            if not list(original.glob("*/" + name + ".png")):
                if layer == "monochrome":
                    continue
                raise ValueError("falta la capa de icono " + layer)
            ET.SubElement(root, layer, {ATTR + "drawable": "@mipmap/" + name})
        write_xml(target, root)
    if adaptive:
        return {ATTR + "icon": "@mipmap/ic_launcher", ATTR + "roundIcon": "@mipmap/ic_launcher_round"}
    if legacy:
        return {ATTR + "icon": legacy}
    raise ValueError("APK sin el icono original de Fyne")


def prepare(args):
    # La demo de los cimientos no es un producto (eu.sytue.fyne.demo, sin tienda): no
    # lleva widgets, y su identificador no pasa la validación de los de la suite.
    # Sin esto, release-mobile se caía al empaquetarla y no salía ningún AAB.
    if args.appdir.resolve().name == "demo":
        return
    config = metadata(args.appdir)
    source = ET.parse(args.manifest)
    root = source.getroot()
    app = root.find("application")
    if root.get("package") != config["id"] or app is None:
        raise ValueError("el manifiesto no corresponde al producto")
    if app.find("receiver[@" + ATTR + "name='eu.sytue.nativo.Widgets']") is not None:
        raise ValueError("receptor de widgets duplicado")
    with tempfile.TemporaryDirectory(prefix="sytue-android-widget-") as temp:
        temp = Path(temp)
        res = temp / "res"
        shutil.copytree(FYNE / "android/res", res)
        app.attrib.update(icons(args.apk_root / "res", res))
        receiver = ET.SubElement(app, "receiver", {ATTR + "name": "eu.sytue.nativo.Widgets",
            ATTR + "exported": "false", ATTR + "label": config["name"]})
        intent = ET.SubElement(receiver, "intent-filter")
        ET.SubElement(intent, "action", {ATTR + "name": "android.appwidget.action.APPWIDGET_UPDATE"})
        ET.SubElement(receiver, "meta-data", {ATTR + "name": "android.appwidget.provider",
            ATTR + "resource": "@xml/sytue_widget_info"})
        fondo(root, app, config["id"])
        manifest = temp / "AndroidManifest.xml"
        write_xml(manifest, root)
        # El layout inicial también lleva el idioma del SO antes del primer callback.
        for locale, labels in config["locales"].items():
            resources = ET.Element("resources")
            for name, value in (("sytue_widget_name", config["name"]), ("sytue_widget_open", labels["open"])):
                ET.SubElement(resources, "string", {"name": name, "formatted": "false"}).text = value.replace("'", "\\'")
            qualifier = "values" if locale == "en" else "values-" + locale
            write_xml(res / qualifier / "sytue_widget.xml", resources)
        compiled, linked = temp / "compiled.zip", temp / "resources.apk"
        subprocess.run([str(args.aapt2), "compile", "--dir", str(res), "-o", str(compiled)], check=True)
        subprocess.run([str(args.aapt2), "link", "-o", str(linked), "--manifest", str(manifest),
            "-I", str(args.android_jar), "--min-sdk-version", str(args.min_sdk),
            "--target-sdk-version", str(args.target_sdk), "--auto-add-overlay", str(compiled)], check=True)
        # Cambiar la tabla sin cambiar los XML enlazados dejaría referencias inválidas.
        shutil.rmtree(args.apk_root / "res")
        with zipfile.ZipFile(linked) as archive:
            for name in archive.namelist():
                if name in ("AndroidManifest.xml", "resources.arsc") or name.startswith("res/"):
                    archive.extract(name, args.apk_root)
        assets = args.apk_root / "assets"
        assets.mkdir(exist_ok=True)
        (assets / "sytue-widget.json").write_text(json.dumps(config, ensure_ascii=False, separators=(",", ":")))


# --- Sytue Office (SYTUE_ARBOL=cmd-local) -----------------------------------------------
# La edición local no lleva widgets ni el broker de sesión (PLAN-EDICION-LOCAL §7.2): de lo
# de arriba solo le sirven los iconos. Lo que sí puede llevar es ML Kit GenAI (la IA del
# sistema, Gemini Nano): sus componentes y sus recursos, que `android-mlkit.py preparar`
# deja en una carpeta, se enlazan aquí con los iconos, y de la tabla resultante salen las
# clases R de cada biblioteca (Gradle las genera igual: con los identificadores finales).
LOCAL = ("eu.sytue.nativo.Widgets", "eu.sytue.nativo.RefrescoWidgets", "eu.sytue.nativo.RelevoWidgets",
         "eu.sytue.nativo.SesionProveedor", "eu.sytue.nativo.SesionDesbloqueo")


def r_java(final, library, package):
    """La clase R de `package` con los símbolos de su R.txt y los valores de la tabla final."""
    values = {}
    for line in final.splitlines():
        parts = line.split(" ", 3)
        if len(parts) == 4:
            values[(parts[1], parts[2])] = (parts[0], parts[3])
    types = {}
    for line in library.splitlines():
        parts = line.split(" ", 3)
        if len(parts) != 4:
            continue
        kind, rtype, name = parts[0], parts[1], parts[2]
        if (rtype, name) not in values:
            raise ValueError("%s.R.%s.%s no está en la tabla enlazada" % (package, rtype, name))
        final_kind, value = values[(rtype, name)]
        if final_kind != kind:
            raise ValueError("%s.R.%s.%s cambia de tipo al enlazar" % (package, rtype, name))
        java_type = "int[]" if kind == "int[]" else "int"
        types.setdefault(rtype, []).append("        public static final %s %s = %s;" % (java_type, name, value))
    out = ["// Generado por android-widgets.py --edicion local: no se edita.",
           "package %s;" % package, "", "public final class R {", "    private R() {}"]
    for rtype in sorted(types):
        out += ["", "    public static final class %s {" % rtype, "        private %s() {}" % rtype]
        out += types[rtype]
        out.append("    }")
    out.append("}")
    return "\n".join(out) + "\n"


def prepare_office(args):
    source = ET.parse(args.manifest)
    root = source.getroot()
    app = root.find("application")
    package = root.get("package") or ""
    if not re.fullmatch(r"eu\.sytue\.office\.[a-z]+", package) or app is None:
        raise ValueError("el manifiesto no es de Sytue Office")
    for name in LOCAL:
        if root.find(".//*[@" + ATTR + "name='" + name + "']") is not None:
            raise ValueError("Sytue Office no declara " + name + " (PLAN-EDICION-LOCAL §7.2)")
    with tempfile.TemporaryDirectory(prefix="sytue-android-office-") as temp:
        temp = Path(temp)
        res = temp / "res"
        res.mkdir()
        app.attrib.update(icons(args.apk_root / "res", res))
        overlays = []
        if args.mlkit:
            fragment = ET.parse(args.mlkit / "manifiesto.xml").getroot()
            for child in fragment:
                if child.tag == "application":
                    for item in child:
                        for key, value in item.attrib.items():
                            item.set(key, value.replace("${applicationId}", package))
                        app.append(item)
                else:
                    root.insert(list(root).index(app), child)
            # Cada biblioteca como superposición, en el orden del fichero fijado; la app al final.
            for n, folder in enumerate(sorted((args.mlkit / "res").iterdir())):
                flat = temp / ("lib-%02d.zip" % n)
                subprocess.run([str(args.aapt2), "compile", "--dir", str(folder), "-o", str(flat)], check=True)
                overlays.append(flat)
        manifest = temp / "AndroidManifest.xml"
        write_xml(manifest, root)
        compiled, linked, symbols = temp / "app.zip", temp / "resources.apk", temp / "R.txt"
        subprocess.run([str(args.aapt2), "compile", "--dir", str(res), "-o", str(compiled)], check=True)
        command = [str(args.aapt2), "link", "-o", str(linked), "--manifest", str(manifest),
                   "-I", str(args.android_jar), "--min-sdk-version", str(args.min_sdk),
                   "--target-sdk-version", str(args.target_sdk), "--auto-add-overlay",
                   "--output-text-symbols", str(symbols)]
        for flat in overlays + [compiled]:
            command += ["-R", str(flat)]
        subprocess.run(command, check=True)
        shutil.rmtree(args.apk_root / "res")
        with zipfile.ZipFile(linked) as archive:
            for name in archive.namelist():
                if name in ("AndroidManifest.xml", "resources.arsc") or name.startswith("res/"):
                    archive.extract(name, args.apk_root)
        if args.mlkit and args.r_java:
            final = symbols.read_text()
            for library in sorted((args.mlkit / "rtxt").glob("*.txt")):
                pkg = library.stem
                target = args.r_java / pkg.replace(".", "/") / "R.java"
                target.parent.mkdir(parents=True, exist_ok=True)
                target.write_text(r_java(final, library.read_text(), pkg))


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    for key in ("appdir", "manifest", "apk-root", "aapt2", "android-jar"):
        parser.add_argument("--" + key, type=Path, required=True)
    parser.add_argument("--min-sdk", type=int, required=True)
    parser.add_argument("--target-sdk", type=int, required=True)
    # `local`: Sytue Office (SYTUE_ARBOL=cmd-local). Sin él, la conectada de siempre.
    parser.add_argument("--edicion", choices=("conectada", "local"), default="conectada")
    # Solo con --edicion local: la carpeta de `android-mlkit.py preparar` y dónde dejar las R.
    parser.add_argument("--mlkit", type=Path)
    parser.add_argument("--r-java", type=Path)
    parsed = parser.parse_args()
    if parsed.edicion == "local":
        prepare_office(parsed)
    elif parsed.mlkit or parsed.r_java:
        parser.error("--mlkit y --r-java son de --edicion local")
    else:
        prepare(parsed)
