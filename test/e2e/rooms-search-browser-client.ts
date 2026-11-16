// SPDX-License-Identifier: Apache-2.0
import { SuiteMatrix } from "../../apps/frontend/packages/offline/src/matrix/client";
import { MatrixSessionProvenanceMissing } from "../../apps/frontend/packages/offline/src/matrix/session-provenance";
import { setRoomsMatrix } from "../../apps/frontend/apps/rooms/src/client/matrix/client";
import { searchEncryptedMessages } from "../../apps/frontend/apps/rooms/src/client/matrix/search";
import {
  clearMatrixMessages,
  knownMessage,
} from "../../apps/frontend/apps/rooms/src/client/matrix/messages";
const state = { stage: "loaded", ok: false, reason: "" };
let suite: SuiteMatrix | undefined;
Object.assign(window, {
  roomsSearchProbe: state,
  async runRoomsSearch(fixture: any, name: string) {
    let recovery = "invalid artificial key";
    try {
      if (fixture.artificial !== true)
        throw Error("synthetic fixture required");
      state.stage = "open-rust";
      suite = await SuiteMatrix.open(
        fixture.bootstrap,
        {
          saveRecoveryKey: async () => {
            throw Error("must recover");
          },
          readRecoveryKey: async () => recovery,
          authenticate: async () => {
            throw Error("must not reset identity");
          },
        },
        {
          readSession: async () => fixture.sessions[name],
          forgetSession: async () => {},
          refreshBootstrap: async () => fixture.bootstrap,
        },
      );
      state.stage = "reject-wrong-recovery";
      let denied = false;
      try {
        await suite.recover();
      } catch {
        denied = true;
      }
      if (!denied || (await suite.trusted()))
        throw Error("wrong recovery trusted");
      recovery = fixture.recovery;
      state.stage = "recover-native-backup";
      await suite.recover();
      if (!(await suite.trusted())) throw Error("recovered device untrusted");
      state.stage = "reject-anonymous-backup";
      const rejectAnonymous = async (): Promise<void> => {
        let blocked = false;
        try {
          await suite!.read(
            fixture.channel,
            fixture.anonymous_reference,
            fixture.bootstrap.actor,
          );
        } catch (error) {
          blocked =
            error instanceof MatrixSessionProvenanceMissing &&
            error.requiresOriginalDevice;
        }
        if (!blocked)
          throw Error(
            "anonymous backup session became authenticated Rooms content",
          );
      };
      await rejectAnonymous();
      await suite.backup();
      await rejectAnonymous();
      setRoomsMatrix(suite);
      state.stage = "scan-real-history";
      let result = await searchEncryptedMessages(
        "REUNION viernes from:ana in:#general on:2024-03-05 -borrador",
      );
      if (
        result.messages.length !== 1 ||
        result.messages[0]?.id !== fixture.reply_id ||
        !result.messages[0]?.parentId
      )
        throw Error("deep encrypted reply missing");
      if (knownMessage(fixture.reply_id).body !== result.messages[0].body)
        throw Error("result cache changed content");
      const reference = result.messages[0].matrix_event!;
      denied = false;
      try {
        await suite.read(fixture.channel, reference, "another-actor");
      } catch {
        denied = true;
      }
      if (!denied) throw Error("foreign sender accepted");
      state.stage = "file-and-operators";
      result = await searchEncryptedMessages("is:pinned has:file has:link");
      if (
        result.messages.length !== 1 ||
        result.messages[0]?.id !== fixture.file_id ||
        result.messages[0]?.attachments?.length !== 1
      )
        throw Error("encrypted file/link/pin missing");
      const file = result.messages[0].attachments![0]!;
      const objectURL = await suite.download(file);
      try {
        if ((await (await fetch(objectURL)).text()) !== "private")
          throw Error("native attachment changed");
      } finally {
        suite.releaseAttachment(objectURL);
      }
      result = await searchEncryptedMessages("is:saved");
      if (
        result.messages.length !== 1 ||
        result.messages[0]?.id !== fixture.reply_id
      )
        throw Error("saved metadata missing");
      state.stage = "migrate-original-browser-session";
      const internal = suite as any;
      const sdkCrypto = suite.client.getCrypto()!;
      sdkCrypto.setEncryptedEventPublicationGuard(undefined);
      let historical;
      try {
        historical = await suite.send(
          fixture.channel,
          { body: "browser-" + name + " pre-migration" },
          crypto.randomUUID(),
        );
      } finally {
        sdkCrypto.setEncryptedEventPublicationGuard((event) =>
          internal.attestOwnPublication(event),
        );
      }
      const raw = await suite.client.fetchRoomEvent(
        historical.room_id,
        historical.event_id,
      );
      const original = suite.client.getEventMapper()({
        ...raw,
        room_id: historical.room_id,
      });
      await suite.client.decryptEventIfNeeded(original);
      const session = internal.sdkSession(original);
      if (await internal.roomProvenance().contains(session))
        throw Error("historical browser session was already attested");
      const migrated = await suite.backfillRoomSessionProvenance();
      if (
        migrated.authenticated < 1 ||
        migrated.blocked < 1 ||
        !(await internal.roomProvenance().contains(session))
      )
        throw Error(
          "original browser session migration did not preserve authenticated and unresolved history",
        );
      await rejectAnonymous();
      const resumed = await suite.backfillRoomSessionProvenance();
      if (resumed.blocked < 1)
        throw Error(
          "unresolved anonymous history was permanently skipped by checkpoint",
        );
      await rejectAnonymous();
      state.stage = "send-to-native";
      const sent = await suite.send(
        fixture.channel,
        { body: "browser-" + name + " encrypted reply" },
        crypto.randomUUID(),
      );
      await suite.backup();
      state.stage = "complete";
      state.ok = true;
      return sent;
    } catch (error) {
      state.reason = error instanceof Error ? error.name : "Error";
      state.stage = "failed:" + state.stage;
      throw new Error(state.stage + ":" + state.reason);
    } finally {
      recovery = "";
      clearMatrixMessages();
      setRoomsMatrix(null);
      suite?.close();
      suite = undefined;
    }
  },
});
