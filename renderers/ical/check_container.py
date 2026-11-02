"""Run the semantics verifier and real probe under offline Job restrictions."""

import argparse
import json
import subprocess
from datetime import datetime, timezone
from pathlib import Path


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--image", required=True)
    args = parser.parse_args()
    prefix = [
        "docker",
        "run",
        "--rm",
        "--network",
        "none",
        "--read-only",
        "--user",
        "1000:1000",
        "--cap-drop",
        "ALL",
        "--security-opt",
        "no-new-privileges",
    ]
    subprocess.run([*prefix, args.image], check=True, timeout=60)
    directory = Path(__file__).parent / "fixtures"
    cases = json.loads((directory / "cases.json").read_text(encoding="utf-8"))
    for name in ("range-moved-in", "custom-zone-dst", "unknown-zone"):
        case = next(item for item in cases if item["Name"] == name)
        request = {
            "Calendar": (directory / f"{name}.ics").read_text(encoding="utf-8"),
            "Timezone": case["Timezone"],
            "From": case["From"],
            "To": case["To"],
        }
        result = subprocess.run(
            [*prefix, "-i", "--entrypoint", "/app/ical-probe", args.image],
            input=json.dumps(request),
            text=True,
            capture_output=True,
            timeout=15,
            check=False,
        )
        if "Error" in case:
            if result.returncode != 1 or result.stderr != case["Error"] + "\n":
                raise RuntimeError("probe did not return its safe error class")
            if result.stdout:
                raise RuntimeError("failed probe returned partial output")
            continue
        if result.returncode != 0 or result.stderr:
            raise RuntimeError("probe failed inside the restricted image")
        rows = []
        for item in json.loads(result.stdout):
            # Compare instants independently of the preserved source offset.
            start = datetime.fromisoformat(item["Start"]).astimezone(timezone.utc)
            end = datetime.fromisoformat(item["End"]).astimezone(timezone.utc)
            rows.append(
                "|".join(
                    [
                        item["UID"],
                        item["RecurrenceID"],
                        item["Summary"],
                        start.isoformat().replace("+00:00", "Z"),
                        end.isoformat().replace("+00:00", "Z"),
                    ]
                )
            )
        if rows != case["Want"]:
            raise RuntimeError("probe returned incorrect moved/timezone instances")
    print("Restricted semantics tests and real probe checks passed.")


if __name__ == "__main__":
    main()
