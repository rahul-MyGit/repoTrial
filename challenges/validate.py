#!/usr/bin/env python3
"""Check that a public challenge fails for its documented baseline reason."""

import argparse
import json
import pathlib
import subprocess
import sys


def resolve_within(root: pathlib.Path, relative: str) -> pathlib.Path:
    path = (root / relative).resolve()
    if not path.is_relative_to(root.resolve()):
        raise ValueError(f"path escapes challenge: {relative}")
    return path


def validate(manifest_path: pathlib.Path) -> None:
    manifest_path = manifest_path.resolve()
    manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    package = manifest_path.parent
    source = resolve_within(package, manifest["sourceRoot"])
    if not source.is_dir():
        raise ValueError(f"sourceRoot is not a directory: {source}")

    entrypoint = resolve_within(source, manifest["entrypoint"])
    if not entrypoint.is_file():
        raise ValueError(f"entrypoint is missing: {entrypoint}")

    def matching_files(patterns: list[str]) -> set[pathlib.Path]:
        matches = set()
        for pattern in patterns:
            if pathlib.PurePosixPath(pattern).is_absolute() or ".." in pathlib.PurePosixPath(pattern).parts:
                raise ValueError(f"unsafe path pattern: {pattern}")
            found = {path for path in source.glob(pattern) if path.is_file()}
            if not found:
                raise ValueError(f"path pattern matches no files: {pattern}")
            for path in found:
                resolve_within(source, str(path.relative_to(source)))
            matches.update(found)
        return matches

    writable = matching_files(manifest["allowedPatchPaths"])
    visible_tests = matching_files(manifest["visibleTestPaths"])
    if writable & visible_tests:
        raise ValueError("visible tests must not be patchable")

    command = manifest["baselineCommand"]
    if not command or not all(isinstance(part, str) and part for part in command):
        raise ValueError("baselineCommand must be a nonempty argument vector")
    timeout = manifest["limits"]["timeoutSeconds"]
    result = subprocess.run(
        command,
        cwd=source,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        timeout=timeout,
        check=False,
    )
    output = result.stdout
    expected = manifest["baselineExpected"]
    if result.returncode != expected["exitCode"] or expected["outputContains"] not in output:
        raise ValueError(
            f"baseline mismatch: exit={result.returncode}, wanted={expected['exitCode']}; "
            f"missing marker={expected['outputContains'] not in output}\n{output}"
        )
    print(f"validated {manifest['slug']} v{manifest['version']}: expected baseline failure observed")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("manifest", type=pathlib.Path)
    args = parser.parse_args()
    try:
        validate(args.manifest)
    except (OSError, ValueError, KeyError, json.JSONDecodeError, subprocess.TimeoutExpired) as error:
        print(f"challenge validation failed: {error}", file=sys.stderr)
        raise SystemExit(1) from error
