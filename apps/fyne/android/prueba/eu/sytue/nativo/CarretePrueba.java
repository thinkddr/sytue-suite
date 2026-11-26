// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

import android.Manifest;
import android.app.Activity;
import android.content.Context;
import android.database.MatrixCursor;
import android.os.Build;
import java.io.File;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.util.Arrays;
import java.util.List;
import java.util.Map;

/**
 * La galería del teléfono (Carrete) contra un MediaStore de mentira con la forma del de
 * Android (android/prueba/android): que lista las fotos y los VÍDEOS, estos con la «v», su
 * duración y sus lados (lo que lee `deLineas` en Go, apps/nucleo/carrete); que se salta lo que
 * guardó Sytue; que sin permiso para los vídeos siguen las fotos; que antes de la API 29 no
 * filtra por carpeta; que copia el original de un vídeo de su tabla, con el GPS si hay permiso;
 * que los permisos se piden y se leen como toca; y que guardar deja la foto en «Pictures/Sytue
 * Photos». En la JVM, antes de armar una APK.
 */
public final class CarretePrueba {
    private static final String FOTOS = "content://media/external/images/media";
    private static final String VIDEOS = "content://media/external/video/media";

    private static int comprobaciones;

    private static void comprobar(boolean ok, String mensaje) {
        comprobaciones++;
        if (!ok) {
            throw new AssertionError(mensaje);
        }
    }

    /** Un teléfono con dos fotos, dos vídeos y lo que guardó Sytue. */
    private static Context telefono() {
        Context ctx = new Context();
        ctx.resolver.fila(FOTOS, "_id", 11L, "_display_name", "IMG_1.jpg", "mime_type", "image/jpeg",
                "datetaken", 1714564800000L, "date_added", 1714564000L, "_size", 2048L, "relative_path", "DCIM/Camera/");
        ctx.resolver.fila(FOTOS, "_id", 12L, "_display_name", "guardada.jpg", "mime_type", "image/jpeg",
                "datetaken", 1714564800000L, "date_added", 1714564000L, "_size", 10L, "relative_path", "Pictures/Sytue Photos/");
        ctx.resolver.fila(FOTOS, "_id", 13L, "_display_name", "con\ttab\nraro.png", "mime_type", "image/png",
                "datetaken", null, "date_added", 1700000000L, "_size", null, "relative_path", "Pictures/");
        ctx.resolver.fila(VIDEOS, "_id", 22L, "_display_name", "VID_2.mov", "mime_type", "video/quicktime",
                "datetaken", null, "date_added", 1714000000L, "_size", 1234L, "relative_path", "DCIM/Camera/",
                "duration", null, "width", 0L, "height", -1L);
        ctx.resolver.fila(VIDEOS, "_id", 21L, "_display_name", "VID_1.mp4", "mime_type", "video/mp4",
                "datetaken", 1714568400000L, "date_added", 1714568000L, "_size", 9876543L, "relative_path", "DCIM/Camera/",
                "duration", 83500L, "width", 1920L, "height", 1080L);
        ctx.resolver.fila(VIDEOS, "_id", 23L, "_display_name", "bajado.mp4", "mime_type", "video/mp4",
                "datetaken", 1714568400000L, "date_added", 1714568000L, "_size", 5L, "relative_path", "Pictures/Sytue Photos/",
                "duration", 1000L, "width", 640L, "height", 360L);
        return ctx;
    }

    private static final String LINEAS_FOTOS = "11\tIMG_1.jpg\timage/jpeg\t1714564800000\t2048\n"
            + "13\tcon tab raro.png\timage/png\t1700000000000\t-1\n";
    private static final String LINEAS_VIDEOS = "v21\tVID_1.mp4\tvideo/mp4\t1714568400000\t9876543\tv\t83500\t1920\t1080\n"
            + "v22\tVID_2.mov\tvideo/quicktime\t1714000000000\t1234\tv\t0\t0\t0\n";

    private static void listarFotosYVideos() {
        Build.VERSION.SDK_INT = 36;
        Context ctx = telefono();
        String s = Carrete.listar(ctx);
        comprobar(s.equals(LINEAS_FOTOS + LINEAS_VIDEOS), "las líneas:\n" + s);
        List<String> q = ctx.resolver.consultas;
        comprobar(q.size() == 2, "dos consultas: " + q);
        comprobar(q.get(0).startsWith(FOTOS + " ") && !q.get(0).contains("duration"), "la de fotos no pide duración: " + q.get(0));
        comprobar(q.get(1).startsWith(VIDEOS + " ") && q.get(1).contains("duration,width,height"),
                "la de vídeos pide duración y lados: " + q.get(1));
        comprobar(q.get(0).contains("relative_path NOT LIKE ?") && q.get(1).contains("relative_path NOT LIKE ?"),
                "las dos se saltan lo de Sytue: " + q);
    }

    private static void sinPermisoParaVideos() {
        Build.VERSION.SDK_INT = 36;
        Context ctx = telefono();
        ctx.resolver.prohibidas.add(VIDEOS);
        String s = Carrete.listar(ctx);
        comprobar(s.equals(LINEAS_FOTOS), "sin permiso para los vídeos, las fotos siguen:\n" + s);
    }

    private static void antesDeLaApi29() {
        Build.VERSION.SDK_INT = 28;
        try {
            Context ctx = telefono();
            String s = Carrete.listar(ctx);
            comprobar(!ctx.resolver.consultas.get(0).contains("WHERE") && !ctx.resolver.consultas.get(1).contains("WHERE"),
                    "antes de la 29 no hay RELATIVE_PATH: " + ctx.resolver.consultas);
            comprobar(s.contains("\nv23\tbajado.mp4\tvideo/mp4\t1714568400000\t5\tv\t1000\t640\t360\n"),
                    "sin carpeta que mirar, sale todo:\n" + s);
        } finally {
            Build.VERSION.SDK_INT = 36;
        }
    }

    private static void copiarUnVideo() throws Exception {
        Build.VERSION.SDK_INT = 36;
        Context ctx = telefono();
        byte[] video = "un vídeo de verdad no, pero sus bytes sí".getBytes(StandardCharsets.UTF_8);
        byte[] foto = "una foto".getBytes(StandardCharsets.UTF_8);
        ctx.resolver.contenido.put(VIDEOS + "/21", video);
        ctx.resolver.contenido.put(FOTOS + "/11", foto);
        File dir = Files.createTempDirectory("carrete-prueba").toFile();
        try {
            File destino = new File(dir, "v21");
            comprobar(Carrete.copiar(ctx, "v21", destino.getPath()).isEmpty(), "copiar el vídeo");
            comprobar(Arrays.equals(Files.readAllBytes(destino.toPath()), video), "el vídeo copiado no es el original");
            comprobar(ctx.resolver.abiertas.get(0).equals(VIDEOS + "/21"), "sin permiso de ubicación, la uri tal cual: " + ctx.resolver.abiertas);
            ctx.concedidos.add(Manifest.permission.ACCESS_MEDIA_LOCATION);
            File otra = new File(dir, "11");
            comprobar(Carrete.copiar(ctx, "11", otra.getPath()).isEmpty(), "copiar la foto");
            comprobar(Arrays.equals(Files.readAllBytes(otra.toPath()), foto), "la foto copiada no es la original");
            comprobar(ctx.resolver.abiertas.get(1).equals(FOTOS + "/11?requireOriginal=1"), "con permiso, el original: " + ctx.resolver.abiertas);
            comprobar(!Carrete.copiar(ctx, "v99", new File(dir, "v99").getPath()).isEmpty(), "un vídeo que no está");
            comprobar(Carrete.copiar(ctx, "vabc", new File(dir, "x").getPath()).equals("id no válido"), "un id roto");
        } finally {
            for (File f : dir.listFiles()) {
                f.delete();
            }
            dir.delete();
        }
    }

    private static void permisos() {
        Build.VERSION.SDK_INT = 36;
        Context ctx = new Context();
        comprobar(Carrete.permiso(ctx) == 0, "sin permisos, 0");
        ctx.concedidos.add(Manifest.permission.READ_MEDIA_VISUAL_USER_SELECTED);
        comprobar(Carrete.permiso(ctx) == 2, "solo las elegidas, 2");
        ctx.concedidos.add(Manifest.permission.READ_MEDIA_IMAGES);
        comprobar(Carrete.permiso(ctx) == 1, "todas, 1");
        comprobar(!Carrete.pedir(ctx), "sin actividad no se pide");
        Activity a = new Activity();
        comprobar(Carrete.pedir(a) && Arrays.asList(a.pedidos).contains(Manifest.permission.READ_MEDIA_VIDEO)
                && Arrays.asList(a.pedidos).contains(Manifest.permission.READ_MEDIA_IMAGES), "pide fotos y vídeos: " + Arrays.toString(a.pedidos));
        a.foco = false;
        comprobar(!Carrete.conFoco(a) && Carrete.conFoco(ctx), "el foco");
    }

    private static void guardar() throws Exception {
        Build.VERSION.SDK_INT = 36;
        Context ctx = new Context();
        // Files.createTempFile y no File.createTempFile: el fichero nace solo para su dueño
        // (CodeQL, «Local information disclosure in a temporary directory»).
        File f = Files.createTempFile("carrete-prueba", ".jpg").toFile();
        try {
            Files.write(f.toPath(), "jpeg".getBytes(StandardCharsets.UTF_8));
            String id = Carrete.guardar(ctx, f.getPath(), "foto.jpg", "image/jpeg");
            String tabla = "content://media/external_primary/images/media";
            comprobar(!id.isEmpty(), "guardar devuelve el id");
            Map<String, Object> fila = ctx.resolver.tablas.get(tabla).get(0);
            comprobar("Pictures/Sytue Photos".equals(fila.get("relative_path")) && Integer.valueOf(0).equals(fila.get("is_pending"))
                    && "foto.jpg".equals(fila.get("_display_name")), "la fila guardada: " + fila);
            comprobar(new String(ctx.resolver.contenido.get(tabla + "/" + id), StandardCharsets.UTF_8).equals("jpeg"), "los bytes guardados");
            Build.VERSION.SDK_INT = 28;
            comprobar(Carrete.guardar(ctx, f.getPath(), "foto.jpg", "image/jpeg").isEmpty(), "antes de la 29, a descargas");
        } finally {
            Build.VERSION.SDK_INT = 36;
            f.delete();
        }
    }

    public static void main(String[] args) throws Exception {
        listarFotosYVideos();
        sinPermisoParaVideos();
        antesDeLaApi29();
        copiarUnVideo();
        permisos();
        guardar();
        comprobar(MatrixCursor.vivos == 0, "cursores sin cerrar: " + MatrixCursor.vivos);
        System.out.println("CarretePrueba: " + comprobaciones + " comprobaciones en verde");
    }
}
