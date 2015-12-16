project=swift-consometer
builddockerimage=docker-registry.corp.cloudwatt.com/golang-gbbuilder
rundockerimage=docker-registry.corp.cloudwatt.com/swift-consometer
version=$(shell git describe --tags)

all: ${project}

${project}: build

build:
	gb build all

static:
	./build-static

build-indocker:
	docker run --rm --name=${project}-build -v $(shell pwd)/:/build/code ${builddockerimage} make static

dockerimage: build-indocker
	docker build -t ${rundockerimage}:${version} .
	docker push ${rundockerimage}

deploy: build
	scp bin/swift-consometer d-bstinf-0000.adm.lab0.aub.cloudwatt.net:~
