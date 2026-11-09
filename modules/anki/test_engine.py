"""Offline contract tests against Anki's real collection and scheduler."""
import base64
import tempfile
import unittest
import struct
import zlib
from pathlib import Path

from engine import Engine


class EngineTests(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.engine = Engine(Path(self.directory.name))

    def tearDown(self):
        self.engine.close()
        self.directory.cleanup()

    def legacy(self, action, **params):
        return self.engine.legacy(action, params)

    def word(self, word="hello"):
        self.legacy("createDeck", deck="WinOne 生词本")
        self.legacy("createModel", modelName="WinOne Word", inOrderFields=["Word", "Meaning"],
                    cardTemplates=[{"Name": "Recall", "Front": "{{Word}}", "Back": "{{FrontSide}}<hr>{{Meaning}}"}], css="")
        return self.legacy("addNote", note={"deckName": "WinOne 生词本", "modelName": "WinOne Word",
                    "fields": {"Word": word, "Meaning": "你好"}, "tags": ["winone"]})

    def test_notes_media_and_collection_survive_reopen(self):
        note_id = self.word()
        self.assertGreater(note_id, 0)
        self.assertEqual(self.legacy("findNotes", query='"deck:WinOne 生词本"'), [note_id])
        audio = base64.b64encode(b"ID3-offline-fixture").decode()
        self.assertEqual(self.legacy("storeMediaFile", filename="hello.mp3", data=audio), "hello.mp3")
        self.engine.close()
        self.engine = Engine(Path(self.directory.name))
        self.assertEqual(self.legacy("notesInfo", notes=[note_id])[0]["fields"]["Meaning"]["value"], "你好")
        self.assertEqual(self.legacy("retrieveMediaFile", filename="hello.mp3"), audio)

    def test_review_uses_official_queue_and_undo(self):
        self.word()
        self.assertTrue(self.legacy("guiDeckReview", name="WinOne 生词本"))
        first = self.legacy("guiCurrentCard")
        self.assertIn("hello", first["question"])
        self.assertTrue(self.legacy("guiShowAnswer"))
        self.assertTrue(self.legacy("guiAnswerCard", ease=3))
        self.assertEqual(self.legacy("getNumCardsReviewedToday"), 1)
        self.engine.invoke("collection.undo", {})
        self.assertEqual(self.legacy("getNumCardsReviewedToday"), 0)

    def test_internal_navigation_is_an_event_and_network_is_explicit(self):
        self.word()
        ids = self.legacy("guiBrowse", query='"deck:WinOne 生词本"')
        self.assertEqual(len(ids), 1)
        self.assertEqual(self.engine.take_events()[-1]["page"], "browser")
        with self.assertRaisesRegex(Exception, "explicit|联网"):
            self.legacy("sync")

    def test_native_overview_and_editor_keep_drafts_until_save(self):
        note_id = self.word()
        overview = self.engine.invoke("winone.overview")
        self.assertTrue(overview["fsrs"])
        editor = self.engine.invoke("editors.openNote", {"noteId": str(note_id)})
        self.engine.invoke("editors.updateDraft", {"editorId": editor["id"], "patch": {"fields": {"Meaning": "updated"}}})
        self.assertEqual(self.legacy("notesInfo", notes=[note_id])[0]["fields"]["Meaning"]["value"], "你好")
        with self.assertRaisesRegex(Exception, "editor|编辑"):
            self.engine.invoke("notes.delete", {"ids": [str(note_id)]})
        self.engine.invoke("winone.editor.save", {"editorId": editor["id"]})
        self.assertEqual(self.legacy("notesInfo", notes=[note_id])[0]["fields"]["Meaning"]["value"], "updated")

    def test_official_backend_catalog_exposes_preferences_and_all_services(self):
        catalog = self.engine.invoke("winone.backend.catalog")
        self.assertIn("get_preferences", catalog)
        self.assertIn("get_image_occlusion_note", catalog)
        preferences = self.engine.invoke("winone.backend.call", {"method": "get_preferences", "params": {}})
        self.assertIn("scheduling", preferences)

    def test_api_describe_reports_real_capability_status(self):
        description = self.engine.invoke("api.describe")
        self.assertEqual(description["ankiVersion"], "26.09.4")
        self.assertTrue(description["schema"])
        self.assertTrue(all(item["available"] for item in description["methods"]), [m for m in description["methods"] if not m["available"]])

    def test_profiles_and_apkg_keep_real_data_and_media(self):
        self.word()
        archive = str(Path(self.directory.name) / "words.apkg")
        self.engine.invoke("winone.export", {"path": archive, "kind": ".apkg"})
        self.engine.invoke("winone.profiles.create", {"name": "第二个档案"})
        self.legacy("loadProfile", name="第二个档案")
        self.assertEqual(self.legacy("findNotes", query=""), [])
        self.engine.invoke("winone.import.run", {"path": archive})
        self.assertEqual(len(self.legacy("findNotes", query="hello")), 1)
        self.legacy("loadProfile", name="WinOne")
        self.assertEqual(len(self.legacy("findNotes", query="hello")), 1)

    def test_collection_replace_requires_confirmation_and_text_export_works(self):
        self.word()
        archive = str(Path(self.directory.name) / "collection.colpkg")
        self.engine.invoke("winone.export", {"path": archive, "kind": ".colpkg"})
        with self.assertRaisesRegex(Exception, "Confirm"):
            self.engine.invoke("winone.import.run", {"path": archive})
        target = Path(self.directory.name) / "notes.txt"
        self.engine.invoke("winone.export", {"path": str(target), "kind": ".txt"})
        self.assertIn("hello", target.read_text(encoding="utf-8"))

    def test_typing_uses_official_answer_comparison_and_media_controls(self):
        self.legacy("createDeck", deck="Typing")
        self.legacy("createModel", modelName="Typing", inOrderFields=["Word", "Meaning", "Audio"],
                    cardTemplates=[{"Name": "Typing", "Front": "{{Meaning}}<br>{{type:Word}}", "Back": "{{FrontSide}}<hr>{{type:Word}}{{Audio}}"}])
        self.legacy("addNote", note={"deckName": "Typing", "modelName": "Typing", "fields": {"Word": "hello", "Meaning": "greeting", "Audio": "[sound:hello.mp3]"}})
        self.legacy("guiDeckReview", name="Typing")
        value = self.engine.invoke("winone.review", {"typed": "hello"})
        self.assertTrue(value["typing"])
        self.assertNotIn("[[type:", value["rendered"]["answerHtml"])
        self.assertIn("typeGood", value["rendered"]["answerHtml"])
        self.assertIn("audio controls", value["rendered"]["answerHtml"])

    def test_image_occlusion_roundtrip_uses_official_note_type_and_cards(self):
        def chunk(kind, data):
            return struct.pack(">I", len(data)) + kind + data + struct.pack(">I", zlib.crc32(kind + data))
        png = b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", 10, 10, 8, 2, 0, 0, 0))
        png += chunk(b"IDAT", zlib.compress((b"\0" + b"\xff\xff\xff" * 10) * 10)) + chunk(b"IEND", b"")
        path = Path(self.directory.name) / "diagram.png"
        path.write_bytes(png)
        self.legacy("createDeck", deck="Images")
        deck_id = self.legacy("deckNamesAndIds")["Images"]
        self.engine.invoke("winone.image.save", {"path": str(path), "deckId": str(deck_id), "occlusions": [
            {"ordinal": 1, "shapes": [{"shape": "rect", "properties": [{"name": "left", "value": ".1"},
                {"name": "top", "value": ".2"}, {"name": "width", "value": ".3"}, {"name": "height", "value": ".4"}]}]}]})
        ids = self.legacy("findNotes", query='deck:Images')
        self.assertEqual(len(ids), 1)
        result = self.engine.invoke("winone.image.open", {"noteId": str(ids[0])})
        self.assertEqual(result["occlusions"][0]["shapes"][0]["shape"], "rect")
        self.assertTrue(base64.b64decode(result["imageData"]).startswith(b"\x89PNG"))
        self.assertEqual(len(self.legacy("findCards", query="deck:Images")), 1)

    def test_legacy_batch_note_tags_templates_and_scheduling_roundtrip(self):
        note_id = self.word()
        cards = self.legacy("findCards", query="hello")
        self.legacy("addTags", notes=[note_id], tags="phrase important")
        self.assertIn("important", self.legacy("getNoteTags", note=note_id))
        self.legacy("removeTags", notes=[note_id], tags="phrase")
        self.legacy("suspend", cards=cards)
        self.assertEqual(self.legacy("areSuspended", cards=cards), [True])
        self.legacy("unsuspend", cards=cards)
        self.assertEqual(self.legacy("areSuspended", cards=cards), [False])
        self.legacy("updateModelStyling", model={"name": "WinOne Word", "css": ".card{font-size:24px}"})
        self.assertIn("24px", self.legacy("modelStyling", modelName="WinOne Word")["css"])
        self.legacy("updateNoteFields", note={"id": note_id, "fields": {"Meaning": "updated"}})
        results = self.legacy("multi", actions=[{"action": "notesInfo", "version": 6, "params": {"notes": [note_id]}},
                                             {"action": "version", "version": 6}])
        self.assertEqual(results[0]["result"][0]["fields"]["Meaning"]["value"], "updated")
        self.assertEqual(results[1]["result"], 6)

    def test_json_rpc_operation_records_and_editor_compatibility(self):
        note_id = self.word()
        reply = self.engine.invoke("winone.rpc", {"request": {"jsonrpc": "2.0", "id": "1", "method": "editors.openNote", "params": {"noteId": str(note_id)}}})
        operation = reply["result"]["operationId"]
        status = self.engine.invoke("operations.get", {"operationId": operation})
        self.assertEqual(status["state"], "succeeded")
        self.assertEqual(status["result"]["variant"], "custom")
        self.engine.invoke("winone.editor.close", {"editorId": status["result"]["id"]})
        self.assertEqual(self.legacy("guiAddCards"), 0)
        self.assertTrue(self.legacy("guiAddNoteSetData", note={"fields": {"Word": "world"}}, append=False))

    def test_custom_scheduling_is_scoped_and_uses_official_answer_state(self):
        from anki import frontend_pb2
        self.word()
        self.legacy("guiDeckReview", name="WinOne 生词本")
        state = self.engine.invoke("winone.review")
        response = self.engine.invoke("winone.frontend", {"method": "getSchedulingStatesWithContext", "key": state["mutationKey"]})
        scheduling = frontend_pb2.SchedulingStatesWithContext.FromString(base64.b64decode(response))
        scheduling.states.good.custom_data = '{"winone":1}'
        request = frontend_pb2.SetSchedulingStatesRequest(key=state["mutationKey"], states=scheduling.states)
        payload = base64.b64encode(request.SerializeToString()).decode()
        with self.assertRaisesRegex(Exception, "changed"):
            self.engine.invoke("winone.frontend", {"method": "setSchedulingStates", "key": "old-card", "data": payload})
        self.engine.invoke("winone.frontend", {"method": "setSchedulingStates", "key": state["mutationKey"], "data": payload})
        self.legacy("guiShowAnswer")
        self.legacy("guiAnswerCard", ease=3)
        card = self.engine.col.get_card(int(state["card"]["id"]))
        self.assertIn("winone", card.custom_data)

    def test_windows_sync_credentials_are_encrypted_without_network(self):
        import os
        if os.name != "nt":
            self.skipTest("Windows credential protection")
        from lifecycle import protect
        value = b"isolated-test-credential"
        encrypted = protect(value)
        self.assertNotIn(value, encrypted)
        self.assertEqual(protect(encrypted, decrypt=True), value)

    def test_legacy_batch_is_preflighted_before_any_mutation(self):
        with self.assertRaisesRegex(Exception,"last executable"):
            self.legacy("multi",actions=[{"action":"createDeck","params":{"deck":"must not exist"}},
                                        {"action":"guiExitAnki"},{"action":"version"}])
        self.assertNotIn("must not exist",self.legacy("deckNames"))
        nested={"action":"version"}
        for _ in range(18):
            nested={"action":"multi","params":{"actions":[nested]}}
        with self.assertRaisesRegex(Exception,"nesting limit"):
            self.legacy("multi",actions=[nested])

    def test_empty_default_counts_and_deck_delete_confirmation(self):
        self.word()
        counts=self.engine.invoke("decks.getCounts",{"deck":{"name":"Default"}})
        self.assertEqual(counts["total"],0)
        with self.assertRaisesRegex(Exception,"not valid"):
            self.engine.invoke("decks.delete",{"decks":[{"name":"WinOne 生词本"}],"deleteCards":False})
        self.assertEqual(len(self.legacy("findNotes",query="hello")),1)

    def test_csv_preview_mapping_and_import(self):
        self.word()
        source=Path(self.directory.name)/"words.txt"
        source.write_text("#separator:tab\n#html:false\n#notetype:WinOne Word\n#deck:WinOne 生词本\nworld\t世界\n",encoding="utf-8")
        preview=self.engine.invoke("winone.import.preview",{"path":str(source)})
        self.assertIn("metadata",preview)
        self.engine.invoke("winone.import.run",{"path":str(source),"metadata":preview["metadata"]})
        self.assertEqual(len(self.legacy("findNotes",query="world")),1)


if __name__ == "__main__":
    unittest.main()
