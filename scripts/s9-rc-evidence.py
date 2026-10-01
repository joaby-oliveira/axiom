#!/usr/bin/env python3
"""Collect bounded local RC install/first-run stages, never full T24 acceptance.

The exact candidate comes from a reviewed descriptor (--candidate); nothing about
any RC is hardcoded here. Default mode writes a plan-only envelope and starts no
process. Run --execute-install only after human authority for that exact envelope
digest. Runtime executables are resolved on PATH for first-run detection but are
never executed; no Provider writes, credentials or raw Runtime logs are used.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import re
import selectors
import shutil
import signal
import stat
import subprocess
import sys
import time

SAFE_PATH = "/usr/bin:/bin:/usr/sbin:/sbin"
CANDIDATE_SCHEMA = "axiom-s9-rc-candidate/v1"
TAG_PATTERN = re.compile(r"^v(\d+)\.(\d+)\.(\d+)-rc\.(\d+)$")
ROWS = {("Darwin", "arm64"): "macos-27-arm64", ("Linux", "x86_64"): "linux-amd64",
        ("Linux", "aarch64"): "linux-arm64"}
RUNTIMES = ("codex", "claude")
OUTPUT_CAP = 65536
FIRST_RUN_CASES = {"codex": ("codex",), "claude": ("claude",), "both": ("codex", "claude")}
# Fail-closed installer state cases; each runs in its own isolated HOME.
REFUSALS = {
    "foreign": "install_error: foreign binary preserved",
    "modified": "install_error: modified binary preserved",
    "unsafe": "install_error: unsafe destination ownership, permissions, ACL, or type",
}


def version_key(tag):
    match = TAG_PATTERN.match(tag or "")
    return tuple(int(part) for part in match.groups()) if match else None


def expected_outcome(code, timed_out, expect):
    """A refusal case passes only by failing; a timeout never passes."""
    return not timed_out and (code != 0) == (expect == "failure")


def load_candidate(path, label):
    """Read one reviewed candidate descriptor; any deviation is a usage error."""
    if not path.is_absolute() or path.is_symlink() or not path.is_file():
        raise ValueError(f"{label} must be an absolute regular file")
    raw = path.read_bytes()
    value = json.loads(raw)
    tag = value.get("tag")
    version = tag[1:] if isinstance(tag, str) else ""
    expected_assets = {"SHA256SUMS"} | {f"axiom-{version}-{row}.tar.gz" for row in ROWS.values()}
    assets = value.get("assets")
    if (value.get("schema") != CANDIDATE_SCHEMA or version_key(tag) is None
            or not re.fullmatch(r"[0-9a-f]{40}", value.get("sourceRevision", ""))
            or not isinstance(value.get("releaseId"), int)
            or not str(value.get("installerURL", "")).startswith("https://raw.githubusercontent.com/rgomids/axiom/")
            or not re.fullmatch(r"[0-9a-f]{64}", value.get("installerSHA256", ""))
            or not isinstance(assets, dict) or set(assets) != expected_assets
            or not all(re.fullmatch(r"[0-9a-f]{64}", str(item)) for item in assets.values())):
        raise ValueError(f"{label} descriptor is not a valid exact RC candidate")
    return value, hashlib.sha256(raw).hexdigest()


def valid_provenance(event, candidate):
    value = event.get("provenance", {})
    return (event.get("status") == "success" and value.get("product") == "Axiom"
            and value.get("version") == candidate["tag"][1:]
            and value.get("revision") == candidate["sourceRevision"][:12]
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


def bounded_run(label, argv, env, cwd, output, timeout):
    """Run one process group with a deadline and a streaming output cap.

    Returns (observation, retained text). Any capture error stops and reaps the
    group, then re-raises with the observed exit code attached.
    """
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
                                       env=env, cwd=cwd, start_new_session=True)
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
                    keep = chunk[:max(0, OUTPUT_CAP - len(retained))]
                    stream.write(keep)
                    retained.extend(keep)
        try:
            code = process.wait(timeout=max(0.01, deadline - time.monotonic()))
        except subprocess.TimeoutExpired:
            timed_out = True
            stop_group(process)
            code = process.wait()
    except BaseException as error:
        if process is not None:
            stop_group(process)
            process.wait()
        error.exit_code = None if process is None else process.returncode
        raise
    finally:
        if process is not None and process.stdout is not None:
            process.stdout.close()
    observation = {"label": label, "argv": argv, "attempt": 1, "exitCode": code,
                   "timedOut": timed_out, "durationSeconds": round(time.time() - started, 3),
                   "outputSHA256": hasher.hexdigest(), "outputBytes": size,
                   "retainedSHA256": digest(output.read_bytes()), "outputTruncated": size > OUTPUT_CAP}
    return observation, bytes(retained).decode("utf-8", "replace")


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


def discover_runtimes():
    """Locate Runtime executables on the operator PATH without executing them."""
    found = {}
    for name in RUNTIMES:
        located = shutil.which(name)
        if not located:
            found[name] = {"status": "unavailable"}
            continue
        real = Path(located).resolve()
        if not real.is_file():
            found[name] = {"status": "unavailable", "located": located}
            continue
        found[name] = {"status": "located-not-executed", "located": located, "realPath": str(real),
                       "sha256": file_digest(real), "bytes": real.stat().st_size}
    return found


def block_device(path):
    """Name the macOS block device holding path, in process (no subprocess)."""
    import ctypes
    import ctypes.util
    libc = ctypes.CDLL(ctypes.util.find_library("c"))
    libc.devname.restype = ctypes.c_char_p
    libc.devname.argtypes = [ctypes.c_int32, ctypes.c_uint16]
    name = libc.devname(os.stat(path).st_dev, stat.S_IFBLK)
    if not name or name == b"??":
        raise ValueError("cannot identify the Evidence filesystem device")
    return "/dev/" + name.decode()


def fields(text, prefix):
    return dict(line.split("=", 1) for line in text.splitlines() if line.startswith(prefix) and "=" in line)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--version", required=True, help="exact RC pin vX.Y.Z-rc.N; never a channel")
    parser.add_argument("--candidate", required=True, type=Path, help="reviewed candidate descriptor")
    parser.add_argument("--prior-candidate", type=Path,
                        help="older published RC descriptor used only as owned-upgrade/downgrade precondition")
    parser.add_argument("--evidence-dir", required=True, type=Path)
    parser.add_argument("--execute-install", action="store_true",
                        help="execute only the separately authorized local stage")
    parser.add_argument("--approved-envelope-sha256",
                        help="exact digest explicitly authorized by the human; required for execution")
    args = parser.parse_args()
    if version_key(args.version) is None:
        parser.error("--version must be an exact vX.Y.Z-rc.N pin")
    try:
        candidate, candidate_sha = load_candidate(args.candidate, "--candidate")
        prior, prior_sha = (load_candidate(args.prior_candidate, "--prior-candidate")
                            if args.prior_candidate else (None, None))
    except (OSError, ValueError) as error:
        parser.error(str(error))
    if candidate["tag"] != args.version:
        parser.error("--version differs from the candidate descriptor")
    if prior is not None and not version_key(prior["tag"]) < version_key(candidate["tag"]):
        parser.error("--prior-candidate must be an older RC")
    tag, revision = candidate["tag"], candidate["sourceRevision"]
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
        prior_manifest = json.loads((root / "manifest.json").read_text())
        if prior_manifest.get("mode") != "plan-only" or prior_manifest.get("observations") != []:
            parser.error("Evidence target is not an unexecuted plan")
        if digest((root / "envelope.json").read_bytes()) != args.approved_envelope_sha256:
            parser.error("envelope drift; obtain fresh human authority")
    elif os.path.lexists(root):
        parser.error("Evidence target already exists; choose a new directory")
    os.umask(0o077)
    if not args.execute_install:
        root.mkdir(mode=0o700)
    artifacts = root / "artifacts"
    homes = root / "homes"
    runtime_paths = root / "runtime-path"
    runtimes = discover_runtimes()
    manifest = {
        "schemaVersion": 2, "rcTag": tag, "sourceRevision": revision, "releaseId": candidate["releaseId"],
        "candidateSHA256": candidate_sha, "priorCandidate": prior and prior["tag"],
        "observedAtUnix": int(time.time()), "scope": "local-install-and-first-run-stage-only",
        "host": {"os": platform.system(), "architecture": platform.machine(),
                 "kernel": platform.release(), "macOSVersion": platform.mac_ver()[0]},
        "mode": "execute-install" if args.execute_install else "plan-only",
        "observations": [], "t24": "blocked", "t25": "blocked",
        "humanDecision": "PENDING", "runtimeInvocation": "not performed",
        "providerMutation": "not performed", "executionIDs": [],
        "cleanupDisposition": "Evidence and isolated HOMEs retained; no deletion",
        "limitations": ["No complete native acceptance row",
                        "No real Codex/Claude invocation, GitHub journey or dogfood graph",
                        "No Runtime/Provider failure, recovery or native S7 suite in this stage"],
    }

    def home_env(home, path=SAFE_PATH, project_roots=False):
        env = {"HOME": str(home), "PATH": path, "LANG": "C", "LC_ALL": "C", "TMPDIR": str(root / "tmp")}
        if project_roots:
            env.update({"LINGO_STATE_ROOT": str(home / "state"), "LINGO_PROJECTS_ROOT": str(home / "projects")})
        return env

    curl = ["/usr/bin/curl", "--disable", "--proto", "=https", "--proto-redir", "=https", "--tlsv1.2", "-fsSL",
            "--connect-timeout", "20", "--max-time", "60", "--max-filesize", str(OUTPUT_CAP)]
    api = "https://api.github.com/repos/rgomids/axiom"
    main_home = homes / "main"
    binary = main_home / ".local/bin/axiom"
    installer = artifacts / "remote-install.sh"

    def install(home, selected):
        return ["/bin/sh", str(installer), "--version", selected]

    # Every process below is materialized in the envelope; execution runs exactly this list.
    steps = []

    def step(label, argv, env, expect="success", check=None, fixture=None):
        steps.append({"label": label, "argv": argv, "env": env, "expect": expect,
                      "check": check, "fixture": fixture})

    system = platform.system()
    if system == "Darwin":
        step("filesystem", ["/usr/sbin/diskutil", "info", "-plist", block_device(parent)], home_env(main_home))
    else:
        step("filesystem", ["/usr/bin/findmnt", "-n", "-o", "FSTYPE,SOURCE,TARGET", "--target", str(parent)],
             home_env(main_home))
    step("published-metadata", curl + [f"{api}/releases/tags/{tag}"], home_env(main_home), check="release")
    step("published-latest", curl + [f"{api}/releases/latest"], home_env(main_home), check="latest")
    step("published-tag", curl + [f"{api}/git/ref/tags/{tag}"], home_env(main_home), check="tag")
    if prior is not None:
        step("prior-published-metadata", curl + [f"{api}/releases/tags/{prior['tag']}"], home_env(main_home),
             check="prior-release")
    step("download-installer", curl + [candidate["installerURL"], "-o", str(installer)], home_env(main_home),
         check="installer")
    main_env = home_env(main_home, project_roots=True)
    step("install", install(main_home, tag), main_env, check="installed")
    step("version", [str(binary), "--json", "version"], main_env, check="provenance")
    step("first-run-neither", [str(binary), "--json", "first-run"], main_env, check="first-run:")
    step("first-run-neither-rerun", [str(binary), "--json", "first-run"], main_env, check="first-run:")
    step("reinstall", install(main_home, tag), main_env, check="unchanged")
    unavailable_cases = []
    for case, names in FIRST_RUN_CASES.items():
        if any(runtimes[name]["status"] != "located-not-executed" for name in names):
            unavailable_cases.append(case)
            continue
        case_home = homes / f"first-run-{case}"
        env = home_env(case_home, f"{runtime_paths / case}:{SAFE_PATH}")
        fixture = {"action": "symlink Runtime executables into a private PATH directory; never executed",
                   "directory": str(runtime_paths / case),
                   "links": {name: runtimes[name]["realPath"] for name in names}}
        step(f"first-run-{case}", [str(binary), "--json", "first-run"], env,
             check="first-run:" + ",".join(names), fixture=fixture)
        if case == "both":
            step("first-run-both-rerun", [str(binary), "--json", "first-run"], env,
                 check="first-run-idempotent:" + ",".join(names))
    for case, message in REFUSALS.items():
        case_home = homes / case
        env = home_env(case_home)
        fixture = {"foreign": "write a foreign 0700 file at HOME/.local/bin/axiom with no receipt",
                   "modified": "append one byte to the installed binary after a clean install",
                   "unsafe": "create HOME/.local/bin with mode 0777"}[case]
        if case == "modified":
            step("modified-clean-install", install(case_home, tag), env, check="installed")
        step(f"{case}-refused", install(case_home, tag), env, expect="failure",
             check="refused:" + message, fixture={"action": fixture})
    if prior is not None:
        env = home_env(homes / "upgrade")
        step("upgrade-prior-install", install(homes / "upgrade", prior["tag"]), env, check="prior-installed")
        step("upgrade-owned", install(homes / "upgrade", tag), env, check="upgraded")
        step("upgrade-version", [str(homes / "upgrade/.local/bin/axiom"), "--json", "version"], env,
             check="provenance")
        step("downgrade-refused", install(homes / "upgrade", prior["tag"]), env, expect="failure",
             check="refused:install_error: owned upgrade refused")
    else:
        manifest["limitations"].append("No prior RC descriptor: owned upgrade and downgrade refusal not planned")
    if unavailable_cases:
        manifest["limitations"].append("First-run cases not planned (Runtime not on operator PATH): "
                                       + ", ".join(unavailable_cases))

    envelope = {"action": "isolated local remote-install, first-run matrix and installer-state stage",
                "target": str(root), "rcTag": tag, "sourceRevision": revision,
                "releaseId": candidate["releaseId"], "candidateSHA256": candidate_sha,
                "priorCandidate": prior and {"tag": prior["tag"], "sha256": prior_sha},
                "installerURL": candidate["installerURL"], "installerSHA256": candidate["installerSHA256"],
                "collectorSHA256": file_digest(Path(__file__).resolve()),
                "assetSHA256": candidate["assets"], "row": ROWS.get((system, platform.machine())),
                "runtimeExecutables": runtimes, "steps": steps,
                "timeoutSecondsPerCommand": 600, "maximumStageAttempts": 1,
                "networkRetries": "installer-owned bounded retries; no harness retry",
                "outputCapBytes": OUTPUT_CAP,
                "allowedEffects": ["HTTPS public release reads", "writes inside Evidence target",
                                   "installer-owned temporary download directories under Evidence tmp",
                                   "symlinks to Runtime executables inside Evidence runtime-path"],
                "forbiddenEffects": ["Runtime invocation", "GitHub mutation", "credential access",
                                     "host HOME, installation or skill roots", "release publication/promotion",
                                     "deletion of any file outside Evidence target"],
                "cleanupOwnership": "operator; retained isolated files only; no automatic deletion",
                "runtimeProfile": "none; Runtime executables are detected, never executed",
                "retain": "commands/exits, bounded CLI output, hashes, receipts, inventories, limitations"}
    envelope_bytes = (json.dumps(envelope, indent=2) + "\n").encode()
    if args.execute_install and digest(envelope_bytes) != args.approved_envelope_sha256:
        parser.error("current execution differs from approved envelope; review again")
    if not args.execute_install:
        (root / "envelope.json").write_bytes(envelope_bytes)
    manifest["envelopeSHA256"] = digest((root / "envelope.json").read_bytes())
    manifest["plannedSteps"] = [item["label"] for item in steps]

    def save():
        (root / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n")
        (root / "acceptance.md").write_text(
            f"# {tag}: local installation and first-run stage\n\n"
            f"Revision: {revision}. Mode: {manifest['mode']}. "
            f"Stage: {manifest.get('stageResult', 'pending')}.\n\n"
            "T24: blocked. T25: blocked. Human decision: PENDING.\n\n"
            "See manifest.json for actual observations and limitations. "
            "Successful steps do not establish a complete platform row.\n")

    def run(label, argv, command_env, cwd, expect="success", timeout=600):
        output = artifacts / (label + ".txt")
        try:
            observation, text = bounded_run(label, argv, command_env, cwd, output, timeout)
        except BaseException as error:
            manifest["observations"].append({"label": label, "argv": argv, "attempt": 1,
                                             "exitCode": getattr(error, "exit_code", None),
                                             "result": "capture-failed; group stopped; no next command"})
            raise
        observation.update({"expected": expect, "retainedOutput": str(output.relative_to(root))})
        manifest["observations"].append(observation)
        save()
        if not expected_outcome(observation["exitCode"], observation["timedOut"], expect):
            raise RuntimeError(f"{label} failed: exit {observation['exitCode']}, expected {expect}; "
                               "inspect retained output")
        return text

    def receipt_of(home):
        return fields((home / ".local/state/axiom/install/installation.receipt").read_text(), "")

    def check_installed(label, text, home, selected, status="installed"):
        installed = fields(text, "install_")
        asset = installed.get("install_asset")
        receipt = receipt_of(home)
        installed_binary = home / ".local/bin/axiom"
        if (f"install_status={status}" not in text.splitlines()
                or asset != f"axiom-{selected['tag'][1:]}-{ROWS[row]}.tar.gz"
                or installed.get("install_tag") != selected["tag"] or asset not in selected["assets"]
                or installed.get("install_asset_sha256") != selected["assets"][asset]
                or receipt.get("archiveSha256") != selected["assets"][asset]
                or receipt.get("version") != selected["tag"][1:]
                or receipt.get("revision") != selected["sourceRevision"][:12]
                or receipt.get("sha256") != file_digest(installed_binary)):
            raise RuntimeError(f"{label}: installed archive/receipt/binary identity mismatch")
        return {"tag": selected["tag"], "asset": asset, "assetSHA256": selected["assets"][asset],
                "row": installed.get("install_row"), "receiptSHA256": file_digest(
                    home / ".local/state/axiom/install/installation.receipt"),
                "binarySHA256": file_digest(installed_binary)}

    def check_first_run(label, text, home, names, idempotent=False):
        result = json.loads(text)
        report = result.get("firstRun", {})
        states = {entry.get("runtime"): entry for entry in report.get("runtimes", [])}
        expected_state = "already_configured" if idempotent else "configured"
        if (result.get("status") != "success" or report.get("detected") != len(names)
                or report.get("failed") != 0 or set(states) != set(RUNTIMES)):
            raise RuntimeError(f"{label}: first-run result mismatch")
        for name in RUNTIMES:
            entry = states[name]
            if name in names and (not entry.get("present") or entry.get("state") != expected_state):
                raise RuntimeError(f"{label}: {name} not {expected_state}")
            if name not in names and (entry.get("present") or entry.get("state") != "absent"):
                raise RuntimeError(f"{label}: {name} unexpectedly present")
        # User-global scope under the isolated HOME: default skill roots, no override.
        scope = {}
        for name, skill_root in (("codex", home / ".agents/skills"), ("claude", home / ".claude/skills")):
            files = sorted(skill_root.glob("axiom-*/SKILL.md")) if skill_root.is_dir() else []
            if (len(files) == 5) != (name in names):
                raise RuntimeError(f"{label}: {name} user-global skill set mismatch")
            scope[name] = {str(path.relative_to(home)): file_digest(path) for path in files}
        return {"report": report, "userGlobalSkills": scope}

    def apply_fixture(item):
        fixture = item["fixture"]
        if not fixture:
            return
        if "links" in fixture:
            directory = Path(fixture["directory"])
            directory.mkdir(mode=0o700, parents=True)
            for name, target in fixture["links"].items():
                (directory / name).symlink_to(target)
            return
        case = item["label"].split("-", 1)[0]
        home = homes / case
        bin_dir = home / ".local/bin"
        if case == "foreign":
            bin_dir.mkdir(mode=0o700, parents=True)
            (bin_dir / "axiom").write_bytes(b"foreign content, not an Axiom installation\n")
            (bin_dir / "axiom").chmod(0o700)
        elif case == "modified":
            with (bin_dir / "axiom").open("ab") as stream:
                stream.write(b"\0")
        elif case == "unsafe":
            bin_dir.mkdir(mode=0o700, parents=True)
            bin_dir.chmod(0o777)

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
        if any(shutil.which(name, path=SAFE_PATH) for name in RUNTIMES):
            raise RuntimeError("Runtime executable on isolated PATH; stage refused")
        row = (platform.system(), platform.machine())
        if row not in ROWS:
            raise RuntimeError("unsupported native host")
        if row[0] == "Darwin" and platform.mac_ver()[0] != "27.0":
            raise RuntimeError("macOS 27.0 required")
        artifacts.mkdir(mode=0o700)
        (root / "tmp").mkdir(mode=0o700)
        homes.mkdir(mode=0o700)
        for item in steps:
            home = Path(item["env"]["HOME"])
            if not home.exists():
                home.mkdir(mode=0o700)
        manifest["initialRootInventory"] = snapshot(main_home)
        results = manifest.setdefault("results", {})
        for item in steps:
            label, check = item["label"], item["check"] or ""
            apply_fixture(item)
            home = Path(item["env"]["HOME"])
            before = snapshot(home) if item["expect"] == "failure" or check == "unchanged" else None
            text = run(label, item["argv"], item["env"], str(home), item["expect"])
            if before is not None and snapshot(home) != before:
                raise RuntimeError(f"{label}: isolated installation inventory changed")
            if check == "release" or check == "prior-release":
                selected = candidate if check == "release" else prior
                metadata = json.loads(text)
                observed = {entry["name"]: entry.get("digest") for entry in metadata.get("assets", [])}
                if (metadata.get("id") != selected["releaseId"] or metadata.get("tag_name") != selected["tag"]
                        or metadata.get("target_commitish") != selected["sourceRevision"]
                        or metadata.get("immutable") is not True or metadata.get("prerelease") is not True
                        or metadata.get("draft") is not False
                        or observed != {name: "sha256:" + value for name, value in selected["assets"].items()}):
                    raise RuntimeError(f"{label}: published immutable candidate/asset metadata mismatch")
                results[label] = {key: metadata[key] for key in
                                  ("id", "tag_name", "target_commitish", "immutable", "prerelease", "draft")}
                results[label]["assets"] = observed
            elif check == "latest":
                latest = json.loads(text)
                if latest.get("tag_name") == tag or latest.get("prerelease") is not False:
                    raise RuntimeError("latest release is not a stable release other than the candidate")
                results[label] = {"tag_name": latest.get("tag_name"), "prerelease": latest.get("prerelease")}
            elif check == "tag":
                value = json.loads(text)
                if value.get("object", {}).get("type") != "commit" or value["object"].get("sha") != revision:
                    raise RuntimeError("published lightweight tag revision mismatch")
            elif check == "installer":
                results[label] = digest(installer.read_bytes())
                if results[label] != candidate["installerSHA256"]:
                    raise RuntimeError("remote installer differs from exact RC source; review required")
            elif check == "installed":
                results[label] = check_installed(label, text, home, candidate)
            elif check == "prior-installed":
                results[label] = check_installed(label, text, home, prior)
            elif check == "upgraded":
                results[label] = check_installed(label, text, home, candidate, "upgraded")
            elif check == "unchanged":
                if "install_status=unchanged" not in text:
                    raise RuntimeError("reinstall no-op not observed")
            elif check == "provenance":
                event = json.loads(text)
                if not valid_provenance(event, candidate):
                    raise RuntimeError(f"{label}: installed version/source provenance mismatch")
                results[label] = event["provenance"]
            elif check.startswith("first-run"):
                names = tuple(name for name in check.split(":", 1)[1].split(",") if name)
                results[label] = check_first_run(label, text, home, names, check.startswith("first-run-idempotent"))
                if (check.startswith("first-run-idempotent") and results[label]["userGlobalSkills"]
                        != results["first-run-both"]["userGlobalSkills"]):
                    raise RuntimeError(f"{label}: idempotent first-run changed skill content")
            elif check.startswith("refused:"):
                if check.split(":", 1)[1] not in text:
                    raise RuntimeError(f"{label}: expected fail-closed message not observed")
                results[label] = {"refused": True, "inventoryUnchanged": True}
            elif label == "filesystem":
                # Native semantics only: APFS on macOS, ext4 on Linux; never a container overlay.
                probe = root / "tmp" / "AxiomCase"
                probe.write_bytes(b"x")
                insensitive = (root / "tmp" / "axiomcase").exists()
                probe.unlink()
                if row[0] == "Darwin":
                    kind = "APFS" if "<string>apfs</string>" in text.lower() else "unexpected"
                    valid = kind == "APFS" and insensitive
                else:
                    kind = (text.split() or ["unexpected"])[0]
                    valid = kind == "ext4" and not insensitive
                results[label] = {"filesystem": kind, "caseInsensitive": insensitive}
                if not valid:
                    raise RuntimeError("filesystem is not the native supported row filesystem")
            save()
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
