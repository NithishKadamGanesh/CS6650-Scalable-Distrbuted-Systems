import json
import sys

def main():
    if len(sys.argv) < 3:
        print("Usage: python compare_counts.py <local_json> <reducer_json>")
        sys.exit(1)

    local_path = sys.argv[1]
    reducer_path = sys.argv[2]

    with open(local_path, "r", encoding="utf-8") as f:
        local = json.load(f)
    with open(reducer_path, "r", encoding="utf-8") as f:
        reducer = json.load(f)

    local_keys = set(local.keys())
    reducer_keys = set(reducer.keys())

    missing_in_reducer = sorted(local_keys - reducer_keys)
    extra_in_reducer = sorted(reducer_keys - local_keys)

    mismatched = []
    for k in (local_keys & reducer_keys):
        if int(local[k]) != int(reducer[k]):
            mismatched.append((k, int(local[k]), int(reducer[k])))

    mismatched.sort(key=lambda x: abs(x[1] - x[2]), reverse=True)

    print(f"LOCAL_UNIQUE={len(local_keys)}")
    print(f"REDUCER_UNIQUE={len(reducer_keys)}")
    print(f"MISSING_IN_REDUCER={len(missing_in_reducer)}")
    print(f"EXTRA_IN_REDUCER={len(extra_in_reducer)}")
    print(f"MISMATCHED_COUNTS={len(mismatched)}")

    # Show top 20 diffs
    print("\nTOP_20_COUNT_DIFFS (word, local, reducer):")
    for w, a, b in mismatched[:20]:
        print(f"{w} {a} {b}")

if __name__ == "__main__":
    main()
