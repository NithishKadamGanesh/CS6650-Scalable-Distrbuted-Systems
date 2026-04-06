# HW9 Submission Checklist

## What to submit

1. Git repository URL
   Add the remote URL for the larger course repository manually when you submit.
2. The `Hw9/` project directory
   Includes the leader-follower implementation, leaderless implementation, load tester, Dockerfiles, compose files, tests, scripts, and instructions copy.
3. The PDF report
   `Hw9/Hw9_Report.pdf`
4. Raw load-test outputs
   `Hw9/results/`
5. Generated graphs
   `Hw9/graphs/`
6. Result screenshots used in the report
   `Hw9/screenshots/`

## Main deliverables in this folder

- Code: `Hw9/leader-follower/`, `Hw9/leaderless/`, `Hw9/loadtester/`
- Tests: `Hw9/tests/consistency_test.go`
- Docker: `Hw9/leader-follower/Dockerfile`, `Hw9/leader-follower/docker-compose.yml`, `Hw9/leaderless/Dockerfile`, `Hw9/leaderless/docker-compose.yml`
- Report PDF: `Hw9/Hw9_Report.pdf`
- Summary CSV for the report: `Hw9/artifacts/verification/load_summary.csv`
- Verification logs: `Hw9/artifacts/verification/`

## Commit

- Current HW9 commit: `b99f973` for the initial HW9 snapshot
- Make one more commit after the generated artifacts if you want the final report/results included in git
