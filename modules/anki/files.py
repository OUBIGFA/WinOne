"""Native import/export over the official Anki format implementations."""
from pathlib import Path
from google.protobuf.json_format import MessageToDict, ParseDict
from anki import import_export_pb2


def invoke(engine, method, params):
    col = engine.col
    path = str(Path(params.get("path", ".")).resolve())
    if method == "winone.import.preview":
        if not Path(path).is_file():
            raise ValueError("Import file does not exist")
        kind = Path(path).suffix.lower()
        result = {"path": path, "kind": kind, "replacesCollection": kind == ".colpkg"}
        if kind in (".csv", ".tsv", ".txt"):
            result["metadata"] = MessageToDict(col.get_csv_metadata(path, None), always_print_fields_with_no_presence=True)
        elif kind not in (".apkg", ".zip", ".colpkg", ".json", ".anki-json", ".db"):
            raise ValueError("Unsupported import format")
        return result
    if method == "winone.import.schema":
        from backend import message_schema
        return message_schema(import_export_pb2.CsvMetadata.DESCRIPTOR)
    if method == "winone.import.run":
        engine.runtime.gui.guard("application.importFile", {})
        kind = Path(path).suffix.lower()
        result = {"completed": True}
        if kind == ".colpkg":
            if params.get("replaceConfirmed") is not True:
                raise ValueError("Confirm replacing the current collection before importing")
            folder = engine.directory / "backups"
            folder.mkdir(exist_ok=True)
            col.create_backup(backup_folder=str(folder), force=True, wait_for_completion=True)
            col.close()
            try:
                col._backend.import_collection_package(col_path=col.path, backup_path=path,
                        media_folder=str(engine.directory / "collection.media"), media_db=str(engine.directory / "collection.media.db2"))
            finally:
                col.reopen()
        elif kind in (".apkg", ".zip"):
            options = params.get("options", {"withScheduling": True, "withDeckConfigs": True})
            result["log"] = MessageToDict(col.import_anki_package(ParseDict({"packagePath": path, "options": options}, import_export_pb2.ImportAnkiPackageRequest())))
        elif kind in (".json", ".anki-json"):
            result["log"] = MessageToDict(col.import_json_file(path))
        elif kind in (".csv", ".tsv", ".txt"):
            metadata = params.get("metadata") or MessageToDict(col.get_csv_metadata(path, None))
            result["log"] = MessageToDict(col.import_csv(ParseDict({"path": path, "metadata": metadata}, import_export_pb2.ImportCsvRequest())))
        elif kind == ".db":
            from anki.foreign_data.mnemosyne import serialize
            result["log"] = MessageToDict(col.import_json_string(serialize(path, col.decks.current()["id"])))
        else:
            raise ValueError("Unsupported import format")
        engine.runtime.invalidate_predictions()
        pending = getattr(engine.runtime.gui, "import_completion", None)
        if pending is not None:
            pending.resolve({"completed": True})
        return result
    if method == "winone.export":
        kind = params.get("kind", Path(path).suffix.lower())
        if kind == ".colpkg":
            try:
                col.export_collection_package(path, params.get("includeMedia", True), params.get("legacy", False))
            finally:
                col.reopen()
        elif kind == ".apkg":
            from anki.collection import DeckIdLimit
            limit = DeckIdLimit(deck_id=int(params["deckId"])) if params.get("deckId") else None
            col.export_anki_package(out_path=path, options=import_export_pb2.ExportAnkiPackageOptions(
                with_scheduling=params.get("includeScheduling", True), with_media=params.get("includeMedia", True),
                with_deck_configs=True, legacy=params.get("legacy", False)), limit=limit)
        elif kind == ".txt":
            from anki.collection import DeckIdLimit
            limit = DeckIdLimit(deck_id=int(params["deckId"])) if params.get("deckId") else None
            col.export_note_csv(out_path=path, limit=limit, with_html=params.get("includeHTML", True),
                                with_tags=True, with_deck=True, with_notetype=True, with_guid=True)
        else:
            raise ValueError("Unsupported export format")
        return {"path": path}
    raise ValueError("Unknown file operation")
