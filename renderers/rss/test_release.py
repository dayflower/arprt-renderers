"""Check version isolation and fail-closed publication guards offline."""

import json
import os
import runpy
import subprocess
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

import release


class ReleaseTests(unittest.TestCase):
    def test_tag_is_renderer_specific_and_canonical(self):
        self.assertEqual(release.version_from_tag("rss/v0.1.0"), "0.1.0")
        for tag in (
            "v0.1.0",
            "other/v0.1.0",
            "picsum/v0.1.0",
            "rss/v01.1.0",
            "rss/v0.1",
            "rss/v0.1.0-rc.1",
            "rss/v0.1.0\n",
        ):
            with self.subTest(tag=tag), self.assertRaises(ValueError):
                release.version_from_tag(tag)

    def test_all_pages_and_partial_platform_publications_are_reserved(self):
        for tag in ("0.1.0", "0.1.0-amd64", "0.1.0-arm64"):
            pages = [[], [{"metadata": {"container": {"tags": [tag]}}}]]
            with self.subTest(tag=tag), self.assertRaises(ValueError):
                release.require_unused(pages, "0.1.0")
        release.require_unused(
            [[{"metadata": {"container": {"tags": ["0.2.0"]}}}]], "0.1.0"
        )

    def test_api_errors_fail_closed_except_explicit_404(self):
        for error in ("HTTP 401", "HTTP 403", "HTTP 429", "HTTP 500", "timeout"):
            with (
                self.subTest(error=error),
                patch.object(
                    release.subprocess,
                    "run",
                    return_value=subprocess.CompletedProcess([], 1, "", f"({error})"),
                ),
                self.assertRaises(RuntimeError),
            ):
                release.api_optional("unused")
        with patch.object(
            release.subprocess,
            "run",
            return_value=subprocess.CompletedProcess([], 1, "", "gh: (HTTP 404)"),
        ):
            self.assertIsNone(release.api_optional("unused"))

    def test_existing_release_reserves_deleted_image_version(self):
        with (
            patch.object(release, "run", return_value="User"),
            patch.object(release, "api_optional", side_effect=(None, {"id": 1})),
            self.assertRaisesRegex(ValueError, "GitHub release already exists"),
        ):
            release.unused("dayflower/arprt-renderers", "rss/v0.1.0")

    def test_guard_requires_matching_version_and_annotated_tag(self):
        with (
            patch.object(release.Path, "read_text", return_value="0.2.0"),
            self.assertRaisesRegex(ValueError, "VERSION"),
        ):
            release.guard("rss/v0.1.0")
        with (
            patch.object(release.Path, "read_text", return_value="0.1.0"),
            patch.object(release, "run", return_value="commit"),
            self.assertRaisesRegex(ValueError, "annotated tag"),
        ):
            release.guard("rss/v0.1.0")

    def test_guard_rejects_different_checkout_or_commit_outside_main(self):
        with (
            patch.object(release.Path, "read_text", return_value="0.1.0"),
            patch.object(release, "run", side_effect=("tag", "tagged", "other")),
            self.assertRaisesRegex(ValueError, "checkout must match"),
        ):
            release.guard("rss/v0.1.0")
        with (
            patch.object(release.Path, "read_text", return_value="0.1.0"),
            patch.object(release, "run", side_effect=("tag", "tagged", "tagged")),
            patch.object(
                release.subprocess,
                "run",
                side_effect=subprocess.CalledProcessError(1, "git merge-base"),
            ),
            self.assertRaises(subprocess.CalledProcessError),
        ):
            release.guard("rss/v0.1.0")

    def test_guard_emits_exact_tagged_revision(self):
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / "outputs"
            with (
                patch.object(release.Path, "read_text", return_value="0.1.0"),
                patch.object(
                    release, "run", side_effect=("tag", "revision", "revision")
                ),
                patch.object(release.subprocess, "run") as command,
                patch.dict(os.environ, {"GITHUB_OUTPUT": str(output)}),
            ):
                release.guard("rss/v0.1.0")
            command.assert_called_once_with(
                ["git", "merge-base", "--is-ancestor", "revision", "origin/main"],
                check=True,
            )
            self.assertEqual(
                output.read_text(),
                "tag=rss/v0.1.0\nversion=0.1.0\nrevision=revision\n",
            )

    def test_rss_package_and_paginated_lookup(self):
        with (
            patch.object(release, "run", return_value="Organization"),
            patch.object(release, "api_optional", side_effect=([[]], None)) as api,
        ):
            release.unused("example/renderers", "rss/v0.1.0")
        self.assertEqual(
            api.call_args_list[0].args,
            (
                "orgs/example/packages/container/arprt-renderer-rss/versions?per_page=100",
            ),
        )
        self.assertEqual(api.call_args_list[0].kwargs, {"paginate": True})
        self.assertEqual(
            api.call_args_list[1].args,
            ("repos/example/renderers/releases/tags/rss/v0.1.0",),
        )

    def test_release_notes_require_both_platforms_and_valid_digest(self):
        script = Path(__file__).with_name("release-notes.py")
        index = {
            "digest": "sha256:" + "a" * 64,
            "manifests": [
                {"platform": {"os": "linux", "architecture": arch}}
                for arch in ("amd64", "arm64")
            ],
        }
        environment = {
            "IMAGE": "ghcr.io/example/arprt-renderer-rss",
            "RELEASE_VERSION": "0.1.0",
            "SOURCE_REVISION": "b" * 40,
        }
        with (
            patch.dict(os.environ, environment),
            patch.object(Path, "read_text", return_value=json.dumps(index)),
            patch.object(Path, "write_text") as write,
        ):
            runpy.run_path(str(script))
        notes = write.call_args.args[0]
        self.assertIn(environment["IMAGE"] + "@" + index["digest"], notes)
        self.assertIn(environment["SOURCE_REVISION"], notes)
        self.assertIn("No live feeds, webcappi browser", notes)
        for invalid in (
            dict(index, digest="invalid"),
            dict(index, manifests=index["manifests"][:1]),
            dict(index, manifests=[index["manifests"][0]] * 2),
        ):
            with (
                self.subTest(index=invalid),
                patch.object(Path, "read_text", return_value=json.dumps(invalid)),
                patch.object(Path, "write_text") as write,
                self.assertRaises(ValueError),
            ):
                runpy.run_path(str(script))
            write.assert_not_called()


if __name__ == "__main__":
    unittest.main()
