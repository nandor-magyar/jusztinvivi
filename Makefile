IMAGE   ?= ghcr.io/nandor-magyar/jusztinvivien
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
PORT    ?= 8080
BUMP    ?= patch

.DEFAULT_GOAL := help
.PHONY: help serve build server image run push release clean

help: ## list targets
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-8s %s\n", $$1, $$2}'

serve: ## hugo dev server
	hugo server -s site --disableFastRender

build: ## production build into public/
	hugo --minify --gc -s site -d ../public

server: build ## run the Go server on :$(PORT) with the production build
	PORT=$(PORT) go run .

image: ## build the container image, tagged $(VERSION) and latest
	docker build --build-arg VERSION=$(VERSION) -t $(IMAGE):$(VERSION) -t $(IMAGE):latest .

run: image ## run the image on localhost:$(PORT)
	docker run --rm -p $(PORT):8080 -e TURNSTILE_SECRET -e CONTACT_EMAIL $(IMAGE):$(VERSION)

push: image ## push $(VERSION) and latest to the registry
	docker push $(IMAGE):$(VERSION)
	docker push $(IMAGE):latest

release: ## tag the next semver on HEAD (BUMP=patch|minor|major), push it and create a GitHub release, CI builds the image
	@test -z "$$(git status --porcelain)" || { echo "working tree not clean"; exit 1; }
	@cur=$$(git tag --list 'v*' --sort=-v:refname | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$$' | head -n1); \
	set -- $$(echo "$${cur:-v0.0.0}" | tr -d v | tr . ' '); \
	case "$(BUMP)" in \
	  major) v=v$$(($$1+1)).0.0 ;; \
	  minor) v=v$$1.$$(($$2+1)).0 ;; \
	  patch) v=v$$1.$$2.$$(($$3+1)) ;; \
	  *) echo "BUMP must be patch, minor or major"; exit 1 ;; \
	esac; \
	git tag -a "$$v" -m "$$v" && git push origin "$$v" && gh release create "$$v" --verify-tag --generate-notes --title "$$v" && echo "released $$v (was $${cur:-none})"

clean: ## remove build output
	rm -rf public site/resources site/.hugo_build.lock
