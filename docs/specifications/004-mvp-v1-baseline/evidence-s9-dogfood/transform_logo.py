"""Deterministic removal of the light background outside the app-icon outline.

Standard library only (zlib PNG codec). Flood-fills near-white pixels connected
to the image border (the exterior) and makes them transparent, then un-mixes the
anti-aliased ring between the exterior and the icon body so the outline keeps
its colour with fractional alpha. Pixels not reachable from the border through
light pixels (the icon, including its white interior drawing) are copied
unchanged with alpha 255.

Usage: transform_logo.py <source 8-bit RGB PNG> <target RGBA PNG>
"""
import struct
import sys
import zlib
from collections import deque

LIGHT = 200  # min channel for a pixel to belong to the light exterior
RING = 3  # anti-alias band width, in pixels, measured from the exterior


def read_png(path):
    data = open(path, "rb").read()
    assert data[:8] == b"\x89PNG\r\n\x1a\n", "not a PNG"
    offset, idat, header = 8, b"", None
    while offset < len(data):
        length, kind = struct.unpack(">I4s", data[offset:offset + 8])
        body = data[offset + 8:offset + 8 + length]
        offset += 12 + length
        if kind == b"IHDR":
            header = struct.unpack(">IIBBBBB", body)
        elif kind == b"IDAT":
            idat += body
    width, height, depth, colour, _, _, interlace = header
    assert (depth, colour, interlace) == (8, 2, 0), "expected 8-bit RGB non-interlaced"
    raw, stride, bpp = zlib.decompress(idat), width * 3, 3
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
                predictor = left if pa <= pb and pa <= pc else up if pb <= pc else corner
                line[i] = (line[i] + predictor) & 255
            else:
                assert kind == 0, "unknown filter"
        rows.append(line)
        previous = line
    return width, height, rows


def write_png(path, width, height, rgba):
    def chunk(kind, body):
        return struct.pack(">I", len(body)) + kind + body + struct.pack(">I", zlib.crc32(kind + body) & 0xFFFFFFFF)

    raw = b"".join(b"\x00" + bytes(rgba[y * width * 4:(y + 1) * width * 4]) for y in range(height))
    with open(path, "wb") as handle:
        handle.write(b"\x89PNG\r\n\x1a\n")
        handle.write(chunk(b"IHDR", struct.pack(">IIBBBBB", width, height, 8, 6, 0, 0, 0)))
        handle.write(chunk(b"IDAT", zlib.compress(raw, 9)))
        handle.write(chunk(b"IEND", b""))


def main(source, target):
    width, height, rows = read_png(source)

    def pixel(x, y):
        row = rows[y]
        return row[3 * x], row[3 * x + 1], row[3 * x + 2]

    exterior = bytearray(width * height)
    queue = deque()
    border = [(x, y) for x in range(width) for y in (0, height - 1)]
    border += [(x, y) for y in range(height) for x in (0, width - 1)]
    for x, y in border:
        if min(pixel(x, y)) >= LIGHT and not exterior[y * width + x]:
            exterior[y * width + x] = 1
            queue.append((x, y))
    while queue:
        x, y = queue.popleft()
        for nx, ny in ((x + 1, y), (x - 1, y), (x, y + 1), (x, y - 1)):
            if 0 <= nx < width and 0 <= ny < height and not exterior[ny * width + nx] and min(pixel(nx, ny)) >= LIGHT:
                exterior[ny * width + nx] = 1
                queue.append((nx, ny))

    distance = [0 if e else RING + 1 for e in exterior]
    queue = deque((i % width, i // width) for i, e in enumerate(exterior) if e)
    while queue:
        x, y = queue.popleft()
        d = distance[y * width + x]
        if d >= RING:
            continue
        for nx, ny in ((x + 1, y), (x - 1, y), (x, y + 1), (x, y - 1)):
            i = ny * width + nx
            if 0 <= nx < width and 0 <= ny < height and distance[i] > d + 1:
                distance[i] = d + 1
                queue.append((nx, ny))

    # Icon body colour: per-channel median of dark pixels just inside the ring.
    inside = [pixel(i % width, i // width) for i, d in enumerate(distance) if d == RING + 1]
    inside = [p for p in inside if max(p) < 80]
    body = tuple(sorted(p[c] for p in inside)[len(inside) // 2] for c in range(3))

    rgba = bytearray(width * height * 4)
    for y in range(height):
        for x in range(width):
            i = y * width + x
            r, g, b = pixel(x, y)
            if exterior[i]:
                value = (0, 0, 0, 0)
            elif distance[i] <= RING:
                # p = a*body + (1-a)*white, solved for a from the mean channel.
                alpha = sum((255 - c) / max(1, 255 - k) for c, k in zip((r, g, b), body)) / 3
                alpha = max(0.0, min(1.0, alpha))
                if alpha == 0:
                    value = (0, 0, 0, 0)
                else:
                    colour = tuple(max(0, min(255, round((c - (1 - alpha) * 255) / alpha))) for c in (r, g, b))
                    value = colour + (round(alpha * 255),)
            else:
                value = (r, g, b, 255)
            rgba[4 * i:4 * i + 4] = bytes(value)
    write_png(target, width, height, rgba)
    print(f"size={width}x{height} body={body} exterior_pixels={sum(exterior)} ring_pixels={sum(1 for d in distance if 0 < d <= RING)}")


if __name__ == "__main__":
    main(sys.argv[1], sys.argv[2])
