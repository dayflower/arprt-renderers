#!/usr/bin/env python3
"""Guard Picsum releases and refuse existing GHCR version/platform tags."""

import argparse
import json
import os
import re
import subprocess
from pathlib import Path

RENDERER_DIR = Path(__file__).resolve().parent
PACKAGE = "arprt-renderer-picsum"


def run(*args):
    return subprocess.check_output(args, text=True).strip()


def version_from_tag(tag):
    number = r"(?:0|[1-9][0-9]*)"
    if not re.fullmatch(rf"picsum/v{number}\.{number}\.{number}", tag):
        raise ValueError("expected picsum/vMAJOR.MINOR.PATCH (stable releases only)")
    return tag.removeprefix("picsum/v")


def guard(tag):
    version = version_from_tag(tag)
    if (RENDERER_DIR / "VERSION").read_text().strip() != version:
        raise ValueError("tag must match renderers/picsum/VERSION")
    ref = "refs/tags/" + tag
    if run("git", "cat-file", "-t", ref) != "tag":
        raise ValueError("release requires an annotated tag")
    revision = run("git", "rev-parse", ref + "^{commit}")
    if revision != run("git", "rev-parse", "HEAD"):
        raise ValueError("checkout must match the tagged commit")
    subprocess.run(
        ["git", "merge-base", "--is-ancestor", revision, "origin/main"], check=True
    )
    outputs = {"tag": tag, "version": version, "revision": revision}
    if os.environ.get("GITHUB_OUTPUT"):
        with open(os.environ["GITHUB_OUTPUT"], "a") as stream:
            for key, value in outputs.items():
                stream.write(f"{key}={value}\n")
    print(json.dumps(outputs, indent=2))


def api_optional(endpoint, paginate=False):
    args = ["gh", "api"]
    if paginate:
        args.extend(("--paginate", "--slurp"))
    result = subprocess.run(
        [*args, endpoint], capture_output=True, text=True, check=False
    )
    if result.returncode:
        # Only an explicit missing resource is acceptable for a first release.
        # Authentication, rate-limit and network failures must stop publication.
        if re.search(r"\(HTTP 404\)", result.stderr):
            return None
        raise RuntimeError(result.stderr)
    return json.loads(result.stdout)


def require_unused(pages, version):
    reserved = {version, version + "-amd64", version + "-arm64"}
    for page in pages:
        for item in page:
            if reserved.intersection(item["metadata"]["container"]["tags"]):
                raise ValueError(
                    "release version/platform tag exists; never overwrite it"
                )


def unused(repository, tag):
    version = version_from_tag(tag)
    owner, _ = repository.split("/")
    owner_type = run("gh", "api", "repos/" + repository, "--jq", ".owner.type")
    prefix = "orgs" if owner_type == "Organization" else "users"
    pages = api_optional(
        f"{prefix}/{owner}/packages/container/{PACKAGE}/versions?per_page=100",
        paginate=True,
    )
    if pages is not None:
        require_unused(pages, version)
    # Also reserve versions for which a release exists even if its image tags
    # have been deleted. A partial publish requires a new version as well.
    if api_optional(f"repos/{repository}/releases/tags/{tag}") is not None:
        raise ValueError("GitHub release already exists; use a new version")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    commands.add_parser("guard").add_argument("tag")
    check = commands.add_parser("unused")
    check.add_argument("repository")
    check.add_argument("tag")
    args = parser.parse_args()
    if args.command == "guard":
        guard(args.tag)
    else:
        unused(args.repository, args.tag)
