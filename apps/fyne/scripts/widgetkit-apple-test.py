#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
"""Regresiones de metadatos observados antes de firmar los widgets."""
import contextlib
import importlib.util
import io
import pathlib
import plistlib
import tempfile
import types
import unittest
from unittest.mock import patch


spec = importlib.util.spec_from_file_location("widgetkit", pathlib.Path(__file__).with_name("widgetkit-apple.py"))
widgetkit = importlib.util.module_from_spec(spec)
spec.loader.exec_module(widgetkit)


class ProductoTests(unittest.TestCase):
    def setUp(self):
        self.temporal = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporal.cleanup)
        self.productos = pathlib.Path(self.temporal.name)
        self.apps = [{"clave": "rooms", "id": "eu.sytue.rooms"}]
        self.plataforma = "ios"
        self.minimo = "15.0"
        self.arquitecturas = "arm64"
        self.tipo = "EXECUTE"
        self.simbolo = "_sytue_widgets_reload"
        self._bundle("Sytue-rooms-Widgets.appex", "eu.sytue.rooms.widgets", True)
        self._bundle("SytueWidgetBridge.framework", "eu.sytue.widgetbridge", False)

    def _bundle(self, nombre, identificador, extension):
        contenido = self.productos / nombre
        if self.plataforma == "macos" and extension:
            contenido /= "Contents"
        contenido.mkdir(parents=True)
        info = {"CFBundleIdentifier": identificador, "CFBundleExecutable": "main"}
        if extension:
            info["NSExtension"] = {"NSExtensionPointIdentifier": "com.apple.widgetkit-extension"}
        (contenido / "Info.plist").write_bytes(plistlib.dumps(info))

    def _comando(self, argumentos):
        extension = "Widgets.appex" in str(argumentos[-1])
        if argumentos[1] == "lipo":
            return self.arquitecturas
        if argumentos[1] == "nm":
            return "0000000100 T " + self.simbolo + "\n"
        if argumentos[1] == "otool":
            tipo = self.tipo if extension else "DYLIB"
            return ("Mach header\nmagic cputype cpusubtype caps filetype ncmds sizeofcmds flags\n"
                    "MH_MAGIC_64 ARM64 ALL 0x00 " + tipo + " 25 4000 0x01\n")
        if argumentos[1] == "vtool":
            return "\n".join("Load command 10\n cmd LC_BUILD_VERSION\n cmdsize 32\n platform IOS\n minos " + self.minimo +
                             "\n sdk 27.1\n ntools 1\n tool LD\n version 1250.0\n" for _ in self.arquitecturas.split())
        raise AssertionError(argumentos)

    def verificar(self):
        with patch.object(widgetkit, "capturar", self._comando), contextlib.redirect_stdout(io.StringIO()):
            widgetkit.verificar(self.productos, self.plataforma, self.apps, "SytueWidgetBridge")

    def test_producto_ios_real_moderno(self):
        self.verificar()

    def test_minimo_del_enlace_antiguo_no_se_oculta_con_plist_moderno(self):
        self.minimo = "7.0"
        with self.assertRaisesRegex(ValueError, "mínimo real"):
            self.verificar()

    def test_objeto_o_archive_no_es_extension_ejecutable(self):
        self.tipo = "OBJECT"
        with self.assertRaisesRegex(ValueError, "tipo Mach-O"):
            self.verificar()

    def test_no_acepta_arquitectura_simulador(self):
        self.arquitecturas = "x86_64"
        with self.assertRaisesRegex(ValueError, "arquitecturas"):
            self.verificar()

    def test_puente_sin_exportacion_no_actualizaria_widgets(self):
        self.simbolo = "_otro_simbolo"
        with self.assertRaisesRegex(ValueError, "puente público"):
            self.verificar()

    def test_producto_macos_universal_con_minimo_13(self):
        self.plataforma, self.minimo, self.arquitecturas = "macos", "13.0", "arm64 x86_64"
        self._bundle("Sytue-rooms-Widgets.appex", "eu.sytue.rooms.widgets", True)
        self.verificar()

    def test_extension_con_version_anterior_se_rechaza_antes_de_firmar(self):
        with patch.object(widgetkit, "capturar", self._comando):
            with self.assertRaisesRegex(ValueError, "versión distinta"):
                widgetkit.verificar(self.productos, self.plataforma, self.apps, "SytueWidgetBridge", "3.14.0", "37")

    def test_extension_de_otro_build_se_rechaza(self):
        for ruta in self.productos.glob("*/Info.plist"):
            info = plistlib.loads(ruta.read_bytes())
            info.update(CFBundleShortVersionString="3.14.0", CFBundleVersion="36")
            ruta.write_bytes(plistlib.dumps(info))
        with patch.object(widgetkit, "capturar", self._comando):
            with self.assertRaisesRegex(ValueError, "build distinto"):
                widgetkit.verificar(self.productos, self.plataforma, self.apps, "SytueWidgetBridge", "3.14.0", "37")


class InsercionTests(unittest.TestCase):
    def setUp(self):
        self.temporal = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporal.cleanup)
        self.raiz = pathlib.Path(self.temporal.name)
        self.productos = self.raiz / "productos"
        self.productos.mkdir()
        for nombre in ("Sytue-rooms-Widgets.appex", "SytueWidgetBridge.framework"):
            (self.productos / nombre).mkdir()
            (self.productos / nombre / "main").write_bytes(b"fixture-ejecutable")
        self.host = self.raiz / "ipa/Payload/Sytue Rooms.app"
        self.host.mkdir(parents=True)
        (self.host / "Info.plist").write_bytes(plistlib.dumps({
            "CFBundleIdentifier": "eu.sytue.rooms", "CFBundleExecutable": "main",
            "CFBundleShortVersionString": "3.14.0", "CFBundleVersion": "37"}))
        (self.host / "main").write_bytes(b"fixture-host")
        self.perfil = self.raiz / "perfil.mobileprovision"
        self.perfil.write_bytes(b"fixture-perfil-sin-datos-reales")
        self.app = {"clave": "rooms", "id": "eu.sytue.rooms"}
        self.generador = types.SimpleNamespace(PUENTE_WIDGETS="SytueWidgetBridge", grupo_widgets=lambda: "group.eu.sytue.suite")
        self.comandos = []
        self.lecturas = []
        self.carga = "@rpath/SytueWidgetBridge.framework/SytueWidgetBridge"
        self.rpath = "@executable_path/Frameworks"

    def capturar(self, argumentos):
        self.lecturas.append(argumentos)
        if "perfil-ruta" in argumentos:
            return str(self.perfil) + "\n"
        if "-L" in argumentos:
            return self.carga + " (compatibility version 1.0.0, current version 1.0.0)\n"
        if "-l" in argumentos:
            return "Load command 10\n cmd LC_RPATH\n path " + self.rpath + " (offset 12)\n"
        raise AssertionError(argumentos)

    def insertar(self):
        with patch.object(widgetkit, "capturar", self.capturar), patch.object(widgetkit, "ejecutar", self.comandos.append):
            with contextlib.redirect_stdout(io.StringIO()):
                widgetkit.insertar(self.productos, self.host, "ios", "Distribución de pruebas", self.app, self.generador)

    def test_firma_anidada_perfil_0644_y_swift_support_dentro_del_ipa(self):
        self.insertar()
        perfil = self.host / "PlugIns/Sytue-rooms-Widgets.appex/embedded.mobileprovision"
        self.assertEqual(perfil.read_bytes(), self.perfil.read_bytes())
        self.assertEqual(perfil.stat().st_mode & 0o777, 0o644)
        self.assertIn("verificar-perfil-grupo", self.comandos[0])
        swift = next(c for c in self.comandos if "swift-stdlib-tool" in c)
        self.assertEqual(swift[swift.index("--unsigned-destination") + 1], self.raiz / "ipa/SwiftSupport/iphoneos")
        firmas = [c for c in self.comandos if c[0] == "codesign" and "-s" in c]
        self.assertEqual(len(firmas), 2)
        self.assertTrue(str(firmas[0][-1]).endswith("SytueWidgetBridge.framework"))
        self.assertTrue(str(firmas[1][-1]).endswith("Sytue-rooms-Widgets.appex"))
        self.assertNotIn(self.host, [c[-1] for c in firmas])

    def test_host_fuera_de_payload_no_puede_escribir_swift_support(self):
        original = self.host
        self.host = self.raiz / "Sytue Rooms.app"
        original.rename(self.host)
        with self.assertRaisesRegex(ValueError, "Payload real"):
            self.insertar()
        self.assertFalse(self.comandos)

    def test_puente_no_cargado_no_produce_widget_inutil(self):
        self.carga = "/System/Library/Frameworks/Foundation.framework/Foundation"
        with self.assertRaisesRegex(ValueError, "no carga el puente"):
            self.insertar()
        self.assertFalse(self.comandos)

    def test_nombre_o_ruta_parcial_del_puente_no_autoriza_insercion(self):
        for carga in (
            "@rpath/SytueWidgetBridge.framework/SytueWidgetBridgeOtro",
            "@rpath/SytueWidgetBridge.framework/../SytueWidgetBridge",
            "@rpath/SytueWidgetBridge.framework/Versions/B/SytueWidgetBridge",
            "/Library/Frameworks/SytueWidgetBridge.framework/SytueWidgetBridge",
        ):
            with self.subTest(carga=carga):
                self.comandos.clear()
                self.carga = carga
                with self.assertRaisesRegex(ValueError, "no carga el puente"):
                    self.insertar()
                self.assertFalse(self.comandos)

    def test_puente_versionado_en_bundle_es_valido(self):
        self.carga = "@rpath/SytueWidgetBridge.framework/Versions/A/SytueWidgetBridge"
        self.insertar()

    def test_rpath_parcial_no_es_ruta_valida(self):
        self.rpath += "Otro"
        with self.assertRaisesRegex(ValueError, "no puede resolver"):
            self.insertar()
        self.assertFalse(self.comandos)

    def test_no_sustituye_plugins_fuera_del_host(self):
        fuera = self.raiz / "fuera"
        fuera.mkdir()
        (self.host / "PlugIns").symlink_to(fuera, target_is_directory=True)
        with self.assertRaisesRegex(ValueError, "dentro del host"):
            self.insertar()
        self.assertFalse(list(fuera.iterdir()))

    def test_macos_firma_antes_del_host_con_perfil_del_destino_correcto(self):
        (self.host / "Contents/MacOS").mkdir(parents=True)
        (self.host / "Info.plist").rename(self.host / "Contents/Info.plist")
        (self.host / "main").rename(self.host / "Contents/MacOS/main")
        (self.productos / "Sytue-rooms-Widgets.appex/Contents").mkdir()
        self.rpath = "@executable_path/../Frameworks"
        for destino in ("macappstore", "developerid"):
            with self.subTest(destino=destino):
                self.comandos.clear()
                self.lecturas.clear()
                with patch.object(widgetkit, "capturar", self.capturar), patch.object(widgetkit, "ejecutar", self.comandos.append):
                    with contextlib.redirect_stdout(io.StringIO()):
                        widgetkit.insertar(self.productos, self.host, destino, "Distribución de pruebas", self.app, self.generador)
                perfil = self.host / "Contents/PlugIns/Sytue-rooms-Widgets.appex/Contents/embedded.provisionprofile"
                self.assertEqual(perfil.stat().st_mode & 0o777, 0o644)
                self.assertEqual(self.lecturas[2][-1], destino)
                swift = next(c for c in self.comandos if "swift-stdlib-tool" in c)
                self.assertNotIn("--unsigned-destination", swift)
                self.assertIn("--options=runtime", swift)
                for firma in (c for c in self.comandos if c[0] == "codesign" and "-s" in c):
                    self.assertIn("--timestamp", firma)
                    self.assertNotEqual(firma[-1], self.host)


if __name__ == "__main__":
    unittest.main()
