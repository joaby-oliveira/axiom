"""Offline executor safety tests; fixtures never establish RC acceptance Evidence."""
import contextlib
import importlib.util
import io
import json
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("executor", Path(__file__).with_name("s9-rc-envelope.py"))
executor = importlib.util.module_from_spec(spec)
spec.loader.exec_module(executor)
CANDIDATE = (Path(__file__).resolve().parents[1]
             / "docs/specifications/004-mvp-v1-baseline/evidence-s9-rc2/candidate-v0.1.2-rc.2.json")


def envelope(steps, **extra):
    candidate, candidate_sha = executor.collector.load_candidate(CANDIDATE, "test")
    value = {"schema": executor.ENVELOPE_SCHEMA, "id": "T", "purpose": "offline fixture",
             "rcTag": candidate["tag"], "sourceRevision": candidate["sourceRevision"],
             "candidateSHA256": candidate_sha, "toolingSHA256": executor.tooling_digests(),
             "runtime": "none", "provider": "none",
             "maxAttempts": 2, "localEffects": [], "externalEffects": [], "forbiddenEffects": [],
             "cleanup": "fixture", "evidencePaths": [], "secretsBoundary": "none",
             "parameters": {"digest": {"pattern": "[0-9a-f]{64}"}},
             "phases": [{"id": "P1", "authority": "local", "steps": steps}]}
    value.update(extra)
    return value


def step(label, argv, **extra):
    value = {"label": label, "argv": argv, "cwd": "/", "env": {"PATH": "/usr/bin:/bin"}, "timeoutSeconds": 30}
    value.update(extra)
    return value


class CommittedEnvelopes(unittest.TestCase):
    def test_every_committed_envelope_and_phase_validates(self):
        for path in sorted(CANDIDATE.parent.glob("envelopes/[B-F].json")):
            value = json.loads(path.read_text())
            executor.validate(value)
            for phase in value["phases"]:
                argv = ["executor", "--envelope", str(path), "--candidate", str(CANDIDATE), "--phase", phase["id"], "--plan"]
                with patch("sys.argv", argv), contextlib.redirect_stdout(io.StringIO()):
                    self.assertEqual(executor.main(), 0, (path.name, phase["id"]))

    def test_projection_never_precedes_its_transition(self):
        # A projection projects the latest transition; start (revision 1) has none.
        value = json.loads((CANDIDATE.parent / "envelopes/E.json").read_text())
        for phase in value["phases"]:
            for step in phase["steps"]:
                if "reconcile" in step["argv"]:
                    revision = int(step["argv"][step["argv"].index("--expected-revision") + 1])
                    self.assertGreater(revision, 1, step["label"])


class EnvelopeExecutor(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.base = Path(self.directory.name)

    def tearDown(self):
        self.directory.cleanup()

    def write(self, value):
        path = self.base / "envelope.json"
        path.write_text(json.dumps(value))
        return path, executor.collector.digest(path.read_bytes())

    def invoke(self, path, *extra, digest=None, evidence="evidence"):
        argv = ["executor", "--envelope", str(path), "--candidate", str(CANDIDATE), "--phase", "P1",
                "--evidence-dir", str(self.base / evidence), *extra]
        if digest:
            argv += ["--approved-envelope-sha256", digest]
        with patch("sys.argv", argv), contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO()):
            return executor.main()

    def manifest(self, name="evidence"):
        return json.loads((self.base / name / "manifest.json").read_text())

    def test_plan_starts_no_process_and_drift_is_refused(self):
        path, digest = self.write(envelope([step("one", ["/bin/echo", "x"])]))
        with patch.object(executor.collector.subprocess, "Popen") as process:
            self.assertEqual(self.invoke(path, "--plan"), 0)
            process.assert_not_called()
        with self.assertRaises(SystemExit):
            self.invoke(path, digest="0" * 64)
        self.assertFalse((self.base / "evidence").exists())

    def test_credential_shaped_environment_and_undeclared_parameters_are_refused(self):
        for bad in (step("one", ["/bin/echo"], env={"GH_TOKEN": "x"}),
                    step("one", ["/bin/echo", "ghp_" + "a" * 36]),
                    step("one", ["/bin/echo", "{{other}}"]),
                    step("one", ["echo"])):
            path, digest = self.write(envelope([bad]))
            with self.assertRaises(SystemExit):
                self.invoke(path, digest=digest)
        self.assertFalse((self.base / "evidence").exists())

    def test_changed_tooling_invalidates_the_envelope(self):
        value = envelope([step("one", ["/bin/echo"])])
        value["toolingSHA256"]["s9-rc-envelope.py"] = "0" * 64
        path, digest = self.write(value)
        with self.assertRaises(SystemExit):
            self.invoke(path, digest=digest)
        self.assertFalse((self.base / "evidence").exists())

    def test_approvals_must_be_exact_and_well_formed(self):
        path, digest = self.write(envelope([step("one", ["/bin/echo", "{{digest}}"])]))
        for approvals in ([], ["--approve", "digest=short"], ["--approve", "other=" + "a" * 64]):
            with self.assertRaises(SystemExit):
                self.invoke(path, *approvals, digest=digest)
        self.assertEqual(self.invoke(path, "--approve", "digest=" + "a" * 64, digest=digest), 0)
        self.assertEqual(self.manifest()["observations"][0]["argv"], ["/bin/echo", "a" * 64])

    def test_required_output_uses_approved_values(self):
        path, digest = self.write(envelope([step("tree", ["/bin/echo", "a" * 64], requireOutput=["{{digest}}"])]))
        self.assertEqual(self.invoke(path, "--approve", "digest=" + "a" * 64, digest=digest), 0)
        self.assertEqual(self.invoke(path, "--approve", "digest=" + "b" * 64, digest=digest, evidence="other"), 1)
        with self.assertRaises(SystemExit):
            self.invoke(path, digest=digest, evidence="third")

    def test_environment_is_not_inherited_and_captures_are_recorded(self):
        path, digest = self.write(envelope([
            step("env", ["/usr/bin/env"], env={"ONLY": "declared"}, requireOutput=["ONLY=declared"]),
            step("json", ["/bin/echo", '{"setup":{"digest":"abc"}}'], capture={"digest": "setup.digest"})]))
        with patch.dict("os.environ", {"LEAK": "host"}):
            self.assertEqual(self.invoke(path, digest=digest), 0)
        record = self.manifest()
        self.assertEqual((self.base / "evidence/artifacts/env.txt").read_text(), "ONLY=declared\n")
        self.assertEqual(record["captures"], {"json.digest": "abc"})
        self.assertEqual([row["status"] for row in record["effects"]], ["exit-0", "exit-0"])
        self.assertEqual(record["humanDecision"], "PENDING")

    def test_expected_refusal_and_unexpected_success_stop_the_phase(self):
        path, digest = self.write(envelope([step("refusal", ["/usr/bin/false"], expect="failure"),
                                            step("unexpected", ["/usr/bin/true"], expect="failure"),
                                            step("never", ["/bin/echo"])]))
        self.assertEqual(self.invoke(path, digest=digest), 1)
        record = self.manifest()
        self.assertEqual([row["label"] for row in record["observations"]], ["refusal", "unexpected"])
        self.assertEqual(record["stageResult"], "failed")

    def test_timeout_with_external_effect_blocks_retry_until_reconciled(self):
        slow = step("post", [sys.executable, "-c", "import time; time.sleep(5)"], timeoutSeconds=1,
                    effects={"external": ["fixture external effect"]})
        path, digest = self.write(envelope([slow]))
        self.assertEqual(self.invoke(path, digest=digest), 1)
        self.assertEqual(self.manifest()["effects"][0]["status"], "unknown")
        with self.assertRaises(SystemExit):
            self.invoke(path, "--attempt", "2", "--previous-attempt-dir", str(self.base / "evidence"),
                        digest=digest, evidence="second")
        self.assertFalse((self.base / "second").exists())

    def test_bounded_retry_after_observed_failure(self):
        path, digest = self.write(envelope([step("fails", ["/usr/bin/false"], effects={"external": ["x"]})]))
        self.assertEqual(self.invoke(path, digest=digest), 1)
        self.assertEqual(self.invoke(path, "--attempt", "2", "--previous-attempt-dir", str(self.base / "evidence"),
                                     digest=digest, evidence="second"), 1)
        self.assertEqual(self.manifest("second")["attempt"], 2)
        with self.assertRaises(SystemExit):
            self.invoke(path, "--attempt", "3", "--previous-attempt-dir", str(self.base / "second"),
                        digest=digest, evidence="third")

    def test_existing_evidence_is_preserved(self):
        path, digest = self.write(envelope([step("one", ["/bin/echo"])]))
        (self.base / "evidence").write_text("preserve")
        with self.assertRaises(SystemExit):
            self.invoke(path, digest=digest)
        self.assertEqual((self.base / "evidence").read_text(), "preserve")


if __name__ == "__main__":
    unittest.main()
