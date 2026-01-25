import time
import requests
import matplotlib.pyplot as plt
import numpy as np

def load_test(url, duration_seconds=30, timeout_seconds=10):
    response_times = []
    errors = 0
    start_time = time.time()
    end_time = start_time + duration_seconds
    print(f"Starting load test for {duration_seconds} seconds...")
    print(f"Target URL: {url}")
    request_num = 0
    while time.time() < end_time:
        request_num += 1
        try:
            t0 = time.time()
            resp = requests.get(url, timeout=timeout_seconds)
            t1 = time.time()
            rt_ms = (t1 - t0) * 1000.0
            response_times.append(rt_ms)
            if resp.status_code == 200:
                print(f"Request {request_num}: {rt_ms:.2f} ms")
            else:
                errors += 1
                print(f"Request {request_num}: status {resp.status_code}, {rt_ms:.2f} ms")
        except requests.exceptions.RequestException as e:
            errors += 1
            print(f"Request {request_num}: failed, {e}")
    return response_times, errors

def print_stats(times_ms, errors):
    if len(times_ms) == 0:
        print("No successful requests recorded.")
        print(f"Errors: {errors}")
        return
    print("\nStatistics:")
    print(f"Total successful requests: {len(times_ms)}")
    print(f"Errors: {errors}")
    print(f"Average: {np.mean(times_ms):.2f} ms")
    print(f"Median: {np.median(times_ms):.2f} ms")
    print(f"95th percentile: {np.percentile(times_ms, 95):.2f} ms")
    print(f"99th percentile: {np.percentile(times_ms, 99):.2f} ms")
    print(f"Max: {np.max(times_ms):.2f} ms")

def plot_results(times_ms):
    if len(times_ms) == 0:
        return
    plt.figure(figsize=(12, 8))
    plt.subplot(2, 1, 1)
    plt.hist(times_ms, bins=50, alpha=0.7)
    plt.xlabel("Response Time (ms)")
    plt.ylabel("Frequency")
    plt.title("Distribution of Response Times")
    plt.subplot(2, 1, 2)
    plt.scatter(range(len(times_ms)), times_ms, alpha=0.6)
    plt.xlabel("Request Number")
    plt.ylabel("Response Time (ms)")
    plt.title("Response Times Over Time")
    plt.tight_layout()
    plt.show()

if __name__ == "__main__":
    EC2_URL = "http://98.93.41.18:8080/albums"  # your public IP
    times, errors = load_test(EC2_URL, duration_seconds=30)
    print_stats(times, errors)
    plot_results(times)
