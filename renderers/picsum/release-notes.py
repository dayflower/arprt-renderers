#!/usr/bin/env python3
"""Write release evidence using the index actually published to GHCR."""

import json
import os
import re
from pathlib import Path

index = json.loads(Path("dist/picsum-index.json").read_text())
digest = index["digest"]
if not re.fullmatch(r"sha256:[0-9a-f]{64}", digest):
    raise ValueError("expected a SHA-256 image index digest")
platforms = [
    item["platform"]["os"] + "/" + item["platform"]["architecture"]
    for item in index["manifests"]
]
if sorted(platforms) != ["linux/amd64", "linux/arm64"]:
    raise ValueError("expected exactly one native image for each release platform")
image = os.environ["IMAGE"]
version = os.environ["RELEASE_VERSION"]
revision = os.environ["SOURCE_REVISION"]
Path("dist/picsum-release.md").write_text(
    f"Picsum {version} is maintained by the independent, unofficial ARPRT project.\n"
    "ARPRT is not affiliated with or endorsed by TRMNL.\n\n"
    f"- Image: `{image}:{version}`\n"
    f"- Immutable reference: `{image}@{digest}`\n"
    f"- Source revision: `{revision}`\n"
    "- Render Context: v1; imported from ARPRT v0.2.2.\n"
    "- Native linux/amd64 and linux/arm64: container build, offline unit tests "
    "and CLI checks under Job security restrictions passed before publication.\n"
    "- No live Picsum, Kubernetes cluster or physical display acceptance is "
    "performed by this release workflow.\n\n"
    "For the first publication, set the GHCR package visibility to public and "
    "verify anonymous pulls on both platforms before advertising public availability.\n"
)
