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


class CollectorSafety(unittest.TestCase):
    def invoke(self, root, execute=False):
        argv = ["collector", "--version", collector.TAG, "--evidence-dir", str(root)]
        if execute:
            self.invoke(root)
            envelope = collector.digest((root / "envelope.json").read_bytes())
            argv += ["--execute-install", "--approved-envelope-sha256", envelope]
        with patch("sys.argv", argv), contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO()):
            return collector.main()

    def test_plan_makes_no_process_or_installation(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory) / "evidence"
            with patch.object(collector.subprocess, "Popen") as process:
                self.assertEqual(self.invoke(root), 0)
                process.assert_not_called()
            record = json.loads((root / "manifest.json").read_text())
            self.assertEqual(record["observations"], [])
            self.assertEqual(record["t24"], "blocked")
            self.assertFalse((root / "isolated-home").exists())

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
            argv = ["collector", "--version", collector.TAG, "--evidence-dir", str(root),
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
            self.assertFalse((root / "isolated-home/.local").exists())
            self.assertEqual(record["stageResult"], "failed")

    def test_official_short_revision_provenance(self):
        event = {"status": "success", "provenance": {"product": "Axiom", "version": collector.TAG[1:],
                 "revision": collector.REVISION[:12], "sourceState": "clean"}}
        self.assertTrue(collector.valid_provenance(event))
        event["provenance"]["revision"] = "foreign"
        self.assertFalse(collector.valid_provenance(event))

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
            self.assertEqual((root / "artifacts/published-metadata.txt").stat().st_size, 65536)

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
