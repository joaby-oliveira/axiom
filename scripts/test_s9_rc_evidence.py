"""Offline safety tests; fixtures never establish RC acceptance Evidence."""
import contextlib
import importlib.util
import io
import json
import os
from pathlib import Path
import tempfile
import sys
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("collector", Path(__file__).with_name("s9-rc-evidence.py"))
collector = importlib.util.module_from_spec(spec)
spec.loader.exec_module(collector)


DESCRIPTORS = {
    "v0.1.2-rc.2": ("859969a07f3807822580431a05b6c78b07691fb1", 399403900),
    "v0.1.2-rc.1": ("f73d6d0c951dd40c5cc97c3794ad7ee5607092b5", 398805148),
}
EVIDENCE = Path(__file__).resolve().parents[1] / "docs/specifications/004-mvp-v1-baseline/evidence-s9-rc2"
CANDIDATE = EVIDENCE / "candidate-v0.1.2-rc.2.json"
PRIOR = EVIDENCE / "prior-candidate-v0.1.2-rc.1.json"
TAG = "v0.1.2-rc.2"


def load(path):
    return collector.load_candidate(path, "test")[0]


class CollectorSafety(unittest.TestCase):
    def invoke(self, root, execute=False, extra=()):
        argv = ["collector", "--version", TAG, "--candidate", str(CANDIDATE), "--evidence-dir", str(root), *extra]
        if execute:
            self.invoke(root, extra=extra)
            envelope = collector.digest((root / "envelope.json").read_bytes())
            argv += ["--execute-install", "--approved-envelope-sha256", envelope]
        with patch("sys.argv", argv), contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO()):
            return collector.main()

    def test_reviewed_descriptors_are_the_exact_candidates(self):
        for path in (CANDIDATE, PRIOR):
            value = load(path)
            self.assertEqual((value["sourceRevision"], value["releaseId"]), DESCRIPTORS[value["tag"]])

    def test_collector_hardcodes_no_candidate_identity(self):
        source = Path(collector.__file__).read_text()
        self.assertIsNone(collector.re.search(r"[0-9a-f]{40}", source))
        self.assertNotIn("rc.1", source)
        self.assertNotIn("rc.2", source)

    def test_floating_or_mismatched_selectors_are_refused(self):
        with tempfile.TemporaryDirectory() as directory:
            for version in ("rc", "--channel", "v0.1.2", "v0.1.2-rc.1"):
                argv = ["collector", "--version", version, "--candidate", str(CANDIDATE),
                        "--evidence-dir", str(Path(directory) / "evidence")]
                with patch("sys.argv", argv), contextlib.redirect_stderr(io.StringIO()), self.assertRaises(SystemExit):
                    collector.main()
            self.assertFalse((Path(directory) / "evidence").exists())

    def test_invalid_descriptor_and_newer_prior_are_refused(self):
        with tempfile.TemporaryDirectory() as directory:
            value = load(CANDIDATE)
            del value["assets"]["SHA256SUMS"]
            broken = Path(directory) / "broken.json"
            broken.write_text(json.dumps(value))
            cases = [["--candidate", str(broken)], ["--candidate", str(CANDIDATE), "--prior-candidate", str(CANDIDATE)]]
            for case in cases:
                argv = ["collector", "--version", TAG, *case, "--evidence-dir", str(Path(directory) / "evidence")]
                with patch("sys.argv", argv), contextlib.redirect_stderr(io.StringIO()), self.assertRaises(SystemExit):
                    collector.main()
            self.assertFalse((Path(directory) / "evidence").exists())

    def test_refusal_steps_pass_only_by_failing(self):
        self.assertTrue(collector.expected_outcome(0, False, "success"))
        self.assertFalse(collector.expected_outcome(0, False, "failure"))
        self.assertTrue(collector.expected_outcome(1, False, "failure"))
        self.assertFalse(collector.expected_outcome(1, False, "success"))
        self.assertFalse(collector.expected_outcome(1, True, "failure"))

    def test_plan_materializes_runtime_detection_without_execution(self):
        with tempfile.TemporaryDirectory() as directory:
            runtime = Path(directory) / "codex-real"
            runtime.write_bytes(b"not executed")
            root = Path(directory) / "evidence"
            located = lambda name, path=None: str(runtime) if name == "codex" and path is None else None
            with patch.object(collector.shutil, "which", side_effect=located), patch.object(collector.subprocess, "Popen") as process:
                self.assertEqual(self.invoke(root, extra=["--prior-candidate", str(PRIOR)]), 0)
                process.assert_not_called()
            envelope = json.loads((root / "envelope.json").read_text())
            labels = [step["label"] for step in envelope["steps"]]
            self.assertIn("first-run-codex", labels)
            self.assertNotIn("first-run-claude", labels)
            self.assertIn("downgrade-refused", labels)
            self.assertTrue(all("--channel" not in step["argv"] for step in envelope["steps"]))
            installs = [step["argv"] for step in envelope["steps"] if step["argv"][0] == "/bin/sh"]
            self.assertEqual({argv[-1] for argv in installs}, {TAG, "v0.1.2-rc.1"})
            self.assertEqual(envelope["runtimeExecutables"]["codex"]["sha256"], collector.digest(b"not executed"))
            record = json.loads((root / "manifest.json").read_text())
            self.assertTrue(any("claude, both" in item for item in record["limitations"]))

    def test_plan_makes_no_process_or_installation(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory) / "evidence"
            with patch.object(collector.subprocess, "Popen") as process:
                self.assertEqual(self.invoke(root), 0)
                process.assert_not_called()
            record = json.loads((root / "manifest.json").read_text())
            self.assertEqual(record["observations"], [])
            self.assertEqual(record["t24"], "blocked")
            self.assertFalse((root / "homes/main").exists())

    def test_existing_evidence_and_symlink_are_preserved(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory) / "evidence"
            root.write_text("preserve")
            with self.assertRaises(SystemExit):
                self.invoke(root)
            self.assertEqual(root.read_text(), "preserve")
            link = Path(directory) / "link"
            link.symlink_to(root)
            with self.assertRaises(SystemExit):
                self.invoke(link)
            self.assertTrue(link.is_symlink())

    def test_drifted_envelope_is_refused_without_overwrite(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory) / "evidence"
            self.invoke(root)
            envelope = collector.digest((root / "envelope.json").read_bytes())
            (root / "envelope.json").write_text("drift")
            argv = ["collector", "--version", TAG, "--candidate", str(CANDIDATE), "--evidence-dir", str(root),
                    "--execute-install", "--approved-envelope-sha256", envelope]
            with patch("sys.argv", argv), contextlib.redirect_stderr(io.StringIO()), self.assertRaises(SystemExit):
                collector.main()
            self.assertEqual((root / "envelope.json").read_text(), "drift")
            self.assertEqual(json.loads((root / "manifest.json").read_text())["mode"], "plan-only")

    def test_runtime_on_path_blocks_before_any_command(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory) / "evidence"
            with patch.object(collector.shutil, "which", return_value="/fake/codex"), patch.object(collector.subprocess, "Popen") as process:
                self.assertEqual(self.invoke(root, True), 1)
                process.assert_not_called()
            record = json.loads((root / "manifest.json").read_text())
            self.assertEqual(record["stageResult"], "failed")
            self.assertEqual(record["observations"], [])

    def test_unsupported_native_host_blocks_before_any_command(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory) / "evidence"
            with patch.object(collector.shutil, "which", return_value=None), patch.object(collector.platform, "system", return_value="Unsupported"), patch.object(collector.subprocess, "Popen") as process:
                self.assertEqual(self.invoke(root, True), 1)
                process.assert_not_called()

    def test_failed_public_read_retains_exit_and_stops_before_download(self):
        calls = []

        class FailedProcess:
            def __init__(self, argv, stdout, **kwargs):
                calls.append(argv)
                read, write = os.pipe()
                os.write(write, b"controlled offline failure\n")
                os.close(write)
                self.stdout = os.fdopen(read, "rb")

            def wait(self, timeout=None):
                return 23

        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory) / "evidence"
            with patch.object(collector.shutil, "which", return_value=None), patch.object(collector.platform, "system", return_value="Linux"), patch.object(collector.platform, "machine", return_value="x86_64"), patch.object(collector.subprocess, "Popen", FailedProcess):
                self.assertEqual(self.invoke(root, True), 1)
            record = json.loads((root / "manifest.json").read_text())
            self.assertEqual(len(calls), 1)
            self.assertEqual(record["observations"][-1]["exitCode"], 23)
            self.assertFalse((root / "homes/main/.local").exists())
            self.assertEqual(record["stageResult"], "failed")

    def test_official_short_revision_provenance(self):
        candidate = load(CANDIDATE)
        event = {"status": "success", "provenance": {"product": "Axiom", "version": TAG[1:],
                 "revision": candidate["sourceRevision"][:12], "sourceState": "clean"}}
        self.assertTrue(collector.valid_provenance(event, candidate))
        event["provenance"]["revision"] = load(PRIOR)["sourceRevision"][:12]
        self.assertFalse(collector.valid_provenance(event, candidate))

    def test_timeout_after_stdout_closes_is_recorded(self):
        class TimeoutProcess:
            pid = 123456789

            def __init__(self, argv, **kwargs):
                read, write = os.pipe()
                os.close(write)
                self.stdout = os.fdopen(read, "rb")

            def wait(self, timeout=None):
                if timeout is not None:
                    raise collector.subprocess.TimeoutExpired("offline-fixture", timeout)
                return -9

        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory) / "evidence"
            with patch.object(collector.shutil, "which", return_value=None), patch.object(collector.platform, "system", return_value="Linux"), patch.object(collector.platform, "machine", return_value="x86_64"), patch.object(collector.subprocess, "Popen", TimeoutProcess), patch.object(collector.os, "killpg") as kill:
                self.assertEqual(self.invoke(root, True), 1)
                kill.assert_called_once_with(TimeoutProcess.pid, collector.signal.SIGKILL)
            record = json.loads((root / "manifest.json").read_text())
            self.assertTrue(record["observations"][0]["timedOut"])
            self.assertEqual(record["observations"][0]["exitCode"], -9)

    def test_output_is_bounded_during_capture(self):
        actual_popen = collector.subprocess.Popen

        def noisy_fixture(argv, **kwargs):
            return actual_popen([sys.executable, "-c", "import sys; sys.stdout.write('x'*100000)"], **kwargs)

        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory) / "evidence"
            with patch.object(collector.shutil, "which", return_value=None), patch.object(collector.platform, "system", return_value="Linux"), patch.object(collector.platform, "machine", return_value="x86_64"), patch.object(collector.subprocess, "Popen", noisy_fixture):
                self.assertEqual(self.invoke(root, True), 1)
            record = json.loads((root / "manifest.json").read_text())
            self.assertEqual(record["observations"][0]["outputBytes"], 100000)
            self.assertTrue(record["observations"][0]["outputTruncated"])
            self.assertEqual((root / "artifacts/filesystem.txt").stat().st_size, 65536)

    def test_capture_error_stops_group_and_prevents_next_command(self):
        calls = []

        class CaptureProcess:
            pid = 123456789
            returncode = None

            def __init__(self, argv, **kwargs):
                calls.append(argv)
                read, write = os.pipe()
                os.write(write, b"fixture")
                os.close(write)
                self.stdout = os.fdopen(read, "rb")

            def wait(self, timeout=None):
                self.returncode = -9
                return self.returncode

        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory) / "evidence"
            with patch.object(collector.shutil, "which", return_value=None), patch.object(collector.platform, "system", return_value="Linux"), patch.object(collector.platform, "machine", return_value="x86_64"), patch.object(collector.subprocess, "Popen", CaptureProcess), patch.object(collector.os, "killpg") as kill, patch.object(collector.os, "read", side_effect=OSError("offline capture error")):
                self.assertEqual(self.invoke(root, True), 1)
                kill.assert_called_once_with(CaptureProcess.pid, collector.signal.SIGKILL)
            record = json.loads((root / "manifest.json").read_text())
            self.assertEqual(len(calls), 1)
            self.assertEqual(record["observations"][0]["exitCode"], -9)
            self.assertEqual(record["stageResult"], "failed")

    def test_interrupt_stops_group_and_persists_unknown_effects(self):
        class InterruptedProcess:
            pid = 123456789
            returncode = None

            def __init__(self, argv, **kwargs):
                read, write = os.pipe()
                os.close(write)
                self.stdout = os.fdopen(read, "rb")

            def wait(self, timeout=None):
                if timeout is not None:
                    raise KeyboardInterrupt
                self.returncode = -9
                return self.returncode

        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory) / "evidence"
            with patch.object(collector.shutil, "which", return_value=None), patch.object(collector.platform, "system", return_value="Linux"), patch.object(collector.platform, "machine", return_value="x86_64"), patch.object(collector.subprocess, "Popen", InterruptedProcess), patch.object(collector.os, "killpg") as kill:
                self.assertEqual(self.invoke(root, True), 130)
                kill.assert_called_once_with(InterruptedProcess.pid, collector.signal.SIGKILL)
            record = json.loads((root / "manifest.json").read_text())
            self.assertEqual(record["stageResult"], "interrupted; effects require inspection")
            self.assertEqual(record["observations"][0]["exitCode"], -9)
            self.assertEqual(record["interruptionSignal"], collector.signal.SIGINT)


if __name__ == "__main__":
    unittest.main()
