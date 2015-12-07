project=swift-consometer
builddockerimage=docker-registry.corp.cloudwatt.com/golang-gbbuilder

all: ${project}

${project}: build

build:
	gb build all

build-indocker:
	docker run --name=${project}-build -v $(shell pwd)/:/build/code ${builddockerimage} make

deploy: build
	scp bin/swift-consometer d-bstinf-0000.adm.lab0.aub.cloudwatt.net:~
