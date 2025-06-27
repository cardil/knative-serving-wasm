# Those are callable targets
TASKS = build \
  clean \
  deploy \
  undeploy \
  tidy \
  update \
  update-deps \
  update-codegen \
  test \
  unit \
  e2e \
  build-test

.PHONY: all
all: build

.PHONY: $(TASKS)
$(TASKS):
	go run ./build/ $(ARGS) $@

.PHONY: help
help:
	go run ./build/ -h
