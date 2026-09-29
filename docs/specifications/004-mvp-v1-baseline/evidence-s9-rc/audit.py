"""Structural checkpoint audit only. Not semantic T25 closure or acceptance."""
import json
from pathlib import Path
import re

root = Path(__file__).resolve().parent
repository = root.parents[3]
coverage = json.loads((root / "coverage.json").read_text())
ids = [row["id"] for row in coverage["rows"]]
assert len(ids) == len(set(ids)), "duplicate coverage identifier"
expected = {f"AC-{i:02d}" for i in range(1, 50)} | {f"FR-{i:03d}" for i in range(1, 68)}
expected |= {f"MVP-SEC-{i:02d}" for i in range(1, 10)} | {f"MVP-NFR-{i:02d}" for i in range(1, 8)}
expected |= {f"SEC-{i:03d}" for i in range(1, 6)} | {f"ADR-{i:04d}" for i in range(1, 10)}
expected |= {f"HD-{i}" for i in range(1, 5)} | {f"CONSTITUTION-{i}" for i in ("II", "III", "IV", "V", "VI")}
assert expected <= set(ids), "required coverage identifier missing"
for row in coverage["rows"]:
    assert row["status"] in {"pending", "confirmed", "failed", "blocked", "not applicable"}
    for field in ("required_observation", "platforms", "evidence_kind", "tooling_command_or_source", "artifact", "authority"):
        assert field in row, (row["id"], field)
assert next(row for row in coverage["rows"] if row["id"] == "AC-24")["status"] == "pending"

tasks = (root.parent / "tasks.md").read_text()
table = {}
for task, dependencies in re.findall(r"^\| (T\d+) \| S\d+ \| [^|]+ \| ([^|]+) \|$", tasks, re.M):
    table[task] = set(re.findall(r"T\d+", dependencies))
table_edges = {(source, target) for target, sources in table.items() for source in sources}
mermaid = tasks.split("```mermaid\nflowchart LR", 1)[1].split("```", 1)[0]
mermaid_edges = set()
for line in mermaid.splitlines():
    nodes = re.findall(r"T\d+", line)
    mermaid_edges.update(zip(nodes, nodes[1:]))
body = {}
for task, section in re.findall(r"^### (T\d+) [^\n]+\n(.*?)(?=^### T\d+ |^## |\Z)", tasks, re.M | re.S):
    dependencies = re.search(r"\*\*Dependencies:\*\* ([^\n]+)", section)
    assert dependencies, task
    body[task] = set(re.findall(r"T\d+", dependencies[1]))
assert table == body, "Task table/body dependency mismatch"
assert table_edges == mermaid_edges, "Task Mermaid dependency mismatch"
pending = set(table)
done = set()
while pending:
    ready = {task for task in pending if table[task] <= done}
    assert ready, "Task DAG cycle or unknown dependency"
    pending -= ready
    done |= ready

checked_links = 0
for path in (root.parent / "evidence-s9.md", root.parent / "evidence-s9-t24.md"):
    for link in re.findall(r"\]\(([^)]+)\)", path.read_text()):
        if "://" in link or link.startswith("#"):
            continue
        target = link.split("#", 1)[0]
        assert (path.parent / target).exists(), (str(path), link)
        checked_links += 1
print(json.dumps({"result": "pass", "scope": "identifier coverage, Task DAG, checkpoint local link paths",
                  "coverageRows": len(ids), "taskCount": len(table), "edgeCount": len(table_edges),
                  "localLinksChecked": checked_links,
                  "limitations": ["anchors/external links not checked", "semantic audits remain blocked by T24"]}))
