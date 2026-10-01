#!/usr/bin/env python3
"""Execute exactly one reviewed phase of a T24 authority envelope; never acceptance.

An envelope (JSON) fixes every argv, cwd, environment, timeout, declared effect
and Evidence path. One invocation runs one phase, only when the human-authorized
envelope SHA-256 matches, and only with the human-approved values for the
parameters that phase names (for example an Axiom preview digest shown by an
earlier phase). Nothing is inherited from the caller environment. The executor
never retries automatically; a later attempt needs the failed attempt's ledger
and is refused while any external effect is ambiguous. T24/T25 are never marked
complete and the human decision stays PENDING.
"""
import argparse
import importlib.util
import json
import os
from pathlib import Path
import re
import signal
import sys
import time

_spec = importlib.util.spec_from_file_location("s9_rc_evidence", Path(__file__).with_name("s9-rc-evidence.py"))
collector = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(collector)

ENVELOPE_SCHEMA = "axiom-s9-rc-envelope/v1"
PLACEHOLDER = re.compile(r"\{\{([a-zA-Z][a-zA-Z0-9]*)\}\}")
SECRET_NAME = re.compile(r"TOKEN|SECRET|PASSWORD|PASSWD|API_?KEY|CREDENTIAL|PRIVATE_KEY", re.I)
SECRET_VALUE = re.compile(r"gh[pousr]_[A-Za-z0-9]{20,}|github_pat_|sk-ant-|sk-[A-Za-z0-9]{20,}|AKIA[0-9A-Z]{16}")
MAX_TIMEOUT = 3600


def fail(message):
    raise SystemExit(f"s9-rc-envelope: {message}")


def placeholders(value):
    if isinstance(value, str):
        return set(PLACEHOLDER.findall(value))
    if isinstance(value, list):
        return set().union(*(placeholders(item) for item in value)) if value else set()
    if isinstance(value, dict):
        return set().union(*(placeholders(item) for item in value.values())) if value else set()
    return set()


def substitute(value, approved):
    if isinstance(value, str):
        return PLACEHOLDER.sub(lambda match: approved[match.group(1)], value)
    if isinstance(value, list):
        return [substitute(item, approved) for item in value]
    if isinstance(value, dict):
        return {key: substitute(item, approved) for key, item in value.items()}
    return value


def tooling_digests():
    """The executor and the collector module it runs are part of the authority."""
    here = Path(__file__).resolve()
    return {"s9-rc-envelope.py": collector.file_digest(here),
            "s9-rc-evidence.py": collector.file_digest(here.with_name("s9-rc-evidence.py"))}


def validate(envelope):
    """Structural checks that make an envelope reviewable and fail closed."""
    required = ("id", "purpose", "rcTag", "sourceRevision", "candidateSHA256", "toolingSHA256", "phases", "maxAttempts",
                "localEffects", "externalEffects", "forbiddenEffects", "cleanup", "evidencePaths",
                "secretsBoundary", "runtime", "provider")
    if envelope.get("schema") != ENVELOPE_SCHEMA or any(key not in envelope for key in required):
        fail("envelope schema or required field missing")
    if envelope["toolingSHA256"] != tooling_digests():
        fail("executor/collector bytes differ from the envelope; review again")
    if not isinstance(envelope["maxAttempts"], int) or not 1 <= envelope["maxAttempts"] <= 3:
        fail("maxAttempts must be an integer between 1 and 3")
    parameters = envelope.get("parameters", {})
    for name, rule in parameters.items():
        re.compile(rule["pattern"])
    phase_ids = [phase.get("id") for phase in envelope["phases"]]
    if len(phase_ids) != len(set(phase_ids)) or not all(phase_ids):
        fail("phase identifiers must be unique")
    for phase in envelope["phases"]:
        if phase.get("authority") not in {"local", "runtime", "provider", "runtime+provider"}:
            fail(f"{phase['id']}: unknown authority class")
        labels = [step.get("label") for step in phase["steps"]]
        if len(labels) != len(set(labels)) or not all(re.fullmatch(r"[a-z0-9][a-z0-9-]*", label or "") for label in labels):
            fail(f"{phase['id']}: step labels must be unique lowercase identifiers")
        for step in phase["steps"]:
            argv, env, cwd = step.get("argv"), step.get("env"), step.get("cwd")
            if (not argv or not all(isinstance(item, str) for item in argv)
                    or not Path(argv[0]).is_absolute() or not Path(cwd or "").is_absolute()
                    or not isinstance(env, dict)):
                fail(f"{step.get('label')}: argv[0] and cwd must be absolute; env explicit")
            if not isinstance(step.get("timeoutSeconds"), int) or not 1 <= step["timeoutSeconds"] <= MAX_TIMEOUT:
                fail(f"{step['label']}: timeoutSeconds out of bounds")
            if step.get("expect", "success") not in {"success", "failure"}:
                fail(f"{step['label']}: expect must be success or failure")
            for name, value in env.items():
                if SECRET_NAME.search(name) or SECRET_VALUE.search(str(value)):
                    fail(f"{step['label']}: credential-shaped environment entry refused")
            if SECRET_VALUE.search(json.dumps(argv)):
                fail(f"{step['label']}: credential-shaped argument refused")
            effects = step.get("effects", {})
            if set(effects) - {"local", "external"}:
                fail(f"{step['label']}: effects must be local/external lists")
            unknown = placeholders([argv, env, cwd, step.get("requireOutput", [])]) - set(parameters)
            if unknown:
                fail(f"{step['label']}: undeclared parameter {sorted(unknown)}")


def json_field(text, path):
    for candidate in (text, *reversed(text.strip().splitlines())):
        try:
            value = json.loads(candidate)
            break
        except ValueError:
            continue
    else:
        return None
    for part in path.split("."):
        if isinstance(value, list) and part.isdigit() and int(part) < len(value):
            value = value[int(part)]
        elif isinstance(value, dict) and part in value:
            value = value[part]
        else:
            return None
    return value


def ambiguous_external(manifest):
    """Steps with declared external effects whose outcome was not observed."""
    ledger = manifest.get("effects", [])
    return [row["label"] for row in ledger if row.get("external") and row.get("status") == "unknown"]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--envelope", required=True, type=Path)
    parser.add_argument("--candidate", required=True, type=Path)
    parser.add_argument("--phase", required=True)
    parser.add_argument("--evidence-dir", type=Path)
    parser.add_argument("--plan", action="store_true", help="validate and print the envelope digest only")
    parser.add_argument("--approved-envelope-sha256")
    parser.add_argument("--approve", action="append", default=[], metavar="NAME=VALUE")
    parser.add_argument("--attempt", type=int, default=1)
    parser.add_argument("--previous-attempt-dir", type=Path)
    args = parser.parse_args()
    if not args.envelope.is_absolute() or args.envelope.is_symlink() or not args.envelope.is_file():
        fail("--envelope must be an absolute regular file")
    raw = args.envelope.read_bytes()
    envelope_sha = collector.digest(raw)
    envelope = json.loads(raw)
    validate(envelope)
    try:
        candidate, candidate_sha = collector.load_candidate(args.candidate, "--candidate")
    except (OSError, ValueError) as error:
        fail(str(error))
    if (candidate["tag"], candidate["sourceRevision"], candidate_sha) != (
            envelope["rcTag"], envelope["sourceRevision"], envelope["candidateSHA256"]):
        fail("envelope is not bound to this exact candidate descriptor")
    phase = next((item for item in envelope["phases"] if item["id"] == args.phase), None)
    if phase is None:
        fail("unknown phase")
    needed = placeholders([[step["argv"], step["env"], step["cwd"], step.get("requireOutput", [])]
                           for step in phase["steps"]])
    if args.plan:
        print(json.dumps({"envelope": envelope["id"], "envelopeSHA256": envelope_sha, "phase": phase["id"],
                          "authority": phase["authority"], "requiredApprovals": sorted(needed),
                          "steps": [step["label"] for step in phase["steps"]]}, indent=2))
        return 0
    if args.approved_envelope_sha256 != envelope_sha:
        fail("envelope drift or missing authority; obtain fresh human authority for this exact digest")
    approved = {}
    for item in args.approve:
        name, _, value = item.partition("=")
        rule = envelope.get("parameters", {}).get(name)
        if rule is None or name in approved or not re.fullmatch(rule["pattern"], value):
            fail(f"approval {name!r} is undeclared, duplicated or malformed")
        approved[name] = value
    if set(approved) != needed:
        fail(f"phase {phase['id']} needs exactly the approvals {sorted(needed)}")
    if not 1 <= args.attempt <= envelope["maxAttempts"]:
        fail("attempt exceeds the envelope maximum")
    if args.attempt > 1:
        previous = args.previous_attempt_dir
        if previous is None or not (previous / "manifest.json").is_file():
            fail("a later attempt requires the previous attempt ledger")
        prior = json.loads((previous / "manifest.json").read_text())
        if (prior.get("envelopeSHA256") != envelope_sha or prior.get("phase") != phase["id"]
                or prior.get("attempt") != args.attempt - 1 or prior.get("stageResult") != "failed"):
            fail("previous attempt is not the failed preceding attempt of this phase")
        pending = ambiguous_external(prior)
        if pending:
            fail(f"ambiguous external effect in {pending}; reconcile before any retry")
    elif args.previous_attempt_dir is not None:
        fail("--previous-attempt-dir is only valid for a later attempt")
    if args.evidence_dir is None or not args.evidence_dir.is_absolute():
        fail("--evidence-dir must be absolute")
    parent = args.evidence_dir.parent.resolve(strict=True)
    root = parent / args.evidence_dir.name
    if os.path.lexists(root):
        fail("Evidence target already exists; choose a new directory")
    os.umask(0o077)
    root.mkdir(mode=0o700)
    artifacts = root / "artifacts"
    artifacts.mkdir(mode=0o700)
    manifest = {"schemaVersion": 1, "envelope": envelope["id"], "envelopeSHA256": envelope_sha,
                "rcTag": envelope["rcTag"], "sourceRevision": envelope["sourceRevision"],
                "phase": phase["id"], "authority": phase["authority"], "attempt": args.attempt,
                "maxAttempts": envelope["maxAttempts"], "approvals": approved,
                "startedAtUnix": int(time.time()), "observations": [], "effects": [], "captures": {},
                "t24": "in-progress; not complete", "t25": "blocked", "humanDecision": "PENDING",
                "cleanupOwnership": envelope["cleanup"], "limitations": []}

    def save():
        (root / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n")

    interruption = {"signal": signal.SIGINT}

    def interrupt(signum, _frame):
        interruption["signal"] = signum
        raise KeyboardInterrupt

    prior_termination = signal.signal(signal.SIGTERM, interrupt)
    save()
    try:
        for step in phase["steps"]:
            argv, env, cwd = (substitute(step[key], approved) for key in ("argv", "env", "cwd"))
            effects = step.get("effects", {})
            ledger = {"label": step["label"], "local": effects.get("local", []),
                      "external": effects.get("external", []), "status": "unknown"}
            manifest["effects"].append(ledger)
            save()
            output = artifacts / (step["label"] + ".txt")
            try:
                observation, text = collector.bounded_run(step["label"], argv, env, cwd, output,
                                                          step["timeoutSeconds"])
            except BaseException as error:
                manifest["observations"].append({"label": step["label"], "argv": argv, "attempt": args.attempt,
                                                 "exitCode": getattr(error, "exit_code", None),
                                                 "result": "capture-failed; group stopped; effects unknown"})
                raise
            observation.update({"attempt": args.attempt, "cwd": cwd, "expected": step.get("expect", "success"),
                                "retainedOutput": str(output.relative_to(root))})
            manifest["observations"].append(observation)
            # A timeout leaves effects unknown; an observed exit settles the ledger row.
            ledger["status"] = "unknown" if observation["timedOut"] else f"exit-{observation['exitCode']}"
            for name, path in step.get("capture", {}).items():
                manifest["captures"][f"{step['label']}.{name}"] = json_field(text, path)
            save()
            if not collector.expected_outcome(observation["exitCode"], observation["timedOut"],
                                              step.get("expect", "success")):
                raise RuntimeError(f"{step['label']}: exit {observation['exitCode']}, "
                                   f"expected {step.get('expect', 'success')}")
            missing = [needle for needle in substitute(step.get("requireOutput", []), approved) if needle not in text]
            if missing:
                raise RuntimeError(f"{step['label']}: required output not observed: {missing}")
        manifest["stageResult"] = "phase-steps-confirmed; T24 not complete"
        save()
        print(f"phase={phase['id']} result=confirmed evidence={root}")
        return 0
    except KeyboardInterrupt:
        manifest["stageResult"] = "interrupted; effects require inspection"
        manifest["interruptionSignal"] = interruption["signal"]
        save()
        print("phase interrupted; retained effects require inspection", file=sys.stderr)
        return 128 + interruption["signal"]
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
