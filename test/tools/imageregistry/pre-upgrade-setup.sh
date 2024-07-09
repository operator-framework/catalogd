#!/bin/bash

set -euo pipefail

TEST_CATALOG_NAME=${TEST_CATALOG_NAME:-"test-catalog"}
TEST_CATALOG_IMAGE=${TEST_CATALOG_IMAGE:-"docker-registry.catalogd-e2e.svc:5000/test-catalog:e2e"}

kubectl apply -f - << EOF
apiVersion: catalogd.operatorframework.io/v1alpha1
kind: ClusterCatalog
metadata:
  name: ${TEST_CATALOG_NAME}
spec:
  source:
    type: image
    image:
      ref: ${TEST_CATALOG_IMAGE}
      insecureSkipTLSVerify: true
EOF

kubectl wait --for=condition=Unpacked --timeout=60s ClusterCatalog $TEST_CATALOG_NAME
