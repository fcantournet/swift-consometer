project=swift-consometer
builddockerimage=r.cwpriv.net/golang-gbbuilder
rundockerimage=r.cwpriv.net/swift-consometer
version=$(shell git describe --tags)

all: ${project}

${project}: build

build:
	gb build all

static:
	./build-static

build-indocker:
	docker run --rm --name=${project}-build -v $(shell pwd)/:/build/code ${builddockerimage} make static

deploy: build
	scp bin/swift-consometer d-bstinf-0000.adm.lab0.aub.cloudwatt.net:~
