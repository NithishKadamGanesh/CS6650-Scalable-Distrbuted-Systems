import csv
import matplotlib.pyplot as plt

inputs = []
local_times = []
distributed_times = []

with open("runs.csv", "r") as f:
    reader = csv.DictReader(f)
    for row in reader:
        inputs.append(row["input"])
        local_times.append(float(row["local_ms"]))
        distributed_times.append(float(row["distributed_ms"]))

plt.figure()
plt.plot(inputs, local_times, marker="o", label="Local")
plt.plot(inputs, distributed_times, marker="o", label="Distributed (MapReduce)")

plt.xlabel("Input file")
plt.ylabel("Execution time (ms)")
plt.title("Local vs Distributed Word Count Performance")
plt.legend()
plt.grid(True)

plt.savefig("performance_comparison.png")
plt.show()
