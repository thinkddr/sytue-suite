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
    "open": "widgets.open", "search": "search.label", "compose": "widgets.compose", "create": "widgets.create",
    "unread": "notifications.unread", "files": "dataflow.item.files",
    "events": "dataflow.item.events", "documents": "dataflow.item.documents",
    "contacts": "dataflow.item.contacts", "notes": "widgets.notes", "rows": "widgets.rows",
    "pending": "widgets.pending", "channels": "widgets.channels", "photos": "widgets.photos",
    "episodes": "widgets.episodes", "apps": "widgets.apps",
    "calendars": "widgets.calendars", "address_books": "widgets.address_books",
    "storage_percent": "widgets.storage",
}


def metadata(appdir):
    data = tomllib.loads((appdir / "FyneApp.toml").read_text())
    details, dev = data["Details"], data["Development"]
    bundle, scheme = details["ID"], dev["esquema"]
    if not re.fullmatch(r"eu\.sytue\.[a-z][a-z0-9]*", bundle) or not re.fullmatch(r"sytue-[a-z][a-z0-9]*", scheme):
        raise ValueError("identificador o esquema de widget inválido")
    product = scheme.removeprefix("sytue-")
    actions = dev.get("widget_acciones", "").split(",")
    allowed = {"open", "search"}
    if product == "mail":
        allowed.add("compose")
    if product == "calendar":
        allowed.add("create")
    if not actions or actions[0] != "open" or len(actions) != len(set(actions)) or not set(actions) <= allowed:
        raise ValueError("acciones del widget no permitidas")
    locales = {}
    for source in sorted((FYNE.parent / "nucleo/i18n/locales").glob("*.json")):
        catalog = json.loads(source.read_text())
        locales[source.stem] = {key: catalog[label] for key, label in LABELS.items()}
    if "en" not in locales or not isinstance(locales["en"]["unread"], dict):
        raise ValueError("catálogo nativo incompleto")
    return {"version": 1, "id": bundle, "name": details["Name"], "scheme": scheme,
            "product": product, "actions": actions, "locales": locales}


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
        manifest = temp / "AndroidManifest.xml"
        write_xml(manifest, root)
        # El layout inicial también lleva el idioma del SO antes del primer callback.
        for locale, labels in config["locales"].items():
            resources = ET.Element("resources")
            for name, value in (("sytue_widget_name", config["name"]), ("sytue_widget_open", labels["open"]),
                                ("sytue_widget_search", labels["search"])):
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


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    for key in ("appdir", "manifest", "apk-root", "aapt2", "android-jar"):
        parser.add_argument("--" + key, type=Path, required=True)
    parser.add_argument("--min-sdk", type=int, required=True)
    parser.add_argument("--target-sdk", type=int, required=True)
    prepare(parser.parse_args())
