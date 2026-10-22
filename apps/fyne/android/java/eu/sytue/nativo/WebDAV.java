// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

import java.io.File;
import java.io.FileInputStream;
import java.io.FileNotFoundException;
import java.io.FileOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.HttpURLConnection;
import java.net.URI;
import java.net.URL;
import java.net.URLEncoder;
import java.nio.charset.StandardCharsets;
import java.text.ParseException;
import java.text.SimpleDateFormat;
import java.util.ArrayList;
import java.util.List;
import java.util.Locale;
import java.util.TimeZone;
import javax.xml.parsers.DocumentBuilderFactory;
import org.w3c.dom.Element;
import org.w3c.dom.Node;
import org.w3c.dom.NodeList;

/**
 * El cliente WebDAV del proveedor de documentos: lo justo de RFC 4918 para «Archivos»
 * (PROPFIND, GET, PUT, MKCOL, MOVE, DELETE) contra el `/dav` de Sytue Drive.
 *
 * Es Java sin Android a propósito, para probarlo en la JVM (`android/prueba`). Las rutas
 * son las de Drive, sin codificar y relativas a la raíz de WebDAV: `/` es Mi unidad y
 * `/Facturas/enero.pdf` un fichero.
 *
 * `HttpURLConnection` solo admite los métodos de HTTP/1.1, no los de WebDAV: esos van como
 * POST con `X-HTTP-Method-Override`, que el `/dav` de Drive atiende
 * (services/drive/internal/webdav).
 */
final class WebDAV {
    /** Un elemento de una carpeta. Tamaño -1 y modificado 0 si el servidor no lo dice. */
    static final class Entrada {
        final String ruta;
        final boolean carpeta;
        final long tamano;
        final long modificado;
        final String tipo;

        Entrada(String ruta, boolean carpeta, long tamano, long modificado, String tipo) {
            this.ruta = ruta;
            this.carpeta = carpeta;
            this.tamano = tamano;
            this.modificado = modificado;
            this.tipo = tipo;
        }

        String nombre() {
            return ruta.substring(ruta.lastIndexOf('/') + 1);
        }
    }

    private static final String DAV = "DAV:";
    private static final int PLAZO_MS = 30000;

    private final String base; // https://drive.…/dav, sin barra al final
    private final String rutaBase; // /dav
    private final String autorizacion;

    /** `url` es la raíz de WebDAV; `autorizacion`, la cabecera entera (`Basic …`). */
    WebDAV(String url, String autorizacion) {
        this.base = url.endsWith("/") ? url.substring(0, url.length() - 1) : url;
        this.rutaBase = URI.create(this.base).getPath();
        this.autorizacion = autorizacion;
    }

    /** La carpeta que contiene `ruta`. */
    static String padre(String ruta) {
        int i = ruta.lastIndexOf('/');
        return i <= 0 ? "/" : ruta.substring(0, i);
    }

    /** `nombre` dentro de la carpeta `carpeta`. */
    static String dentro(String carpeta, String nombre) {
        return ("/".equals(carpeta) ? "" : carpeta) + "/" + nombre;
    }

    /** La URL de `ruta`, con cada tramo codificado. */
    String url(String ruta) {
        StringBuilder b = new StringBuilder(base);
        for (String tramo : ruta.split("/")) {
            if (tramo.isEmpty()) {
                continue;
            }
            try {
                b.append('/').append(URLEncoder.encode(tramo, "UTF-8").replace("+", "%20"));
            } catch (java.io.UnsupportedEncodingException e) {
                throw new IllegalStateException(e);
            }
        }
        return b.toString();
    }

    private HttpURLConnection abrir(String metodo, String ruta) throws IOException {
        HttpURLConnection c = (HttpURLConnection) new URL(url(ruta)).openConnection();
        c.setConnectTimeout(PLAZO_MS);
        c.setReadTimeout(PLAZO_MS);
        c.setInstanceFollowRedirects(false);
        c.setRequestProperty("Authorization", autorizacion);
        switch (metodo) {
            case "GET":
            case "PUT":
            case "DELETE":
                c.setRequestMethod(metodo);
                break;
            default:
                c.setRequestMethod("POST");
                c.setRequestProperty("X-HTTP-Method-Override", metodo);
        }
        return c;
    }

    /** Manda la petición con `cuerpo` (o vacía) y falla si no es 2xx. */
    private static void enviar(HttpURLConnection c, byte[] cuerpo) throws IOException {
        if (!"GET".equals(c.getRequestMethod()) && !"DELETE".equals(c.getRequestMethod())) {
            c.setDoOutput(true);
            c.setFixedLengthStreamingMode(cuerpo.length);
            try (OutputStream o = c.getOutputStream()) {
                o.write(cuerpo);
            }
        }
        comprobar(c);
    }

    private static void comprobar(HttpURLConnection c) throws IOException {
        int s = c.getResponseCode();
        if (s >= 200 && s < 300) {
            return;
        }
        c.disconnect();
        if (s == 404) {
            throw new FileNotFoundException("Sytue Drive: no existe");
        }
        throw new IOException("Sytue Drive contesta " + s);
    }

    /** PROPFIND con `profundidad` 0 (el propio elemento) o 1 (y lo de dentro). */
    List<Entrada> listar(String ruta, int profundidad) throws IOException {
        HttpURLConnection c = abrir("PROPFIND", ruta);
        c.setRequestProperty("Depth", Integer.toString(profundidad));
        c.setRequestProperty("Content-Type", "application/xml; charset=utf-8");
        enviar(c, PROPIEDADES);
        try (InputStream in = c.getInputStream()) {
            return multistatus(in, rutaBase);
        } finally {
            c.disconnect();
        }
    }

    /** Lo que hay DENTRO de la carpeta `ruta`, sin ella. */
    List<Entrada> hijos(String ruta) throws IOException {
        List<Entrada> todo = listar(ruta, 1);
        List<Entrada> out = new ArrayList<>(todo.size());
        for (Entrada e : todo) {
            if (!e.ruta.equals(ruta)) {
                out.add(e);
            }
        }
        return out;
    }

    /** El propio elemento, o FileNotFoundException si no existe. */
    Entrada uno(String ruta) throws IOException {
        for (Entrada e : listar(ruta, 0)) {
            if (e.ruta.equals(ruta)) {
                return e;
            }
        }
        throw new FileNotFoundException(ruta);
    }

    boolean existe(String ruta) throws IOException {
        try {
            uno(ruta);
            return true;
        } catch (FileNotFoundException no) {
            return false;
        }
    }

    /** Baja `ruta` a `destino`: escribe al lado y renombra, para no dejarlo a medias. */
    void bajar(String ruta, File destino) throws IOException {
        HttpURLConnection c = abrir("GET", ruta);
        comprobar(c);
        File tmp = new File(destino.getPath() + ".bajando");
        try (InputStream in = c.getInputStream(); OutputStream o = new FileOutputStream(tmp)) {
            byte[] b = new byte[64 * 1024];
            for (int n; (n = in.read(b)) > 0; ) {
                o.write(b, 0, n);
            }
        } finally {
            c.disconnect();
        }
        if (!tmp.renameTo(destino)) {
            tmp.delete();
            throw new IOException("no se puede dejar la copia en la caché");
        }
    }

    /** Sube `origen` como `ruta`: si existe, es una versión nueva del mismo fichero. */
    void subir(String ruta, File origen, String tipo) throws IOException {
        HttpURLConnection c = abrir("PUT", ruta);
        c.setDoOutput(true);
        c.setFixedLengthStreamingMode(origen.length());
        if (tipo != null) {
            c.setRequestProperty("Content-Type", tipo);
        }
        try (InputStream in = new FileInputStream(origen); OutputStream o = c.getOutputStream()) {
            byte[] b = new byte[64 * 1024];
            for (int n; (n = in.read(b)) > 0; ) {
                o.write(b, 0, n);
            }
        }
        try {
            comprobar(c);
        } finally {
            c.disconnect();
        }
    }

    void crearCarpeta(String ruta) throws IOException {
        HttpURLConnection c = abrir("MKCOL", ruta);
        try {
            enviar(c, new byte[0]);
        } finally {
            c.disconnect();
        }
    }

    /** MOVE sin pisar: si `hasta` existe, el servidor contesta 412 y esto falla. */
    void mover(String desde, String hasta) throws IOException {
        HttpURLConnection c = abrir("MOVE", desde);
        c.setRequestProperty("Destination", url(hasta));
        c.setRequestProperty("Overwrite", "F");
        try {
            enviar(c, new byte[0]);
        } finally {
            c.disconnect();
        }
    }

    void borrar(String ruta) throws IOException {
        HttpURLConnection c = abrir("DELETE", ruta);
        try {
            enviar(c, new byte[0]);
        } finally {
            c.disconnect();
        }
    }

    private static final byte[] PROPIEDADES = ("<?xml version=\"1.0\" encoding=\"utf-8\"?>"
            + "<d:propfind xmlns:d=\"DAV:\"><d:prop><d:resourcetype/><d:getcontentlength/>"
            + "<d:getlastmodified/><d:getcontenttype/></d:prop></d:propfind>").getBytes(StandardCharsets.UTF_8);

    /** Lee un 207 Multi-Status. `rutaBase` es el prefijo de WebDAV (`/dav`) que se quita. */
    static List<Entrada> multistatus(InputStream in, String rutaBase) throws IOException {
        Element raiz;
        try {
            DocumentBuilderFactory f = DocumentBuilderFactory.newInstance();
            f.setNamespaceAware(true);
            f.setExpandEntityReferences(false);
            try {
                f.setFeature("http://apache.org/xml/features/disallow-doctype-decl", true);
            } catch (Exception sinEsa) {
                // el analizador de Android no la conoce, y tampoco expande entidades externas
            }
            raiz = f.newDocumentBuilder().parse(in).getDocumentElement();
        } catch (Exception e) {
            throw new IOException("respuesta de WebDAV ilegible", e);
        }
        List<Entrada> out = new ArrayList<>();
        NodeList respuestas = raiz.getElementsByTagNameNS(DAV, "response");
        for (int i = 0; i < respuestas.getLength(); i++) {
            Element r = (Element) respuestas.item(i);
            String href = texto(r, "href");
            if (href == null) {
                continue;
            }
            String ruta;
            try {
                ruta = URI.create(href.trim()).getPath();
            } catch (IllegalArgumentException malo) {
                continue;
            }
            if (ruta.startsWith(rutaBase)) {
                ruta = ruta.substring(rutaBase.length());
            }
            while (ruta.length() > 1 && ruta.endsWith("/")) {
                ruta = ruta.substring(0, ruta.length() - 1);
            }
            if (ruta.isEmpty()) {
                ruta = "/";
            }
            Element prop = propOk(r);
            if (prop == null) {
                continue;
            }
            NodeList tipoRecurso = prop.getElementsByTagNameNS(DAV, "collection");
            String largo = texto(prop, "getcontentlength");
            String tipo = texto(prop, "getcontenttype");
            out.add(new Entrada(ruta, tipoRecurso.getLength() > 0, numero(largo), fecha(texto(prop, "getlastmodified")),
                    tipo == null || tipo.isEmpty() ? null : tipo));
        }
        return out;
    }

    /** El `prop` del `propstat` con 200. */
    private static Element propOk(Element respuesta) {
        NodeList ps = respuesta.getElementsByTagNameNS(DAV, "propstat");
        for (int i = 0; i < ps.getLength(); i++) {
            Element p = (Element) ps.item(i);
            String status = texto(p, "status");
            if (status != null && status.contains(" 200")) {
                NodeList prop = p.getElementsByTagNameNS(DAV, "prop");
                return prop.getLength() > 0 ? (Element) prop.item(0) : null;
            }
        }
        return null;
    }

    private static String texto(Element e, String nombre) {
        NodeList l = e.getElementsByTagNameNS(DAV, nombre);
        if (l.getLength() == 0) {
            return null;
        }
        Node n = l.item(0);
        return n.getTextContent();
    }

    private static long numero(String s) {
        try {
            return s == null ? -1 : Long.parseLong(s.trim());
        } catch (NumberFormatException e) {
            return -1;
        }
    }

    private static long fecha(String s) {
        if (s == null) {
            return 0;
        }
        SimpleDateFormat f = new SimpleDateFormat("EEE, dd MMM yyyy HH:mm:ss zzz", Locale.US);
        f.setTimeZone(TimeZone.getTimeZone("GMT"));
        try {
            return f.parse(s.trim()).getTime();
        } catch (ParseException e) {
            return 0;
        }
    }
}
