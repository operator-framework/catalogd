#!/usr/bin/env bash

set -e

# compare-catalog.sh compares the contents of the clusterCatalog created by registry.sh
# matches the expected contents in testdata/catalogs/test-catalog/expected_all.json.

# delete any pods persisting from earlier tests
kubectl delete --ignore-not-found --wait -f test/tools/imageregistry/compare-catalog.pod.yaml
kubectl create -f test/tools/imageregistry/compare-catalog.pod.yaml
# wait till pod terminates
kubectl wait -f test/tools/imageregistry/compare-catalog.pod.yaml --for=jsonpath='{.status.containerStatuses[0].state.terminated}'
kubectl logs --all-containers=true -f -n catalogd-e2e catalogd-compare
if [ "$(kubectl get -f test/tools/imageregistry/compare-catalog.pod.yaml -o jsonpath={.status.containerStatuses[0].state.terminated.exitCode})" -ne 0 ]; then
  exit 1
fi
