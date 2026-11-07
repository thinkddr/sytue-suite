// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Paths;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.Collections;
import java.util.List;
import java.util.TimeZone;

/**
 * El mismo contrato de privacidad que widgets_test.go, ejecutado en la JVM remota. El
 * primer argumento es apps/nucleo/widgets/testdata/contrato.tsv: los casos comunes con Go
 * y Swift no se copian aquí.
 */
public final class WidgetSnapshotPrueba {
    private static final long NOW = 1792500000L;
    private static int checks;

    private static void check(boolean ok, String message) {
        checks++;
        if (!ok) {
            throw new AssertionError(message);
        }
    }

    private static WidgetSnapshot make(long version, String product, String generation,
            long updated, long expires, boolean active, WidgetSnapshot.Counter... counters) {
        return new WidgetSnapshot(version, product, generation, updated, expires, active, Arrays.asList(counters));
    }

    private static final WidgetSnapshot.Entorno ENTORNO =
            new WidgetSnapshot.Entorno("generation-one", "marca", "marca", false, false);

    private static boolean visible(WidgetSnapshot s) {
        return s.visible("mail", ENTORNO, NOW);
    }

    private static String campo(String valor) {
        return "-".equals(valor) ? "" : valor;
    }

    /** Los casos de testdata/contrato.tsv, con la misma lectura que contrato_test.go. */
    private static void contrato(String fichero) throws IOException {
        int visibles = 0, rancios = 0, vigentes = 0;
        for (String linea : Files.readAllLines(Paths.get(fichero), StandardCharsets.UTF_8)) {
            if (linea.isEmpty() || linea.startsWith("#")) {
                continue;
            }
            String[] c = linea.split("\t", -1);
            switch (c[0]) {
                case "visible": {
                    WidgetSnapshot.Entorno e = new WidgetSnapshot.Entorno(campo(c[2]), campo(c[3]), campo(c[4]),
                            "1".equals(c[5]), "1".equals(c[6]));
                    WidgetSnapshot s = new WidgetSnapshot(Long.parseLong(c[7]), "mail", campo(c[9]),
                            Long.parseLong(c[10]), Long.parseLong(c[11]), "1".equals(c[8]),
                            Collections.singletonList(new WidgetSnapshot.Counter("unread", 3)));
                    check(s.visible("mail", e, Long.parseLong(c[12])) == "1".equals(c[13]), "contrato visible: " + c[1]);
                    visibles++;
                    break;
                }
                case "rancio": {
                    WidgetSnapshot s = make(2, "mail", "g", Long.parseLong(c[2]), Long.parseLong(c[2]) + 60, true);
                    check(s.rancio(Long.parseLong(c[3])) == "1".equals(c[4]), "contrato rancio: " + c[1]);
                    rancios++;
                    break;
                }
                case "vigente": {
                    WidgetSnapshot.Item item = new WidgetSnapshot.Item("x", "", "", "", Long.parseLong(c[3]),
                            Long.parseLong(c[4]), "1".equals(c[5]), false, false);
                    TimeZone zona = TimeZone.getTimeZone(c[2]);
                    check(zona.getID().equals(c[2]), "zona conocida: " + c[2]);
                    check(WidgetSnapshot.vigente(item, Long.parseLong(c[6]), zona) == "1".equals(c[7]),
                            "contrato vigente: " + c[1] + " en " + c[2]);
                    vigentes++;
                    break;
                }
                default:
                    throw new AssertionError("tipo de caso desconocido: " + c[0]);
            }
        }
        check(visibles >= 15 && rancios >= 4 && vigentes >= 10, "el contrato se ha quedado corto");
    }

    public static void main(String[] args) throws IOException {
        check(args.length == 1, "uso: WidgetSnapshotPrueba apps/nucleo/widgets/testdata/contrato.tsv");
        contrato(args[0]);
        WidgetSnapshot.Counter unread = new WidgetSnapshot.Counter("unread", 7);
        WidgetSnapshot valid = make(2, "mail", "generation-one", NOW, NOW + 120, true, unread);
        check(visible(valid), "valid snapshot");
        for (String kind : Arrays.asList("calendars", "address_books")) {
            check(visible(make(2, "mail", "generation-one", NOW, NOW + 120, true,
                    new WidgetSnapshot.Counter(kind, 2))), "complete collection kind " + kind);
        }
        for (long percent : new long[] {0, 32, 100}) {
            check(visible(make(2, "mail", "generation-one", NOW, NOW + 120, true,
                    new WidgetSnapshot.Counter("storage_percent", percent))), "valid storage percentage");
        }
        check(!visible(make(2, "mail", "generation-one", NOW, NOW + 120, true,
                new WidgetSnapshot.Counter("storage_percent", 101))), "invalid storage percentage");
        check(!valid.visible("mail", new WidgetSnapshot.Entorno("generation-two", "marca", "marca", false, false), NOW), "late old account response");
        check(!valid.visible("mail", new WidgetSnapshot.Entorno("", "marca", "marca", false, false), NOW), "logout");
        check(!valid.visible("rooms", ENTORNO, NOW), "different product");
        check(!valid.visible("mail", new WidgetSnapshot.Entorno("generation-one", "marca", "marca", false, true), NOW), "session/OS lock");
        check(!valid.visible("mail", ENTORNO, NOW + 120), "expiry");
        check(!visible(make(1, "mail", "generation-one", NOW, NOW + 120, true)), "v1 snapshot");
        check(!visible(make(3, "mail", "generation-one", NOW, NOW + 120, true)), "future schema");
        check(!visible(make(2, "mail", "", NOW, NOW + 120, true)), "empty generation");
        check(!visible(make(2, "mail", "generation-one", 0, NOW + 120, true)), "zero timestamp");
        check(!visible(make(2, "mail", "generation-one", NOW + 6, NOW + 120, true)), "future timestamp");
        check(visible(make(2, "mail", "generation-one", NOW, NOW + 86400, true)), "one day TTL");
        check(!visible(make(2, "mail", "generation-one", NOW, NOW + 86401, true)), "excessive TTL");
        check(!visible(make(2, "mail", "generation-one", NOW, NOW, true)), "empty TTL");
        check(!visible(make(2, "mail", "generation-one", NOW, NOW + 120, false)), "inactive");
        check(!visible(make(2, "mail", "generation-one", NOW, NOW + 120, true,
                new WidgetSnapshot.Counter("subject", 1))), "no message content category");
        check(!visible(make(2, "mail", "generation-one", NOW, NOW + 120, true,
                new WidgetSnapshot.Counter("unread", -1))), "negative count");
        check(!visible(make(2, "mail", "generation-one", NOW, NOW + 120, true,
                new WidgetSnapshot.Counter("unread", 1_000_000_000_001L))), "count bound");
        check(!visible(make(2, "mail", "generation-one", NOW, NOW + 120, true, unread, unread)), "duplicate kind");
        List<WidgetSnapshot.Counter> values = new ArrayList<>(Collections.nCopies(13, unread));
        check(!visible(new WidgetSnapshot(2, "mail", "generation-one", NOW, NOW + 120, true, values)), "list bound");
        values.clear();
        values.add(unread);
        WidgetSnapshot detached = new WidgetSnapshot(2, "mail", "generation-one", NOW, NOW + 120, true, values);
        values.clear();
        check(detached.counters.size() == 1, "defensive copy");
        WidgetSnapshot.Item okItem = new WidgetSnapshot.Item(
                "Revisión trimestral", "Sala Norte", "10:00", "#953be7", NOW + 60, NOW + 3600, false, true, false);
        WidgetSnapshot.CalendarGrid okCal = new WidgetSnapshot.CalendarGrid(
                2026, 11, Arrays.asList(3, 14, 28), Collections.singletonMap("3", "#953be7"));
        WidgetSnapshot rich = new WidgetSnapshot(
                2, "calendar", "generation-one", NOW, NOW + 120, true,
                Collections.singletonList(new WidgetSnapshot.Counter("events", 2)),
                Collections.singletonList(okItem), okCal);
        check(rich.visible("calendar", ENTORNO, NOW), "rich calendar snapshot");
        check(!new WidgetSnapshot(2, "mail", "generation-one", NOW, NOW + 120, true, values,
                Collections.singletonList(new WidgetSnapshot.Item("enc:v1:secret", "", "", "", 0, 0, false, false, false)), null)
                .visible("mail", ENTORNO, NOW), "reject encrypted title");
        check(!new WidgetSnapshot(2, "mail", "generation-one", NOW, NOW + 120, true, values,
                Collections.singletonList(new WidgetSnapshot.Item("Title", "ENC:V1:secret", "", "", 0, 0, false, false, false)), null)
                .visible("mail", ENTORNO, NOW), "reject encrypted subtitle");
        check(!new WidgetSnapshot(2, "mail", "generation-one", NOW, NOW + 120, true, values,
                Collections.nCopies(9, okItem), null)
                .visible("mail", ENTORNO, NOW), "reject >8 items");
        check(!new WidgetSnapshot(2, "calendar", "generation-one", NOW, NOW + 120, true, values, null,
                new WidgetSnapshot.CalendarGrid(2026, 2, Collections.singletonList(29), null))
                .visible("calendar", ENTORNO, NOW), "reject non-leap Feb 29");
        check(!new WidgetSnapshot(2, "calendar", "generation-one", NOW, NOW + 120, true, values, null,
                new WidgetSnapshot.CalendarGrid(2026, 11, Arrays.asList(14, 3), null))
                .visible("calendar", ENTORNO, NOW), "reject unsorted busy days");
        check(!new WidgetSnapshot(2, "calendar", "generation-one", NOW, NOW + 120, true, values, null,
                new WidgetSnapshot.CalendarGrid(2026, 11, null, Collections.singletonMap("03", "#953be7")))
                .visible("calendar", ENTORNO, NOW), "reject non-canonical day key");
        check(!valid.visible("mail", new WidgetSnapshot.Entorno("generation-one", "vieja", "marca", false, false), NOW),
                "session changed by a build that does not know the epoch");
        check(!valid.visible("mail", new WidgetSnapshot.Entorno("generation-one", "marca", "marca", true, false), NOW),
                "closed session marker");
        check(!valid.visible("mail", null, NOW), "no environment");
        TimeZone madrid = TimeZone.getTimeZone("Europe/Madrid");
        // 2026-11-07 12:00 en Madrid: el próximo repintado es lo primero que cambia.
        long mediodia = 1794049200L;
        long medianoche = 1794092400L + 5; // 2026-11-08 00:00:05 en Madrid
        check(WidgetSnapshot.proximoRepintado(null, mediodia, madrid) == medianoche, "without snapshot, midnight");
        WidgetSnapshot.Item reunion = new WidgetSnapshot.Item("Reunión", "", "", "", mediodia - 600, mediodia + 1800,
                false, false, false);
        WidgetSnapshot.Item terminado = new WidgetSnapshot.Item("Antes", "", "", "", mediodia - 7200, mediodia - 3600,
                false, false, false);
        WidgetSnapshot agenda = new WidgetSnapshot(2, "calendar", "generation-one", mediodia, mediodia + 86400, true,
                Collections.<WidgetSnapshot.Counter>emptyList(), Arrays.asList(terminado, reunion), null);
        check(WidgetSnapshot.proximoRepintado(agenda, mediodia, madrid) == mediodia + 1800, "repaint when an event ends");
        check(WidgetSnapshot.proximoRepintado(agenda, mediodia + 1800, madrid) == medianoche, "then at midnight");
        WidgetSnapshot pronto = new WidgetSnapshot(2, "mail", "generation-one", mediodia, mediodia + 60, true,
                Collections.<WidgetSnapshot.Counter>emptyList(), Arrays.asList(reunion), null);
        check(WidgetSnapshot.proximoRepintado(pronto, mediodia, madrid) == mediodia + 60, "repaint at expiry; mail items never end");
        check(agenda.conVigentes(mediodia, madrid).items.size() == 1
                && agenda.conVigentes(mediodia, madrid).items.get(0).title.equals("Reunión"), "only current calendar rows");
        check(pronto.conVigentes(mediodia + 3600, madrid) == pronto, "other products are not trimmed");
        System.out.println("Widget snapshot: " + checks + " checks passed");
    }
}
