"""Embed the project's OpenAPI document and static Swagger UI assets."""

from pathlib import Path
import sys


def write_assets(output, source_paths):
    output.parent.mkdir(parents=True, exist_ok=True)
    with output.open("w", encoding="ascii") as header:
        header.write("#pragma once\n\nnamespace disk::swagger_assets\n{\n")
        for path in source_paths:
            name = path.stem.replace("-", "_")
            data = path.read_bytes()
            header.write(f"inline constexpr unsigned char {name}[] = {{\n")
            for start in range(0, len(data), 20):
                chunk = data[start:start + 20]
                header.write("    " + ", ".join(f"0x{byte:02x}" for byte in chunk) + ",\n")
            header.write("};\n\n")
        header.write("} // namespace disk::swagger_assets\n")


if __name__ == "__main__":
    write_assets(Path(sys.argv[1]), map(Path, sys.argv[2:]))
