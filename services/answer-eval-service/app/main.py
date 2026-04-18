from __future__ import annotations

import argparse
import asyncio
import csv
import json
import os
import time
from collections import Counter
from datetime import UTC, datetime
from pathlib import Path
from statistics import mean, stdev
from typing import Any

import httpx


DEFAULT_MODES = ["baseline", "reasoning", "agentic"]
DEFAULT_QUESTIONS_FILE = Path("questions/cpp-small.json")
CSV_FIELDS = [
    "run_id",
    "timestamp_utc",
    "question_id",
    "question",
    "mode",
    "repeat",
    "ok",
    "status_code",
    "latency_ms",
    "error",
    "answer_chars",
    "answer_words",
    "input_tokens",
    "output_tokens",
    "total_tokens",
    "limitations_count",
    "citation_count",
    "citation_unique_chunks",
    "citation_duplicate_chunks",
    "citation_unique_files",
    "context_block_count",
    "context_unique_chunks",
    "context_duplicate_chunks",
    "context_unique_files",
    "context_total_chars",
    "context_avg_chars",
    "context_avg_score",
    "context_max_score",
    "source_keyword_count",
    "source_vector_count",
    "source_graph_count",
    "source_context_expansion_count",
    "source_other_count",
]


def main() -> None:
    args = parse_args()
    asyncio.run(run_eval(args))


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Compare query-service /answer modes.")
    parser.add_argument(
        "--query-service-url",
        default=os.getenv("QUERY_SERVICE_URL", "http://localhost:8082"),
        help="Base URL for query-service.",
    )
    parser.add_argument(
        "--questions-file",
        type=Path,
        default=Path(os.getenv("EVAL_QUESTIONS_FILE", str(DEFAULT_QUESTIONS_FILE))),
        help="JSON file containing question objects with id and query fields.",
    )
    parser.add_argument(
        "--output-dir",
        type=Path,
        default=Path(os.getenv("EVAL_OUTPUT_DIR", "results")),
        help="Directory for CSV and JSONL output.",
    )
    parser.add_argument(
        "--modes",
        default=os.getenv("EVAL_MODES", ",".join(DEFAULT_MODES)),
        help="Comma-separated answer modes to run.",
    )
    parser.add_argument(
        "--repeats",
        type=int,
        default=int(os.getenv("EVAL_REPEATS", "1")),
        help="Number of times to run each question/mode pair.",
    )
    parser.add_argument("--repo-url", default=os.getenv("EVAL_REPO_URL"))
    parser.add_argument("--branch", default=os.getenv("EVAL_BRANCH", "main"))
    parser.add_argument("--snapshot-id", default=os.getenv("EVAL_SNAPSHOT_ID"))
    parser.add_argument("--top-k", type=int, default=int(os.getenv("EVAL_TOP_K", "8")))
    parser.add_argument(
        "--context-token-budget",
        type=int,
        default=int(os.getenv("EVAL_CONTEXT_TOKEN_BUDGET", "12000")),
    )
    parser.add_argument(
        "--timeout-seconds",
        type=float,
        default=float(os.getenv("EVAL_TIMEOUT_SECONDS", "180")),
    )
    parser.add_argument(
        "--delay-seconds",
        type=float,
        default=float(os.getenv("EVAL_DELAY_SECONDS", "0")),
        help="Optional delay between requests.",
    )
    parser.add_argument(
        "--plots",
        type=parse_bool,
        default=parse_bool(os.getenv("EVAL_PLOTS", "true")),
        help="Whether to generate matplotlib PNG plots.",
    )
    return parser.parse_args()


async def run_eval(args: argparse.Namespace) -> None:
    questions = load_questions(args.questions_file)
    modes = [mode.strip() for mode in args.modes.split(",") if mode.strip()]
    run_id = datetime.now(UTC).strftime("%Y%m%dT%H%M%SZ")
    output_dir = args.output_dir
    output_dir.mkdir(parents=True, exist_ok=True)

    csv_path = output_dir / f"answer-eval-{run_id}.csv"
    jsonl_path = output_dir / f"answer-eval-{run_id}.jsonl"
    answer_url = args.query_service_url.rstrip("/") + "/answer"

    print(f"Running {len(questions)} questions x {len(modes)} modes x {args.repeats} repeats")
    print(f"Query service: {answer_url}")
    print(f"Writing metrics: {csv_path}")
    print(f"Writing raw runs: {jsonl_path}")

    metrics_rows: list[dict[str, Any]] = []
    timeout = httpx.Timeout(args.timeout_seconds)
    async with httpx.AsyncClient(timeout=timeout) as client:
        with csv_path.open("w", newline="", encoding="utf-8") as csv_file, jsonl_path.open(
            "w",
            encoding="utf-8",
        ) as jsonl_file:
            writer = csv.DictWriter(csv_file, fieldnames=CSV_FIELDS)
            writer.writeheader()

            for repeat in range(1, args.repeats + 1):
                for question in questions:
                    for mode in modes:
                        payload = build_payload(args, question["query"], mode)
                        record = await run_once(client, answer_url, payload)
                        metrics = compute_metrics(
                            run_id=run_id,
                            question_id=question["id"],
                            question=question["query"],
                            mode=mode,
                            repeat=repeat,
                            record=record,
                        )
                        metrics_rows.append(metrics)
                        writer.writerow(metrics)
                        jsonl_file.write(
                            json.dumps(
                                {
                                    "metrics": metrics,
                                    "request": payload,
                                    "response": record.get("response"),
                                },
                                ensure_ascii=False,
                            )
                            + "\n"
                        )
                        csv_file.flush()
                        jsonl_file.flush()

                        print(
                            f"{question['id']} {mode} repeat={repeat} "
                            f"ok={metrics['ok']} latency_ms={metrics['latency_ms']} "
                            f"context_blocks={metrics['context_block_count']} citations={metrics['citation_count']}"
                        )
                        if args.delay_seconds > 0:
                            await asyncio.sleep(args.delay_seconds)

    if args.plots:
        plots_dir = output_dir / f"plots-{run_id}"
        plot_paths = generate_plots(metrics_rows, plots_dir)
        print(f"Writing plots: {plots_dir}")
        for path in plot_paths:
            print(f"  {path}")

    print("Done.")


def parse_bool(value: str | bool) -> bool:
    if isinstance(value, bool):
        return value
    return value.strip().lower() in {"1", "true", "yes", "y", "on"}


def load_questions(path: Path) -> list[dict[str, str]]:
    with path.open(encoding="utf-8") as file:
        data = json.load(file)

    if not isinstance(data, list):
        raise ValueError("questions file must contain a JSON array")

    questions = []
    for idx, item in enumerate(data, start=1):
        if not isinstance(item, dict):
            raise ValueError(f"question #{idx} must be an object")
        question_id = str(item.get("id") or f"q{idx}").strip()
        query = str(item.get("query") or "").strip()
        if not query:
            raise ValueError(f"question {question_id} is missing query")
        questions.append({"id": question_id, "query": query})
    return questions


def build_payload(args: argparse.Namespace, query: str, mode: str) -> dict[str, Any]:
    payload: dict[str, Any] = {
        "query": query,
        "branch": args.branch,
        "top_k": args.top_k,
        "context_token_budget": args.context_token_budget,
        "mode": mode,
    }
    if args.repo_url:
        payload["repo_url"] = args.repo_url
    if args.snapshot_id:
        payload["snapshot_id"] = args.snapshot_id
    return payload


async def run_once(client: httpx.AsyncClient, answer_url: str, payload: dict[str, Any]) -> dict[str, Any]:
    started = time.perf_counter()
    try:
        response = await client.post(answer_url, json=payload)
        latency_ms = round((time.perf_counter() - started) * 1000, 2)
        try:
            body = response.json()
        except json.JSONDecodeError:
            body = {"raw_body": response.text}
        return {
            "ok": response.is_success,
            "status_code": response.status_code,
            "latency_ms": latency_ms,
            "response": body,
            "error": "" if response.is_success else response.text[:1000],
        }
    except Exception as err:
        latency_ms = round((time.perf_counter() - started) * 1000, 2)
        return {
            "ok": False,
            "status_code": 0,
            "latency_ms": latency_ms,
            "response": {},
            "error": str(err),
        }


def compute_metrics(
    *,
    run_id: str,
    question_id: str,
    question: str,
    mode: str,
    repeat: int,
    record: dict[str, Any],
) -> dict[str, Any]:
    response = record.get("response") if isinstance(record.get("response"), dict) else {}
    answer = response.get("answer") or ""
    citations = response.get("citations") if isinstance(response.get("citations"), list) else []
    context_blocks = response.get("context_blocks") if isinstance(response.get("context_blocks"), list) else []
    limitations = response.get("limitations") if isinstance(response.get("limitations"), list) else []
    token_usage = response.get("token_usage") if isinstance(response.get("token_usage"), dict) else {}

    citation_chunks = [item.get("chunk_id") for item in citations if isinstance(item, dict) and item.get("chunk_id")]
    citation_files = [item.get("file_path") for item in citations if isinstance(item, dict) and item.get("file_path")]
    context_chunks = [
        item.get("chunk_id") for item in context_blocks if isinstance(item, dict) and item.get("chunk_id")
    ]
    context_files = [
        item.get("file_path") for item in context_blocks if isinstance(item, dict) and item.get("file_path")
    ]
    source_counts = count_sources(context_blocks)
    context_lengths = [
        len(item.get("content") or "") for item in context_blocks if isinstance(item, dict)
    ]
    scores = [
        float(item.get("score") or 0)
        for item in context_blocks
        if isinstance(item, dict) and isinstance(item.get("score"), int | float)
    ]

    return {
        "run_id": run_id,
        "timestamp_utc": datetime.now(UTC).isoformat(),
        "question_id": question_id,
        "question": question,
        "mode": mode,
        "repeat": repeat,
        "ok": record.get("ok", False),
        "status_code": record.get("status_code", 0),
        "latency_ms": record.get("latency_ms", 0),
        "error": record.get("error", ""),
        "answer_chars": len(answer),
        "answer_words": len(answer.split()),
        "input_tokens": optional_number(token_usage.get("input_tokens")),
        "output_tokens": optional_number(token_usage.get("output_tokens")),
        "total_tokens": optional_number(token_usage.get("total_tokens")),
        "limitations_count": len(limitations),
        "citation_count": len(citations),
        "citation_unique_chunks": len(set(citation_chunks)),
        "citation_duplicate_chunks": max(0, len(citation_chunks) - len(set(citation_chunks))),
        "citation_unique_files": len(set(citation_files)),
        "context_block_count": len(context_blocks),
        "context_unique_chunks": len(set(context_chunks)),
        "context_duplicate_chunks": max(0, len(context_chunks) - len(set(context_chunks))),
        "context_unique_files": len(set(context_files)),
        "context_total_chars": sum(context_lengths),
        "context_avg_chars": round(sum(context_lengths) / len(context_lengths), 2) if context_lengths else 0,
        "context_avg_score": round(sum(scores) / len(scores), 4) if scores else 0,
        "context_max_score": round(max(scores), 4) if scores else 0,
        "source_keyword_count": source_counts["keyword"],
        "source_vector_count": source_counts["vector"],
        "source_graph_count": source_counts["graph"],
        "source_context_expansion_count": source_counts["context_expansion"],
        "source_other_count": sum(
            count for source, count in source_counts.items() if source not in {"keyword", "vector", "graph", "context_expansion"}
        ),
    }


def count_sources(context_blocks: list[Any]) -> Counter[str]:
    counts: Counter[str] = Counter()
    for block in context_blocks:
        if not isinstance(block, dict):
            continue
        sources = block.get("sources")
        if not isinstance(sources, list):
            continue
        for source in sources:
            if isinstance(source, str) and source:
                counts[source] += 1
    return counts


def optional_number(value: Any) -> int:
    if isinstance(value, bool):
        return 0
    if isinstance(value, int | float):
        return int(value)
    return 0


def generate_plots(rows: list[dict[str, Any]], output_dir: Path) -> list[Path]:
    if not rows:
        return []

    os.environ.setdefault("MPLCONFIGDIR", str(output_dir / ".matplotlib-cache"))
    os.environ.setdefault("XDG_CACHE_HOME", str(output_dir / ".cache"))
    import matplotlib

    matplotlib.use("Agg")
    import matplotlib.pyplot as plt

    output_dir.mkdir(parents=True, exist_ok=True)
    plot_paths = [
        plot_latency_by_mode(rows, output_dir, plt),
        plot_latency_by_question(rows, output_dir, plt),
        plot_tokens_by_mode(rows, output_dir, plt),
        plot_tokens_by_question(rows, output_dir, plt),
        plot_retrieval_volume_by_mode(rows, output_dir, plt),
        plot_source_mix_by_mode(rows, output_dir, plt),
    ]
    return [path for path in plot_paths if path is not None]


def plot_latency_by_mode(rows: list[dict[str, Any]], output_dir: Path, plt: Any) -> Path:
    mode_values = values_by_mode(rows, "latency_ms")
    modes = list(mode_values)
    averages = [mean(mode_values[mode]) for mode in modes]
    errors = [stdev(mode_values[mode]) if len(mode_values[mode]) > 1 else 0 for mode in modes]

    fig, ax = plt.subplots(figsize=(8, 5))
    ax.bar(modes, averages, yerr=errors, capsize=4, color=["#4c78a8", "#f58518", "#54a24b"][: len(modes)])
    ax.set_title("Mean Latency By Mode")
    ax.set_xlabel("Mode")
    ax.set_ylabel("Latency (ms)")
    ax.grid(axis="y", alpha=0.25)
    fig.tight_layout()

    path = output_dir / "latency_by_mode.png"
    fig.savefig(path, dpi=160)
    plt.close(fig)
    return path


def plot_latency_by_question(rows: list[dict[str, Any]], output_dir: Path, plt: Any) -> Path:
    questions = ordered_unique(str(row["question_id"]) for row in rows)
    modes = ordered_unique(str(row["mode"]) for row in rows)
    grouped = {
        (str(row["question_id"]), str(row["mode"])): []
        for row in rows
    }
    for row in rows:
        grouped.setdefault((str(row["question_id"]), str(row["mode"])), []).append(float(row["latency_ms"]))

    width = 0.8 / max(1, len(modes))
    x_positions = list(range(len(questions)))

    fig, ax = plt.subplots(figsize=(max(10, len(questions) * 1.4), 5.5))
    for idx, mode in enumerate(modes):
        offsets = [x + (idx - (len(modes) - 1) / 2) * width for x in x_positions]
        values = [
            mean(grouped.get((question, mode), [0]))
            for question in questions
        ]
        ax.bar(offsets, values, width=width, label=mode)

    ax.set_title("Mean Latency By Question")
    ax.set_xlabel("Question")
    ax.set_ylabel("Latency (ms)")
    ax.set_xticks(x_positions)
    ax.set_xticklabels(questions, rotation=35, ha="right")
    ax.legend()
    ax.grid(axis="y", alpha=0.25)
    fig.tight_layout()

    path = output_dir / "latency_by_question.png"
    fig.savefig(path, dpi=160)
    plt.close(fig)
    return path


def plot_retrieval_volume_by_mode(rows: list[dict[str, Any]], output_dir: Path, plt: Any) -> Path:
    modes = ordered_unique(str(row["mode"]) for row in rows)
    metrics = [
        ("context_block_count", "Context blocks"),
        ("citation_count", "Citations"),
        ("context_unique_files", "Unique files"),
    ]
    width = 0.8 / len(metrics)
    x_positions = list(range(len(modes)))

    fig, ax = plt.subplots(figsize=(9, 5))
    for idx, (metric, label) in enumerate(metrics):
        offsets = [x + (idx - (len(metrics) - 1) / 2) * width for x in x_positions]
        values = [mean(values_for(rows, mode, metric)) for mode in modes]
        ax.bar(offsets, values, width=width, label=label)

    ax.set_title("Mean Retrieval Volume By Mode")
    ax.set_xlabel("Mode")
    ax.set_ylabel("Count")
    ax.set_xticks(x_positions)
    ax.set_xticklabels(modes)
    ax.legend()
    ax.grid(axis="y", alpha=0.25)
    fig.tight_layout()

    path = output_dir / "retrieval_volume_by_mode.png"
    fig.savefig(path, dpi=160)
    plt.close(fig)
    return path


def plot_tokens_by_mode(rows: list[dict[str, Any]], output_dir: Path, plt: Any) -> Path:
    modes = ordered_unique(str(row["mode"]) for row in rows)
    token_metrics = [
        ("input_tokens", "Input tokens"),
        ("output_tokens", "Output tokens"),
    ]
    bottoms = [0.0 for _ in modes]

    fig, ax = plt.subplots(figsize=(9, 5))
    for metric, label in token_metrics:
        values = [mean(values_for(rows, mode, metric)) for mode in modes]
        ax.bar(modes, values, bottom=bottoms, label=label)
        bottoms = [current + value for current, value in zip(bottoms, values)]

    ax.set_title("Mean Final Answer Token Usage By Mode")
    ax.set_xlabel("Mode")
    ax.set_ylabel("Tokens")
    ax.legend()
    ax.grid(axis="y", alpha=0.25)
    fig.tight_layout()

    path = output_dir / "tokens_by_mode.png"
    fig.savefig(path, dpi=160)
    plt.close(fig)
    return path


def plot_tokens_by_question(rows: list[dict[str, Any]], output_dir: Path, plt: Any) -> Path:
    questions = ordered_unique(str(row["question_id"]) for row in rows)
    modes = ordered_unique(str(row["mode"]) for row in rows)
    grouped: dict[tuple[str, str], list[float]] = {}
    for row in rows:
        grouped.setdefault((str(row["question_id"]), str(row["mode"])), []).append(float(row["total_tokens"]))

    width = 0.8 / max(1, len(modes))
    x_positions = list(range(len(questions)))

    fig, ax = plt.subplots(figsize=(max(10, len(questions) * 1.4), 5.5))
    for idx, mode in enumerate(modes):
        offsets = [x + (idx - (len(modes) - 1) / 2) * width for x in x_positions]
        values = [mean(grouped.get((question, mode), [0])) for question in questions]
        ax.bar(offsets, values, width=width, label=mode)

    ax.set_title("Mean Final Answer Tokens By Question")
    ax.set_xlabel("Question")
    ax.set_ylabel("Total tokens")
    ax.set_xticks(x_positions)
    ax.set_xticklabels(questions, rotation=35, ha="right")
    ax.legend()
    ax.grid(axis="y", alpha=0.25)
    fig.tight_layout()

    path = output_dir / "tokens_by_question.png"
    fig.savefig(path, dpi=160)
    plt.close(fig)
    return path


def plot_source_mix_by_mode(rows: list[dict[str, Any]], output_dir: Path, plt: Any) -> Path:
    modes = ordered_unique(str(row["mode"]) for row in rows)
    source_metrics = [
        ("source_keyword_count", "keyword"),
        ("source_vector_count", "vector"),
        ("source_graph_count", "graph"),
        ("source_context_expansion_count", "context expansion"),
        ("source_other_count", "other"),
    ]

    bottoms = [0.0 for _ in modes]
    fig, ax = plt.subplots(figsize=(9, 5))
    for metric, label in source_metrics:
        values = [mean(values_for(rows, mode, metric)) for mode in modes]
        ax.bar(modes, values, bottom=bottoms, label=label)
        bottoms = [current + value for current, value in zip(bottoms, values)]

    ax.set_title("Mean Context Source Mix By Mode")
    ax.set_xlabel("Mode")
    ax.set_ylabel("Source labels per context block")
    ax.legend()
    ax.grid(axis="y", alpha=0.25)
    fig.tight_layout()

    path = output_dir / "source_mix_by_mode.png"
    fig.savefig(path, dpi=160)
    plt.close(fig)
    return path


def values_by_mode(rows: list[dict[str, Any]], metric: str) -> dict[str, list[float]]:
    grouped: dict[str, list[float]] = {}
    for row in rows:
        grouped.setdefault(str(row["mode"]), []).append(float(row[metric]))
    return grouped


def values_for(rows: list[dict[str, Any]], mode: str, metric: str) -> list[float]:
    values = [float(row[metric]) for row in rows if row["mode"] == mode]
    return values or [0.0]


def ordered_unique(values: Any) -> list[str]:
    seen = set()
    ordered = []
    for value in values:
        if value in seen:
            continue
        seen.add(value)
        ordered.append(value)
    return ordered


if __name__ == "__main__":
    main()
