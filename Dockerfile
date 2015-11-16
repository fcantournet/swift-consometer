FROM scratch
MAINTAINER Félix Cantournet <felix.cantournet@cloudwatt.com>
COPY bin/swift-consometer /swift-consometer
COPY etc/swift/consometer.yaml /etc/swift/consometer.yaml
ENTRYPOINT ["/swift-consometer"]
