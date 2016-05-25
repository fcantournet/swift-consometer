node('dockerHost_int0'){
  cloudwatt.init()

  stage 'build application'
  cloudwatt_build{
    docker pull r.cwpriv.net/jenkins/golang-gbbuilder
    withDockerContainer(args: '-e HTTP_PROXY=$HTTP_PROXY -e HTTPS_PROXY=$HTTP_PROXY', image: 'r.cwpriv.net/jenkins/golang-gbbuilder')     {sh './build-static artifacts'}
  }

  stage 'build docker'
  cloudwatt.trigger_parameterized_build('Docker/swift-consometer_build', "${env.JOB_NAME}", "master")
}
