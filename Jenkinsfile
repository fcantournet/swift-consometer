node('dockerHost_int0'){
  stage 'Clean workspace'
  deleteDir()

  stage 'Checkout'
  checkout scm

  stage 'build application'
  sh '''echo -n $(git describe --exact-match --tags $(git log -n1 --pretty="%h")) > gittagname;
        echo -n $(git rev-parse --verify HEAD) > gitcommit'''
  withDockerContainer(args: '-e HTTP_PROXY=$HTTP_PROXY -e HTTPS_PROXY=$HTTP_PROXY', image: 'r.cwpriv.net/jenkins/golang-gbbuilder') {
    sh './build-static artifacts'
  }
  archive 'artifacts/'

  stage 'build docker'
  env.GIT_TAG_NAME = readFile 'gittagname'
  env.GIT_COMMIT = readFile 'gitcommit'
  cloudwatt.trigger_parameterized_build('Docker/swift-consometer_build', "${env.JOB_NAME}", "${env.GIT_COMMIT}", "master", "${env.GIT_TAG_NAME}")
}
