COMMIT_HASH := $(shell git rev-parse --short HEAD)
IMAGE_NAME := j18e/tempctl
IMAGE_FULL := $(IMAGE_NAME):$(COMMIT_HASH)

build:
	GOOS=linux GOARCH=386 go build -o ./tempctl main.go

docker-build:
	docker build -t $(IMAGE_FULL) .

docker-push:
	docker push $(IMAGE_FULL)

all: build docker-build docker-push
