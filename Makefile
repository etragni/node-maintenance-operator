CONTROLLER_GEN ?= controller-gen
CLUSTER ?= kind

.PHONY: tidy generate manifests install run test kind clean-kind

tidy:
	go mod tidy

generate:
	$(CONTROLLER_GEN) object paths="./api/..."

manifests:
	$(CONTROLLER_GEN) rbac:roleName=nodemaintenance-role crd paths="./..." output:crd:artifacts:config=config/crd output:rbac:artifacts:config=config/rbac

install: manifests
	kubectl apply -f config/crd

run: generate install
	go run ./cmd

test:
	go test ./internal/maintenance/... -v

kind:
	kind create cluster --name $(CLUSTER) --config hack/kind-config.yaml

clean-kind:
	kind delete cluster --name $(CLUSTER)
