# Those are callable targets
TASKS = \
  build \
  build-test \
  clean \
  deploy \
  e2e \
  images \
  publish \
  test \
  tidy \
  undeploy \
  unit \
  update \
  update-codegen \
  update-deps

.PHONY: all
all: build

.PHONY: $(TASKS)
$(TASKS):
	@go run ./build/ $(ARGS) $@

.PHONY: help
help:
	@go run ./build/ -h
