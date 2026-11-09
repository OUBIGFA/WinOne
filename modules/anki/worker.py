"""Private stdio worker; no public listener and no automatic network work."""
import argparse
import contextlib
import json
import sys
sys.dont_write_bytecode = True
import queue
import threading
from pathlib import Path


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--profile", required=True)
    parser.add_argument("--library", required=True)
    parser.add_argument("--source", required=True)
    args = parser.parse_args()
    # Embedded CPython deliberately omits the script directory from sys.path.
    # Add only the shipped worker and pinned dependency directories.
    sys.path[:0] = [str(Path(__file__).resolve().parent), str(Path(args.library).resolve())]
    import os
    os.environ["WINONE_ANKICONNECT_SOURCE"] = str(Path(args.source).resolve())
    sys.stdout.reconfigure(encoding="utf-8")
    sys.stdin.reconfigure(encoding="utf-8")
    output = sys.stdout
    # Upstream diagnostics must never corrupt the transport. They go to the
    # private process stderr, which the host bounds and does not persist.
    with contextlib.redirect_stdout(sys.stderr):
        from engine import Engine
        engine = Engine(Path(args.profile))
    requests=queue.Queue(maxsize=16)
    active={"id":None,"cancellable":False}
    lock=threading.Lock()
    def read_requests():
        try:
            for line in sys.stdin:
                try:
                    request=json.loads(line)
                except Exception:
                    requests.put(line)
                    continue
                if request.get("method")=="cancel":
                    with lock:
                        if active["id"]==request.get("target") and active["cancellable"]:
                            try:
                                engine.col.set_wants_abort()
                                engine.col.abort_sync()
                                engine.col.abort_media_sync()
                            except Exception as error:
                                print("Anki cancellation failed:",type(error).__name__,file=sys.stderr)
                    continue
                requests.put(line)
        finally:
            requests.put(None)
    threading.Thread(target=read_requests,daemon=True).start()
    try:
        while True:
            line=requests.get()
            if line is None:break
            request = None
            try:
                if len(line) > 96 * 1024 * 1024:
                    raise ValueError("Request exceeds maximum size")
                request = json.loads(line)
                if request["method"] == "close":
                    break
                with lock:
                    active["id"]=request["id"]
                    active["cancellable"]=request["method"] in ("winone.import.run","winone.export","winone.sync.run","winone.sync.login","winone.backend.call") or request["method"]=="legacy" and request["params"].get("action") in ("sync","importPackage","exportPackage")
                with contextlib.redirect_stdout(sys.stderr):
                    engine.set_owner(request.get("owner", 0))
                    if request["method"] == "legacy":
                        result = engine.legacy(request["params"]["action"], request["params"].get("params"))
                    else:
                        result = engine.invoke(request["method"], request.get("params"))
                response = {"id": request["id"], "result": result, "events": engine.take_events()}
            except Exception as error:
                response = {"id": request.get("id") if isinstance(request, dict) else None,
                            "error": {"message": str(error), "kind": getattr(error, "kind", "anki")},"events":engine.take_events()}
            finally:
                with lock:active["id"]=None;active["cancellable"]=False
            output.write(json.dumps(response, ensure_ascii=False) + "\n")
            output.flush()
            if engine.exit_requested:
                break
    finally:
        with contextlib.redirect_stdout(sys.stderr):
            engine.close()


if __name__ == "__main__":
    main()
