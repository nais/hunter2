NAME := hunter2
REPO := navikt/${NAME}
TAG := $(shell date +'%Y-%m-%d')-$(shell git rev-parse --short HEAD)

.PHONY: test install build docker-build docker-push

test: fmt vet
	go test

install:
	go install

build: test
	go build -o ${NAME}

docker-build: build
	docker build -t "$(REPO):$(TAG)" -t "$(REPO):latest" .

docker-push:
	docker push "$(REPO)"

local:
	go run main.go

fmt:
	go fmt ./...

vet:
	go vet ./...
