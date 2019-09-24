#!/usr/bin/env bash

set -e

echo "Starting benchmark..."

# Execute benchmarks multiple times.
for i in {1..5}
do
   echo " + run #$i"
   go test -bench=. >> .benchruns
done

echo -e "\nStatistics"
echo "=========="

# Use an absolute path because the setup-go
# GitHub action does not set the PATH env var.
# See https://github.com/actions/setup-go/issues/14.
"$(go env GOPATH)/bin/benchstat" .benchruns | tee benchstats
rm -f .benchruns
