"""Capture the production shell on a private desktop with isolated profiles.

Use build/test-ui-design.ps1. Accessibility actions navigate the real pages;
no mouse/keyboard input is sent to the user's desktop or existing windows.
"""
import argparse
import base64
import ctypes
import json
import os
from pathlib import Path
import subprocess
import time
import uuid


def wait_for(read, timeout=30):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        value = read()
        if value:
            return value
        time.sleep(0.1)
    raise AssertionError("private UI did not become ready")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", required=True)
    args = parser.parse_args()
    root = Path(__file__).resolve().parents[2]
    output = Path(args.output).resolve()
    assert output.is_relative_to(root / "_temp")
    app_dir = output / "app"
    pages = ["overview", "capture", "selection", "quick-assistant", "interconnect",
             "wintray", "convenience", "ai-services", "settings"]
    receipts = []
    user32 = ctypes.windll.user32
    user32.FindWindowW.restype = ctypes.c_void_p
    user32.SetWindowPos.argtypes = [ctypes.c_void_p, ctypes.c_void_p, ctypes.c_int,
                                  ctypes.c_int, ctypes.c_int, ctypes.c_int, ctypes.c_uint]
    user32.PostMessageW.argtypes = [ctypes.c_void_p, ctypes.c_uint, ctypes.c_size_t, ctypes.c_ssize_t]
    for theme, language in [(t, l) for t in ("light", "dark") for l in ("zh-CN", "en-US")]:
        case = output / f"{theme}-{language}"
        for name in ("profile", "local", "roaming", "scratch", "shots"):
            (case / name).mkdir(parents=True, exist_ok=True)
        profile = case / "profile"
        translations = {}
        for path in (root / "apps/desktop-mygo/i18n").glob(f"*{language}.json"):
            translations.update(json.loads(path.read_text(encoding="utf-8-sig")))
        modules = ["always-on-top", "capture", "quick-assistant", "selection", "stroke", "timed-power", "wintray"]
        (profile / "configuration.json").write_text(json.dumps({"committed": {
            "schemaVersion": 1, "revision": 0, "builtinVersion": 5,
            "values": {"host": {"schemaVersion": 1, "revision": 0, "theme": theme,
                "language": language, "openShortcut": "", "startup": {"runAtLogon": False, "startMinimized": True}}},
            "moduleEnabled": dict.fromkeys(modules, False)}}), encoding="utf-8")
        (profile / "desktop-ui.json").write_text(json.dumps({"version": 1, "expandedWidth": 224,
                                                          "sidebarStartup": "expanded"}), encoding="utf-8")
        pipe = "\\\\.\\pipe\\WinOne-design-" + uuid.uuid4().hex
        socket_path = str(case / (uuid.uuid4().hex[:8] + ".sock"))
        assert len(socket_path.encode()) < 108
        environment = os.environ | {"MYGO_AUTOMATION": socket_path, "LOCALAPPDATA": str(case / "local"),
            "APPDATA": str(case / "roaming"), "TEMP": str(case / "scratch"), "TMP": str(case / "scratch")}

        def rpc(method, params=None):
            request = case / "request.json"
            request.write_text(json.dumps(params or {}, ensure_ascii=False), encoding="utf-8")
            result = subprocess.run([str(app_dir / "WinOne.NativeAutomation.exe"), "-socket", socket_path,
                "-method", method, "-params-file", str(request)], capture_output=True, timeout=25, env=environment)
            assert result.returncode == 0, result.stderr.decode("utf-8", "replace")
            return json.loads(result.stdout.decode("utf-8-sig"))

        with (case / "host.log").open("wb") as host_log, (case / "window.log").open("wb") as window_log:
            host = subprocess.Popen([str(app_dir / "WinOne.Host.exe"), "--pipe", pipe, "--data-dir", str(profile),
                "--standard-session"], env=environment, stdout=host_log, stderr=subprocess.STDOUT)
            (case / "host.pid").write_text(str(host.pid), encoding="ascii")
            app = None
            try:
                wait_for(lambda: ctypes.windll.kernel32.WaitNamedPipeW(ctypes.c_wchar_p(pipe), 200))
                app = subprocess.Popen([str(app_dir / "WinOne.exe"), "--pipe", pipe, "--data-dir", str(profile),
                    "--standard-session", "--open-settings"], env=environment, stdout=window_log, stderr=subprocess.STDOUT)
                wait_for(lambda: Path(socket_path).exists())
                wait_for(lambda: rpc("windows")["windows"])
                wait_for(lambda: any(n.get("key") == "settings" for n in rpc("tree")["nodes"]))
                window = rpc("windows")["windows"][0]
                hwnd = user32.FindWindowW(None, window["title"])
                assert hwnd
                # Both sizes use the full production shell. The rail can be
                # collapsed by its real button, not by altering page markup.
                for width, height in ((1280, 900), (820, 720)):
                    assert user32.SetWindowPos(hwnd, None, 20, 20, width, height, 0x14)
                    time.sleep(0.3)
                    if width == 820:
                        # Locate by translation to keep this check in step with UI copy.
                        label = translations["navigation.collapseSidebar"]
                        node = next(n for n in rpc("tree")["nodes"] if n.get("label") == label)
                        rpc("act", {"id": node["id"], "action": "press"})
                    for page in pages:
                        candidates = [n for n in rpc("tree")["nodes"] if n.get("key") == page and "press" in n.get("actions", [])]
                        assert candidates, page
                        rpc("act", {"id": candidates[-1]["id"], "action": "press"})
                        time.sleep(0.3)
                        tree = rpc("tree")["nodes"]
                        assert not any(n.get("key") in ("chat", "onebot", "flash-chat")
                                       and "press" in n.get("actions", []) for n in tree)
                        nav = next(n for n in tree if n.get("key") == page and "press" in n.get("actions", []))
                        assert "checked" in nav.get("states", []), (page, nav)
                        capture = rpc("capture")
                        name = f"{page}-{width}"
                        (case / "shots" / (name + ".png")).write_bytes(base64.b64decode(capture["png"]))
                        (case / "shots" / (name + ".json")).write_text(json.dumps(tree, ensure_ascii=False), encoding="utf-8")
                        receipts.append(f"{theme}/{language}/{name}")
                user32.PostMessageW(hwnd, 0x10, 0, 0)
                app.wait(timeout=10)
            finally:
                if app and app.poll() is None:
                    app.terminate()
                    app.wait(timeout=10)
                # The PowerShell private desktop owner requests Host tray exit.
    (output / "result.json").write_text(json.dumps({"captures": receipts,
        "scope": "9 production pages; two themes, languages and window sizes; real navigation; removed modules absent"}, indent=2), encoding="utf-8")


if __name__ == "__main__":
    main()
