#!/usr/bin/env python3
"""Run offline tests and the shipped Prometheus CLI under ARPRT Job restrictions."""

import argparse
import subprocess
import tarfile
import tempfile
from pathlib import Path

PROMETHEUS = Path(__file__).resolve().parent


def check(image):
    platform = subprocess.check_output(
        ["docker", "image", "inspect", "--format", "{{.Os}}/{{.Architecture}}", image],
        text=True,
    ).strip()
    assert platform in ("linux/amd64", "linux/arm64")
    native_arch = subprocess.check_output(
        ["docker", "version", "--format", "{{.Server.Arch}}"], text=True
    ).strip()
    if platform != "linux/" + native_arch:
        raise ValueError("container-check requires a native Docker server architecture")
    with tempfile.TemporaryDirectory(prefix="prometheus-check-") as directory:
        root = Path(directory)
        root.chmod(0o755)
        # Inspect the shipped filesystem, not the builder or test-export stage.
        container = subprocess.check_output(
            ["docker", "create", image], text=True
        ).strip()
        archive = root / "runtime.tar"
        try:
            subprocess.run(
                ["docker", "export", "--output", str(archive), container], check=True
            )
        finally:
            subprocess.run(
                ["docker", "rm", container], check=True, stdout=subprocess.DEVNULL
            )
        with tarfile.open(archive) as runtime:
            files = {member.name for member in runtime if member.isfile()}
        required = {
            "app/renderer",
            "etc/ssl/certs/ca-certificates.crt",
            "usr/share/licenses/arprt-renderer-prometheus/Go-LICENSE",
            "usr/share/licenses/arprt-renderer-prometheus/LICENSE",
            "usr/share/licenses/arprt-renderer-prometheus/THIRD_PARTY_NOTICES.md",
        }
        # Docker may inject these files when creating a container.
        injected = {
            "etc/hostname",
            "etc/hosts",
            "etc/resolv.conf",
            ".dockerenv",
            "dev/console",
        }
        assert files <= required | injected, "unexpected file in production filesystem"
        assert required <= files, "missing production file"
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
                str(PROMETHEUS / "Dockerfile"),
                str(PROMETHEUS.parents[1]),
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
                "--memory=256m",
                "--cpus=1",
                "--env=TMPDIR=/arprt",
                "--env=PROMETHEUS_CLI=/app/renderer",
                "--env=PROMETHEUS_DEFAULT_PATHS=1",
                "--mount",
                f"type=bind,src={output},dst=/arprt",
                "--mount",
                f"type=bind,src={PROMETHEUS / 'fixtures/render-context-v1.json'},dst=/arprt/context.json,readonly",
                "--mount",
                f"type=bind,src={PROMETHEUS / 'fixtures'},dst=/app/fixtures,readonly",
                "--mount",
                f"type=bind,src={PROMETHEUS / 'fixtures/render-context-v1.json'},dst=/input/context.json,readonly",
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
        assert not list(output.rglob(".prometheus-*.tmp"))
    print(
        "PASS: offline tests and production CLI, default/custom paths, palettes and rotations"
    )


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--image", required=True)
    check(parser.parse_args().image)
