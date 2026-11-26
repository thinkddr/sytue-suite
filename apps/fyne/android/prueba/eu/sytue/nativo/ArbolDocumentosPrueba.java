// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

import android.app.Activity;
import android.content.Context;
import android.content.Intent;
import android.net.Uri;
import android.os.StatFs;
import android.provider.ProveedorFalso;
import java.io.File;
import java.io.FileNotFoundException;
import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.util.Arrays;

/**
 * ArbolDocumentos (la carpeta de Drive elegida con el selector, DR-12) contra un Android de
 * mentira (android/prueba/arbol) y un proveedor que se porta como el del almacenamiento del
 * teléfono sobre una carpeta temporal: elegir y guardar el permiso de lectura y escritura,
 * y crear, listar, renombrar, mover, borrar, abrir y enseñar documentos.
 */
public final class ArbolDocumentosPrueba {
    private static int comprobaciones;

    private static void exigir(boolean valor, String caso) {
        comprobaciones++;
        if (!valor) {
            throw new AssertionError(caso);
        }
    }

    private static String[] doc(String linea) { return ArbolDocumentos.campos(linea); }

    public static void main(String[] argumentos) throws Exception {
        File base = Files.createTempDirectory("arbol").toFile();
        File descargas = new File(base, "Download");
        exigir(descargas.mkdir(), "la carpeta de prueba");
        ProveedorFalso p = new ProveedorFalso(base);
        ProveedorFalso.actual = p;
        String arbol = "content://com.android.externalstorage.documents/tree/" + Uri.encode("primary:Download");

        // Una línea por documento, con lo raro escapado.
        String raro = "a\tb\nc\\d";
        exigir(Arrays.equals(ArbolDocumentos.campos(ArbolDocumentos.escapar(raro) + "\t1"), new String[] {raro, "1"}), "escapar");

        // ELEGIR: el selector se abre pidiendo leer, escribir y guardar el permiso.
        Activity a = new Activity();
        a.resultado = Activity.RESULT_OK;
        a.respuesta = new Intent("resultado").setData(Uri.parse(arbol))
                .addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION | Intent.FLAG_GRANT_WRITE_URI_PERMISSION
                        | Intent.FLAG_GRANT_PERSISTABLE_URI_PERMISSION);
        String elegido = ArbolDocumentos.elegir(a);
        exigir(elegido.equals(arbol + "\nDownload\nrw"), "lo elegido: " + elegido);
        exigir(Intent.ACTION_OPEN_DOCUMENT_TREE.equals(a.pedido.accion), "el selector de carpetas");
        int pedidas = a.pedido.getFlags();
        exigir((pedidas & Intent.FLAG_GRANT_WRITE_URI_PERMISSION) != 0 && (pedidas & Intent.FLAG_GRANT_PERSISTABLE_URI_PERMISSION) != 0,
                "pide escribir y guardar el permiso");
        exigir(a.fm.vivos == 0, "el fragmento se quita");
        exigir(ArbolDocumentos.vigente(a, arbol), "el permiso de leer y escribir queda guardado");
        ArbolDocumentos.soltar(a, arbol);
        exigir(!ArbolDocumentos.vigente(a, arbol), "soltar lo devuelve");
        ArbolDocumentos.soltar(a, arbol); // y soltar dos veces no es un error

        // Un proveedor que solo deja leer, o que no deja guardar el permiso.
        Activity b = new Activity();
        exigir(ArbolDocumentos.elegido(b.cr, Uri.parse(arbol), Intent.FLAG_GRANT_READ_URI_PERMISSION).endsWith("\nr"), "solo lectura");
        exigir(b.cr.guardados.isEmpty(), "de solo lectura no se guarda nada");
        b.cr.sinPersistir = true;
        exigir(ArbolDocumentos.elegido(b.cr, Uri.parse(arbol),
                Intent.FLAG_GRANT_READ_URI_PERMISSION | Intent.FLAG_GRANT_WRITE_URI_PERMISSION).endsWith("\nt"), "temporal");
        b.cr.guardados.add(new android.content.UriPermission(Uri.parse(arbol), Intent.FLAG_GRANT_READ_URI_PERMISSION));
        exigir(!ArbolDocumentos.vigente(b, arbol), "un permiso de solo lectura no vale");
        a.resultado = Activity.RESULT_CANCELED;
        exigir(ArbolDocumentos.elegir(a).isEmpty(), "cancelar");
        exigir(ArbolDocumentos.elegir(new Context()).equals("!"), "sin actividad");

        // LOS DOCUMENTOS.
        Context c = new Context();
        String[] raiz = doc(ArbolDocumentos.raiz(c, arbol));
        exigir(raiz[0].equals("primary:Download") && raiz[1].equals("1") && raiz[4].equals("Download"), "la raíz");
        String[] carpeta = doc(ArbolDocumentos.crear(c, arbol, raiz[0], "Proyectos", true));
        exigir(carpeta[1].equals("1") && new File(descargas, "Proyectos").isDirectory(), "crear una carpeta");
        // El tipo sale de la extensión, y el proveedor no le añade otra.
        String[] informe = doc(ArbolDocumentos.crear(c, arbol, carpeta[0], "Informe.docx", false));
        exigir(informe[4].equals("Informe.docx") && informe[1].equals("0") && informe[2].equals("0"), "crear un .docx: " + informe[4]);
        String[] raro2 = doc(ArbolDocumentos.crear(c, arbol, carpeta[0], ".plan.md.123.tmp", false));
        exigir(raro2[4].equals(".plan.md.123.tmp"), "un temporal sin tipo se llama como se pidió: " + raro2[4]);
        String[] otra = doc(ArbolDocumentos.crear(c, arbol, carpeta[0], "Informe.docx", false));
        exigir(otra[4].equals("Informe (1).docx"), "el proveedor cambia un nombre repetido");
        Files.write(new File(descargas, "Proyectos/Informe.docx").toPath(), "hola".getBytes(StandardCharsets.UTF_8));
        String[] hijos = ArbolDocumentos.hijos(c, arbol, carpeta[0]).split("\n");
        exigir(hijos.length == 3, "tres hijos");
        boolean visto = false;
        for (String h : hijos) {
            String[] d = doc(h);
            if (d[4].equals("Informe.docx")) {
                visto = d[2].equals("4") && Long.parseLong(d[3]) > 0;
            }
        }
        exigir(visto, "tamaño y fecha");
        exigir(ArbolDocumentos.hijos(c, arbol, raiz[0]).split("\n").length == 1, "la raíz tiene una carpeta");

        String[] renombrado = doc(ArbolDocumentos.renombrar(c, arbol, informe[0], "Final.docx"));
        exigir(renombrado[0].equals("primary:Download/Proyectos/Final.docx"), "renombrar cambia el id: " + renombrado[0]);
        String[] movido = doc(ArbolDocumentos.mover(c, arbol, renombrado[0], carpeta[0], raiz[0]));
        exigir(movido[0].equals("primary:Download/Final.docx") && new File(descargas, "Final.docx").isFile(), "mover");
        try {
            ArbolDocumentos.datos(c, arbol, informe[0]);
            exigir(false, "lo que ya no está");
        } catch (FileNotFoundException esperado) {
            comprobaciones++;
        }
        p.sinMover = true;
        try {
            ArbolDocumentos.mover(c, arbol, movido[0], raiz[0], carpeta[0]);
            exigir(false, "mover sin saber");
        } catch (UnsupportedOperationException esperado) {
            comprobaciones++;
        }

        int fd = ArbolDocumentos.abrir(c, arbol, movido[0], "wt");
        exigir(p.abiertos.get(fd).equals("primary:Download/Final.docx wt"), "abrir para escribir vaciando");
        try {
            ArbolDocumentos.abrir(c, arbol, carpeta[0], "r");
            exigir(false, "abrir una carpeta");
        } catch (FileNotFoundException esperado) {
            comprobaciones++;
        }

        ArbolDocumentos.borrar(c, arbol, carpeta[0]);
        exigir(!new File(descargas, "Proyectos").exists(), "borrar una carpeta con lo de dentro");
        try {
            ArbolDocumentos.borrar(c, arbol, carpeta[0]);
            exigir(false, "borrar lo que no está");
        } catch (IOException esperado) {
            comprobaciones++;
        }

        // ENSEÑAR: con la app del sistema, que puede leer y escribir en su sitio.
        ArbolDocumentos.ver(c, arbol, movido[0], true);
        Intent elegir = c.lanzados.get(0);
        exigir(Intent.ACTION_CHOOSER.equals(elegir.accion) && (elegir.getFlags() & Intent.FLAG_ACTIVITY_NEW_TASK) != 0, "el selector de apps");
        Intent ver = elegir.dentro;
        exigir(Intent.ACTION_VIEW.equals(ver.accion) && ver.datos.toString().endsWith(Uri.encode("primary:Download/Final.docx"))
                && ver.tipo.equals("application/vnd.openxmlformats-officedocument.wordprocessingml.document"), "abrir con su app");
        exigir((ver.getFlags() & Intent.FLAG_GRANT_WRITE_URI_PERMISSION) != 0, "la otra app puede guardar");
        ArbolDocumentos.ver(c, arbol, movido[0], false);
        Intent compartir = c.lanzados.get(1).dentro;
        exigir(Intent.ACTION_SEND.equals(compartir.accion) && compartir.flujo != null
                && (compartir.getFlags() & Intent.FLAG_GRANT_WRITE_URI_PERMISSION) == 0, "compartir solo deja leer");

        // ABRIR EN UNA APP DE SYTUE OFFICE: un VIEW explícito a su paquete, con el tipo que
        // declara y permiso de leer y escribir; sin ella instalada (o sin ese tipo), false.
        String docx = "application/vnd.openxmlformats-officedocument.wordprocessingml.document";
        exigir(!ArbolDocumentos.verEn(c, arbol, movido[0], "eu.sytue.office.docus", docx), "Docus sin instalar");
        exigir(c.lanzados.size() == 2, "sin la app no se lanza nada");
        c.instalados.put("eu.sytue.office.docus", java.util.Set.of(docx));
        exigir(!ArbolDocumentos.verEn(c, arbol, movido[0], "eu.sytue.office.docus", "application/pdf"), "un tipo que no declara");
        exigir(ArbolDocumentos.verEn(c, arbol, movido[0], "eu.sytue.office.docus", docx), "Docus instalada");
        Intent enDocus = c.lanzados.get(2);
        exigir(Intent.ACTION_VIEW.equals(enDocus.accion) && "eu.sytue.office.docus".equals(enDocus.paquete) && enDocus.dentro == null
                && enDocus.datos.toString().endsWith(Uri.encode("primary:Download/Final.docx")) && docx.equals(enDocus.tipo),
                "un VIEW explícito, sin selector");
        exigir((enDocus.getFlags() & Intent.FLAG_GRANT_READ_URI_PERMISSION) != 0
                && (enDocus.getFlags() & Intent.FLAG_GRANT_WRITE_URI_PERMISSION) != 0, "Docus puede leerlo y guardarlo");

        // EL ESPACIO: el del volumen del teléfono; de otro proveedor no se sabe.
        exigir(ArbolDocumentos.espacio(c, arbol).equals("1000\t400"), "espacio del teléfono");
        StatFs.ruta = "/storage/1234-5678";
        exigir(ArbolDocumentos.espacio(c, "content://com.android.externalstorage.documents/tree/" + Uri.encode("1234-5678:Fotos"))
                .equals("1000\t400"), "espacio de la tarjeta");
        exigir(ArbolDocumentos.espacio(c, "content://com.otra.nube/tree/raiz").isEmpty(), "de otro proveedor no se sabe");
        exigir(ArbolDocumentos.tipoDe("carpeta").equals("application/octet-stream"), "tipo genérico sin extensión");
        exigir(ArbolDocumentos.tipoDe("FOTO.JPG").equals("image/jpeg"), "la extensión, sin mayúsculas");

        System.out.println("ArbolDocumentos: " + comprobaciones + " comprobaciones en verde");
    }
}
