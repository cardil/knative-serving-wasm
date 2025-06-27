# Those are callable targets
TASKS = build clean deploy undeploy update update-deps update-codegen \
  test e2e unit build-test

.PHONY: all
all: build

.PHONY: $(TASKS)
$(TASKS):
	go run ./build/ $(ARGS) $@

.PHONY: help
help:
	go run ./build/ -h
