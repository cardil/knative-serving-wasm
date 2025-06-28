# Those are callable targets
TASKS = $(shell go run ./build/ --list)

.PHONY: all
all: build

.PHONY: $(TASKS)
$(TASKS):
	@go run ./build/ $(ARGS) $@

.PHONY: help
help:
	@go run ./build/ --help
