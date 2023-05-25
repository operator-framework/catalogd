# Build info
export GO_BUILD_TAGS  ?= ''
export GIT_COMMIT     ?= $(shell git rev-parse HEAD)
export GIT_VERSION    ?= $(shell git describe --tags --always --dirty)
export GIT_TREE_STATE ?= $(shell [ -z "$(shell git status --porcelain)" ] && echo "clean" || echo "dirty")
export VERSION_PKG    ?= $(shell go list -m)/internal/version

export IMAGE_REPO                ?= quay.io/operator-framework/catalogd
export IMAGE_TAG                 ?= devel
IMAGE=$(IMAGE_REPO):$(IMAGE_TAG)

# setup-envtest on *nix uses XDG_DATA_HOME, falling back to HOME, as the default storage directory. Some CI setups
# don't have XDG_DATA_HOME set; in those cases, we set it here so setup-envtest functions correctly. This shouldn't
# affect developers.
export XDG_DATA_HOME ?= /tmp/.local/share

# bingo manages consistent tooling versions for things like kind, kustomize, etc.
include .bingo/Variables.mk

# Dependencies
CERT_MGR_VERSION        ?= v1.11.0
ENVTEST_SERVER_VERSION = $(shell go list -m k8s.io/client-go | cut -d" " -f2 | sed 's/^v0\.\([[:digit:]]\{1,\}\)\.[[:digit:]]\{1,\}$$/1.\1.x/')

# Cluster configuration
KIND_CLUSTER_NAME       ?= catalogd
CATALOGD_NAMESPACE      ?= catalogd-system

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk commands is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

.PHONY: help
help: ## Display this help.
	awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

clean: ## Remove binaries and test artifacts
	rm -rf bin

.PHONY: generate
generate: $(CONTROLLER_GEN) ## Generate code and manifests.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet -tags $(GO_BUILD_TAGS) ./...

.PHONY: test
test-unit: generate fmt vet $(SETUP_ENVTEST) ## Run tests.
	eval $$($(SETUP_ENVTEST) use -p env $(ENVTEST_SERVER_VERSION)) && go test ./... -coverprofile cover.out

.PHONY: tidy
tidy: ## Update dependencies
	go mod tidy

.PHONY: verify
verify: tidy fmt vet generate ## Verify the current code generation and lint
	git diff --exit-code

##@ Build

BINARIES=manager
LINUX_BINARIES=$(join $(addprefix linux/,$(BINARIES)), )

BUILDCMD = sh -c 'mkdir -p $(BUILDBIN) && $(GORELEASER) build $(GORELEASER_ARGS) --id $(notdir $@) --single-target -o $(BUILDBIN)/$(notdir $@)'
BUILDDEPS = goreleaser

.PHONY: build
build: $(BINARIES)  ## Build all project binaries for the local OS and architecture.

.PHONY: build-linux
build-linux: $(LINUX_BINARIES) ## Build all project binaries for GOOS=linux and the local architecture.

.PHONY: $(BINARIES)
$(BINARIES): BUILDBIN = bin
$(BINARIES): $(BUILDDEPS)
	$(BUILDCMD)

.PHONY: $(LINUX_BINARIES)
$(LINUX_BINARIES): BUILDBIN = bin/linux
$(LINUX_BINARIES): $(BUILDDEPS)
	GOOS=linux $(BUILDCMD)

.PHONY: run
run: generate kind-cluster install ## Create a kind cluster and install a local build of catalogd

.PHONY: build-container
build-container: build-linux ## Build docker image for catalogd.
	docker build -f Dockerfile -t $(IMAGE) bin/linux

##@ Deploy

.PHONY: kind-cluster
kind-cluster: $(KIND) kind-cluster-cleanup ## Standup a kind cluster
	$(KIND) create cluster --name $(KIND_CLUSTER_NAME)
	$(KIND) export kubeconfig --name $(KIND_CLUSTER_NAME)

.PHONY: kind-cluster-cleanup
kind-cluster-cleanup: $(KIND) ## Delete the kind cluster
	$(KIND) delete cluster --name $(KIND_CLUSTER_NAME)

.PHONY: kind-load
kind-load: $(KIND) ## Load the built images onto the local cluster
	$(KIND) export kubeconfig --name $(KIND_CLUSTER_NAME)
	$(KIND) load docker-image $(IMAGE) --name $(KIND_CLUSTER_NAME)


.PHONY: install
install: build-container kind-load deploy wait ## Install local catalogd

.PHONY: deploy
deploy: $(KUSTOMIZE) ## Deploy Catalogd to the K8s cluster specified in ~/.kube/config.
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(IMAGE)
	$(KUSTOMIZE) build config/default | kubectl apply -f -

.PHONY: undeploy
undeploy: $(KUSTOMIZE) ## Undeploy Catalogd from the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/default | kubectl delete --ignore-not-found=true -f -

wait:
	kubectl wait --for=condition=Available --namespace=$(CATALOGD_NAMESPACE) deployment/catalogd-controller-manager --timeout=60s

##@ Release

export ENABLE_RELEASE_PIPELINE ?= false
export GORELEASER_ARGS         ?= --snapshot --clean
export CERT_MGR_VERSION        ?= $(CERT_MGR_VERSION)
release: $(GORELEASER) ## Runs goreleaser for catalogd. By default, this will run only as a snapshot and will not publish any artifacts unless it is run with different arguments. To override the arguments, run with "GORELEASER_ARGS=...". When run as a github action from a tag, this target will publish a full release.
	$(GORELEASER) $(GORELEASER_ARGS)

quickstart: $(KUSTOMIZE) generate ## Generate the installation release manifests and scripts
	$(KUSTOMIZE) build config/default | sed "s/:devel/:$(GIT_VERSION)/g" > catalogd.yaml
