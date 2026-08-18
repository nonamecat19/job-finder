#!/usr/bin/env python3
"""
Compare shared type definitions between index.ts (hand-maintained) and
generated.ts (tygo output). Reports divergence classes and, in strictness
mode, fails if any field that was `T | null` on the hand-written side would
become `T?` after consolidation.

Derivation chain: types flow `apps/api/internal/dto/*.go` →
`apps/api/tygo.yaml` (enum_style: union, type_mappings) →
`packages/shared/src/generated.ts` → `index.ts` (re-export + narrow only).

Modes:
  default        — report pairs, identical, drifted, and per-class counts
  --check-strictness — fail non-zero if any field weakens (SC-005 guard)
  --check-duplicates  — fail non-zero if any shape is defined in both files
  --check-go-nullability — fail non-zero if a Go `*T` field without `omitempty`
      (which the server always sends, as null when unset) has no `Nullable`
      entry in index.ts. The generated `field?: T` is wrong about the wire for
      such fields; index.ts must restore `T | null`.
"""

import argparse
import re
import sys
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent
INDEX_TS = REPO_ROOT / "packages" / "shared" / "src" / "index.ts"
GENERATED_TS = REPO_ROOT / "packages" / "shared" / "src" / "generated.ts"
DTO_DIR = REPO_ROOT / "apps" / "api" / "internal" / "dto"


INTERFACE_RE = re.compile(r"^export\s+interface\s+(\w+)\s*\{", re.MULTILINE)
FIELD_RE = re.compile(r"^\s*(\w+)(\?)?\s*:\s*(.+?)(?:;|$)", re.MULTILINE)


def normalize_type(t: str) -> str:
    """Strip comments and whitespace for comparison."""
    t = re.sub(r"/\*.*?\*/", "", t)
    t = re.sub(r"//.*", "", t)
    return " ".join(t.split())


def parse_interfaces(path: Path) -> dict[str, dict[str, tuple[bool, str]]]:
    """Return {Name: {field: (optional, type)}}."""
    text = path.read_text()
    result: dict[str, dict[str, tuple[bool, str]]] = {}
    for m in INTERFACE_RE.finditer(text):
        name = m.group(1)
        body_start = m.end()
        depth = 1
        i = body_start
        while i < len(text) and depth > 0:
            if text[i] == "{":
                depth += 1
            elif text[i] == "}":
                depth -= 1
            i += 1
        body = text[body_start : i - 1]
        fields: dict[str, tuple[bool, str]] = {}
        for fm in FIELD_RE.finditer(body):
            fname = fm.group(1)
            optional = fm.group(2) == "?"
            ftype = normalize_type(fm.group(3).rstrip(",").strip())
            fields[fname] = (optional, ftype)
        result[name] = fields
    return result


GO_STRUCT_RE = re.compile(r"type\s+(\w+)\s+struct\s*{(.*?)}", re.S)
GO_FIELD_RE = re.compile(
    r"(\w+)\s+(\*[\w\[\].]+|\*?map\[[\w\[\]]+\][\w\[\].]+)\s+`json:\"([^\"]+)\"`"
)
NULLABLE_RE = re.compile(
    r"Nullable<(?:Omit<)?Gen\.(\w+)(?:,\s*((?:'[^']*'\s*(?:\|\s*'[^']*'\s*)*)))?(?:>)?,\s*((?:'[^']*'\s*(?:\|\s*'[^']*'\s*)*))>"
)
RESTATED_RE = re.compile(r"&\s*\{\s*([^}]+)\s*\}")
RESTATED_FIELD_RE = re.compile(r"(\w+)\s*:")


def go_pointer_without_omitempty() -> dict[str, list[str]]:
    """Return {GoStructName: [json field names of *T fields lacking omitempty]}."""
    result: dict[str, list[str]] = {}
    for path in DTO_DIR.glob("*.go"):
        text = path.read_text()
        for m in GO_STRUCT_RE.finditer(text):
            name = m.group(1)
            fields: list[str] = []
            for fm in GO_FIELD_RE.finditer(m.group(2)):
                if not fm.group(2).startswith("*"):
                    continue
                tag_parts = fm.group(3).split(",")
                if "omitempty" not in tag_parts:
                    fields.append(tag_parts[0])
            if fields:
                result[name] = fields
    return result


def index_nullability_coverage() -> dict[str, set[str]]:
    """Return {GenTypeName: set of field names narrowed via Nullable}."""
    text = INDEX_TS.read_text()
    coverage: dict[str, set[str]] = {}
    for m in NULLABLE_RE.finditer(text):
        fields = set(re.findall(r"'([^']*)'", m.group(2) or "")) | set(
            re.findall(r"'([^']*)'", m.group(3) or "")
        )
        coverage[m.group(1)] = fields
    restated: set[str] = set()
    for m in RESTATED_RE.finditer(text):
        for fm in RESTATED_FIELD_RE.finditer(m.group(1)):
            restated.add(fm.group(1))
    for gtype in list(coverage):
        coverage[gtype] |= restated
    return coverage


def check_go_nullability() -> list[str]:
    """Return uncovered Go pointer-no-omitempty fields on the public surface.

    A field is covered if index.ts names it in a `Nullable<Gen.X, '…'>` list
    for its type, or restates it in an intersection block.
    """
    coverage = index_nullability_coverage()
    index_text = INDEX_TS.read_text()
    problems: list[str] = []
    for gtype, fields in sorted(go_pointer_without_omitempty().items()):
        if not re.search(rf"Gen\.{gtype}\b", index_text):
            continue
        covered = coverage.get(gtype, set())
        for f in fields:
            if f not in covered:
                problems.append(f"Gen.{gtype}.{f}")
    return problems


def compare(
    index_ifaces: dict[str, dict[str, tuple[bool, str]]],
    gen_ifaces: dict[str, dict[str, tuple[bool, str]]],
):
    common = set(index_ifaces) & set(gen_ifaces)
    pairs = len(common)
    identical = 0
    drifted = 0
    null_union = 0
    alias_lost = 0
    literal_union_lost = 0
    missing_field = 0
    weakened: list[str] = []

    for name in sorted(common):
        idx = index_ifaces[name]
        gen = gen_ifaces[name]
        all_fields = set(idx) | set(gen)
        diffs = 0
        for f in sorted(all_fields):
            in_idx = f in idx
            in_gen = f in gen
            if not in_idx:
                missing_field += 1
                diffs += 1
                continue
            if not in_gen:
                missing_field += 1
                diffs += 1
                continue
            i_opt, i_type = idx[f]
            g_opt, g_type = gen[f]

            if i_opt == g_opt and i_type == g_type:
                continue

            diffs += 1

            if not i_opt and g_opt and i_type.endswith("| null"):
                null_union += 1
                base = i_type[: -len(" | null")].strip()
                if g_type != base:
                    weakened.append(f"{name}.{f}: index={i_type} gen={g_type}")
                continue

            if not i_opt and not g_opt:
                if "|" in i_type and g_type == "string":
                    literal_union_lost += 1
                    continue

            if not i_opt and not g_opt:
                if i_type != g_type:
                    alias_lost += 1
                    continue

            if i_opt != g_opt:
                null_union += 1
                continue

        if diffs == 0:
            identical += 1
        else:
            drifted += 1

    return {
        "pairs": pairs,
        "identical": identical,
        "drifted": drifted,
        "null_union": null_union,
        "alias_lost": alias_lost,
        "literal_union_lost": literal_union_lost,
        "missing_field": missing_field,
        "weakened": weakened,
    }


def main():
    parser = argparse.ArgumentParser(description="Compare shared type definitions")
    parser.add_argument(
        "--check-strictness",
        action="store_true",
        help="Fail non-zero if any field weakens",
    )
    parser.add_argument(
        "--check-duplicates",
        action="store_true",
        help="Fail non-zero if any shape is defined in both files",
    )
    parser.add_argument(
        "--check-go-nullability",
        action="store_true",
        help="Fail non-zero if a Go *T field without omitempty is not Nullable",
    )
    args = parser.parse_args()

    if args.check_go_nullability:
        problems = check_go_nullability()
        if problems:
            print(f"GO NULLABILITY FAILURE: {len(problems)} uncovered fields:")
            for p in problems:
                print(f"  {p}")
            print("Each Go *T field without omitempty always hits the wire as null;")
            print("index.ts must name it in a Nullable list or restate it inline.")
            sys.exit(1)
        print(
            "go-nullability check passed — all Go pointer-no-omitempty fields covered"
        )
        sys.exit(0)

    index_ifaces = parse_interfaces(INDEX_TS)
    gen_ifaces = parse_interfaces(GENERATED_TS)

    result = compare(index_ifaces, gen_ifaces)

    if args.check_duplicates:
        common = set(index_ifaces) & set(gen_ifaces)
        if common:
            print(f"DUPLICATE: {len(common)} shapes defined in both files:")
            for name in sorted(common):
                print(f"  {name}")
            sys.exit(1)
        print("pairs=0 — no duplicated shapes")
        sys.exit(0)

    if args.check_strictness:
        if result["weakened"]:
            print(f"STRICTNESS FAILURE: {len(result['weakened'])} weakened fields:")
            for w in result["weakened"]:
                print(f"  {w}")
            sys.exit(1)
        print("strictness check passed — 0 weakened fields")
        sys.exit(0)

    print(
        f"pairs={result['pairs']} identical={result['identical']} drifted={result['drifted']}"
    )
    print(
        f"null-union={result['null_union']} alias-lost={result['alias_lost']} "
        f"literal-union-lost={result['literal_union_lost']} missing-field={result['missing_field']}"
    )


if __name__ == "__main__":
    main()
