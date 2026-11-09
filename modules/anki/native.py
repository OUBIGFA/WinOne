"""Task-shaped reads for the native UI; mutations still use official services."""


def invoke(engine, method, params):
    if method=="winone.import.cancel":
        pending=getattr(engine.runtime.gui,"import_completion",None)
        if pending is not None:
            from winone_ankiconnect.protocol import ApiError
            pending.resolve(error=ApiError("cancelled","Import cancelled"))
        return {"cancelled":True}
    if method == "winone.frontend":
        from frontend import invoke
        return invoke(engine, params)
    if method.startswith("winone.image."):
        from occlusion import invoke
        return invoke(engine, method, params)
    if method == "winone.rpc":
        from engine import resolved
        return resolved(engine.runtime.dispatcher.request(params["request"], {"allowed": True}))
    if method == "winone.legacy":
        return engine.handler(params["request"])
    if method == "winone.tools":
        from tools import describe
        return describe()
    if method.startswith(("winone.import.", "winone.export")):
        from files import invoke
        return invoke(engine, method, params)
    if method.startswith(("winone.sync.", "winone.profiles.")):
        from lifecycle import invoke
        return invoke(engine, method, params)
    service = engine.runtime.service()
    if method == "winone.overview":
        decks = service.decks_list()
        for deck in decks:
            deck["counts"] = service.decks_getCounts({"id": deck["id"]})
        undo = engine.col.undo_status()
        return {"decks": decks, "noteTypes": service.noteTypes_list(),
                "profiles": engine.profiles(), "activeProfile": engine.runtime.profile_name,
                "fsrs": engine.col.get_config("fsrs", False),
                "undo": undo.undo, "redo": undo.redo}
    if method == "winone.browser":
        engine.runtime.gui.view="browser"
        engine.runtime.gui.query=params.get("query", "")
        found = service.notes_search(query=params.get("query", ""), limit=params.get("limit", 50), cursor=params.get("cursor"))
        return {"notes": [service.notes_get(id) for id in found["ids"]], "nextCursor": found["nextCursor"]}
    if method == "winone.review":
        from rendering import review
        if engine.runtime.gui.view!="review":
            engine.runtime.gui.view="review"
            engine.runtime.gui.next_card()
        return review(engine, params.get("typed", ""))
    if method == "winone.fsrs":
        if type(params.get("enabled")) is not bool:
            raise ValueError("FSRS enabled must be boolean")
        engine.col.set_config("fsrs", params["enabled"], undoable=True)
        return {"enabled": engine.col.get_config("fsrs", False)}
    if method == "winone.redo":
        engine.col.redo()
        return {"changed": True}
    if method == "winone.backup":
        folder = engine.directory / "backups"
        folder.mkdir(exist_ok=True)
        created = engine.col.create_backup(backup_folder=str(folder), force=True, wait_for_completion=True)
        return {"created": created, "files": [p.name for p in sorted(folder.glob("*.colpkg"))]}
    if method == "winone.events":
        return engine.take_events()
    if method == "winone.review.action":
        gui=engine.runtime.gui
        gui.check_review(params["stateToken"])
        card=gui.card
        action=params["action"]
        if action=="buryCard":engine.col.sched.bury_cards([card.id])
        elif action=="buryNote":engine.col.sched.bury_notes([card.nid])
        elif action=="suspendCard":engine.col.sched.suspend_cards([card.id])
        elif action=="suspendNote":engine.col.sched.suspend_cards(engine.col.get_note(card.nid).card_ids())
        elif action=="mark":
            note=card.note()
            note.remove_tag("marked") if note.has_tag("marked") else note.add_tag("marked")
            engine.col.update_note(note)
        elif action=="flag":engine.col.set_user_flag_for_cards(int(params["flag"]),[card.id])
        elif action=="reset":engine.col.sched.schedule_cards_as_new([card.id],restore_position=True)
        elif action=="deleteNote":engine.col.remove_notes([card.nid])
        else:raise ValueError("Unknown review action")
        if action not in ("mark","flag"):
            gui.next_card()
        elif action=="flag":
            card.load()
            gui.card_snapshot=card._to_backend_card().SerializeToString()
        engine.events.append({"type":"review.changed"})
        return {"changed":True}
    if method == "winone.editor.save":
        return engine.runtime.gui.save_editor(**params)
    if method == "winone.editor.get":
        record = engine.runtime.gui.editors.get(params["editorId"])
        if record is None:
            raise ValueError("Editor has closed")
        engine.runtime.gui.editor_owners[params["editorId"]] = engine.caller
        return record
    if method == "winone.idle":
        return {"idle": not engine.runtime.gui.editors}
    if method == "winone.editor.close":
        engine.runtime.gui.editors.pop(params["editorId"], None)
        engine.runtime.gui.editor_owners.pop(params["editorId"], None)
        return {"closed": True}
    if method == "winone.backend.catalog":
        from backend import catalog
        return catalog()
    if method == "winone.backend.call":
        from backend import call
        return call(engine, params["method"], params.get("params", {}))
    raise ValueError("Unknown native learning operation: " + method)
