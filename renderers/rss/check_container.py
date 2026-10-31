#!/usr/bin/env python3
"""Run offline tests and the shipped RSS CLI under ARPRT Job restrictions."""

import argparse
import subprocess
import tempfile
from pathlib import Path

RSS = Path(__file__).resolve().parent


def check(image):
    platform = subprocess.check_output(
        ["docker", "image", "inspect", "--format", "{{.Os}}/{{.Architecture}}", image],
        text=True,
    ).strip()
    assert platform in ("linux/amd64", "linux/arm64")
    with tempfile.TemporaryDirectory(prefix="rss-check-") as directory:
        root = Path(directory)
        root.chmod(0o755)
        inputs, output = root / "inputs", root / "output"
        inputs.mkdir()
        output.mkdir(mode=0o777)
        output.chmod(0o777)
        subprocess.run(
            [
                "docker",
                "build",
                "--platform",
                platform,
                "--target",
                "test-export",
                "--output",
                f"type=local,dest={inputs}",
                "-f",
                str(RSS / "Dockerfile"),
                str(RSS.parents[1]),
            ],
            check=True,
        )
        # Test HTTP servers listen only on this container's loopback interface.
        # The test binary launches /app/renderer, including its default paths.
        subprocess.run(
            [
                "docker",
                "run",
                "--rm",
                "--network=none",
                "--read-only",
                "--user=1000:1000",
                "--cap-drop=ALL",
                "--security-opt=no-new-privileges",
                "--ipc=none",
                "--pids-limit=64",
                "--memory=192m",
                "--cpus=0.5",
                "--env=TMPDIR=/arprt",
                "--env=RSS_CLI=/app/renderer",
                "--mount",
                f"type=bind,src={output},dst=/arprt",
                "--mount",
                f"type=bind,src={RSS / 'fixtures/render-context-v1.json'},dst=/arprt/context.json,readonly",
                "--mount",
                f"type=bind,src={RSS / 'fixtures'},dst=/app/fixtures,readonly",
                "--mount",
                f"type=bind,src={RSS / 'fixtures/render-context-v1.json'},dst=/input/context.json,readonly",
                "--mount",
                f"type=bind,src={inputs},dst=/tests,readonly",
                "--entrypoint=/tests/renderer.test",
                image,
                "-test.v",
                "-test.timeout=180s",
            ],
            check=True,
        )
        assert (output / "output.png").stat().st_uid == 1000
        assert not list(output.rglob(".rss-*.tmp"))
    print(
        "PASS: offline tests and production CLI, default/custom paths, palettes and rotations"
    )


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--image", required=True)
    check(parser.parse_args().image)
