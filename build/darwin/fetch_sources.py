#!/usr/bin/env python3
"""Collect the corresponding source of every component bundled by bundle_media.py.

Usage: fetch_sources.py <path/to/ViPlay.app> <output-directory> [formula ...]

For each formula in Contents/Resources/licenses/bundled-components.json this writes the upstream
source archive Homebrew builds from, its additional resources and external patches, and the formula
itself, which holds the build recipe and inline patches.
"""
import json
import os
import shutil
import subprocess
import sys
import tarfile


def run(*args):
    result = subprocess.run(args, capture_output=True, text=True)
    if result.returncode != 0:
        raise SystemExit(f"{' '.join(args[:3])} failed ({result.returncode}):\n{result.stderr.strip()}")
    return result.stdout


# Formulae installed from Homebrew's JSON API omit resources and patches, so the Ruby source is loaded instead.
EXTRAS_SCRIPT = """
# no_autobump! only steers version bumps and is rejected outside its tap, so it is dropped before loading.
contents = File.read(ARGV[1]).gsub(/^\\s*no_autobump!.*$/, "")
formula = Formulary.from_contents(ARGV[0], Pathname(ARGV[1]), contents)
downloads = formula.resources + formula.patchlist.select(&:external?).map(&:resource)
downloads.each do |download|
  download.fetch
  puts "SOURCE\t#{download.name}\t#{download.cached_download}"
end
"""


def store(cached, output, archive_stem):
    """Copy a Homebrew download into output; VCS checkouts are archived without their metadata."""
    if os.path.isdir(cached):
        archive = os.path.join(output, f"{archive_stem}.tar.gz")
        with tarfile.open(archive, "w:gz") as tar:
            tar.add(cached, arcname=archive_stem, filter=lambda info: None if ".git" in info.name.split("/") else info)
        return archive
    name = os.path.basename(cached).split("--", 1)[-1]
    archive = os.path.join(output, name if name.startswith(archive_stem.split("-")[0]) else f"{archive_stem}-{name}")
    shutil.copy2(cached, archive)
    return archive


def fetch(formula, bundled_version, output):
    details = json.loads(run("brew", "info", "--json=v2", formula))["formulae"][0]
    # Bundled versions carry Homebrew's revision suffix (for example 9.0.2_1); the source is the same.
    upstream_version = bundled_version.split("_")[0]
    if details["versions"]["stable"] != upstream_version:
        raise SystemExit(f"{formula}: bundled {bundled_version} but Homebrew now builds {details['versions']['stable']}; repackage first")
    run("brew", "fetch", "--build-from-source", formula)
    cached = run("brew", "--cache", "--build-from-source", formula).strip()
    archives = [store(cached, output, f"{formula}-{upstream_version}")]
    recipe = os.path.join(output, f"{formula}.rb")
    with open(recipe, "w") as handle:
        handle.write(run("brew", "cat", formula))
    for line in run("brew", "ruby", "-e", EXTRAS_SCRIPT, formula, recipe).splitlines():
        if not line.startswith("SOURCE\t"):
            continue
        _, name, path = line.split("\t", 2)
        archives.append(store(path, output, f"{formula}-{upstream_version}-{name}"))
    return ", ".join(os.path.basename(archive) for archive in archives)


def main():
    if len(sys.argv) < 3:
        sys.exit(__doc__)
    app, output, only = sys.argv[1], sys.argv[2], set(sys.argv[3:])
    with open(os.path.join(app, "Contents", "Resources", "licenses", "bundled-components.json")) as handle:
        manifest = json.load(handle)
    os.makedirs(output, exist_ok=True)
    for formula, component in sorted(manifest.items()):
        if only and formula not in only:
            continue
        print(f"{formula}: {fetch(formula, component['version'], output)}")


if __name__ == "__main__":
    main()
