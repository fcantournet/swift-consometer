FROM scratch
MAINTAINER Félix Cantournet <felix.cantournet@cloudwatt.com>
COPY bin/swift-consometer /swift-consometer
ENTRYPOINT ["/swift-consometer"]
