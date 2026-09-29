#!/usr/bin/env python3
"""Collect a bounded local RC installation stage, never full T24 acceptance.

Run --execute-install only after human authority for the printed local envelope.
No Runtime executables, Provider writes, credentials or raw Runtime logs are used.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import selectors
import shutil
import signal
import stat
import subprocess
import sys
import time

TAG = "v0.1.2-rc.1"
REVISION = "f73d6d0c951dd40c5cc97c3794ad7ee5607092b5"
URL = "https://raw.githubusercontent.com/rgomids/axiom/main/scripts/install.sh"
SAFE_PATH = "/usr/bin:/bin:/usr/sbin:/sbin"
INSTALLER_SHA256 = "3bf39863f4c542f04181ec9e76e86b587ec64cfb26e905a80be875ad87558dc7"
ASSETS = {
    "SHA256SUMS": "d8442bc6f4ff53a26ca29640c5613b951d9c47f06c217b23649000b388213f55",
    "axiom-0.1.2-rc.1-linux-amd64.tar.gz": "aa26c3808541cb2f8ee82baa21212fe86f8055eef27ff9d92e00550c514ac949",
    "axiom-0.1.2-rc.1-linux-arm64.tar.gz": "58ed74526e82703d72a60a02905df180aeadc5e742fd5e93f179a306500883fc",
    "axiom-0.1.2-rc.1-macos-27-arm64.tar.gz": "a474ae70e09d85065831c601afe0e500957cdbeaabc1febbc8a73bf63d53339d",
}


def valid_provenance(event):
    value = event.get("provenance", {})
    return (event.get("status") == "success" and value.get("product") == "Axiom"
            and value.get("version") == TAG[1:] and value.get("revision") == REVISION[:12]
            and value.get("sourceState") == "clean")


def stop_group(process):
    try:
        os.killpg(process.pid, signal.SIGKILL)
    except ProcessLookupError:
        pass


def digest(data):
    return hashlib.sha256(data).hexdigest()


def file_digest(path):
    result = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(65536), b""):
            result.update(chunk)
    return result.hexdigest()


def snapshot(root):
    rows = []
    for path in sorted(root.rglob("*")):
        if len(rows) >= 1000:
            raise RuntimeError("isolated root inventory exceeds 1000 entries")
        info = path.lstat()
        if stat.S_ISLNK(info.st_mode):
            raise RuntimeError("unexpected symlink in isolated installation")
        row = {"path": str(path.relative_to(root)), "mode": oct(stat.S_IMODE(info.st_mode)),
               "type": "directory" if path.is_dir() else "file", "bytes": info.st_size}
        if path.is_file():
            row["sha256"] = file_digest(path)
        rows.append(row)
    return rows


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--version", required=True, choices=[TAG])
    parser.add_argument("--evidence-dir", required=True, type=Path)
    parser.add_argument("--execute-install", action="store_true",
                        help="execute only the separately authorized local stage")
    parser.add_argument("--approved-envelope-sha256",
                        help="exact digest explicitly authorized by the human; required for execution")
    args = parser.parse_args()
    # New private directory only; preserve existing Evidence, including symlinks.
    if not args.evidence_dir.is_absolute():
        parser.error("--evidence-dir must be absolute")
    parent = args.evidence_dir.parent.resolve(strict=True)
    root = parent / args.evidence_dir.name
    if args.execute_install and not args.approved_envelope_sha256:
        parser.error("execution requires an explicitly authorized envelope SHA-256")
    if not args.execute_install and args.approved_envelope_sha256:
        parser.error("approved digest is only valid with --execute-install")
    if args.execute_install:
        expected_entries = {"manifest.json", "envelope.json", "acceptance.md"}
        if (not root.is_dir() or root.is_symlink()
                or {p.name for p in root.iterdir()} != expected_entries
                or any(p.is_symlink() for p in root.iterdir())):
            parser.error("execution requires an untouched plan-only Evidence directory")
        for path in [root, *root.iterdir()]:
            info = path.stat()
            if info.st_uid != os.getuid() or stat.S_IMODE(info.st_mode) & 0o077:
                parser.error("unsafe plan ownership or permissions; preserve and review")
        prior = json.loads((root / "manifest.json").read_text())
        if prior.get("mode") != "plan-only" or prior.get("observations") != []:
            parser.error("Evidence target is not an unexecuted plan")
        if digest((root / "envelope.json").read_bytes()) != args.approved_envelope_sha256:
            parser.error("envelope drift; obtain fresh human authority")
    elif os.path.lexists(root):
        parser.error("Evidence target already exists; choose a new directory")
    os.umask(0o077)
    if not args.execute_install:
        root.mkdir(mode=0o700)
    artifacts = root / "artifacts"
    manifest = {
        "schemaVersion": 1, "rcTag": TAG, "sourceRevision": REVISION,
        "observedAtUnix": int(time.time()), "scope": "local-install-stage-only",
        "host": {"os": platform.system(), "architecture": platform.machine(),
                 "kernel": platform.release(), "macOSVersion": platform.mac_ver()[0]},
        "mode": "execute-install" if args.execute_install else "plan-only",
        "observations": [], "t24": "blocked", "t25": "blocked",
        "humanDecision": "PENDING", "runtimeInvocation": "not performed",
        "providerMutation": "not performed", "executionIDs": [],
        "cleanupDisposition": "Evidence and isolated HOME retained; no deletion",
        "limitations": ["No complete native acceptance row",
                         "No real Codex/Claude/GitHub journey or dogfood graph",
                         "No upgrade, failure, recovery or native S7 suite in this stage"],
    }
    home = root / "isolated-home"
    env = {"HOME": str(home), "PATH": SAFE_PATH, "LANG": "C", "LC_ALL": "C",
           "TMPDIR": str(root / "tmp"),
           "LINGO_STATE_ROOT": str(home / "state"),
           "LINGO_PROJECTS_ROOT": str(home / "projects"),
           "AXIOM_CODEX_SKILLS_ROOT": str(home / "codex-skills"),
           "CLAUDE_CONFIG_DIR": str(home / "claude")}
    install = ["/bin/sh", str(artifacts / "remote-install.sh"), "--version", TAG]
    binary = str(home / ".local/bin/axiom")
    commands = [install, [binary, "--json", "version"], [binary, "--json", "first-run"], install]
    curl = ["/usr/bin/curl", "--disable", "--proto", "=https", "--proto-redir", "=https", "--tlsv1.2", "-fsSL",
            "--connect-timeout", "20", "--max-time", "60", "--max-filesize", "65536"]
    preflight = [curl + [f"https://api.github.com/repos/rgomids/axiom/releases/tags/{TAG}"],
                 curl + [f"https://api.github.com/repos/rgomids/axiom/git/ref/tags/{TAG}"],
                 curl + [URL, "-o", str(artifacts / "remote-install.sh")]]
    envelope = {"action": "isolated local remote-install/no-Runtime/reinstall stage",
                "target": str(home), "rcTag": TAG, "sourceRevision": REVISION,
                "installerURL": URL, "installerSHA256": INSTALLER_SHA256,
                "collectorSHA256": file_digest(Path(__file__).resolve()),
                "assetSHA256": ASSETS, "environment": env, "commands": preflight + commands,
                "timeoutSecondsPerCommand": 600, "maximumStageAttempts": 1,
                "networkRetries": "installer-owned bounded retries; no harness retry",
                "allowedEffects": ["HTTPS public release reads", "writes inside Evidence target",
                                   "installer-owned temporary download directories"],
                "forbiddenEffects": ["Runtime invocation", "GitHub mutation", "credential access",
                                     "host installation roots", "release publication/promotion"],
                "cleanupOwnership": "operator; retained isolated files only",
                "runtimeProfile": "none; both Runtime executables excluded from PATH",
                "retain": "commands/exits, bounded CLI output, hashes, receipt, limitations"}
    envelope_bytes = (json.dumps(envelope, indent=2) + "\n").encode()
    if args.execute_install and digest(envelope_bytes) != args.approved_envelope_sha256:
        parser.error("current execution differs from approved envelope; review again")
    if not args.execute_install:
        (root / "envelope.json").write_bytes(envelope_bytes)
    manifest["envelopeSHA256"] = digest((root / "envelope.json").read_bytes())

    def save():
        (root / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n")
        (root / "acceptance.md").write_text(
            f"# {TAG}: local installation stage\n\n"
            f"Mode: {manifest['mode']}. Stage: {manifest.get('stageResult', 'pending')}.\n\n"
            "T24: blocked. T25: blocked. Human decision: PENDING.\n\n"
            "See manifest.json for actual observations and limitations. "
            "Successful steps do not establish a complete platform row.\n")

    def run(label, argv, command_env=None, cwd=None, timeout=600):
        output = artifacts / (label + ".txt")
        started = time.time()
        process = None
        hasher = hashlib.sha256()
        size = 0
        retained = bytearray()
        timed_out = False
        deadline = time.monotonic() + timeout
        try:
            # Open before spawn; capture errors always stop and reap the group.
            with output.open("wb") as stream, selectors.DefaultSelector() as selector:
                process = subprocess.Popen(argv, stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
                                           env=command_env, cwd=cwd, start_new_session=True)
                selector.register(process.stdout, selectors.EVENT_READ)
                while selector.get_map():
                    if time.monotonic() >= deadline and not timed_out:
                        timed_out = True
                        stop_group(process)
                    for key, _ in selector.select(0.1):
                        chunk = os.read(key.fd, 65536)
                        if not chunk:
                            selector.unregister(key.fileobj)
                            continue
                        size += len(chunk)
                        hasher.update(chunk)
                        keep = chunk[:max(0, 65536 - len(retained))]
                        stream.write(keep)
                        retained.extend(keep)
            try:
                code = process.wait(timeout=max(0.01, deadline - time.monotonic()))
            except subprocess.TimeoutExpired:
                timed_out = True
                stop_group(process)
                code = process.wait()
        except BaseException:
            if process is not None:
                stop_group(process)
                process.wait()
            manifest["observations"].append({"label": label, "argv": argv,
                                             "exitCode": None if process is None else process.returncode,
                                             "result": "capture-failed; group stopped; no next command"})
            raise
        finally:
            if process is not None and process.stdout is not None:
                process.stdout.close()
        full_digest = hasher.hexdigest()
        data = bytes(retained)
        # Only these public verifier/installer/local CLI commands are captured.
        # Bound retained output; never capture vendor sessions or arbitrary tests.
        observation = {"label": label, "argv": argv, "exitCode": code,
                       "timedOut": timed_out, "durationSeconds": round(time.time() - started, 3),
                       "outputSHA256": full_digest, "outputBytes": size,
                       "retainedOutput": str(output.relative_to(root)),
                       "retainedSHA256": digest(output.read_bytes()),
                       "outputTruncated": size > 65536}
        manifest["observations"].append(observation)
        save()
        if timed_out or code != 0:
            raise RuntimeError(f"{label} failed: exit {code}; inspect retained output")
        return data

    save()
    if not args.execute_install:
        manifest["stageResult"] = "pending-human-authority"
        save()
        print(f"plan={root}; envelope_sha256={manifest['envelopeSHA256']}")
        return 0
    interruption_signal = signal.SIGINT

    def interrupt(signum, _frame):
        nonlocal interruption_signal
        interruption_signal = signum
        raise KeyboardInterrupt

    prior_termination = signal.signal(signal.SIGTERM, interrupt)
    try:
        if any(shutil.which(name, path=SAFE_PATH) for name in ("codex", "claude")):
            raise RuntimeError("Runtime executable on isolated PATH; stage refused")
        row = (platform.system(), platform.machine())
        if row not in (("Darwin", "arm64"), ("Linux", "x86_64"), ("Linux", "aarch64")):
            raise RuntimeError("unsupported native host")
        if row[0] == "Darwin" and platform.mac_ver()[0] != "27.0":
            raise RuntimeError("macOS 27.0 required")
        artifacts.mkdir(mode=0o700)
        (root / "tmp").mkdir(mode=0o700)
        home.mkdir(mode=0o700)
        manifest["initialRootInventory"] = snapshot(home)
        metadata = json.loads(run("published-metadata", preflight[0], env, home))
        observed_assets = {entry["name"]: entry.get("digest") for entry in metadata.get("assets", [])}
        if (metadata.get("tag_name") != TAG or metadata.get("target_commitish") != REVISION
                or metadata.get("immutable") is not True or metadata.get("prerelease") is not True
                or metadata.get("draft") is not False
                or len(metadata.get("assets", [])) != len(ASSETS)
                or observed_assets != {name: "sha256:" + value for name, value in ASSETS.items()}):
            raise RuntimeError("published immutable candidate/asset metadata mismatch")
        manifest["releaseIdentity"] = {key: metadata[key] for key in
                                      ("id", "tag_name", "target_commitish", "immutable", "prerelease", "draft")}
        manifest["releaseAssets"] = observed_assets
        tag = json.loads(run("published-tag", preflight[1], env, home))
        if tag.get("object", {}).get("type") != "commit" or tag["object"].get("sha") != REVISION:
            raise RuntimeError("published lightweight tag revision mismatch")
        remote = artifacts / "remote-install.sh"
        run("download-installer", preflight[2], env, home)
        manifest["installerSHA256"] = digest(remote.read_bytes())
        if manifest["installerSHA256"] != INSTALLER_SHA256:
            raise RuntimeError("main remote installer differs from exact RC source; review required")
        for label, argv in zip(("install", "version", "first-run-neither", "reinstall"), commands):
            if label == "reinstall":
                manifest["beforeReinstallInventory"] = snapshot(home)
            run(label, argv, env, home)
        version = json.loads((artifacts / "version.txt").read_text())
        provenance = version.get("provenance", {})
        if not valid_provenance(version):
            raise RuntimeError("installed version/source provenance mismatch")
        first_run = json.loads((artifacts / "first-run-neither.txt").read_text())
        if (first_run.get("status") != "success" or first_run.get("firstRun", {}).get("detected") != 0
                or first_run.get("firstRun", {}).get("failed") != 0):
            raise RuntimeError("no-Runtime first-run result mismatch")
        manifest["versionProvenance"] = provenance
        manifest["firstRun"] = first_run["firstRun"]
        manifest["afterReinstallInventory"] = snapshot(home)
        if manifest["beforeReinstallInventory"] != manifest["afterReinstallInventory"]:
            raise RuntimeError("reinstall changed isolated installation inventory")
        if "install_status=unchanged" not in (artifacts / "reinstall.txt").read_text():
            raise RuntimeError("reinstall no-op not observed")
        receipt = home / ".local/state/axiom/install/installation.receipt"
        receipt_fields = dict(line.split("=", 1) for line in receipt.read_text().splitlines())
        installed_fields = dict(line.split("=", 1) for line in
                                (artifacts / "install.txt").read_text().splitlines()
                                if line.startswith("install_") and "=" in line)
        asset = installed_fields.get("install_asset")
        if (asset not in ASSETS or installed_fields.get("install_asset_sha256") != ASSETS[asset]
                or receipt_fields.get("archiveSha256") != ASSETS[asset]
                or receipt_fields.get("version") != TAG[1:]
                or receipt_fields.get("revision") != REVISION[:12]
                or receipt_fields.get("sha256") != file_digest(Path(binary))):
            raise RuntimeError("observed installed archive/receipt/binary identity mismatch")
        manifest["installedAsset"] = {"name": asset, "sha256": ASSETS[asset]}
        manifest["receipt"] = {"path": str(receipt.relative_to(root)),
                               "sha256": digest(receipt.read_bytes())}
        manifest["installedBinarySHA256"] = file_digest(Path(binary))
        manifest["stageResult"] = "local-steps-confirmed; full-acceptance-blocked"
        save()
        return 0
    except KeyboardInterrupt:
        manifest["stageResult"] = "interrupted; effects require inspection"
        manifest["interruptionSignal"] = interruption_signal
        manifest["limitations"].append("Interruption may leave local effects; preserve roots and reconcile before any new run")
        save()
        print("stage interrupted; retained effects require inspection", file=sys.stderr)
        return 128 + interruption_signal
    except (OSError, RuntimeError, ValueError) as error:
        manifest["stageResult"] = "failed"
        manifest["limitations"].append(str(error))
        save()
        print(str(error), file=sys.stderr)
        return 1
    finally:
        signal.signal(signal.SIGTERM, prior_termination)


if __name__ == "__main__":
    sys.exit(main())
