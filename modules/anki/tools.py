"""Complete structured native forms for Anki's less frequent management tasks."""
from winone_ankiconnect.contract import SCHEMA
from backend import catalog

# Frequent reading/review/editor actions have dedicated screens. The remaining
# original operations retain all their fields through the upstream schema.
def describe():
    groups = {"decks": "decks", "deckPresets": "scheduling", "scheduling": "scheduling",
              "notes": "notes", "cards": "cards", "tags": "tags", "noteTypes": "types",
              "media": "media", "reviews": "statistics", "statistics": "statistics",
              "packages": "files", "collection": "collection", "advanced": "advanced"}
    actions = []
    for name, spec in SCHEMA["x-methods"].items():
        prefix = name.split(".")[0]
        if prefix not in groups:
            continue
        actions.append({"id": name, "group": groups[prefix], "schema": spec["params"], "backend": False,
                        "mutates": spec["effect"] not in ("read", "readOnly")})
    for name, schema in catalog().items():
        # Transport, collection lifetime and remote account protocols have
        # dedicated host flows. They are not independent study operations.
        if name.startswith(("sync_", "abort_", "full_upload", "ankihub_", "ankiweb_", "github_")) or name in {
                "open_collection", "close_collection", "import_collection_package", "export_collection_package", "add_media_from_url"}:
            continue
        actions.append({"id": name, "group": "advanced", "schema": schema, "backend": True,
                        "mutates": not name.startswith(("get_", "find_", "search_", "describe_", "check_", "render_"))})
    return {"actions": actions, "definitions": SCHEMA}
