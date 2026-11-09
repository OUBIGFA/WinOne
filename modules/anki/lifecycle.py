"""Profiles, explicit synchronization and native import lifecycle."""
import ctypes
import json
import os
import uuid
from ctypes import wintypes
from anki.sync_pb2 import SyncAuth


def atomic_json(path, value):
    temporary = path.with_suffix(path.suffix + ".tmp")
    temporary.write_text(json.dumps(value, ensure_ascii=False), encoding="utf-8")
    os.replace(temporary, path)


def protect(data, decrypt=False):
    """Bind sync credentials to the current Windows user, without UI prompts."""
    if os.name != "nt":
        raise RuntimeError("Secure credentials require the Windows credential protector")
    class Blob(ctypes.Structure):
        _fields_ = [("size", wintypes.DWORD), ("data", ctypes.POINTER(ctypes.c_ubyte))]
    buffer = (ctypes.c_ubyte * len(data)).from_buffer_copy(data)
    source, target = Blob(len(data), buffer), Blob()
    library = ctypes.WinDLL("crypt32", use_last_error=True)
    operation = library.CryptUnprotectData if decrypt else library.CryptProtectData
    operation.argtypes = [ctypes.POINTER(Blob), ctypes.c_void_p, ctypes.c_void_p, ctypes.c_void_p, ctypes.c_void_p, wintypes.DWORD, ctypes.POINTER(Blob)]
    operation.restype = wintypes.BOOL
    # CRYPTPROTECT_UI_FORBIDDEN ensures no credential or elevation prompt.
    if not operation(ctypes.byref(source), None, None, None, None, 1, ctypes.byref(target)):
        raise ctypes.WinError(ctypes.get_last_error())
    try:
        return ctypes.string_at(target.data, target.size)
    finally:
        free = ctypes.WinDLL("kernel32").LocalFree
        free.argtypes = [ctypes.c_void_p]
        free.restype = ctypes.c_void_p
        free(target.data)


def load_auth(engine):
    path = engine.directory / "sync-auth.dat"
    return SyncAuth.FromString(protect(path.read_bytes(), decrypt=True)) if path.exists() else None


def store_auth(engine, auth):
    path = engine.directory / "sync-auth.dat"
    temporary = path.with_suffix(".tmp")
    temporary.write_bytes(protect(auth.SerializeToString()))
    os.replace(temporary, path)


def sync(engine, include_media=True, direction=None):
    auth = load_auth(engine)
    if auth is None:
        raise ValueError("Synchronization requires explicit sign-in first")
    engine.runtime.gui.guard("collection.sync", {})
    from anki import sync_pb2
    if direction:
        if direction not in ("upload", "download"):
            raise ValueError("Select upload or download")
        (engine.directory / "backups").mkdir(exist_ok=True)
        engine.col.create_backup(backup_folder=str(engine.directory / "backups"), force=True, wait_for_completion=True)
        engine.col.close_for_full_sync()
        try:
            engine.col.full_upload_or_download(auth=auth, upload=direction == "upload", server_usn=None)
        finally:
            engine.col.reopen(after_full_sync=True)
        if include_media:
            engine.col.sync_media(auth)
        return {"collectionComplete": True, "mediaComplete": include_media}
    result = engine.col.sync_collection(auth, False)
    if result.new_endpoint:
        auth.endpoint = result.new_endpoint
        store_auth(engine, auth)
    if result.required not in (sync_pb2.SyncStatusResponse.NO_CHANGES, sync_pb2.SyncStatusResponse.NORMAL_SYNC):
        engine.events.append({"type": "navigate", "page": "sync", "required": int(result.required)})
        raise ValueError("A full sync is required. Choose upload or download in the sync page.")
    if include_media:
        engine.col.sync_media(auth)
    return {"collectionComplete": True, "mediaComplete": include_media}


def invoke(engine, method, params):
    if method == "winone.profiles.create":
        name = params["name"].strip()
        if not name or name in engine.profile_record["profiles"]:
            raise ValueError("Profile name is empty or already exists")
        engine.profile_record["profiles"][name] = "profiles/" + uuid.uuid4().hex
        atomic_json(engine.root / "profiles.json", engine.profile_record)
        return {"name": name}
    if method == "winone.sync.status":
        return {"signedIn": (engine.directory / "sync-auth.dat").exists()}
    if method == "winone.sync.login":
        auth = engine.col.sync_login(params["username"], params["password"], params.get("endpoint") or None)
        store_auth(engine, auth)
        return {"signedIn": True}
    if method == "winone.sync.run":
        if params.get("direction") and params.get("confirmed") is not True:
            raise ValueError("Confirm the full synchronization direction first")
        return sync(engine, params.get("includeMedia", True), params.get("direction"))
    if method == "winone.sync.logout":
        path = engine.directory / "sync-auth.dat"
        # Credentials are intentionally erased by the user's Sign out action.
        if path.exists():
            path.unlink()
        return {"signedIn": False}
    raise ValueError("Unknown profile/sync operation")
