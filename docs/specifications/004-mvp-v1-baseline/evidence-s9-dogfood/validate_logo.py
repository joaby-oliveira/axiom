"""Validate the transformed documentation logo against its source (standard library only).

Usage: validate_logo.py <source RGB PNG> <result RGBA PNG>
Prints one JSON report; exits 1 when any check fails.
"""
import hashlib
import json
import struct
import sys
import zlib


def decode(path):
    data = open(path, "rb").read()
    assert data[:8] == b"\x89PNG\r\n\x1a\n", "not a PNG"
    offset, idat, header, kinds = 8, b"", None, []
    while offset < len(data):
        length, kind = struct.unpack(">I4s", data[offset:offset + 8])
        body = data[offset + 8:offset + 8 + length]
        crc = struct.unpack(">I", data[offset + 8 + length:offset + 12 + length])[0]
        assert zlib.crc32(kind + body) & 0xFFFFFFFF == crc, f"bad CRC in {kind!r}"
        kinds.append(kind.decode())
        offset += 12 + length
        if kind == b"IHDR":
            header = struct.unpack(">IIBBBBB", body)
        elif kind == b"IDAT":
            idat += body
    width, height, depth, colour, _, _, interlace = header
    assert depth == 8 and interlace == 0 and colour in (2, 6)
    bpp = 3 if colour == 2 else 4
    raw, stride = zlib.decompress(idat), width * bpp
    rows, previous, position = [], bytearray(stride), 0
    for _ in range(height):
        kind = raw[position]
        line = bytearray(raw[position + 1:position + 1 + stride])
        position += 1 + stride
        for i in range(stride):
            left = line[i - bpp] if i >= bpp else 0
            up = previous[i]
            corner = previous[i - bpp] if i >= bpp else 0
            if kind == 1:
                line[i] = (line[i] + left) & 255
            elif kind == 2:
                line[i] = (line[i] + up) & 255
            elif kind == 3:
                line[i] = (line[i] + (left + up) // 2) & 255
            elif kind == 4:
                p = left + up - corner
                pa, pb, pc = abs(p - left), abs(p - up), abs(p - corner)
                line[i] = (line[i] + (left if pa <= pb and pa <= pc else up if pb <= pc else corner)) & 255
        rows.append(line)
        previous = line
    return {"width": width, "height": height, "colour_type": colour, "bpp": bpp, "rows": rows, "chunks": kinds,
            "sha256": hashlib.sha256(data).hexdigest(), "bytes": len(data)}


def main(source_path, result_path):
    source, result = decode(source_path), decode(result_path)
    w, h = result["width"], result["height"]

    def src(x, y):
        r = source["rows"][y]
        return tuple(r[3 * x:3 * x + 3])

    def res(x, y):
        r = result["rows"][y]
        return tuple(r[4 * x:4 * x + 4])

    alpha_hist = {"transparent": 0, "partial": 0, "opaque": 0}
    opaque_changed, max_composite_delta = 0, 0
    for y in range(h):
        for x in range(w):
            r, g, b, a = res(x, y)
            alpha_hist["transparent" if a == 0 else "opaque" if a == 255 else "partial"] += 1
            if a == 255 and (r, g, b) != src(x, y):
                opaque_changed += 1
            # Composite over white must reproduce the source (the source background was white-ish).
            if a > 0:
                composite = tuple(round((c * a + 255 * (255 - a)) / 255) for c in (r, g, b))
                max_composite_delta = max(max_composite_delta, max(abs(p - q) for p, q in zip(composite, src(x, y))))
    corners = {name: res(x, y)[3] for name, (x, y) in {
        "top_left": (0, 0), "top_right": (w - 1, 0), "bottom_left": (0, h - 1), "bottom_right": (w - 1, h - 1),
        "inner_top_left_8px": (8, 8), "inner_bottom_right_8px": (w - 9, h - 9)}.items()}
    centre = [(w // 2, h // 2), (627, 460), (w // 2, 300), (300, 300), (w - 300, h - 300)]
    interior = {f"{x},{y}": {"source": list(src(x, y)), "result": list(res(x, y))} for x, y in centre}
    checks = {
        "png_valid_crc_and_decode": True,
        "rgba_colour_type_6": result["colour_type"] == 6,
        "dimensions_equal_source": (w, h) == (source["width"], source["height"]),
        "outer_corners_transparent": all(value == 0 for value in corners.values()),
        "interior_samples_opaque_and_unchanged": all(v["result"][3] == 255 and v["result"][:3] == v["source"] for v in interior.values()),
        "every_opaque_pixel_equals_source": opaque_changed == 0,
        "composite_over_white_matches_source_max_delta_le_2": max_composite_delta <= 2,
        "has_transparent_exterior_and_opaque_icon": alpha_hist["transparent"] > 0 and alpha_hist["opaque"] > 0,
    }
    report = {
        "schema": "axiom-s9-dogfood-logo-validation/v1",
        "source": {"sha256": source["sha256"], "bytes": source["bytes"], "size": [source["width"], source["height"]], "colour_type": source["colour_type"]},
        "result": {"sha256": result["sha256"], "bytes": result["bytes"], "size": [w, h], "colour_type": result["colour_type"], "chunks": result["chunks"]},
        "alpha_histogram": alpha_hist, "corner_alpha": corners, "interior_samples": interior,
        "opaque_pixels_changed": opaque_changed, "max_composite_over_white_delta": max_composite_delta,
        "checks": checks, "result_status": "pass" if all(checks.values()) else "fail",
    }
    print(json.dumps(report, indent=2, sort_keys=True))
    return 0 if all(checks.values()) else 1


if __name__ == "__main__":
    sys.exit(main(sys.argv[1], sys.argv[2]))
