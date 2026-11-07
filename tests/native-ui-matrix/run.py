"""Explicit private-desktop checks against the real WinOne Host and native window.

Run through build/test-native-ui-matrix.ps1. No real account/model is used: the
schedule executes a genuine local script, and profile/group edits use the real
Cindy services and stores. Captures are actual native-window frames.
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


def wait_for(read, predicate=lambda x: bool(x), timeout=30):
    deadline = time.monotonic() + timeout
    last = None
    while time.monotonic() < deadline:
        last = read()
        if predicate(last):
            return last
        time.sleep(0.08)
    raise AssertionError(f"condition not reached: {last!r}")


def main():
    args = argparse.ArgumentParser()
    args.add_argument("--output", required=True)
    options = args.parse_args()
    root = Path(__file__).resolve().parents[2]
    output = Path(options.output).resolve()
    assert output.is_relative_to(root / "_temp"), "use an isolated repository output"
    app_dir, profile = output / "app", output / "profile"
    profile.mkdir(exist_ok=True)
    for name in ("local", "roaming", "home", "scratch", "shots"):
        (output / name).mkdir(exist_ok=True)
    pipe = "\\\\.\\pipe\\WinOne-ui-matrix-" + uuid.uuid4().hex
    (output / "pipe.txt").write_text(pipe, encoding="ascii")
    socket_path = str(output / "ui.sock")
    assert len(socket_path.encode()) < 108, "Windows Unix socket path is too long"
    environment = os.environ | {
        "MYGO_AUTOMATION": socket_path, "LOCALAPPDATA": str(output / "local"),
        "APPDATA": str(output / "roaming"), "USERPROFILE": str(output / "home"),
        "HOME": str(output / "home"), "TEMP": str(output / "scratch"), "TMP": str(output / "scratch"),
    }
    # Disable every system module before the real Host is launched. This test
    # never registers autostart, sends physical desktop input or runs a power action.
    modules = ["always-on-top", "capture", "quick-assistant", "selection", "stroke", "timed-power", "wintray"]
    (profile / "configuration.json").write_text(json.dumps({"committed": {
        "schemaVersion": 1, "revision": 0, "builtinVersion": 5,
        "values": {"host": {"schemaVersion": 1, "revision": 0, "theme": "light", "language": "zh-CN", "openShortcut": "", "startup": {"runAtLogon": False, "startMinimized": True}}},
        "moduleEnabled": dict.fromkeys(modules, False),
    }}), encoding="utf-8")
    translations = {}
    for path in sorted((root / "apps/desktop-mygo/i18n").glob("*zh-CN.json")):
        translations.update(json.loads(path.read_text(encoding="utf-8-sig")))
    tr = lambda key: translations.get(key, key)
    receipts = []
    out_log = (output / "window.log").open("wb")
    host_log = (output / "host-process.log").open("wb")
    host_process = subprocess.Popen([str(app_dir / "WinOne.Host.exe"), "--pipe", pipe, "--data-dir", str(profile), "--standard-session"], env=environment, stdout=host_log, stderr=subprocess.STDOUT)
    wait_for(lambda: bool(ctypes.windll.kernel32.WaitNamedPipeW(ctypes.c_wchar_p(pipe), 200)))
    app = subprocess.Popen([str(app_dir / "WinOne.exe"), "--pipe", pipe, "--data-dir", str(profile), "--standard-session", "--open-settings"], env=environment, stdout=out_log, stderr=subprocess.STDOUT)

    def rpc(method, params=None):
        # The repository's Go client works on Windows Python builds that do
        # not expose AF_UNIX; protocol and real native controls are unchanged.
        request = output / "rpc-params.json"
        request.write_text(json.dumps(params or {}, ensure_ascii=False), encoding="utf-8")
        result = subprocess.run([str(app_dir / "WinOne.NativeAutomation.exe"), "-socket", socket_path, "-method", method, "-params-file", str(request)], capture_output=True, timeout=25, env=environment)
        assert result.returncode == 0, (method, result.stderr.decode("utf-8", "replace"))
        return json.loads(result.stdout.decode("utf-8-sig"))

    def host(method, params=None):
        result = subprocess.run([str(app_dir / "WinOne.exe"), "--hostcall", pipe, method, json.dumps(params or {}, ensure_ascii=False)], capture_output=True, timeout=40, env=environment | {"MYGO_AUTOMATION": ""})
        assert result.returncode == 0, (method, result.stderr.decode("utf-8", "replace"))
        value = json.loads(result.stdout.decode("utf-8-sig"))
        assert not (isinstance(value, dict) and value.get("ok") is False), (method, value)
        return value

    def nodes():
        return rpc("tree")["nodes"]

    def find(label, action="press"):
        return next((n for n in reversed(nodes()) if n.get("label") == label and action in n.get("actions", []) and "disabled" not in n.get("states", [])), None)

    def act(label, action="press", text=None):
        node = wait_for(lambda: find(label, action))
        if "offscreen" in node.get("states", []):
            rpc("act", {"id": node["id"], "action": "scrollIntoView"})
        rpc("act", {"id": node["id"], "action": action, **({"text": text} if text is not None else {})})
        time.sleep(0.12)

    def shot(name):
        result = rpc("capture")
        (output / "shots" / (name + ".png")).write_bytes(base64.b64decode(result["png"]))
        (output / (name + "-tree.json")).write_text(json.dumps(nodes(), ensure_ascii=False, indent=2), encoding="utf-8")

    try:
        wait_for(lambda: Path(socket_path).exists() if app.poll() is None else False)
        def ready_windows():
            try:
                return rpc("windows")["windows"]
            except AssertionError as error:
                # A stale socket file can predate the new process's listener.
                # Retry only connection startup, not a real application failure.
                if "dial unix" not in str(error):
                    raise
                return None
        wait_for(ready_windows)
        rpc("act", {"key": "onebot", "action": "press"})
        act(tr("nativeOneBot.defaultModels"))
        act(tr("onebotCindy.bots.globalSettings.showReplyTarget"))
        wait_for(lambda: json.loads((profile / "desktop-onebot-view.json").read_text()), lambda x: x.get("showReplyTarget") is True)
        shot("default-model-reply-target")
        act(tr("nativeOneBot.close"))
        bots = []
        for name in ("Matrix Alpha", "Matrix Beta"):
            bots.append(host("chat.onebot.profiles.create", {"input": {"name": name, "description": "Isolated native matrix", "avatar": "🐼", "locale": "zh-CN"}}))
        act(tr("nativeOneBot.refresh"))
        group = host("chat.onebot.groups.create", {"input": {"name": "Matrix Group", "botIds": [b["id"] for b in bots]}})
        act(tr("nativeOneBot.refresh"))
        act("Matrix Group")
        act(tr("nativeOneBot.groupSettings"))
        shot("group-settings")
        delete_label = tr("onebotCindy.bots.groupChat.settings.delete")
        act(delete_label)
        assert any(g["id"] == group["groupId"] for g in host("chat.onebot.groups.list")["groups"]), "unconfirmed group was deleted"
        act(tr("nativeOneBot.cancel"))
        act(delete_label)
        act(delete_label)
        wait_for(lambda: host("chat.onebot.groups.list")["groups"], lambda groups: all(g["id"] != group["groupId"] for g in groups))
        receipts.append("native group deletion: confirm/cancel/confirm and real store removal")
        wait_for(lambda: find(tr("nativeOneBot.createGroup")))
        shot("group-deleted")
        # Windows' native context menu is outside MyGo's socket protocol and
        # does not open on the isolated non-input desktop. Do not fake those
        # profile routes: the in-process production-component tests cover them.

        rpc("act", {"key": "chat", "action": "press"})
        act(tr("chat.automations"))
        # A real protocol script, run by Cindy's owned Windows job. No model
        # response or mocked schedule completion stands in for execution.
        script = output / "schedule.mjs"
        script.write_text("console.log(JSON.stringify({protocol:'cindy-script/1',type:'complete',resultText:'Matrix script executed'}));", encoding="utf-8")
        work = output / "work"
        work.mkdir(exist_ok=True)
        schedule = host("chat.automations.create", {"input": {"name": "Matrix Script", "prompt": "", "kind": "cron", "cronExpr": "0 9 * * *", "timezone": "UTC", "agentKind": "pi", "recurring": True, "manual": True, "workspaceKind": "project", "workingDir": str(work), "executionMode": "script", "scriptConfig": {"command": f'"{app_dir / "shared/ai/runtime/bun.exe"}" "{script}"', "capabilities": []}, "notify": {"desktop": False, "feishu": False}}})
        act(tr("nativeAutomation.refresh"))
        act("Matrix Script")
        act(tr("nativeAutomation.run"))
        runs = wait_for(lambda: host("chat.automations.runs", {"id": schedule["id"]}), lambda rs: any(r.get("status") == "success" for r in rs))
        assert any(r.get("resultText") == "Matrix script executed" for r in runs), runs
        act(tr("nativeAutomation.pause"))
        wait_for(lambda: host("chat.automations.list"), lambda rs: any(s["id"] == schedule["id"] and s["status"] == "paused" for s in rs))
        act(tr("nativeAutomation.resume"))
        wait_for(lambda: host("chat.automations.list"), lambda rs: any(s["id"] == schedule["id"] and s["status"] == "active" for s in rs))
        shot("automation-executed")
        act(tr("nativeAutomation.templates"))
        shot("automation-templates")
        act(tr("nativeAutomation.close"))
        act(tr("nativeAutomation.delete"))
        act(tr("nativeAutomation.confirm"))
        wait_for(lambda: host("chat.automations.deleted"), lambda rs: any(s["id"] == schedule["id"] for s in rs))
        act(tr("nativeAutomation.deleted"))
        act(tr("nativeAutomation.restore"))
        wait_for(lambda: host("chat.automations.list"), lambda rs: any(s["id"] == schedule["id"] for s in rs))
        act(tr("nativeAutomation.close"))
        receipts.append("native automation run/pause/resume/templates/delete/restore against real Cindy scheduler")
        shot("automation-restored")
        (output / "result.json").write_text(json.dumps({"passed": receipts, "unverified": ["private-desktop Windows profile/context menus", "real accounts/market network", "physical microphone/speaker/IME/suspend", "full browser annotation window flow"]}, indent=2), encoding="utf-8")
        # Normal WM_CLOSE for this exact child window only; Host tray shutdown
        # is requested by the private desktop wrapper after the client exits.
        window = rpc("windows")["windows"][0]
        ctypes.windll.user32.PostMessageW(ctypes.windll.user32.FindWindowW(None, window["title"]), 0x10, 0, 0)
        app.wait(timeout=10)
    except Exception:
        if app.poll() is None:
            try: shot("failure")
            except Exception: pass
        raise
    finally:
        if app.poll() is None:
            app.terminate()
            app.wait(timeout=10)
        out_log.close()
        host_log.close()
        # The wrapper owns this exact Host path/PID and requests its tray exit.
        (output / "host.pid").write_text(str(host_process.pid), encoding="ascii")


if __name__ == "__main__":
    main()
