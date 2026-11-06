"""Offline contract tests for the common release coordinator."""

import base64
import contextlib
import importlib.util
import io
import json
import subprocess
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

SPEC = importlib.util.spec_from_file_location(
    "release_cli", Path(__file__).with_name("release.py")
)
cli = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(cli)

SHA = "a" * 40
OTHER = "b" * 40
DIGEST = "sha256:" + "c" * 64
TAG = "weather/v0.3.0"


def run_record(identifier=10, **changes):
    data = dict(
        id=identifier,
        head_sha=SHA,
        head_branch=TAG,
        event="push",
        status="completed",
        conclusion="success",
        html_url=f"https://example.com/runs/{identifier}",
    )
    data.update(changes)
    return data


def index_record():
    return dict(
        digest=DIGEST,
        manifests=[
            dict(
                digest="sha256:" + "d" * 64,
                platform=dict(os="linux", architecture=arch),
            )
            for arch in ("amd64", "arm64")
        ],
    )


def release_record():
    return dict(
        tag_name=TAG,
        draft=False,
        prerelease=False,
        html_url="https://example.com/release",
        body=f"`{SHA}` `ghcr.io/dayflower/arprt-renderer-weather@{DIGEST}`",
        assets=[dict(name="weather-index.json", state="uploaded", id=99)],
    )


class FakeRelease(cli.Release):
    """Explicit fake Git/GitHub boundary; no test can publish a real release."""

    def __init__(self):
        super().__init__()
        self.refs = {}
        self.release = None
        self.workflow_runs = [run_record()]
        self.ci_runs = [run_record(head_branch="main")]
        self.jobs = [
            {"name": name, "conclusion": "success"}
            for name in (
                "common",
                "weather / source",
                "weather / native (amd64, ubuntu-24.04)",
                "weather / native (arm64, ubuntu-24.04-arm)",
            )
        ]
        self.local_tag = ""
        self.main_status = "ahead"
        self.local_sha = SHA
        self.version = "0.3.0"
        self.commands = []
        self.api_calls = []
        self.push_code = 0
        self.push_timeout = False
        self.push_delivered = True
        self.asset = index_record()
        self.repo = cli.REPOSITORY
        self.origin = f"https://github.com/{cli.REPOSITORY}.git"
        self.dirty = False
        self.bad_object = False
        self.bad_tag_name = False
        self.state_requests = []
        self.slept = []

    def command(self, *args):
        self.commands.append(args)
        if args[:2] == ("git", "push"):
            if self.push_delivered:
                self.refs[TAG] = SHA
            if self.push_timeout:
                raise cli.Problem("Push timed out.", "timeout")
            return subprocess.CompletedProcess(args, self.push_code, "", "")
        if args[:2] == ("gh", "api"):
            return subprocess.CompletedProcess(args, 0, json.dumps(self.asset), "")
        raise AssertionError(args)

    def git(self, *args):
        self.commands.append(("git", *args))
        if args[0] == "rev-parse":
            if args[1] == "--show-toplevel":
                return str(self.root)
            if args[-1] == "refs/tags/" + TAG:
                return OTHER
            return self.local_sha
        if args[:2] == ("status", "--porcelain"):
            return " M changed" if self.dirty else ""
        if args[:2] == ("remote", "get-url"):
            return self.origin
        if args[0] == "show":
            return self.version
        if args[0] == "cat-file":
            return "tag" if args[1] == "-t" else ""
        if args[0] == "for-each-ref":
            return self.local_tag
        if args[0] == "tag":
            self.local_tag = f"refs/tags/{TAG} tag {SHA}"
            return ""
        raise AssertionError(args)

    def api(self, path, optional=False):
        self.api_calls.append(path)
        if path == "":
            return {"full_name": self.repo}
        if path == "branches/main":
            return {"commit": {"sha": OTHER}}
        if path.startswith("compare/"):
            return {"status": self.main_status}
        if path.startswith("git/ref/tags/"):
            if TAG not in self.refs:
                return None
            return {
                "object": {"type": "commit" if self.bad_object else "tag", "sha": OTHER}
            }
        if path.startswith("git/tags/"):
            return {
                "tag": "wrong" if self.bad_tag_name else TAG,
                "object": {"type": "commit", "sha": self.refs[TAG]},
            }
        if path.startswith("releases/tags/"):
            return self.release
        if path.startswith("contents/"):
            return {
                "content": base64.b64encode((self.version + "\n").encode()).decode()
            }
        if "ci.yaml/runs?" in path:
            return {"workflow_runs": self.ci_runs}
        if "release-weather.yaml/runs?" in path:
            return {"workflow_runs": self.workflow_runs}
        if "/jobs?" in path:
            return {"jobs": self.jobs}
        raise AssertionError(path)

    def status(self, renderer, version, watch=False, timeout=900):
        self.state_requests.append((renderer, version, watch, timeout))
        return dict(renderer=renderer, version=version, state="running")


class VersionTests(unittest.TestCase):
    def test_arithmetic_and_explicit_version(self):
        for change, expected in [
            ("major", "1.0.0"),
            ("minor", "0.3.0"),
            ("patch", "0.2.4"),
            ("2.5.7", "2.5.7"),
        ]:
            self.assertEqual(cli.next_version("0.2.3", change), expected)
        for current, change in [
            ("01.2.3", "minor"),
            ("0.2.3", "0.2.3"),
            ("0.2.3", "0.2.2"),
            ("0.2.3", "1.0.0-rc1"),
            ("0.2.3", "v1.0.0"),
            ("0.2.3", "--minor"),
        ]:
            with (
                self.subTest(current=current, change=change),
                self.assertRaises(cli.Problem),
            ):
                cli.next_version(current, change)

    def test_positional_bump_and_invalid_options(self):
        for change in ("major", "minor", "patch", "1.2.3"):
            args = cli.parser().parse_args(["bump", "weather", change])
            self.assertEqual(args.change, change)
        with contextlib.redirect_stderr(io.StringIO()):
            for args in (
                ["bump", "weather", "--minor"],
                ["bump", "unknown", "minor"],
                ["publish", "weather"],
                ["status", "weather", "--version", "1.0.0", "--timeout", "nan"],
            ):
                with self.assertRaises(SystemExit):
                    cli.parser().parse_args(args)

    def test_bump_changes_only_version_in_real_git_checkout(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            path = root / "renderers/weather/VERSION"
            path.parent.mkdir(parents=True)
            path.write_text("0.2.0\n")
            path.chmod(0o644)
            (root / "README.md").write_text("untouched\n")
            for args in (
                ["init", "-q"],
                ["add", "."],
                [
                    "-c",
                    "user.name=Test",
                    "-c",
                    "user.email=test@example.com",
                    "commit",
                    "-q",
                    "-m",
                    "test: synthetic fixture",
                ],
            ):
                subprocess.run(
                    ["git", *args], cwd=root, check=True, capture_output=True
                )
            service = cli.Release(root)
            result = service.bump("weather", "minor", dry_run=True)
            self.assertEqual(result["version"], "0.3.0")
            self.assertEqual(path.read_text(), "0.2.0\n")
            service.bump("weather", "minor")
            self.assertEqual(path.read_text(), "0.3.0\n")
            self.assertEqual(path.stat().st_mode & 0o777, 0o644)
            self.assertEqual((root / "README.md").read_text(), "untouched\n")
            changed = service.git("diff", "--name-only").splitlines()
            self.assertEqual(changed, ["renderers/weather/VERSION"])
            self.assertEqual(sorted(p.name for p in path.parent.iterdir()), ["VERSION"])
            with self.assertRaises(cli.Problem):
                service.bump("weather", "patch")


class CheckTests(unittest.TestCase):
    def test_ready_and_no_mutations(self):
        service = FakeRelease()
        result = service.check("weather", SHA)
        self.assertEqual(result["revision"], SHA)
        self.assertEqual(result["state"], "ready")
        self.assertIn("GHCR", result["message"])
        self.assertFalse(
            any(
                command[:2] in (("git", "push"), ("git", "tag"))
                for command in service.commands
            )
        )
        self.assertTrue(any("head_sha=" + SHA in path for path in service.api_calls))

    def test_preflight_rejects_wrong_ci_main_tags_and_release(self):
        cases = {
            "not on main": lambda s: setattr(s, "main_status", "diverged"),
            "no CI": lambda s: setattr(s, "ci_runs", []),
            "wrong SHA": lambda s: setattr(
                s, "ci_runs", [run_record(head_branch="main", head_sha=OTHER)]
            ),
            "PR CI": lambda s: setattr(
                s, "ci_runs", [run_record(head_branch="main", event="pull_request")]
            ),
            "pending": lambda s: setattr(
                s,
                "ci_runs",
                [run_record(head_branch="main", status="in_progress", conclusion=None)],
            ),
            "latest failed": lambda s: setattr(
                s,
                "ci_runs",
                [
                    run_record(1, head_branch="main"),
                    run_record(2, head_branch="main", conclusion="failure"),
                ],
            ),
            "skipped native": lambda s: s.jobs[2].update(conclusion="skipped"),
            "existing tag": lambda s: s.refs.update({TAG: SHA}),
            "existing Release": lambda s: setattr(s, "release", release_record()),
            "wrong repository": lambda s: setattr(
                s, "origin", "https://github.com/example/other.git"
            ),
        }
        for name, mutate in cases.items():
            with self.subTest(name=name):
                service = FakeRelease()
                mutate(service)
                with self.assertRaises(cli.Problem):
                    service.check("weather", SHA)

    def test_revision_is_resolved_before_reading_version(self):
        service = FakeRelease()
        result = service.check("weather", "HEAD")
        self.assertEqual(result["revision"], SHA)
        self.assertIn(
            ("git", "show", SHA + ":renderers/weather/VERSION"), service.commands
        )
        with self.assertRaises(cli.Problem):
            service.target("weather", "--all")

    def test_annotated_tag_identity(self):
        for option in ("bad_object", "bad_tag_name"):
            service = FakeRelease()
            service.refs[TAG] = SHA
            setattr(service, option, True)
            with self.assertRaises(cli.Problem):
                service.remote_tag(TAG)


class PublishTests(unittest.TestCase):
    def publish(self, service, **kwargs):
        with contextlib.redirect_stderr(io.StringIO()):
            return service.publish("weather", SHA, **kwargs)

    def test_dry_run_does_not_tag_push_or_wait(self):
        service = FakeRelease()
        self.assertEqual(self.publish(service, dry_run=True)["state"], "dry_run")
        self.assertFalse(service.state_requests)
        self.assertFalse(
            any(c[:2] in (("git", "tag"), ("git", "push")) for c in service.commands)
        )

    def test_create_tag_push_exact_ref_and_wait(self):
        service = FakeRelease()
        self.publish(service)
        self.assertIn(
            ("git", "tag", "-a", TAG, "-m", "Weather 0.3.0", SHA), service.commands
        )
        self.assertIn(
            ("git", "push", "origin", f"{OTHER}:refs/tags/{TAG}"),
            service.commands,
        )
        self.assertEqual(service.state_requests, [("weather", "0.3.0", True, 900)])

    def test_existing_tag_resumes_without_push(self):
        service = FakeRelease()
        service.refs[TAG] = SHA
        self.publish(service, no_wait=True)
        self.assertFalse(
            any(c[:2] in (("git", "tag"), ("git", "push")) for c in service.commands)
        )
        self.assertEqual(service.state_requests[0][2], False)

    def test_conflicting_remote_or_local_tag_and_dirty_tree_stop(self):
        for name in ("remote", "local", "lightweight", "dirty"):
            with self.subTest(name=name):
                service = FakeRelease()
                if name == "remote":
                    service.refs[TAG] = OTHER
                if name == "local":
                    service.local_tag = f"refs/tags/{TAG} tag {OTHER}"
                if name == "lightweight":
                    service.local_tag = f"refs/tags/{TAG} commit "
                if name == "dirty":
                    service.dirty = True
                with self.assertRaises(cli.Problem):
                    self.publish(service)
                self.assertFalse(
                    any(c[:2] == ("git", "push") for c in service.commands)
                )

    def test_network_failure_after_push_observes_remote_without_retry(self):
        service = FakeRelease()
        service.push_code = 1
        self.publish(service)
        self.assertEqual(sum(c[:2] == ("git", "push") for c in service.commands), 1)
        service = FakeRelease()
        service.push_code, service.push_delivered = 1, False
        with self.assertRaises(cli.Problem) as caught:
            self.publish(service)
        self.assertEqual(caught.exception.state, "unknown")
        self.assertFalse(service.state_requests)

    def test_push_timeout_can_resume_when_server_received_tag(self):
        service = FakeRelease()
        service.push_timeout = True
        self.publish(service)
        self.assertEqual(len(service.state_requests), 1)
        self.assertEqual(sum(c[:2] == ("git", "push") for c in service.commands), 1)
        service = FakeRelease()
        service.push_timeout = True
        service.push_delivered = False
        with self.assertRaises(cli.Problem):
            self.publish(service)
        self.assertFalse(service.state_requests)

    def test_existing_failed_run_never_reruns_or_pushes(self):
        service = FakeRelease()
        service.refs[TAG] = SHA
        # Use actual status logic with the fake GitHub boundary.
        service.status = lambda *args, **kwargs: cli.Release.status(
            service, *args, **kwargs
        )
        service.workflow_runs[0]["conclusion"] = "failure"
        self.assertEqual(self.publish(service)["state"], "failed")
        self.assertFalse(any(c[:2] == ("git", "push") for c in service.commands))


class StatusTests(unittest.TestCase):
    def snapshot(self, service):
        return service.snapshot("weather", "0.3.0")

    def test_phase_transitions_and_no_mutations(self):
        service = FakeRelease()
        self.assertEqual(self.snapshot(service)["state"], "not_started")
        service.refs[TAG] = SHA
        service.workflow_runs = []
        self.assertEqual(self.snapshot(service)["state"], "awaiting_run")
        service.workflow_runs = [run_record(status="queued", conclusion=None)]
        self.assertEqual(self.snapshot(service)["state"], "running")
        service.workflow_runs = [run_record(conclusion="failure")]
        self.assertEqual(self.snapshot(service)["state"], "failed")
        service.workflow_runs = [run_record()]
        service.release = release_record()
        result = self.snapshot(service)
        self.assertEqual(result["state"], "published")
        self.assertEqual(result["digest"], DIGEST)
        self.assertIn("manually", result["message"])
        self.assertFalse(any(c[0] == "git" for c in service.commands))

    def test_inconsistent_release_evidence_is_rejected(self):
        for name in (
            "no tag",
            "no run",
            "no release",
            "draft",
            "missing asset",
            "wrong notes",
            "wrong version",
            "invalid digest",
            "wrong platform",
        ):
            with self.subTest(name=name):
                service = FakeRelease()
                service.refs[TAG] = SHA
                service.release = release_record()
                if name == "no tag":
                    service.refs.clear()
                if name == "no run":
                    service.workflow_runs.clear()
                if name == "no release":
                    service.release = None
                if name == "draft":
                    service.release["draft"] = True
                if name == "missing asset":
                    service.release["assets"].clear()
                if name == "wrong notes":
                    service.release["body"] = "wrong"
                if name == "wrong version":
                    service.version = "0.2.0"
                if name == "invalid digest":
                    service.asset["digest"] = "sha256:short"
                if name == "wrong platform":
                    service.asset["manifests"][1]["platform"]["architecture"] = "amd64"
                with self.assertRaises(cli.Problem):
                    self.snapshot(service)

    def test_latest_run_and_wrong_sha_branch_are_not_confused(self):
        service = FakeRelease()
        service.workflow_runs = [
            run_record(1),
            run_record(2, conclusion="failure"),
            run_record(3, head_sha=OTHER),
            run_record(4, head_branch="main"),
        ]
        self.assertEqual(service.runs("weather", SHA, TAG)[0]["id"], 2)

    def test_wait_timeout_is_resumable_and_deadline_is_cleared(self):
        service = FakeRelease()
        service.refs[TAG] = SHA
        service.workflow_runs = []
        with (
            patch.object(cli.time, "monotonic", side_effect=[0, 2]),
            self.assertRaises(cli.Problem) as caught,
        ):
            cli.Release.status(service, "weather", "0.3.0", watch=True, timeout=1)
        self.assertEqual(caught.exception.state, "timeout")
        self.assertEqual(caught.exception.details["revision"], SHA)
        self.assertIsNone(service.deadline)

    def test_wait_polls_until_published_without_state_file(self):
        service = FakeRelease()
        service.refs[TAG] = SHA
        service.release = release_record()
        snapshots = [
            dict(state=state, tag=TAG, run_url=None)
            for state in ("running", "published")
        ]
        with (
            patch.object(service, "snapshot", side_effect=snapshots),
            patch.object(cli.time, "sleep") as sleep,
        ):
            result = cli.Release.status(service, "weather", "0.3.0", watch=True)
        self.assertEqual(result["state"], "published")
        sleep.assert_called_once()


class BoundaryTests(unittest.TestCase):
    def test_api_errors_are_not_absence_and_404_is_explicit(self):
        service = cli.Release()
        for code in (401, 403, 429, 500, None, 404):
            result = subprocess.CompletedProcess([], 1, "", f"gh: error (HTTP {code})")
            with patch.object(service, "command", return_value=result):
                if code == 404:
                    self.assertIsNone(service.api("releases/tags/x", optional=True))
                else:
                    with self.assertRaises(cli.Problem):
                        service.api("releases/tags/x", optional=True)
        with (
            patch.object(
                service,
                "command",
                return_value=subprocess.CompletedProcess([], 0, "bad", ""),
            ),
            self.assertRaises(cli.Problem),
        ):
            service.api("")

    def test_pagination_does_not_require_new_gh_slurp(self):
        service = cli.Release()
        with patch.object(
            service, "api", side_effect=[{"jobs": list(range(100))}, {"jobs": [100]}]
        ) as api:
            self.assertEqual(
                len(service.pages("actions/runs/1/jobs?filter=latest", "jobs")), 101
            )
        self.assertEqual(
            api.call_args_list[1].args[0],
            "actions/runs/1/jobs?filter=latest&per_page=100&page=2",
        )

    def test_json_failure_has_one_result_and_preserves_target(self):
        problem = cli.Problem("timeout", "timeout", renderer="weather", revision=SHA)
        with (
            patch.object(cli.Release, "publish", side_effect=problem),
            contextlib.redirect_stdout(io.StringIO()) as output,
        ):
            code = cli.main(["publish", "weather", "--revision", SHA, "--json"])
        self.assertEqual(code, 3)
        data = json.loads(output.getvalue())
        self.assertEqual(data["state"], "timeout")
        self.assertEqual(data["revision"], SHA)

    def test_process_errors_and_timeout_are_bounded(self):
        service = cli.Release()
        for error in (FileNotFoundError(), subprocess.TimeoutExpired("gh", 30)):
            with (
                patch.object(cli.subprocess, "run", side_effect=error),
                self.assertRaises(cli.Problem),
            ):
                service.command("gh", "api")
        with patch.object(
            cli.subprocess,
            "run",
            return_value=subprocess.CompletedProcess([], 0, "", ""),
        ) as run:
            service.command("git", "status")
        self.assertEqual(run.call_args.kwargs["timeout"], 30)
        self.assertNotIn("shell", run.call_args.kwargs)


if __name__ == "__main__":
    unittest.main()
