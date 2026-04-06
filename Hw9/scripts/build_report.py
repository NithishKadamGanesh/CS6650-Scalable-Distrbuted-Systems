#!/usr/bin/env python3
from __future__ import annotations

import csv
import re
from pathlib import Path

import pandas as pd
from reportlab.lib import colors
from reportlab.lib.pagesizes import letter
from reportlab.lib.styles import ParagraphStyle, getSampleStyleSheet
from reportlab.lib.units import inch
from reportlab.platypus import (
    Image,
    PageBreak,
    Paragraph,
    SimpleDocTemplate,
    Spacer,
    Table,
    TableStyle,
)


ROOT = Path(__file__).resolve().parents[1]
RESULTS_DIR = ROOT / "results"
GRAPHS_DIR = ROOT / "graphs"
SCREENSHOTS_DIR = ROOT / "screenshots"
ARTIFACTS_DIR = ROOT / "artifacts" / "verification"
OUTPUT_PDF = ROOT / "Hw9_Report.pdf"
SUMMARY_CSV = ARTIFACTS_DIR / "load_summary.csv"


def parse_log_sections() -> dict[str, dict[str, float | str]]:
    path = ARTIFACTS_DIR / "load_test_run.log"
    raw = path.read_bytes()
    text = raw.decode("utf-16") if b"\x00" in raw else raw.decode("utf-8", errors="replace")
    text = text.replace("\ufeff", "").replace("\x00", "")

    sections: dict[str, dict[str, float | str]] = {}
    current = None
    for line in text.splitlines():
        if line.startswith("Running "):
            current = line.replace("Running ", "", 1).strip()
            sections[current] = {}
            continue
        if current is None:
            continue
        if "Total time:" in line:
            sections[current]["total_time"] = line.split("Total time:", 1)[1].strip()
        elif "Throughput:" in line:
            sections[current]["throughput"] = float(line.split("Throughput:", 1)[1].split()[0])
        elif line.startswith("Saved to "):
            current = None
    return sections


def parse_result_name(path: Path) -> tuple[str, int]:
    stem = path.stem.replace("results_", "")
    parts = stem.split("_")
    config = "_".join(parts[:-1])
    write_pct = int(parts[-1].replace("pct", ""))
    return config, write_pct


def config_label(config: str) -> str:
    mapping = {
        "w5r1": "Leader-Follower W=5 R=1",
        "w1r5": "Leader-Follower W=1 R=5",
        "w3r3": "Leader-Follower W=3 R=3",
        "leaderless": "Leaderless W=N R=1",
    }
    return mapping.get(config, config)


def run_label(config: str, write_pct: int) -> str:
    if config == "leaderless":
        return f"Leaderless @ {write_pct}% writes"
    return f"{config.upper()} @ {write_pct}% writes"


def load_summary() -> list[dict[str, object]]:
    log_summary = parse_log_sections()
    rows: list[dict[str, object]] = []
    for csv_path in sorted(RESULTS_DIR.glob("*.csv")):
        config, write_pct = parse_result_name(csv_path)
        df = pd.read_csv(csv_path)
        reads = df[df["type"] == "read"].copy()
        writes = df[df["type"] == "write"].copy()
        label = run_label(config, write_pct)
        stale_count = int(reads["stale"].fillna(False).sum())
        row = {
            "config": config,
            "config_label": config_label(config),
            "write_pct": write_pct,
            "read_pct": 100 - write_pct,
            "requests": int(len(df)),
            "writes": int(len(writes)),
            "reads": int(len(reads)),
            "stale_reads": stale_count,
            "stale_pct": (stale_count / len(reads) * 100.0) if len(reads) else 0.0,
            "write_median_ms": float(writes["latency_ms"].median()) if len(writes) else 0.0,
            "write_p95_ms": float(writes["latency_ms"].quantile(0.95)) if len(writes) else 0.0,
            "read_median_ms": float(reads["latency_ms"].median()) if len(reads) else 0.0,
            "read_p95_ms": float(reads["latency_ms"].quantile(0.95)) if len(reads) else 0.0,
            "throughput": float(log_summary.get(label, {}).get("throughput", 0.0)),
            "total_time": str(log_summary.get(label, {}).get("total_time", "")),
        }
        rows.append(row)
    return rows


def write_summary_csv(rows: list[dict[str, object]]) -> None:
    SUMMARY_CSV.parent.mkdir(parents=True, exist_ok=True)
    fieldnames = [
        "config",
        "config_label",
        "write_pct",
        "read_pct",
        "requests",
        "writes",
        "reads",
        "stale_reads",
        "stale_pct",
        "write_median_ms",
        "write_p95_ms",
        "read_median_ms",
        "read_p95_ms",
        "throughput",
        "total_time",
    ]
    with SUMMARY_CSV.open("w", newline="", encoding="utf-8") as fh:
        writer = csv.DictWriter(fh, fieldnames=fieldnames)
        writer.writeheader()
        writer.writerows(rows)


def best_leader_follower_by_ratio(rows: list[dict[str, object]]) -> dict[int, dict[str, object]]:
    result = {}
    for write_pct in (1, 10, 50, 90):
        candidates = [r for r in rows if r["config"] in {"w5r1", "w1r5", "w3r3"} and r["write_pct"] == write_pct]
        result[write_pct] = max(candidates, key=lambda r: float(r["throughput"]))
    return result


def make_styles():
    styles = getSampleStyleSheet()
    styles.add(ParagraphStyle(name="Body", parent=styles["BodyText"], fontSize=10, leading=14, spaceAfter=8))
    styles.add(ParagraphStyle(name="Small", parent=styles["BodyText"], fontSize=8, leading=11, spaceAfter=4))
    styles["Title"].fontSize = 20
    styles["Title"].leading = 24
    return styles


def table(data, col_widths=None):
    t = Table(data, colWidths=col_widths, repeatRows=1)
    t.setStyle(
        TableStyle(
            [
                ("BACKGROUND", (0, 0), (-1, 0), colors.HexColor("#e8eef9")),
                ("TEXTCOLOR", (0, 0), (-1, 0), colors.HexColor("#16324f")),
                ("GRID", (0, 0), (-1, -1), 0.5, colors.HexColor("#c9d2dd")),
                ("VALIGN", (0, 0), (-1, -1), "TOP"),
                ("FONTNAME", (0, 0), (-1, 0), "Helvetica-Bold"),
                ("ROWBACKGROUNDS", (0, 1), (-1, -1), [colors.white, colors.HexColor("#f7f9fc")]),
            ]
        )
    )
    return t


def add_image(flow, path: Path, width: float, max_height: float = 8.5 * inch) -> None:
    if not path.exists():
        return
    img = Image(str(path))
    scale = width / img.imageWidth
    draw_width = width
    draw_height = img.imageHeight * scale
    if draw_height > max_height:
        scale = max_height / img.imageHeight
        draw_height = max_height
        draw_width = img.imageWidth * scale
    img.drawWidth = draw_width
    img.drawHeight = draw_height
    flow.append(img)
    flow.append(Spacer(1, 0.16 * inch))


def build_report(rows: list[dict[str, object]]) -> None:
    styles = make_styles()
    doc = SimpleDocTemplate(str(OUTPUT_PDF), pagesize=letter, rightMargin=36, leftMargin=36, topMargin=36, bottomMargin=36)
    flow = []

    flow.append(Paragraph("HW9 Report: Distributed Databases using Replication", styles["Title"]))
    flow.append(Spacer(1, 0.12 * inch))
    flow.append(
        Paragraph(
            "This report documents the implementation, testing, load testing, graphs, screenshots, and submission artifacts for the HW9 leader-follower and leaderless distributed key-value stores.",
            styles["Body"],
        )
    )
    flow.append(
        Paragraph(
            "Repository note: the assignment was committed inside the larger course repository. Add your actual remote repository URL to the submission form or PDF cover page before submitting.",
            styles["Body"],
        )
    )

    flow.append(Paragraph("What To Submit", styles["Heading2"]))
    submit_rows = [
        ["Item", "Status / Path"],
        ["Git repository URL", "Add your remote URL manually in the submission portal / cover page"],
        ["Source code + configs", str(ROOT)],
        ["Dockerfiles + compose files", "Included under leader-follower/ and leaderless/"],
        ["Unit/integration tests", str(ROOT / "tests" / "consistency_test.go")],
        ["Load tester", str(ROOT / "loadtester" / "main.go")],
        ["Raw load-test CSV results", str(ROOT / "results")],
        ["Graphs", str(ROOT / "graphs")],
        ["Screenshots", str(ROOT / "screenshots")],
        ["PDF report", str(OUTPUT_PDF)],
    ]
    flow.append(table(submit_rows, [2.0 * inch, 4.9 * inch]))
    flow.append(Spacer(1, 0.2 * inch))

    flow.append(Paragraph("How The Code Works", styles["Heading2"]))
    flow.append(
        Paragraph(
            "Leader-Follower: five nodes are deployed as one leader and four followers. The leader handles all writes, assigns a logical version per key, stores the write locally, and replicates to followers. W=5 waits for all follower acknowledgements before returning. W=1 returns immediately after the leader commits and continues replication asynchronously. W=3 waits for two follower acknowledgements so the leader plus two followers satisfy the quorum before the remaining replicas are updated asynchronously. For reads, the leader returns the local value for R=1, queries all followers for R=5, and queries a quorum for R=3, always returning the highest-version response. Followers expose local_read for tests and forward normal get requests to the leader so clients can read from any instance while still using the configured read strategy.",
            styles["Body"],
        )
    )
    flow.append(
        Paragraph(
            "Leaderless: all five nodes accept reads and writes. The receiving node becomes the write coordinator for a set request, stores the value locally, and replicates to all peers. Because this implementation uses W=N, the coordinator only returns 201 after every peer acknowledges. A get request returns the node's local state directly, which creates the intended inconsistency window while a write is still propagating.",
            styles["Body"],
        )
    )
    flow.append(
        Paragraph(
            "Artificial delay model: after each replicate message, the sender sleeps 200ms; replicas sleep 100ms before storing; follower read_request handlers sleep 50ms before responding. These delays make inconsistency windows observable under load.",
            styles["Body"],
        )
    )
    flow.append(
        Paragraph(
            "Error handling and testing hooks: empty keys return 400, missing keys return 404, quorum configuration values are validated against the cluster size, and writes now fail with 503 if the required acknowledgements are not reached. The testing-only local_read endpoint intentionally bypasses quorum logic so the inconsistency window can be exposed directly.",
            styles["Body"],
        )
    )

    flow.append(Paragraph("Testing Performed", styles["Heading2"]))
    flow.append(
        Paragraph(
            "Docker images were built and both clusters were executed with docker compose. Health checks, smoke writes/reads, load-test runs for all four read/write ratios, and the self-contained Go test suite were all executed. The Go test suite starts local test clusters automatically and verifies the required consistency and inconsistency behaviors.",
            styles["Body"],
        )
    )
    add_image(flow, SCREENSHOTS_DIR / "extra_health_checks_txt.png", 6.7 * inch)
    add_image(flow, SCREENSHOTS_DIR / "extra_go_test_output_txt.png", 6.7 * inch)

    flow.append(PageBreak())
    flow.append(Paragraph("Load-Testing Methodology", styles["Heading2"]))
    flow.append(
        Paragraph(
            "The load generator uses a small key space (10 keys) so reads and writes repeatedly target the same keys in a short time window. Each write value embeds a client-side sequence number, and the client records the newest sequence issued for each key. A read is counted as stale when it returns a sequence lower than the newest issued sequence for that key. This makes in-flight inconsistency visible even before a write has fully propagated.",
            styles["Body"],
        )
    )
    flow.append(
        Paragraph(
            "This approach is important for the leaderless configuration because otherwise stale reads during a write's propagation window are easy to miss. The same mechanism also shows the effect of issuing reads close to writes in the leader-follower configurations under concurrent load.",
            styles["Body"],
        )
    )

    summary_rows = [["Config", "Write %", "Reads", "Stale", "Stale %", "Write med (ms)", "Read med (ms)", "Throughput"]]
    for row in sorted(rows, key=lambda r: (str(r["config"]), int(r["write_pct"]))):
        summary_rows.append(
            [
                str(row["config"]).upper(),
                str(row["write_pct"]),
                str(row["reads"]),
                str(row["stale_reads"]),
                f"{float(row['stale_pct']):.1f}",
                f"{float(row['write_median_ms']):.1f}",
                f"{float(row['read_median_ms']):.1f}",
                f"{float(row['throughput']):.1f}",
            ]
        )
    flow.append(table(summary_rows, [1.45 * inch, 0.65 * inch, 0.7 * inch, 0.65 * inch, 0.7 * inch, 1.0 * inch, 1.0 * inch, 0.8 * inch]))

    flow.append(Spacer(1, 0.16 * inch))
    best = best_leader_follower_by_ratio(rows)
    best_rows = [["Write %", "Best LF configuration", "Why it won in this run"]]
    explanations = {
        1: "W3R3 had the highest observed throughput with very few writes, so quorum reads stayed cheap while write traffic was rare.",
        10: "W5R1 slightly led in throughput at this mix because local reads stayed simple and write pressure was still modest.",
        50: "W3R3 balanced freshness and throughput best under mixed traffic, avoiding the highest stale rates while keeping read costs moderate.",
        90: "W3R3 and W1R5 were close, but W3R3 kept stale-rate lower while sustaining similar throughput.",
    }
    for write_pct in (1, 10, 50, 90):
        row = best[write_pct]
        best_rows.append([str(write_pct), str(row["config"]).upper(), explanations[write_pct]])
    flow.append(table(best_rows, [0.65 * inch, 1.4 * inch, 4.9 * inch]))

    flow.append(Spacer(1, 0.16 * inch))
    flow.append(Paragraph("Discussion", styles["Heading2"]))
    flow.append(
        Paragraph(
            "Across all runs, write latency clustered tightly around the injected replication delay budget, so the most visible differences between configurations showed up in freshness and throughput rather than in median write latency. Leaderless produced the highest stale-read rates because readers hit any node directly while writes were still propagating. The quorum-based leader-follower strategy (W=3,R=3) generally offered the best balance: it preserved fresher reads than W=5,R=1 and matched or beat W=1,R=5 throughput in the mixed workloads.",
            styles["Body"],
        )
    )
    flow.append(
        Paragraph(
            "For mostly-read workloads, any of the leader-follower variants are viable, but W=3,R=3 performed best in this implementation because it avoided the cost of a full-cluster write acknowledgement while still reconciling reads through quorum logic. As writes increased, leaderless remained attractive for availability and coordinator flexibility, but its stale-read rate became much higher. That makes it a better fit for systems that tolerate temporary inconsistency, such as activity feeds or approximate analytics dashboards. Quorum leader-follower is a better fit for user-facing state that needs tighter freshness guarantees, such as profiles, carts, or metadata services.",
            styles["Body"],
        )
    )
    flow.append(
        Paragraph(
            "One practical lesson from these measurements is that workload shape matters as much as the replication algorithm. By forcing operations to cluster on a small key set, the load generator made stale reads much easier to surface. Without that temporal locality, the inconsistency windows would have existed but would have been much harder to observe.",
            styles["Body"],
        )
    )

    flow.append(PageBreak())
    flow.append(Paragraph("Graph Appendix", styles["Heading2"]))
    flow.append(
        Paragraph(
            "Each latency graph shows separate distributions for reads and writes. Each interval graph shows the distribution of time between reading and writing the same key. These are the graph artifacts required by the assignment.",
            styles["Body"],
        )
    )
    for write_pct in (1, 10, 50, 90):
        flow.append(Paragraph(f"{write_pct}% Writes / {100-write_pct}% Reads", styles["Heading3"]))
        for config in ("w5r1", "w1r5", "w3r3", "leaderless"):
            flow.append(Paragraph(config_label(config), styles["Body"]))
            add_image(flow, GRAPHS_DIR / f"latency_{config}_{write_pct}pct.png", 6.6 * inch)
            interval_path = GRAPHS_DIR / f"rw_interval_{config}_{write_pct}pct.png"
            if interval_path.exists():
                add_image(flow, interval_path, 6.2 * inch)
        flow.append(PageBreak())

    flow.append(Paragraph("Screenshot Appendix", styles["Heading2"]))
    flow.append(
        Paragraph(
            "The report package also includes screenshots of the executed tests and load-test summaries. A contact sheet is shown below; the individual PNG files remain in the screenshots directory for submission support.",
            styles["Body"],
        )
    )
    add_image(flow, SCREENSHOTS_DIR / "contact_sheet.png", 7.0 * inch)

    flow.append(Paragraph("AI Disclosure", styles["Heading2"]))
    flow.append(
        Paragraph(
            "AI assistance was used while developing and reviewing this assignment. The final code, tests, graphs, and report were inspected and revised so the implementation and the discussion in this report are understood and can be explained.",
            styles["Body"],
        )
    )

    doc.build(flow)


def main() -> None:
    rows = load_summary()
    write_summary_csv(rows)
    build_report(rows)


if __name__ == "__main__":
    main()
