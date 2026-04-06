#!/usr/bin/env python3
"""
Generate graphs from load test CSV results.

Usage:
    python generate_graphs.py results_w5r1_1pct.csv results_w5r1_10pct.csv ...

Each CSV file should have columns:
    type, key, latency_ms, status_code, version, stale, timestamp_unix_ns, rw_interval_ms
"""

import sys
import os
import pandas as pd
import matplotlib.pyplot as plt
import matplotlib
matplotlib.use('Agg')  # Non-interactive backend
import numpy as np


def load_data(filepath):
    """Load a CSV results file."""
    df = pd.read_csv(filepath)
    df['latency_ms'] = pd.to_numeric(df['latency_ms'], errors='coerce')
    df['rw_interval_ms'] = pd.to_numeric(df['rw_interval_ms'], errors='coerce')
    return df


def plot_latency_distribution(df, label, output_dir):
    """Plot latency distributions for reads and writes, showing the long tail."""
    fig, axes = plt.subplots(1, 2, figsize=(14, 5))
    fig.suptitle(f'Latency Distribution — {label}', fontsize=14)

    reads = df[df['type'] == 'read']['latency_ms'].dropna()
    writes = df[df['type'] == 'write']['latency_ms'].dropna()

    # Read latency histogram
    if len(reads) > 0:
        axes[0].hist(reads, bins=50, color='steelblue', edgecolor='black', alpha=0.7)
        axes[0].axvline(reads.median(), color='red', linestyle='--', label=f'Median: {reads.median():.1f}ms')
        axes[0].axvline(reads.quantile(0.99), color='orange', linestyle='--', label=f'P99: {reads.quantile(0.99):.1f}ms')
        axes[0].set_title(f'Read Latency (n={len(reads)})')
        axes[0].set_xlabel('Latency (ms)')
        axes[0].set_ylabel('Count')
        axes[0].legend()
    else:
        axes[0].text(0.5, 0.5, 'No read data', ha='center', va='center', transform=axes[0].transAxes)

    # Write latency histogram
    if len(writes) > 0:
        axes[1].hist(writes, bins=50, color='coral', edgecolor='black', alpha=0.7)
        axes[1].axvline(writes.median(), color='red', linestyle='--', label=f'Median: {writes.median():.1f}ms')
        axes[1].axvline(writes.quantile(0.99), color='orange', linestyle='--', label=f'P99: {writes.quantile(0.99):.1f}ms')
        axes[1].set_title(f'Write Latency (n={len(writes)})')
        axes[1].set_xlabel('Latency (ms)')
        axes[1].set_ylabel('Count')
        axes[1].legend()
    else:
        axes[1].text(0.5, 0.5, 'No write data', ha='center', va='center', transform=axes[1].transAxes)

    plt.tight_layout()
    outpath = os.path.join(output_dir, f'latency_{label.replace(" ", "_").replace("/", "_")}.png')
    plt.savefig(outpath, dpi=150)
    plt.close()
    print(f"  Saved: {outpath}")


def plot_rw_interval(df, label, output_dir):
    """Plot distribution of time intervals between reading and writing the same key."""
    intervals = df[df['type'] == 'read']['rw_interval_ms'].dropna()
    intervals = intervals[intervals > 0]

    if len(intervals) == 0:
        print(f"  No read-write interval data for {label}")
        return

    fig, ax = plt.subplots(figsize=(8, 5))
    ax.hist(intervals, bins=50, color='mediumpurple', edgecolor='black', alpha=0.7)
    ax.axvline(intervals.median(), color='red', linestyle='--', label=f'Median: {intervals.median():.1f}ms')
    ax.set_title(f'Read-Write Interval Distribution — {label}')
    ax.set_xlabel('Time since last write to same key (ms)')
    ax.set_ylabel('Count')
    ax.legend()

    plt.tight_layout()
    outpath = os.path.join(output_dir, f'rw_interval_{label.replace(" ", "_").replace("/", "_")}.png')
    plt.savefig(outpath, dpi=150)
    plt.close()
    print(f"  Saved: {outpath}")


def print_summary(df, label):
    """Print summary statistics."""
    reads = df[df['type'] == 'read']
    writes = df[df['type'] == 'write']
    stale = reads[reads['stale'] == True]

    print(f"\n{'='*60}")
    print(f"  {label}")
    print(f"{'='*60}")
    print(f"  Total requests:    {len(df)}")
    print(f"  Writes:            {len(writes)}")
    print(f"  Reads:             {len(reads)}")
    print(f"  Stale reads:       {len(stale)} ({100*len(stale)/max(len(reads),1):.1f}%)")
    if len(writes) > 0:
        print(f"  Write latency:     median={writes['latency_ms'].median():.1f}ms, "
              f"p95={writes['latency_ms'].quantile(0.95):.1f}ms, "
              f"p99={writes['latency_ms'].quantile(0.99):.1f}ms")
    if len(reads) > 0:
        print(f"  Read latency:      median={reads['latency_ms'].median():.1f}ms, "
              f"p95={reads['latency_ms'].quantile(0.95):.1f}ms, "
              f"p99={reads['latency_ms'].quantile(0.99):.1f}ms")


def main():
    if len(sys.argv) < 2:
        print("Usage: python generate_graphs.py <csv_file1> [csv_file2] ...")
        print("  CSV filenames should follow pattern: results_<config>_<write_pct>pct.csv")
        sys.exit(1)

    output_dir = "graphs"
    os.makedirs(output_dir, exist_ok=True)

    for filepath in sys.argv[1:]:
        # Derive label from filename
        basename = os.path.splitext(os.path.basename(filepath))[0]
        label = basename.replace("results_", "").replace("_", " ")

        print(f"\nProcessing: {filepath} -> {label}")
        df = load_data(filepath)

        print_summary(df, label)
        plot_latency_distribution(df, label, output_dir)
        plot_rw_interval(df, label, output_dir)

    print(f"\nAll graphs saved to ./{output_dir}/")


if __name__ == "__main__":
    main()
