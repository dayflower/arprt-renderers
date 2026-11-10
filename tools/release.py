#!/usr/bin/env python3
"""Coordinate renderer releases without replacing renderer-owned publication."""

import argparse
import base64
import json
import math
import os
import re
import subprocess
import sys
import tempfile
import time
from pathlib import Path
from urllib.parse import quote, urlencode

REPOSITORY = "dayflower/arprt-renderers"
RENDERERS = ("picsum", "rss", "ical", "weather", "prometheus", "countdown")
ROOT = Path(__file__).resolve().parents[1]
VERSION_RE = re.compile(r"(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)")
SHA_RE = re.compile(r"[0-9a-f]{40}")
MANUAL = (
    "Verify anonymous tag/digest pulls and published image execution; then update "
    "README, Screen digest pins and validation records manually. Publication does "
    "not verify a cluster or physical display, or deploy anything."
)


class Problem(Exception):
    def __init__(self, message, state="unknown", code=3, **details):
        super().__init__(message)
        self.state, self.code, self.details = state, code, details


def version_parts(value):
    if not VERSION_RE.fullmatch(value):
        raise Problem(
            "Expected stable MAJOR.MINOR.PATCH without leading zeros.", "invalid", 2
        )
    return tuple(map(int, value.split(".")))


def next_version(current, change):
    parts = list(version_parts(current))
    if change in ("major", "minor", "patch"):
        position = ("major", "minor", "patch").index(change)
        parts[position] += 1
        parts[position + 1 :] = [0] * (2 - position)
        target = ".".join(map(str, parts))
    else:
        target = change
    if version_parts(target) <= version_parts(current):
        raise Problem("New version must be greater than VERSION.", "invalid", 2)
    return target


class Release:
    def __init__(self, root=ROOT):
        self.root = Path(root).resolve()
        self.deadline = None

    def command(self, *args):
        timeout = 30
        if self.deadline is not None:
            timeout = min(timeout, self.deadline - time.monotonic())
            if timeout <= 0:
                raise Problem("Wait timed out; use status to resume.", "timeout")
        env = dict(os.environ, GH_PROMPT_DISABLED="1", GIT_TERMINAL_PROMPT="0")
        try:
            return subprocess.run(
                args,
                cwd=self.root,
                env=env,
                capture_output=True,
                text=True,
                timeout=timeout,
                check=False,
            )
        except subprocess.TimeoutExpired:
            raise Problem(
                "Command timed out; remote state may be unchanged or completed.",
                "timeout",
            ) from None
        except OSError:
            raise Problem(f"Cannot execute {args[0]}; check required tools.") from None

    def git(self, *args):
        result = self.command("git", *args)
        if result.returncode:
            raise Problem("Git command failed: " + " ".join(args[:2]))
        return result.stdout.strip()

    def api(self, path, optional=False):
        result = self.command(
            "gh",
            "api",
            "--hostname",
            "github.com",
            f"repos/{REPOSITORY}/{path}".rstrip("/"),
        )
        if result.returncode:
            # Never interpret auth, rate-limit, network or malformed responses as absence.
            if optional and re.search(r"\(HTTP 404\)", result.stderr):
                return None
            match = re.search(r"\(HTTP (\d+)\)", result.stderr)
            raise Problem(
                "GitHub API request failed" + (f" (HTTP {match[1]})." if match else ".")
            )
        try:
            return json.loads(result.stdout)
        except ValueError:
            raise Problem("GitHub API returned invalid JSON.") from None

    def pages(self, path, key):
        items = []
        # Explicit pagination works with gh versions that do not support --slurp.
        for number in range(1, 11):
            separator = "&" if "?" in path else "?"
            data = self.api(f"{path}{separator}per_page=100&page={number}")
            batch = data[key]
            items.extend(batch)
            if len(batch) < 100:
                return items
        raise Problem("API result limit reached; refine the release query.")

    def local(self, clean=False):
        if Path(self.git("rev-parse", "--show-toplevel")).resolve() != self.root:
            raise Problem(
                "Run the tool from its own repository checkout.", "invalid", 2
            )
        if clean and self.git("status", "--porcelain", "--untracked-files=normal"):
            raise Problem("A clean working tree is required.", "not_ready", 1)

    def remote(self):
        self.local()
        allowed = {
            f"https://github.com/{REPOSITORY}",
            f"https://github.com/{REPOSITORY}.git",
            f"git@github.com:{REPOSITORY}.git",
            f"ssh://git@github.com/{REPOSITORY}.git",
        }
        for options in (("--all",), ("--push", "--all")):
            urls = self.git("remote", "get-url", *options, "origin").splitlines()
            if len(urls) != 1 or urls[0] not in allowed:
                raise Problem(
                    "origin fetch/push URLs must identify " + REPOSITORY, "invalid", 2
                )
        if self.api("")["full_name"].lower() != REPOSITORY:
            raise Problem("Unexpected GitHub repository.", "invalid", 2)

    def target(self, renderer, revision):
        if revision.startswith("-"):
            raise Problem("Revision must not be an option.", "invalid", 2)
        sha = self.git(
            "rev-parse", "--verify", "--end-of-options", revision + "^{commit}"
        )
        if not SHA_RE.fullmatch(sha):
            raise Problem("Expected a complete commit SHA.")
        value = self.git("show", f"{sha}:renderers/{renderer}/VERSION")
        version_parts(value)
        for path in (
            f"renderers/{renderer}/release.py",
            f".github/workflows/release-{renderer}.yaml",
        ):
            self.git("cat-file", "-e", f"{sha}:{path}")
        return dict(
            renderer=renderer,
            version=value,
            revision=sha,
            tag=f"{renderer}/v{value}",
            run_url=None,
            release_url=None,
        )

    def remote_tag(self, tag):
        ref = self.api("git/ref/tags/" + quote(tag, safe=""), optional=True)
        if ref is None:
            return None
        obj = ref["object"]
        if obj["type"] != "tag":
            raise Problem("Existing release tag must be annotated.", "inconsistent", 1)
        data = self.api("git/tags/" + obj["sha"])
        target = data["object"]
        if (
            data["tag"] != tag
            or target["type"] != "commit"
            or not SHA_RE.fullmatch(target["sha"])
        ):
            raise Problem(
                "Existing release tag must directly reference its commit.",
                "inconsistent",
                1,
            )
        return target["sha"]

    def release_record(self, tag):
        return self.api("releases/tags/" + quote(tag, safe=""), optional=True)

    def runs(self, renderer, sha, branch):
        workflow = "ci.yaml" if branch == "main" else f"release-{renderer}.yaml"
        path = f"actions/workflows/{workflow}/runs?" + urlencode(
            {"head_sha": sha, "branch": branch, "event": "push"}
        )
        return sorted(
            (
                run
                for run in self.pages(path, "workflow_runs")
                if run["head_sha"] == sha
                and run["head_branch"] == branch
                and run["event"] == "push"
            ),
            key=lambda run: (run["id"], run.get("run_attempt", 1)),
            reverse=True,
        )

    def preflight(self, target, allow_existing=False):
        sha, tag = target["revision"], target["tag"]
        tip = self.api("branches/main")["commit"]["sha"]
        comparison = self.api(f"compare/{sha}...{tip}")
        if comparison["status"] not in ("ahead", "identical"):
            raise Problem("Target commit is not on remote main.", "not_ready", 1)
        runs = self.runs(target["renderer"], sha, "main")
        if not runs:
            raise Problem(
                "No main push CI exists for this exact commit.", "not_ready", 1
            )
        run = runs[0]
        if run["status"] != "completed" or run["conclusion"] != "success":
            raise Problem(
                "Latest main CI has not succeeded.",
                "not_ready",
                1,
                run_url=run["html_url"],
            )
        # A green workflow with skipped renderer jobs is insufficient.
        jobs = self.pages(f"actions/runs/{run['id']}/jobs?filter=latest", "jobs")
        renderer = target["renderer"]
        required = {
            "common",
            f"{renderer} / source",
            f"{renderer} / native (amd64, ubuntu-24.04)",
            f"{renderer} / native (arm64, ubuntu-24.04-arm)",
        }
        successful = {job["name"] for job in jobs if job["conclusion"] == "success"}
        if not required <= successful:
            raise Problem(
                "Required common/source/native CI jobs did not succeed.", "not_ready", 1
            )
        existing = self.remote_tag(tag)
        if existing is not None and (not allow_existing or existing != sha):
            raise Problem(
                "Release tag already exists or points to another commit.",
                "not_ready",
                1,
            )
        record = self.release_record(tag)
        if record is not None and (existing is None or not allow_existing):
            raise Problem(
                "GitHub Release already exists; do not reuse the version.",
                "not_ready",
                1,
            )
        return existing

    def check(self, renderer, revision):
        self.remote()
        target = self.target(renderer, revision)
        try:
            self.preflight(target)
        except Problem as error:
            error.details = data_without_state(target) | error.details
            raise
        return dict(
            target,
            state="ready",
            message="Ready to tag; GHCR availability is checked by Actions.",
        )

    def bump(self, renderer, change, dry_run=False):
        self.local(clean=True)
        path = self.root / "renderers" / renderer / "VERSION"
        current = path.read_text().strip()
        target = next_version(current, change)
        if not dry_run:
            # Replace only VERSION; preserve its file mode and clean up failed writes.
            temporary = None
            try:
                with tempfile.NamedTemporaryFile(
                    mode="w", dir=path.parent, delete=False
                ) as stream:
                    temporary = Path(stream.name)
                    stream.write(target + "\n")
                temporary.chmod(path.stat().st_mode & 0o777)
                temporary.replace(path)
            finally:
                if temporary is not None:
                    temporary.unlink(missing_ok=True)
        return dict(
            renderer=renderer,
            previous_version=current,
            version=target,
            state="dry_run" if dry_run else "updated",
            message="Update the changelog and review/merge VERSION before publishing.",
        )

    def snapshot(self, renderer, version):
        version_parts(version)
        tag = f"{renderer}/v{version}"
        data = dict(
            renderer=renderer,
            version=version,
            revision=None,
            tag=tag,
            run_url=None,
            release_url=None,
            digest=None,
            message=MANUAL,
        )
        sha = self.remote_tag(tag)
        record = self.release_record(tag)
        if sha is None:
            if record is not None:
                raise Problem(
                    "Release exists without its annotated tag.", "inconsistent", 1
                )
            return dict(data, state="not_started")
        data["revision"] = sha
        # Read remote content: status must work without the tag's commit locally.
        content = self.api(
            "contents/" + f"renderers/{renderer}/VERSION?" + urlencode({"ref": sha})
        )
        if base64.b64decode(content["content"]).decode().strip() != version:
            raise Problem(
                "Tagged VERSION does not match the release tag.", "inconsistent", 1
            )
        runs = self.runs(renderer, sha, tag)
        if not runs:
            if record is not None:
                raise Problem(
                    "Release exists but its workflow cannot be identified.",
                    "inconsistent",
                    1,
                )
            return dict(data, state="awaiting_run")
        run = runs[0]
        data["run_url"] = run["html_url"]
        if run["status"] != "completed":
            return dict(data, state="running")
        if run["conclusion"] != "success":
            return dict(
                data,
                state="failed",
                message="Inspect the workflow; never retry a partial publish automatically.",
            )
        if record is None:
            raise Problem(
                "Workflow succeeded but GitHub Release is missing.", "inconsistent", 1
            )
        if record["draft"] or record["prerelease"] or record["tag_name"] != tag:
            raise Problem("Expected a published stable release.", "inconsistent", 1)
        assets = [
            a
            for a in record["assets"]
            if a["name"] == renderer + "-index.json" and a["state"] == "uploaded"
        ]
        if len(assets) != 1:
            raise Problem(
                "Release index asset is missing or ambiguous.", "inconsistent", 1
            )
        # Let gh handle asset authentication; no URLs from release text are executed.
        asset = self.command(
            "gh",
            "api",
            "--hostname",
            "github.com",
            "-H",
            "Accept: application/octet-stream",
            f"repos/{REPOSITORY}/releases/assets/{assets[0]['id']}",
        )
        if asset.returncode:
            raise Problem("Cannot read release index asset.")
        index = json.loads(asset.stdout)
        digest = index["digest"]
        platforms = sorted(
            (m["platform"]["os"], m["platform"]["architecture"])
            for m in index["manifests"]
        )
        if not re.fullmatch(r"sha256:[0-9a-f]{64}", digest) or platforms != [
            ("linux", "amd64"),
            ("linux", "arm64"),
        ]:
            raise Problem("Invalid release index digest/platforms.", "inconsistent", 1)
        for manifest in index["manifests"]:
            if not re.fullmatch(r"sha256:[0-9a-f]{64}", manifest["digest"]):
                raise Problem("Invalid platform manifest digest.", "inconsistent", 1)
        # These fields are written by the existing renderer-owned notes generator.
        if (
            f"`{sha}`" not in record["body"]
            or f"`ghcr.io/dayflower/arprt-renderer-{renderer}@{digest}`"
            not in record["body"]
        ):
            raise Problem(
                "Release notes do not match tagged source/index.", "inconsistent", 1
            )
        return dict(
            data, state="published", release_url=record["html_url"], digest=digest
        )

    def status(self, renderer, version, watch=False, timeout=900):
        self.remote()
        self.deadline = time.monotonic() + timeout
        try:
            previous = None
            while True:
                data = self.snapshot(renderer, version)
                if watch and data["state"] != previous:
                    print(
                        f"{data['tag']}: {data['state']} {data['run_url'] or ''}",
                        file=sys.stderr,
                    )
                    previous = data["state"]
                if not watch or data["state"] in ("published", "failed"):
                    return data
                remaining = self.deadline - time.monotonic()
                if remaining <= 0:
                    raise Problem(
                        "Wait timed out; use status to resume.",
                        "timeout",
                        **data_without_state(data),
                    )
                time.sleep(min(10, remaining))
        finally:
            self.deadline = None

    def publish(self, renderer, revision, dry_run=False, no_wait=False, timeout=900):
        self.local(clean=True)
        self.remote()
        target = self.target(renderer, revision)
        try:
            existing = self.preflight(target, allow_existing=True)
        except Problem as error:
            error.details = data_without_state(target) | error.details
            raise
        tag, sha = target["tag"], target["revision"]
        # Check local tags even when a matching remote tag already exists.
        local = self.git(
            "for-each-ref",
            "--format=%(refname) %(objecttype) %(*objectname)",
            "refs/tags/" + tag,
        )
        if local:
            if local != f"refs/tags/{tag} tag {sha}":
                raise Problem(
                    "Conflicting or lightweight local tag; resolve manually.",
                    "inconsistent",
                    1,
                )
        if dry_run:
            return dict(
                target,
                state="dry_run",
                message=(
                    "Resume the existing release."
                    if existing
                    else "Create annotated tag, push only that tag, and track its workflow. GHCR checks run in Actions."
                ),
            )
        print(
            f"Release {renderer} {target['version']}: {sha} -> {tag}", file=sys.stderr
        )
        if existing is None:
            if not local:
                title = {"ical": "iCalendar", "rss": "RSS"}.get(
                    renderer, renderer.capitalize()
                )
                self.git("tag", "-a", tag, "-m", f"{title} {target['version']}", sha)
            # Pin the tag object as well as its commit, even if local refs change.
            tag_object = self.git("rev-parse", "--verify", "refs/tags/" + tag)
            if (
                not SHA_RE.fullmatch(tag_object)
                or self.git("cat-file", "-t", tag_object) != "tag"
                or self.git("rev-parse", "--verify", tag_object + "^{commit}") != sha
            ):
                raise Problem("Local tag changed before push.", "inconsistent", 1)
            push_failed = False
            try:
                push = self.command(
                    "git", "push", "origin", f"{tag_object}:refs/tags/{tag}"
                )
                push_failed = push.returncode != 0
            except Problem:
                # Timeout/transport failure can happen after the server accepted the tag.
                push_failed = True
            # Inspect the server even after a successful push; never blindly push again.
            try:
                actual = self.remote_tag(tag)
            except Problem as error:
                error.details = data_without_state(target) | error.details
                raise
            if actual != sha:
                raise Problem(
                    "Tag push is unconfirmed; inspect remote state before retrying.",
                    "unknown",
                    **data_without_state(target),
                )
            if push_failed:
                print(
                    "Push reported an error, but the remote tag is confirmed; tracking it.",
                    file=sys.stderr,
                )
        return self.status(
            renderer, target["version"], watch=not no_wait, timeout=timeout
        )


def data_without_state(data):
    return {k: v for k, v in data.items() if k not in ("state", "message")}


def positive_seconds(value):
    number = float(value)
    if not math.isfinite(number) or number <= 0:
        raise argparse.ArgumentTypeError("timeout must be finite and positive")
    return number


def parser():
    cli = argparse.ArgumentParser(description=__doc__)
    commands = cli.add_subparsers(dest="command", required=True)
    for name in ("bump", "check", "publish", "status"):
        command = commands.add_parser(name)
        command.add_argument("renderer", choices=RENDERERS)
        command.add_argument(
            "--json", action="store_true", help="Print one machine-readable result"
        )
        if name == "bump":
            command.add_argument(
                "change", metavar="major|minor|patch|MAJOR.MINOR.PATCH"
            )
        if name in ("check", "publish"):
            command.add_argument("--revision", required=True)
        if name in ("bump", "publish"):
            command.add_argument("--dry-run", action="store_true")
        if name in ("status", "publish"):
            command.add_argument("--timeout", type=positive_seconds, default=900)
        if name == "status":
            command.add_argument("--version", required=True)
            command.add_argument("--watch", action="store_true")
        if name == "publish":
            command.add_argument("--no-wait", action="store_true")
    return cli


def main(argv=None):
    args = parser().parse_args(argv)
    service = Release()
    code = 0
    try:
        if args.command == "bump":
            data = service.bump(args.renderer, args.change, args.dry_run)
        elif args.command == "check":
            data = service.check(args.renderer, args.revision)
        elif args.command == "publish":
            data = service.publish(
                args.renderer, args.revision, args.dry_run, args.no_wait, args.timeout
            )
        else:
            data = service.status(args.renderer, args.version, args.watch, args.timeout)
        if data["state"] == "failed":
            code = 1
    except Problem as error:
        data = dict(error.details)
        data.update(renderer=args.renderer, state=error.state, message=str(error))
        code = error.code
    except (ValueError, KeyError, TypeError, OSError) as error:
        data = dict(
            renderer=args.renderer,
            state="unknown",
            message=f"Cannot interpret local/remote release data ({type(error).__name__}).",
        )
        code = 3
    except KeyboardInterrupt:
        data = dict(
            renderer=args.renderer,
            state="interrupted",
            message="Interrupted; use status to resume.",
        )
        code = 130
    if args.json:
        print(json.dumps(data, sort_keys=True))
    else:
        for key, value in data.items():
            if value is not None:
                print(f"{key}: {value}")
    return code


if __name__ == "__main__":
    sys.exit(main())
