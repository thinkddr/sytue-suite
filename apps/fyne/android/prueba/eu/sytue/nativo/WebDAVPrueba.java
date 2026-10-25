// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

import com.sun.net.httpserver.HttpServer;
import java.io.File;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.InetSocketAddress;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.util.ArrayList;
import java.util.List;

/**
 * La prueba de {@link WebDAV} en la JVM, contra un servidor de mentira: lo que manda
 * (método, cabeceras, ruta codificada) y lo que entiende de un 207 como el de
 * golang.org/x/net/webdav. La corre `empaquetar.sh android` antes de compilar el dex.
 */
public final class WebDAVPrueba {
    private static final String MULTISTATUS = "<?xml version=\"1.0\" encoding=\"UTF-8\"?>"
            + "<D:multistatus xmlns:D=\"DAV:\">"
            + "<D:response><D:href>/dav/Facturas/</D:href><D:propstat><D:prop>"
            + "<D:resourcetype><D:collection/></D:resourcetype></D:prop>"
            + "<D:status>HTTP/1.1 200 OK</D:status></D:propstat></D:response>"
            + "<D:response><D:href>/dav/Facturas/enero%20y%20m%C3%A1s.pdf</D:href><D:propstat><D:prop>"
            + "<D:resourcetype></D:resourcetype><D:getcontentlength>42</D:getcontentlength>"
            + "<D:getlastmodified>Mon, 19 Oct 2026 10:00:00 GMT</D:getlastmodified>"
            + "<D:getcontenttype>application/pdf</D:getcontenttype></D:prop>"
            + "<D:status>HTTP/1.1 200 OK</D:status></D:propstat>"
            + "<D:propstat><D:prop><D:displayname/></D:prop><D:status>HTTP/1.1 404 Not Found</D:status></D:propstat>"
            + "</D:response>"
            + "<D:response><D:href>/dav/Facturas/sub/</D:href><D:propstat><D:prop>"
            + "<D:resourcetype><D:collection/></D:resourcetype></D:prop>"
            + "<D:status>HTTP/1.1 200 OK</D:status></D:propstat></D:response>"
            + "</D:multistatus>";

    private static void comprobar(boolean ok, String que) {
        if (!ok) {
            throw new AssertionError(que);
        }
    }

    public static void main(String[] args) throws Exception {
        final List<String> visto = new ArrayList<>();
        HttpServer s = HttpServer.create(new InetSocketAddress("127.0.0.1", 0), 0);
        s.createContext("/", intercambio -> {
            String metodo = intercambio.getRequestMethod();
            String override = intercambio.getRequestHeaders().getFirst("X-HTTP-Method-Override");
            if (override != null) {
                metodo = metodo + ">" + override;
            }
            byte[] cuerpo;
            try (InputStream in = intercambio.getRequestBody()) {
                cuerpo = in.readAllBytes();
            }
            visto.add(metodo + " " + intercambio.getRequestURI().getRawPath()
                    + " auth=" + intercambio.getRequestHeaders().getFirst("Authorization")
                    + " depth=" + intercambio.getRequestHeaders().getFirst("Depth")
                    + " dest=" + intercambio.getRequestHeaders().getFirst("Destination")
                    + " cuerpo=" + cuerpo.length);
            byte[] r = new byte[0];
            int status = 201;
            if ("PROPFIND".equals(override)) {
                if (intercambio.getRequestURI().getRawPath().endsWith("/nada")) {
                    status = 404;
                } else {
                    r = MULTISTATUS.getBytes(StandardCharsets.UTF_8);
                    status = 207;
                }
            } else if ("GET".equals(metodo)) {
                r = "hola".getBytes(StandardCharsets.UTF_8);
                status = 200;
            }
            intercambio.sendResponseHeaders(status, r.length == 0 ? -1 : r.length);
            try (OutputStream o = intercambio.getResponseBody()) {
                o.write(r);
            }
        });
        s.start();
        try {
            String base = "http://127.0.0.1:" + s.getAddress().getPort() + "/dav/";
            WebDAV d = new WebDAV(base, "Basic abc");

            List<WebDAV.Entrada> hijos = d.hijos("/Facturas");
            comprobar(hijos.size() == 2, "hijos: " + hijos.size());
            WebDAV.Entrada f = hijos.get(0);
            comprobar(f.ruta.equals("/Facturas/enero y más.pdf") && !f.carpeta, "ruta: " + f.ruta);
            comprobar(f.nombre().equals("enero y más.pdf"), "nombre: " + f.nombre());
            comprobar(f.tamano == 42 && f.tipo.equals("application/pdf"), "tamaño/tipo");
            comprobar(f.modificado == 1792404000000L, "modificado: " + f.modificado);
            comprobar(hijos.get(1).carpeta && hijos.get(1).ruta.equals("/Facturas/sub"), "subcarpeta");
            comprobar(visto.get(0).startsWith("POST>PROPFIND /dav/Facturas auth=Basic abc depth=1"), visto.get(0));
            comprobar(!d.existe("/nada"), "existe /nada");

            File tmp = Files.createTempFile("webdav", ".txt").toFile();
            tmp.deleteOnExit();
            d.bajar("/Facturas/enero y más.pdf", tmp);
            comprobar(new String(Files.readAllBytes(tmp.toPath()), StandardCharsets.UTF_8).equals("hola"), "bajar");
            d.subir("/a+b.txt", tmp, "text/plain");
            d.crearCarpeta("/Nueva carpeta");
            d.mover("/a+b.txt", "/Facturas/c.txt");
            d.borrar("/Facturas/c.txt");
            String[] quiero = {
                "GET /dav/Facturas/enero%20y%20m%C3%A1s.pdf",
                "PUT /dav/a%2Bb.txt",
                "POST>MKCOL /dav/Nueva%20carpeta",
                "POST>MOVE /dav/a%2Bb.txt",
                "DELETE /dav/Facturas/c.txt",
            };
            for (int i = 0; i < quiero.length; i++) {
                String v = visto.get(visto.size() - quiero.length + i);
                comprobar(v.startsWith(quiero[i] + " auth=Basic abc"), "pedido " + i + ": " + v);
            }
            comprobar(visto.get(visto.size() - 2).contains("dest=" + base + "Facturas/c.txt"), "destino del MOVE");
            comprobar(visto.get(visto.size() - 4).endsWith("cuerpo=4"), "cuerpo del PUT");

            comprobar(WebDAV.padre("/a/b").equals("/a") && WebDAV.padre("/a").equals("/"), "padre");
            comprobar(WebDAV.dentro("/", "x").equals("/x") && WebDAV.dentro("/a", "x").equals("/a/x"), "dentro");
            System.out.println("WebDAVPrueba: ok");
        } catch (IOException e) {
            throw new AssertionError(e);
        } finally {
            s.stop(0);
        }
    }
}
