#!/usr/bin/env python3
from datetime import datetime
from pathlib import Path
import os
import shutil
import subprocess


root = Path(__file__).resolve().parent.parent
compose_path = root / "compose.yaml"
original = compose_path.read_text()

if "\n  frontend:\n" in original:
    raise SystemExit("frontend service already exists; no changes made")

port_old = '      - "8081:8080"'
port_new = '      - "127.0.0.1:8081:8080"'
if original.count(port_old) != 1 or original.count("\nvolumes:\n") != 1:
    raise SystemExit("Unexpected Compose layout; no changes made")

frontend = """
  frontend:
    image: disk_front:20261010-dd579be-1
    pull_policy: never
    restart: unless-stopped
    ports:
      - "127.0.0.1:8082:80"
    depends_on:
      go-service:
        condition: service_started
"""
updated = original.replace(port_old, port_new).replace("\nvolumes:\n", frontend + "\nvolumes:\n")

stamp = datetime.now().strftime("%Y%m%d-%H%M%S")
backup = root / f"compose.yaml.bak.frontend.{stamp}"
with compose_path.open("rb") as source, backup.open("xb") as target:
    shutil.copyfileobj(source, target)
backup.chmod(0o600)

temporary = root / "compose.yaml.frontend-new"
if temporary.exists():
    raise SystemExit("Temporary Compose file already exists; no changes made")
try:
    with temporary.open("x", encoding="utf-8", newline="\n") as target:
        target.write(updated)
    temporary.chmod(compose_path.stat().st_mode & 0o777)
    os.replace(temporary, compose_path)
    result = subprocess.run(
        ["docker", "compose", "config", "--quiet"],
        cwd=root,
        capture_output=True,
        text=True,
    )
    if result.returncode:
        raise RuntimeError("docker compose config --quiet failed")
except Exception:
    shutil.copy2(backup, compose_path)
    raise
finally:
    temporary.unlink(missing_ok=True)

print(f"Compose validated; backup: {backup.name}")
