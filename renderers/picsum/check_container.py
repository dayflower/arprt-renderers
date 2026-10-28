#!/usr/bin/env python3
"""Exercise the shipped CLI offline with the controller's Job restrictions."""

import argparse
import json
import struct
import subprocess
import tempfile
import zlib
from pathlib import Path

PICSUM = Path(__file__).resolve().parent


def run(*args):
    subprocess.run(args, check=True)


def chunk(kind, data):
    return (
        struct.pack(">I", len(data))
        + kind
        + data
        + struct.pack(">I", zlib.crc32(kind + data))
    )


def photo():
    # A deterministic, synthetic color gradient; never fetch an external photo.
    rows = bytearray()
    for y in range(480):
        rows.append(0)
        for x in range(800):
            rows.extend((x * 255 // 799, y * 255 // 479, (x + y) % 256))
    return (
        b"\x89PNG\r\n\x1a\n"
        + chunk(b"IHDR", struct.pack(">IIBBBBB", 800, 480, 8, 2, 0, 0, 0))
        + chunk(b"IDAT", zlib.compress(rows))
        + chunk(b"IEND", b"")
    )


def validate(path, palette):
    png = path.read_bytes()
    assert png[:8] == b"\x89PNG\r\n\x1a\n"
    expected = (8, 2) if palette == "rgb" else (2, 0)
    assert struct.unpack(">IIBBBBB", png[16:29]) == (800, 480, *expected, 0, 0, 0)
    if palette != "rgb":
        assert len(png) < 90_000
    offset = 8
    while offset < len(png):
        length = struct.unpack(">I", png[offset : offset + 4])[0]
        body = png[offset + 4 : offset + 8 + length]
        checksum = struct.unpack(">I", png[offset + 8 + length : offset + 12 + length])[
            0
        ]
        assert zlib.crc32(body) == checksum
        offset += 12 + length
    assert offset == len(png)
    assert png[-12:] == chunk(b"IEND", b"")
    assert not list(path.parent.glob(".picsum-*.tmp"))


def check(image):
    with tempfile.TemporaryDirectory(prefix="picsum-check-") as directory:
        root = Path(directory)
        inputs, output = root / "inputs", root / "output"
        inputs.mkdir()
        output.mkdir()
        # GitHub's host UID differs from the renderer's UID; only this disposable
        # output directory grants write access to UID/GID 1000.
        root.chmod(0o755)
        output.chmod(0o777)
        (inputs / "photo.png").write_bytes(photo())
        platform = subprocess.check_output(
            [
                "docker",
                "image",
                "inspect",
                "--format",
                "{{.Os}}/{{.Architecture}}",
                image,
            ],
            text=True,
        ).strip()
        assert platform in ("linux/amd64", "linux/arm64")
        # Compile tests with the same pinned toolchain for the image architecture.
        # Mount this separate binary into the exact production image: no compiler,
        # Python interpreter or test dependencies belong in the runtime image.
        run(
            "docker",
            "build",
            "--platform",
            platform,
            "--target",
            "test-export",
            "--output",
            f"type=local,dest={inputs}",
            "-f",
            str(PICSUM / "Dockerfile"),
            str(PICSUM.parents[1]),
        )

        def docker(context, output_parent="/arprt", extra=()):
            return (
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
                "--memory=128m",
                "--cpus=0.5",
                "--mount",
                f"type=bind,src={output},dst={output_parent}",
                "--mount",
                f"type=bind,src={context},dst={output_parent}/context.json,readonly",
                "--mount",
                f"type=bind,src={inputs},dst=/fixtures,readonly",
                *extra,
                image,
            )

        context = inputs / "context.json"
        base = json.loads((PICSUM / "fixtures/render-context-v1.json").read_text())
        context.write_text(json.dumps(base))
        run(
            *docker(
                context,
                extra=(
                    "--env=TMPDIR=/arprt",
                    "--env=PICSUM_JOB_CHECK=1",
                    "--mount",
                    f"type=bind,src={PICSUM / 'fixtures'},dst=/app/fixtures,readonly",
                    "--entrypoint",
                    "/fixtures/renderer.test",
                ),
            ),
            "-test.v",
        )
        # Verify default paths and gray4 before exercising explicit custom paths.
        run(*docker(context), "--input", "/fixtures/photo.png")
        validate(output / "output.png", "gray4")
        for target_palette, palette in (
            ("gray4", "gray4"),
            ("gray4", "bw"),
            ("gray4", "rgb"),
            ("spectra6", "rgb"),
        ):
            for rotation in (0, 90, 180, 270):
                base["renderTarget"].update(palette=target_palette, rotation=rotation)
                context.write_text(json.dumps(base))
                destination = output / "result.png"
                if destination.exists():
                    destination.unlink()
                # The context and output have separate parents, as allowed by
                # ARPRT. Mount the context file read-only at a custom path too.
                run(
                    *docker(
                        context,
                        "/work",
                        extra=(
                            "--mount",
                            f"type=bind,src={context},dst=/input/context.json,readonly",
                        ),
                    ),
                    "--context",
                    "/input/context.json",
                    "--output",
                    "/work/result.png",
                    "--input",
                    "/fixtures/photo.png",
                    "--palette",
                    palette,
                )
                validate(destination, palette)
                assert destination.stat().st_uid == 1000
        # The shipped CLI must preserve prior output on a failed conversion.
        prior = destination.read_bytes()
        (inputs / "invalid.png").write_bytes(b"invalid image")
        result = subprocess.run(
            [
                *docker(context, "/work"),
                "--context",
                "/work/context.json",
                "--output",
                "/work/result.png",
                "--input",
                "/fixtures/invalid.png",
                "--palette",
                "rgb",
            ],
            capture_output=True,
            text=True,
        )
        assert result.returncode != 0
        assert destination.read_bytes() == prior
        validate(destination, "rgb")
        print(
            "PASS: offline unit tests, default/custom CLI paths, palettes and rotations"
        )


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--image", required=True)
    check(parser.parse_args().image)
