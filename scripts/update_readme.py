import hashlib
import os
import sys
from pathlib import Path


def main() -> int:
    app = os.environ["APP"]
    dist = Path(os.environ["DIST"])
    platforms = os.environ["PLATFORMS"].split()
    readme = Path("readme.md")

    files: dict[str, tuple[str, str]] = {}
    for platform in platforms:
        os_name, arch = platform.split("_", 1)
        ext = ".exe" if "windows" in os_name else ""
        name = f"{app}-{platform}{ext}"
        path = dist / name
        if not path.exists():
            sys.stderr.write(f"missing binary: {path}\n")
            return 1
        data = path.read_bytes()
        sha = hashlib.sha256(data).hexdigest()
        mb = int((len(data) + 500000) / 1_000_000)
        size = f"{mb} MB"
        files[f"`{name}`"] = (size, sha)

    original = readme.read_text()
    lines = original.splitlines(keepends=True)
    out_lines = []
    for line in lines:
        if line.lstrip().startswith("|") and "`" in line:
            parts = line.rstrip("\n").split("|")
            if len(parts) >= 6:
                key = parts[2].strip()
                if key in files:
                    size, sha = files[key]
                    parts[3] = f" {size} "
                    parts[4] = f" `{sha}` "
                    line = "|".join(parts) + "\n"
        out_lines.append(line)

    updated = "".join(out_lines)
    if updated != original:
        readme.write_text(updated)
        print("updated readme.md")
    else:
        print("readme.md already up to date")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
