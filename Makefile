clean:
	rm -rf build/

build-archives:
	mkdir -p build/src_archive
	git archive --format=tar.gz -o build/src_archive/v$(VERSION).tar.gz $(GIT_COMMIT_HASH)
	git archive --format=zip -o build/src_archive/v$(VERSION).zip $(GIT_COMMIT_HASH)

build-binary:
	go build -v -o build/bin/gateway cmd/gateway/main.go

# This builds an image for local testing. To run the image:
# docker run -v $PWD/configs/config.sample.yml:/satsuma/config.yml -p 8080:8080 dianwen/satsuma-gateway-test:latest
build-local-image: build-binary
	docker build -t dianwen/satsuma-gateway-test .

# This builds an image and pushes it up to a registry. This can be used to push
# to the test or production Docker Hub registry by specifying $DOCKER_HUB_REPO.
build-and-push-image: build-binary
	docker buildx build --platform linux/arm64/v8,linux/amd64 --tag $(DOCKER_HUB_REPO):$(VERSION) --tag $(DOCKER_HUB_REPO):latest --build-arg GIT_COMMIT_HASH=$(GIT_COMMIT_HASH) --push .
