#!/usr/bin/env bash

set -e

echo "Starting benchmark..."

# Execute benchmarks multiple times.
for i in {1..10}
do
   echo " + run #$i"
   if [ "$CHARTS" == "true" ]; then
      go test -short -bench=. >> .benchruns
   else
      go test -bench=. >> .benchruns
   fi
done

# Use an absolute path because the setup-go
# GitHub action does not set the PATH env var.
# See https://github.com/actions/setup-go/issues/14.
"$(go env GOPATH)/bin/benchstat" .benchruns | tee benchstats

if [ "$CHARTS" == "true" ]; then
   echo -e "\nGenerating charts..."

   # Convert benchmark statistics to CSV and
   # transform the output to JSON-formatted
   # data tables interpretable by Google Charts.
   "$(go env GOPATH)/bin/benchstat" -csv -norange .benchruns > .benchstats.csv
   go run tools/benchparse/benchparse.go -in .benchstats.csv -out .benchstats.json -omit-bandwidth -omit-allocs

   # Generate chart images and apply trim/border
   # operations using ImageMagick.
   cd tools/charts && npm --silent --no-audit install && cd ../..
   node tools/charts/index.js -f .benchstats.json -d images/benchmarks -n Simple,Complex,CodeMarshal,Map
fi

rm -f .benchruns
rm -f .benchstats.csv
rm -f .benchstats.json
