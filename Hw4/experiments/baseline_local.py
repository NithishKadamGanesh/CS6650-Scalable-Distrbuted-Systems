import json
import re
import sys
import time
from collections import Counter

# Tokenization rule (match what most MapReduce wordcount does)
# If your mapper uses a different rule, update this regex to match it.
TOKEN_RE = re.compile(r"[a-zA-Z']+")

def count_words(text: str) -> Counter:
    words = TOKEN_RE.findall(text.lower())
    return Counter(words)

def main():
    if len(sys.argv) < 2:
        print("Usage: python baseline_local.py <input_file>")
        sys.exit(1)

    input_path = sys.argv[1]

    t0 = time.perf_counter()
    with open(input_path, "r", encoding="utf-8", errors="ignore") as f:
        text = f.read()
    counts = count_words(text)
    t1 = time.perf_counter()

    elapsed_ms = (t1 - t0) * 1000.0
    print(f"LOCAL_TIME_MS={elapsed_ms:.2f}")
    print(f"UNIQUE_WORDS={len(counts)}")
    print(f"TOTAL_TOKENS={sum(counts.values())}")

    # Save output to a file so we can compare with reducer output
    out_path = "local-wordcount.json"
    with open(out_path, "w", encoding="utf-8") as out:
        json.dump(dict(counts), out, indent=2, sort_keys=True)

    print(f"WROTE={out_path}")

if __name__ == "__main__":
    main()
