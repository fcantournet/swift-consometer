node('dockerHost_int0'){
  stage 'Clean workspace'
  deleteDir()

  stage 'Checkout'
  checkout scm

  stage 'build application'
  sh '''echo -n $(git describe --exact-match --tags $(git log -n1 --pretty="%h")) > gittagname; 
        echo -n $(git rev-parse --verify HEAD) > gitcommit'''
  withDockerContainer(args: '-e HTTP_PROXY=$HTTP_PROXY -e HTTPS_PROXY=$HTTP_PROXY', image: 'r.cwpriv.net/jenkins/golang-gbbuilder') {
    sh './build-static'
  }
  archive 'artifacts/'

  stage 'build docker'
  env.GIT_TAG_NAME = readFile 'gittagname'
  env.GIT_COMMIT = readFile 'gitcommit'
  build job: 'Docker/swift-consometer_build', parameters: [[$class: 'StringParameterValue', name: 'upstream_job', value: "${env.JOB_NAME##*/}"], [$class: 'StringParameterValue', name: 'APPLICATION_COMMIT', value: "${env.GIT_COMMIT}"], [$class: 'StringParameterValue', name: 'APPLICATION_BRANCH', value: '${env.BRANCH_NAME}'], [$class: 'StringParameterValue', name: 'APPLICATION_GIT_TAG', value: "${env.GIT_TAG_NAME}"]], propagate: false, wait: false
}
