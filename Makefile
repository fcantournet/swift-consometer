project=swift-consometer
build_docker_image=r.cwpriv.net/jenkins/golang-builder:1.7-latest
version=$(shell git describe --tags)
release_dir=/tmp/release/swift-consometer

all: ${project}

${project}: build

build:
	go install ${project}

release:
	CGO_ENABLED=0 GOOS=linux go build -a -x -tags netgo -ldflags "-X main.AppVersion=${version}" -o ${release_dir}/${project} .
	cp -r ./etc ${release_dir}
	tar -czvf ${release_dir}.tar.gz -C ${release_dir} ${project} etc 


publish-in-docker:
	docker run --rm -w /go/src/swift-consometer -v $(shell pwd):/go/src/swift-consometer \
		-e NEXUS_DEPLOYMENT_PASSWORD -e NEXUS_URL -e HTTP_PROXY -e HTTPS_PROXY \
		${build_docker_image} make publish

publish: release
	nexus-upload ${release_dir}.tar.gz com/cloudwatt/swift ${project} ${version} tar.gz
