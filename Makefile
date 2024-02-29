.PHONY: clean
clean:
	go run github.com/google/ko@latest delete -f config/

.PHONY: deploy
deploy:
	go run github.com/google/ko@latest apply -f config/

.PHONY: update-deps
update: update-deps update-codegen

.PHONY: update-deps
update-deps:
	hack/update-deps.sh --upgrade

.PHONY: update-codegen
update-codegen:
	hack/update-codegen.sh
