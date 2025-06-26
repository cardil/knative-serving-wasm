# Those are callable targets
TASKS = build clean deploy undeploy update update-deps update-codegen

all: build

.PHONY: $(TASKS)
$(TASKS):
	go run ./build/ $(ARGS) $@

.PHONY: help
help:
	go run ./build/ -h
