#!/usr/bin/env python3
# Post-link anti-tamper patcher (issue #84, ADR 0004). Companion to the native-svc
# `tamper` pass: it computes the real checksum of the `shieldtext` section in a
# linked ELF and writes it into the __shield_tamper_expected global, so the
# runtime self-check matches. Pure stdlib ELF64 parsing — runs anywhere python3
# does (WSL and CI), no Go/toolchain needed.
#
#   tamper-patch.py patch <binary>   # compute + store the section checksum
#   tamper-patch.py flip  <binary>   # flip one shieldtext byte (test: simulate tamper)
#
# The checksum must match the runtime: a wrapping u64 sum of the section bytes.
import struct
import sys

SECTION = b"shieldtext"
SYMBOL = b"__shield_tamper_expected"
MASK64 = (1 << 64) - 1


def u16(b, o): return struct.unpack_from("<H", b, o)[0]
def u32(b, o): return struct.unpack_from("<I", b, o)[0]
def u64(b, o): return struct.unpack_from("<Q", b, o)[0]


def parse_sections(b):
    # ELF64 header fields we need.
    if b[:4] != b"\x7fELF" or b[4] != 2:
        raise SystemExit("tamper-patch: not an ELF64 file")
    e_shoff = u64(b, 0x28)
    e_shentsize = u16(b, 0x3A)
    e_shnum = u16(b, 0x3C)
    e_shstrndx = u16(b, 0x3E)
    secs = []
    for i in range(e_shnum):
        o = e_shoff + i * e_shentsize
        secs.append({
            "name": u32(b, o + 0), "type": u32(b, o + 4),
            "addr": u64(b, o + 16), "offset": u64(b, o + 24),
            "size": u64(b, o + 32), "link": u32(b, o + 40),
            "entsize": u64(b, o + 56),
        })
    shstr = secs[e_shstrndx]
    for s in secs:
        end = b.index(b"\x00", shstr["offset"] + s["name"])
        s["str"] = b[shstr["offset"] + s["name"]:end]
    return secs


def find_section(secs, name):
    for s in secs:
        if s["str"] == name:
            return s
    raise SystemExit(f"tamper-patch: section {name!r} not found")


def find_symbol_offset(b, secs, name):
    symtab = next((s for s in secs if s["type"] == 2), None)  # SHT_SYMTAB
    if not symtab:
        raise SystemExit("tamper-patch: no .symtab (don't strip before patching)")
    strtab = secs[symtab["link"]]
    n = symtab["size"] // 24
    for i in range(n):
        o = symtab["offset"] + i * 24
        st_name = u32(b, o)
        end = b.index(b"\x00", strtab["offset"] + st_name)
        if b[strtab["offset"] + st_name:end] == name:
            st_value = u64(b, o + 8)
            st_shndx = u16(b, o + 6)
            sec = secs[st_shndx]
            return sec["offset"] + (st_value - sec["addr"])
    raise SystemExit(f"tamper-patch: symbol {name!r} not found")


def checksum(b, sec):
    return sum(b[sec["offset"]:sec["offset"] + sec["size"]]) & MASK64


def main():
    if len(sys.argv) != 3 or sys.argv[1] not in ("patch", "flip"):
        raise SystemExit("usage: tamper-patch.py patch|flip <binary>")
    mode, path = sys.argv[1], sys.argv[2]
    b = bytearray(open(path, "rb").read())
    secs = parse_sections(b)
    sec = find_section(secs, SECTION)

    if mode == "patch":
        csum = checksum(b, sec)
        off = find_symbol_offset(b, secs, SYMBOL)
        b[off:off + 8] = struct.pack("<Q", csum)
        open(path, "wb").write(b)
        print(f"tamper-patch: shieldtext sum=0x{csum:016x} -> {SYMBOL.decode()} @0x{off:x}")
    else:  # flip: mutate the first byte of the protected section
        off = sec["offset"]
        b[off] ^= 0xFF
        open(path, "wb").write(b)
        print(f"tamper-patch: flipped shieldtext byte @0x{off:x} (simulated tamper)")


if __name__ == "__main__":
    main()
