#!/usr/bin/env python3
"""Test installer upgrades in an isolated Wine prefix (run under xvfb-run -a).

Example:
  xvfb-run -a python packaging/windows/test-upgrade.py \
    --previous old-setup.exe --current new-setup.exe --version v4.0.1 \
    --work-dir /tmp/raikiri-upgrade-check
"""

import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import socket
import subprocess
import time
import urllib.request


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--previous", type=Path, required=True)
    parser.add_argument("--current", type=Path, required=True)
    parser.add_argument("--version", required=True)
    parser.add_argument("--work-dir", type=Path, required=True)
    args = parser.parse_args()
    work = args.work_dir.resolve()
    work.mkdir(parents=True, exist_ok=False)
    env = dict(os.environ, WINEPREFIX=str(work / "wine"), WINEARCH="win64",
               WINEDEBUG="-all", WINEDLLOVERRIDES="mscoree,mshtml=;winemenubuilder.exe=d",
               XDG_CONFIG_HOME=str(work / "config"), XDG_DATA_HOME=str(work / "share"))
    with socket.socket() as port_socket:
        port_socket.bind(("127.0.0.1", 0))
        port = port_socket.getsockname()[1]
    base = f"http://127.0.0.1:{port}"
    log = (work / "wine.log").open("w")

    def wine(*command, capture=False):
        return subprocess.run(["wine", *map(str, command)], env=env, check=True,
                              stdout=subprocess.PIPE if capture else log,
                              stderr=log, text=True, timeout=60)

    def windows_path(variable):
        win = wine("cmd", "/c", f"echo %{variable}%", capture=True).stdout.strip()
        unix = subprocess.check_output(["winepath", "-u", win], env=env,
                                       stderr=log, text=True).strip()
        return win, Path(unix)

    def api(path, payload=None):
        body = json.dumps(payload).encode() if payload is not None else None
        request = urllib.request.Request(base + path, data=body,
                                         headers={"Content-Type": "application/json"})
        with urllib.request.urlopen(request, timeout=2) as response:
            return json.load(response)

    def wait_health(version=None):
        deadline = time.monotonic() + 30
        while time.monotonic() < deadline:
            try:
                health = api("/healthz")
                if version is None or health["version"] == version:
                    return health
            except OSError:
                pass
            time.sleep(0.25)
        raise AssertionError(f"Raikiri did not start with version {version}")

    try:
        wine("wineboot", "--init")
        appdata_win, appdata = windows_path("APPDATA")
        _, local = windows_path("LOCALAPPDATA")
        _, profile = windows_path("USERPROFILE")
        # Wine maps Desktop/Documents to real host folders on first boot.
        # Keep all installer shortcuts and data inside this disposable prefix.
        for folder in profile.iterdir():
            if folder.is_symlink():
                folder.unlink()
                folder.mkdir()
        wine(args.previous.resolve(), "/S")
        installed = local / "Programs/Raikiri/raikiri.exe"
        desktop = profile / "Desktop/Raikiri.lnk"
        programs = appdata / "Microsoft/Windows/Start Menu/Programs"
        startup_dir = next(p for p in programs.iterdir() if p.name.lower() == "startup")
        startup = startup_dir / "Raikiri.lnk"
        assert installed.exists() and desktop.exists() and startup.exists()
        data = appdata / "Raikiri"
        sentinel = data / "upgrade-preserve.txt"
        sentinel.write_text("Keep user data across upgrades.\n")
        # SQLite initializes this empty file normally; its presence suppresses
        # the tray's first-run browser launch during the unattended test.
        (data / "raikiri.db").touch()
        old_hash = hashlib.sha256(installed.read_bytes()).hexdigest()

        def launch():
            return subprocess.Popen(["wine", str(installed), "--tray", "--port", str(port),
                                     "--data-dir", appdata_win + "\\Raikiri"],
                                    env=env, stdout=log, stderr=log)

        old_process = launch()
        old_health = wait_health()
        api("/api/config", {"audioVolume": 37, "ttsVoice": "es-MX-DaliaNeural"})
        before = api("/api/config")
        desktop.unlink()
        wine(args.current.resolve(), "/S")
        old_process.wait(timeout=10)
        assert not desktop.exists(), "upgrade recreated disabled desktop shortcut"
        assert startup.exists(), "upgrade removed enabled startup shortcut"
        assert hashlib.sha256(installed.read_bytes()).hexdigest() != old_hash
        assert sentinel.read_text() == "Keep user data across upgrades.\n"
        registry = wine("reg", "query",
                        r"HKCU\Software\Microsoft\Windows\CurrentVersion\Uninstall\Raikiri",
                        "/v", "DisplayVersion", capture=True).stdout
        assert args.version.removeprefix("v") in registry

        launch()
        health = wait_health(args.version)
        assert api("/api/config") == before, "upgrade changed the saved configuration"
        # Also check the opposite optional component selection on reinstall.
        shutil.copy2(startup, desktop)
        startup.unlink()
        wine(args.current.resolve(), "/S")
        assert desktop.exists() and not startup.exists()
        launch()
        wait_health(args.version)
        assert api("/api/config") == before
        assert sentinel.read_text() == "Keep user data across upgrades.\n"
        result = {"passed": True, "environment": "Wine", "previous": old_health["version"],
                  "installed": health["version"], "running_app_replaced": True,
                  "configuration_preserved": True, "user_data_preserved": True,
                  "shortcut_choices_preserved_both_directions": True,
                  "installed_sha256": hashlib.sha256(installed.read_bytes()).hexdigest()}
        (work / "result.json").write_text(json.dumps(result, indent=2) + "\n")
        print(json.dumps(result, indent=2), flush=True)
    finally:
        subprocess.run(["wineserver", "-k"], env=env, stdout=log, stderr=log, timeout=15)
        log.close()


if __name__ == "__main__":
    main()
