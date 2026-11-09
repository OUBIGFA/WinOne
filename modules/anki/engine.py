"""WinOne-owned Anki collection, official scheduler and AnkiConnect adapter.

All calls are serialized by the worker. Collections and media are always
inside its selected profile; opening the engine never starts HTTP or sync.
"""
import base64
import copy
import json
import os
import uuid
from pathlib import Path
from types import SimpleNamespace

from upstream import load

SOURCE = Path(os.environ.get("WINONE_ANKICONNECT_SOURCE", Path(__file__).resolve().parents[2] / "third_party/anki-connect/plugin"))
LEGACY_METHODS = load(SOURCE)

from anki.collection import Collection
from anki import lang
from winone_ankiconnect.contract import SCHEMA, LEGACY_TARGETS
from winone_ankiconnect.legacy import Accepted, Legacy
from winone_ankiconnect.protocol import ApiError, Deferred, Dispatcher, Operations, completed
from winone_ankiconnect.runtime import Runtime, GUI_METHODS
from winone_ankiconnect.services import Services


class NativeServices(Services):
    # Anki omits an unused Default deck from the due tree. Its public counts
    # still need a truthful zero result in both native and compatibility APIs.
    def decks_getCounts(self, deck, includeChildren=True):
        try:
            return super().decks_getCounts(deck, includeChildren)
        except ApiError as error:
            value=self.resolve(deck,"deck")
            if error.kind!="notFound" or self.col.db.scalar("select count(*) from cards where did=?",value["id"]):
                raise
            return {"deckId":str(value["id"]),"new":0,"learning":0,"review":0,"total":0}


def resolved(value):
    """Resolve synchronous work while preserving actual upstream exceptions."""
    if isinstance(value, Accepted):
        return value.project(resolved(value.pending))
    if isinstance(value, Deferred):
        if not value.done:
            raise RuntimeError("Operation has not completed")
        if value.error:
            raise value.error
        return resolved(value.value)
    return value


class NativeRuntime(Runtime):
    def __init__(self, owner):
        self.owner = owner
        self.schedule = lambda callback: callback()
        self.operations = Operations(self.schedule)
        self.dispatcher = Dispatcher(self)
        self._collection = owner.col
        self._session = uuid.uuid4().hex
        self.profile_name = "WinOne"
        self.tokens, self.cursors = {}, {}
        self.generation = 0
        self.full_reset_required = self.lifecycle_active = False
        self.active_reviewer_card = None
        self.gui = NativeGui(self)

    @property
    def session(self):
        return self._session

    @property
    def mw(self):
        return self.owner

    def service(self):
        return NativeServices(self,self._collection,self.session)

    def finish_changes(self, service, initiator=None):
        if service.changes:
            self.owner.events.append({"type": "collection.changed"})

    def background(self, work, expected=None, after=None, initiator=None):
        try:
            col, session = self.capture(expected)
            service = NativeServices(self, col, session)
            from winone_ankiconnect.media import MediaTransaction
            MediaTransaction.recover(service)
            value = work(service)
            return completed(after(value) if after else value)
        except Exception as error:
            return completed(error=error)
        finally:
            if "service" in locals():
                self.finish_changes(service)

    def describe(self):
        methods = []
        for name in SCHEMA["x-methods"]:
            reason = self.capability_reason(name)
            methods.append({"name": name, "available": reason is None, "reason": reason})
        return {"apiMajor": 1, "ankiVersion": "26.09.4", "schema": copy.deepcopy(SCHEMA),
                "methods": methods, "limits": copy.deepcopy(SCHEMA["x-limits"])}

    def invoke(self, method, params):
        if method not in ("api.describe", "operations.get", "operations.cancel"):
            self.require_capability(method)
        if method in SCHEMA["x-methods"] and SCHEMA["x-methods"][method]["effect"] in ("write", "advancedWrite"):
            self.gui.guard(method, params)
        if method == "api.describe":
            return completed(self.describe())
        if method == "operations.get":
            return completed(self.operations.get(params["operationId"]))
        if method == "operations.cancel":
            return completed(self.operations.cancel(params["operationId"]))
        if method == "api.execute":
            for command in params["commands"]:
                if command["method"] in ("api.execute", "application.exit", "profiles.switch"):
                    raise ApiError("invalidParams", "This command is not allowed inside api.execute")
                self.dispatcher.params(command["method"], command.get("params", {}))
            return completed(self.operations.submit(method, self.session,
                        lambda key: self.execute_commands(params["commands"], self.session, key)))
        if method == "access.requestPermission":
            return self.permission({"allowed": True})
        if SCHEMA["x-methods"][method]["scope"] == "collection":
            self.capture(params.get("session"))
        if SCHEMA["x-methods"][method]["completion"] == "operation":
            return completed(self.operations.submit(method, self.session, lambda key: self.start(method, params)))
        return self.start(method, params)

    def permission(self, context):
        return completed(self.owner.permission(context.get("origin", ""), context.get("allowed", False)))

    def start(self, method, params):
        if method in GUI_METHODS:
            value = self.gui.execute(method, params)
            return value if isinstance(value, Deferred) else completed(value)
        if method == "media.store":
            source = params["source"]
            limit = SCHEMA["x-limits"]["maxMediaBytes"]
            if "data" in source:
                data = base64.b64decode(source["data"], validate=True)
            elif "path" in source:
                with Path(source["path"]).open("rb") as file:
                    data = file.read(limit + 1)
            else:
                from urllib.request import urlopen
                from urllib.parse import urlparse
                if urlparse(source["url"]).scheme not in ("https", "http"):
                    raise ApiError("invalidParams", "Media URL must use HTTP or HTTPS")
                with urlopen(source["url"], timeout=10) as response:
                    data = response.read(limit + 1)
            if len(data) > limit:
                raise ApiError("invalidParams", "Media exceeds the maximum size")
            return self.background(lambda s: s.store_bytes(params["filename"], data,
                        params.get("collision", "rename"), params.get("skipIfHash")))
        return self.background(lambda s: s.execute(method, params), params.get("session"))


class NativeGui:
    """Window actions become events consumed by the MyGo frontend."""
    def __init__(self, runtime):
        self.runtime = runtime
        self.editors = {}
        self.editor_owners = {}
        self.view = "deckBrowser"
        self.query = ""
        self.selected_cards = []
        self.card = None
        self.side = "question"
        self.prediction = None

    def service(self):
        return self.runtime.service()

    def execute(self, method, params, operation_id=None):
        params = {k: v for k, v in params.items() if k != "session"}
        fn = getattr(self, method.replace(".", "_"), None)
        if fn is None:
            raise ApiError("unsupportedCapability", "Native window action is not implemented: " + method)
        return fn(**params)

    def guard(self, method, params):
        if method.startswith(("cards.", "scheduling.")):
            return
        ids = params.get("ids", params.get("noteIds", [params.get("id")])) or []
        for record in self.editors.values():
            affected = method.startswith(("noteTypes.", "collection.", "packages.", "advanced.", "profiles.")) or method in ("application.importFile", "application.exit", "backend.write")
            if method.startswith(("notes.", "tags.")):
                affected = bool(record["noteId"]) and (record["noteId"] in ids or params.get("allNotes", False))
            if affected:
                raise ApiError("editorConflict", "Save or close the affected note editor first")

    def navigate(self, page, **state):
        self.view = page
        self.runtime.owner.events.append({"type": "navigate", "page": page, **state})

    def profiles_list(self):
        return [{"name": name} for name in self.runtime.owner.profiles()]

    def profiles_getActive(self):
        return {"name": self.runtime.profile_name}

    def profiles_switch(self, name):
        self.guard("profiles.switch", {})
        self.runtime.owner.switch_profile(name)
        return self.service().collection_get()

    def browser(self):
        return self if self.view == "browser" else None

    def browser_open(self, query=None, sort=None):
        self.query = query or ""
        self.navigate("browser", query=self.query, sort=sort)
        return self.browser_getSelection()

    def browser_getSelection(self):
        col = self.runtime._collection
        return {"open": self.view == "browser", "selectedCardIds": [str(x) for x in self.selected_cards],
                "selectedNoteIds": [str(x) for x in dict.fromkeys(col.get_card(cid).nid for cid in self.selected_cards)]}

    def browser_select(self, cardIds=None, noteIds=None):
        if cardIds is None:
            cardIds = [str(cid) for id in noteIds for cid in self.service().note(id).card_ids()]
        for id in cardIds:
            self.service().card(id)
        self.selected_cards = [int(x) for x in cardIds]
        self.navigate("browser", query=self.query, cardIds=cardIds)
        return self.browser_getSelection()

    def application_navigate(self, view, deck=None):
        if deck:
            value = self.service().resolve(deck, "deck")
            self.runtime._collection.decks.select(value["id"])
        self.navigate(view, deck=deck)
        self.card = None
        if view == "review":
            self.next_card()
        return {"changed": True}

    def next_card(self):
        col = self.runtime._collection
        queued = col.sched.get_queued_cards()
        self.card = col.get_card(queued.cards[0].card.id) if queued.cards else None
        self.states = queued.cards[0].states if queued.cards else None
        self.scheduling_context = queued.cards[0].context if queued.cards else None
        self.mutation_key = uuid.uuid4().hex
        self.custom_applied = False
        self.card_snapshot = self.card._to_backend_card().SerializeToString() if self.card else None
        if self.card:
            self.card.start_timer()
        self.side = "question"
        self.prediction = self.service().scheduling_predict(str(self.card.id)) if self.card else None

    def reviewer(self):
        return self if self.view == "review" else None

    def reviewer_get(self):
        if self.view != "review" or not self.card:
            return {"active": False, "side": "none", "card": None, "rendered": None, "answers": [], "stateToken": None, "source": "reviewer"}
        return {"active": True, "side": self.side,
                "card": self.service().card_record(self.card),
                "stateToken": self.prediction["stateToken"], "answers": self.prediction["answers"],
                "rendered": self.service().cards_render(str(self.card.id)), "source": "reviewer"}

    def check_review(self, stateToken):
        if not self.card or not self.prediction or stateToken != self.prediction["stateToken"]:
            raise ApiError("conflict", "Review card changed")
        current=self.runtime._collection.get_card(self.card.id)
        if current._to_backend_card().SerializeToString()!=self.card_snapshot:
            raise ApiError("conflict","Review card was edited; refresh before answering")

    def reviewer_showAnswer(self, stateToken):
        self.check_review(stateToken)
        self.side = "answer"
        self.runtime.owner.events.append({"type": "review.changed"})
        return {"changed": True}

    def reviewer_showQuestion(self, stateToken):
        self.check_review(stateToken)
        self.side = "question"
        self.runtime.owner.events.append({"type": "review.changed"})
        return {"changed": True}

    def reviewer_startTimer(self, stateToken):
        self.check_review(stateToken)
        self.card.start_timer()
        return {"changed": True}

    def reviewer_answer(self, stateToken, rating):
        self.check_review(stateToken)
        if self.side != "answer":
            raise ApiError("invalidState", "Show the answer before rating")
        from anki.scheduler.v3 import CardAnswer
        service = self.service()
        native_rating = {"again": CardAnswer.AGAIN,"hard": CardAnswer.HARD,"good": CardAnswer.GOOD,"easy": CardAnswer.EASY}[rating]
        answer = service.col.sched.build_answer(card=self.card,states=self.states,rating=native_rating)
        card_id = str(self.card.id)
        service.change(service.col.sched.answer_card(answer))
        latest = service.col.db.scalar("select max(id) from revlog where cid=?",self.card.id)
        result = {"cardId":card_id,"reviewId":str(latest) if latest else None,"scheduling":service.scheduling_get(card_id)}
        self.runtime.finish_changes(service)
        self.next_card()
        self.runtime.owner.events.append({"type": "review.changed"})
        return result

    def collection_refresh(self, target="windows"):
        self.runtime.invalidate_predictions()
        self.card = None
        if self.view == "review":
            self.next_card()
        return {"changed": True}

    def reviewer_playAudio(self, stateToken):
        self.check_review(stateToken)
        self.runtime.owner.events.append({"type": "audio", "side": self.side, "cardId": str(self.card.id)})
        return {"changed": True}

    def editors_openNote(self, noteId):
        note = self.service().notes_get(noteId)
        return self.open_editor("note", note["fields"], note["tags"], note["noteType"], None, noteId)

    def editors_openAdd(self, note=None):
        if note:
            native, deck = self.service().prepare_note(note)
        else:
            native = self.runtime._collection.new_note(self.runtime._collection.models.current())
            deck = self.runtime._collection.decks.current()
        return self.open_editor("add", dict(native.items()), list(native.tags),
                                self.service().summary(native.note_type()), self.service().summary(deck), None)

    def add_dialog(self):
        for record in self.editors.values():
            if record["kind"] == "add":
                model = self.service().resolve(record["noteType"], "noteType")
                note = self.runtime._collection.new_note(model)
                for key, value in record["fields"].items():
                    note[key] = value
                note.tags = list(record["tags"])
                return SimpleNamespace(identifier=record["id"], editor=SimpleNamespace(note=note))
        return None

    def register(self, dialog, kind):
        if dialog.identifier not in self.editors or self.editors[dialog.identifier]["kind"] != kind:
            raise ApiError("notFound", "Editor closed")
        return dialog.identifier

    def editor_record(self, dialog, kind):
        return copy.deepcopy(self.editors[self.register(dialog, kind)])

    def open_editor(self, kind, fields, tags, note_type, deck, note_id):
        record = {"id": uuid.uuid4().hex, "kind": kind, "variant": "custom", "noteId": note_id,
                  "noteType": note_type, "deck": deck, "fields": fields, "tags": tags,
                  "dirty": False, "session": self.runtime.session}
        self.editors[record["id"]] = record
        self.editor_owners[record["id"]] = self.runtime.owner.caller
        self.navigate("editor", editorId=record["id"])
        return copy.deepcopy(record)

    def editors_updateDraft(self, editorId, patch, mode="replace"):
        record = self.editors.get(editorId)
        if record is None:
            raise ApiError("notFound", "Editor closed")
        if "noteType" in patch:
            model = self.service().resolve(patch["noteType"], "noteType")
            record["noteType"] = self.service().summary(model)
            record["fields"] = {f["name"]: record["fields"].get(f["name"], "") for f in model["flds"]}
        if "deck" in patch:
            record["deck"] = self.service().summary(self.service().resolve(patch["deck"], "deck"))
        for name, value in patch.get("fields", {}).items():
            if name not in record["fields"]:
                raise ApiError("validationFailed", "Unknown field: " + name)
            record["fields"][name] = record["fields"][name] + value if mode == "append" else value
        if "tags" in patch:
            record["tags"] = list(dict.fromkeys(record["tags"] + patch["tags"])) if mode == "append" else patch["tags"]
        record["dirty"] = True
        self.runtime.owner.events.append({"type":"editor.changed","editorId":editorId})
        return copy.deepcopy(record)

    def save_editor(self, editorId):
        record = self.editors.get(editorId)
        if record is None:
            raise ApiError("notFound", "Editor closed")
        service = self.service()
        if record["kind"] == "add":
            result = service.notes_create(noteType=record["noteType"], deck=record["deck"], fields=record["fields"], tags=record["tags"])
        else:
            result = service.notes_update(record["noteId"], {"fields": record["fields"], "tags": record["tags"]})
        del self.editors[editorId]
        self.editor_owners.pop(editorId, None)
        self.runtime.finish_changes(service)
        return result

    def application_exit(self):
        self.guard("application.exit", {})
        self.runtime.owner.exit_requested = True
        self.runtime.owner.events.append({"type": "closeView"})
        return {"accepted": True}

    def application_importFile(self, path=None):
        self.guard("application.importFile", {})
        self.navigate("import", path=path)
        # The operation completes when the native import dialog accepts or
        # cancels it. Selecting a file never silently replaces a collection.
        self.import_completion = Deferred()
        return self.import_completion

    def collection_sync(self, includeMedia=True):
        from lifecycle import sync
        return sync(self.runtime.owner, includeMedia)


class Engine:
    def __init__(self, directory: Path):
        # Qt normally initializes Anki's shared translation/backend helpers.
        # The headless host must do this before field validation or rendering.
        if lang.current_i18n is None:
            lang.set_lang("en_US")
        self.root = directory.resolve()
        self.root.mkdir(parents=True, exist_ok=True)
        record = self.root / "profiles.json"
        self.profile_record = json.loads(record.read_text(encoding="utf-8")) if record.exists() else {"active": "WinOne", "profiles": {"WinOne": "."}}
        self.directory = (self.root / self.profile_record["profiles"][self.profile_record["active"]]).resolve()
        if not self.directory.is_relative_to(self.root):
            raise ValueError("Profile is outside the learning data directory")
        self.directory.mkdir(parents=True, exist_ok=True)
        self.events = []
        self.caller = 0
        self.exit_requested = False
        self.pm = self
        fresh = not (self.directory / "collection.anki2").exists()
        self.col = Collection(str(self.directory / "collection.anki2"))
        if fresh:
            self.col.set_config("fsrs", True)
        self.runtime = NativeRuntime(self)
        self.runtime.profile_name = self.profile_record["active"]

    def profiles(self):
        return list(self.profile_record["profiles"])

    def set_owner(self, pid):
        self.caller = pid
        if os.name != "nt":
            return
        import ctypes
        from ctypes import wintypes
        kernel = ctypes.WinDLL("kernel32", use_last_error=True)
        kernel.OpenProcess.argtypes = [wintypes.DWORD, wintypes.BOOL, wintypes.DWORD]
        kernel.OpenProcess.restype = wintypes.HANDLE
        kernel.WaitForSingleObject.argtypes = [wintypes.HANDLE, wintypes.DWORD]
        kernel.WaitForSingleObject.restype = wintypes.DWORD
        kernel.CloseHandle.argtypes = [wintypes.HANDLE]
        for editor, owner in list(self.runtime.gui.editor_owners.items()):
            if not owner:
                continue
            handle = kernel.OpenProcess(0x00100000, False, owner)
            if not handle:
                # Access denied is not proof of exit. Invalid PID is.
                gone = ctypes.get_last_error() == 87
            else:
                gone = kernel.WaitForSingleObject(handle, 0) == 0
                kernel.CloseHandle(handle)
            if gone:
                self.runtime.gui.editors.pop(editor, None)
                self.runtime.gui.editor_owners.pop(editor, None)

    def window(self):
        return self

    def sync_auth(self):
        from lifecycle import load_auth
        return load_auth(self)

    def switch_profile(self, name):
        from lifecycle import atomic_json
        if name not in self.profile_record["profiles"]:
            raise ApiError("notFound", "Profile not found")
        target = (self.root / self.profile_record["profiles"][name]).resolve()
        if not target.is_relative_to(self.root):
            raise ValueError("Profile is outside the learning data directory")
        if target == self.directory:
            return
        target.mkdir(parents=True, exist_ok=True)
        fresh = not (target / "collection.anki2").exists()
        next_collection = Collection(str(target / "collection.anki2"))
        if fresh:
            next_collection.set_config("fsrs", True)
        self.close_collection()
        self.col, self.directory = next_collection, target
        self.profile_record["active"] = name
        atomic_json(self.root / "profiles.json", self.profile_record)
        self.runtime._collection = self.col
        self.runtime._session = uuid.uuid4().hex
        self.runtime.profile_name = name
        self.runtime.tokens.clear()
        self.runtime.cursors.clear()
        self.runtime.gui.card = None
        self.events.append({"type": "collection.changed"})

    def close(self):
        if self.col:
            self.close_collection()
            self.col = None

    def close_collection(self):
        folder=self.directory/"backups"
        folder.mkdir(exist_ok=True)
        try:
            self.col.create_backup(backup_folder=str(folder),force=False,wait_for_completion=True)
        finally:
            self.col.close()

    def take_events(self):
        events, self.events = self.events, []
        return events

    def reviewer(self):
        return self.runtime.gui.reviewer()

    def legacy_actions(self):
        return list(LEGACY_METHODS)

    def handler(self, request):
        from winone_ankiconnect.legacy import legacy_request
        from winone_ankiconnect.web import format_success_reply, format_exception_reply
        version = 4
        try:
            request = legacy_request(request)
            version = request["version"]
            return format_success_reply(version, self.legacy(request["action"], request.get("params", {})))
        except Exception as error:
            return format_exception_reply(version, error)

    def permission(self, origin, allowed):
        return {"permission": "granted" if allowed else "denied", "requireApiKey": True}

    def logEvent(self, name, value):
        # Upstream RPC logging contains only method/id/timing. Keep it bounded
        # and in memory; no selected text or credentials enter diagnostic files.
        if not hasattr(self, "audit"):
            from collections import deque
            self.audit = deque(maxlen=100)
        self.audit.append((name, value))

    def legacy(self, action, params=None):
        if action not in LEGACY_METHODS:
            raise ApiError("methodNotFound", "Unknown AnkiConnect action: " + action)
        if action == "sync" and self.sync_auth() is None:
            raise ApiError("authenticationRequired", "Sync requires explicit user sign-in first")
        names, defaults = LEGACY_METHODS[action]
        supplied = {key: value for key, value in (params or {}).items()
                    if key in names and not (value is None and key in defaults)}
        values = {**copy.deepcopy(defaults), **supplied}
        if set(names) - set(values):
            raise ApiError("invalidParams", "Missing action parameters")
        if action=="multi":
            self.validate_multi(values["actions"])
        guarded = dict(values)
        note = values.get("note")
        if isinstance(note, dict):
            guarded["id"] = str(note.get("id"))
        elif note is not None:
            guarded["id"] = str(note)
        guarded["ids"] = [str(id) for id in (values.get("notes") or [guarded.get("id")])]
        if action == "replaceTagsInAllNotes":
            guarded["allNotes"] = True
        for target in LEGACY_TARGETS.get(action, []):
            if target in SCHEMA["x-methods"] and SCHEMA["x-methods"][target]["effect"] in ("write", "advancedWrite"):
                self.runtime.gui.guard(target, guarded)
        service = self.runtime.service()
        try:
            value = Legacy(self, service).call(action, values)
            if isinstance(value, Accepted) and not value.pending.done:
                return value.value
            return resolved(value)
        finally:
            self.runtime.finish_changes(service)

    def validate_multi(self, actions, depth=0, state=None):
        from winone_ankiconnect.legacy import legacy_request
        if state is None:
            state={"exit":False}
        if depth>16:
            raise ApiError("invalidParams","multi nesting limit exceeded")
        if not isinstance(actions,list):
            raise ApiError("invalidParams","multi actions must be an array")
        for item in actions:
            if not isinstance(item,dict):
                raise ApiError("invalidParams","multi actions must be request objects")
            try:
                item=legacy_request(item)
                name=item.get("action","")
                if name not in LEGACY_METHODS:
                    continue
            except ApiError:
                continue
            if name=="multi":
                self.validate_multi(item["params"].get("actions"),depth+1,state)
            else:
                if state["exit"]:
                    raise ApiError("invalidParams","guiExitAnki must be the last executable multi action")
                state["exit"]=name=="guiExitAnki"

    def invoke(self, method, params=None):
        if method.startswith("winone."):
            from native import invoke
            return invoke(self, method, params or {})
        values = self.runtime.dispatcher.params(method, params or {})
        result = resolved(self.runtime.invoke(method, values))
        if SCHEMA["x-methods"][method]["completion"] == "operation" and isinstance(result, dict) and result.get("operationId") in self.runtime.operations.completions:
            completion = self.runtime.operations.completions[result["operationId"]]
            if completion.done:
                return resolved(completion)
        return result
