------------------
 Swift-consometer
------------------

swift-consometer is a small agent that polls a swift cluster
to measure the usage of each Account in terms of the total
volume of objects.

This agent needs credentials with `ResellerAdmin` role
to list all the tenants and be able to send a `swift stat`request on
each and all the account.

# Configuration

Configuration is read form a file, you can find the consul-template for this file in `etc/`
On cloudwatt platforms, secrets are acquired from `vault` and a few configuration options are taken from the environment :

| Environment Variable | default | Description                                                                                                                                   |
|----------------------|---------|-----------------------------------------------------------------------------------------------------------------------------------------------|
| `SWIFT_REGION`       | ""      | The region of object-store you want to poll (can be different from the keystone region)                                                       |
| `PERIOD`             | ""      | Interval at which polling occurs (the timeout for a whole run is calculated from this)                                                        |
| `WORKERS`            | 1       | Number of concurrent client connections opened                                                                                                |
| `LOG_LEVEL`          | "INFO"  | Level of verbosity for logs                                                                                                                   |

# Hacking

You can build with `make build-in-docker` or if you have a golang dev environement set up, you can clone this repo in `$GOPATH/src/$SOMETHING` and just call `go build .`

There is a build pipeline set up in Jenkins.
First step will compile the code and build a tar.gz archive with the binary and consul-template file, and the second step will build a docker
container with this archive (downloaded form NEXUS)
Config for the docker image is in https://git.corp.cloudwatt.com/docker/swift-consometer

# Deployment on Kubernetes

This runs as a 1 replica Deployment on kubernetes.
The service isn't considered critical so there is no actual High-Availability setup

The service publishes graphite metrics for its own performance and monitoring (duration of the run, number of Accounts polled succesfully and published in RabbitMQ)
and for the swift capacity planning


# TODO : 
We might want to build alerting based either on metrics published in graphite (like success rate)
or based on probe in kubernetes
