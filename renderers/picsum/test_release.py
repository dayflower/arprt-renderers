"""Check version isolation and fail-closed publication guards offline."""

import subprocess
import unittest
from unittest.mock import patch

import release


class ReleaseTests(unittest.TestCase):
    def test_tag_is_renderer_specific_and_canonical(self):
        self.assertEqual(release.version_from_tag("picsum/v0.1.0"), "0.1.0")
        for tag in (
            "v0.1.0",
            "other/v0.1.0",
            "picsum/v01.1.0",
            "picsum/v0.1",
            "picsum/v0.1.0-rc.1",
            "picsum/v0.1.0\n",
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
            release.unused("dayflower/arprt-renderers", "picsum/v0.1.0")

    def test_guard_requires_matching_version_and_annotated_tag(self):
        with (
            patch.object(release.Path, "read_text", return_value="0.2.0"),
            self.assertRaisesRegex(ValueError, "VERSION"),
        ):
            release.guard("picsum/v0.1.0")
        with (
            patch.object(release.Path, "read_text", return_value="0.1.0"),
            patch.object(release, "run", return_value="commit"),
            self.assertRaisesRegex(ValueError, "annotated tag"),
        ):
            release.guard("picsum/v0.1.0")

    def test_guard_rejects_different_checkout_or_commit_outside_main(self):
        with (
            patch.object(release.Path, "read_text", return_value="0.1.0"),
            patch.object(release, "run", side_effect=("tag", "tagged", "other")),
            self.assertRaisesRegex(ValueError, "checkout must match"),
        ):
            release.guard("picsum/v0.1.0")
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
            release.guard("picsum/v0.1.0")


if __name__ == "__main__":
    unittest.main()
