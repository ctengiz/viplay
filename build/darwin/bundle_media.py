#!/usr/bin/env python3
"""Make ViPlay.app self-contained: bundle FFmpeg, ffprobe, libmpv, and every non-system dylib they load.

Usage: bundle_media.py <path/to/ViPlay.app> [homebrew-prefix]
"""
import json
import os
import shutil
import subprocess
import sys

SYSTEM_PREFIXES = ("/usr/lib/", "/System/")


def run(*args):
    return subprocess.run(args, check=True, capture_output=True, text=True).stdout


def dependencies(binary):
    lines = run("otool", "-L", binary).splitlines()[1:]
    return [line.strip().split(" (")[0] for line in lines]


def rpaths(binary):
    lines = run("otool", "-l", binary).splitlines()
    return [lines[i + 2].split()[1] for i, line in enumerate(lines) if line.strip() == "cmd LC_RPATH"]


def resolve(reference, owner, prefix):
    if reference.startswith("@loader_path/") or reference.startswith("@executable_path/"):
        return os.path.realpath(os.path.join(os.path.dirname(owner), reference.split("/", 1)[1]))
    if reference.startswith("@rpath/"):
        name = reference[len("@rpath/"):]
        candidates = [os.path.join(path.replace("@loader_path", os.path.dirname(owner)), name) for path in rpaths(owner)]
        candidates.append(os.path.join(prefix, "lib", name))
        for candidate in candidates:
            if os.path.exists(candidate):
                return os.path.realpath(candidate)
        raise FileNotFoundError(f"{reference} required by {owner}")
    return os.path.realpath(reference)


def minimum_macos(binary):
    for line in run("otool", "-l", binary).splitlines():
        fields = line.split()
        if len(fields) == 2 and fields[0] == "minos":
            return fields[1]
    return "12.0"


def bundle(app, prefix):
    macos = os.path.join(app, "Contents", "MacOS")
    frameworks = os.path.join(app, "Contents", "Frameworks")
    licenses = os.path.join(app, "Contents", "Resources", "licenses")
    os.makedirs(frameworks, exist_ok=True)

    executables = [os.path.join(macos, "ViPlay")]
    for tool in ("ffmpeg", "ffprobe"):
        target = os.path.join(macos, tool)
        shutil.copy2(os.path.realpath(os.path.join(prefix, "opt", "ffmpeg", "bin", tool)), target)
        os.chmod(target, 0o755)
        executables.append(target)

    # originals maps each bundled file to the path its references are resolved against.
    originals = {path: os.path.realpath(os.path.join(prefix, "opt", "ffmpeg", "bin", os.path.basename(path))) for path in executables[1:]}
    originals[executables[0]] = executables[0]
    bundled = {}
    pending = list(executables)
    while pending:
        binary = pending.pop()
        for reference in dependencies(binary):
            if reference.startswith(SYSTEM_PREFIXES):
                continue
            source = resolve(reference, originals[binary], prefix)
            if source.startswith(SYSTEM_PREFIXES) or os.path.basename(source) == os.path.basename(binary):
                continue
            name = os.path.basename(source)
            target = os.path.join(frameworks, name)
            if name not in bundled:
                shutil.copy2(source, target)
                os.chmod(target, 0o755)
                bundled[name] = source
                originals[target] = source
                pending.append(target)
            run("install_name_tool", "-change", reference, f"@executable_path/../Frameworks/{name}", binary)

    for name in bundled:
        run("install_name_tool", "-id", f"@executable_path/../Frameworks/{name}", os.path.join(frameworks, name))
    binaries = list(executables) + [os.path.join(frameworks, name) for name in bundled]
    # Homebrew bottles target the macOS release they were built on, so the bundle cannot run on anything older.
    minimum = max((minimum_macos(binary) for binary in binaries), key=lambda version: tuple(map(int, version.split("."))))
    run("plutil", "-replace", "LSMinimumSystemVersion", "-string", minimum, os.path.join(app, "Contents", "Info.plist"))
    for binary in binaries:
        run("codesign", "--force", "--sign", "-", binary)

    copy_licenses(prefix, bundled.values(), licenses)
    repository = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
    for notice in ("LICENSE", "THIRD_PARTY_NOTICES.md"):
        shutil.copy2(os.path.join(repository, notice), licenses)
    print(f"Bundled ffmpeg, ffprobe, and {len(bundled)} libraries into {app}; requires macOS {minimum}")


def copy_licenses(prefix, sources, destination):
    """Copy each bundled Homebrew formula's license files and record exact versions for source requests."""
    cellar = os.path.join(prefix, "Cellar") + os.sep
    formulae = {"ffmpeg"}
    for source in sources:
        if source.startswith(cellar):
            formulae.add(source[len(cellar):].split(os.sep)[0])
    info = {entry["name"]: entry for entry in json.loads(run("brew", "info", "--json=v2", *sorted(formulae)))["formulae"]}
    manifest = {}
    for formula in sorted(formulae):
        keg = os.path.realpath(os.path.join(prefix, "opt", formula))
        version = os.path.basename(keg)
        target = os.path.join(destination, formula)
        os.makedirs(target, exist_ok=True)
        for entry in os.listdir(keg):
            # sbom.spdx.json records the SPDX license and exact source URL, including for kegs without license files.
            if (entry == "sbom.spdx.json" or entry.upper().startswith(("LICENSE", "COPYING", "COPYRIGHT", "NOTICE"))) and os.path.isfile(os.path.join(keg, entry)):
                shutil.copy2(os.path.join(keg, entry), target)
        details = info.get(formula, {})
        manifest[formula] = {
            "version": version,
            "license": details.get("license") or "",
            "source": details.get("urls", {}).get("stable", {}).get("url", ""),
            "homepage": details.get("homepage", ""),
        }
    with open(os.path.join(destination, "bundled-components.json"), "w") as handle:
        json.dump(manifest, handle, indent=2, sort_keys=True)


if __name__ == "__main__":
    if len(sys.argv) not in (2, 3):
        sys.exit(__doc__)
    bundle(os.path.abspath(sys.argv[1]), sys.argv[2] if len(sys.argv) == 3 else run("brew", "--prefix").strip())
