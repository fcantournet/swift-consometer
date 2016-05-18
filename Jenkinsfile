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
  trigger_parameterized_build('Docker/swift-consometer_build', "${env.JOB_NAME}", "${env.GIT_COMMIT}", "master", "${env.GIT_TAG_NAME}")
}

def trigger_parameterized_build(DOWNSTREAM_JOB, UPSTREAM_PROJECT, GIT_COMMIT, BRANCH, GIT_TAG_NAME) {
  build job: "${DOWNSTREAM_JOB}",
  parameters: [
    new StringParameterValue
    ('upstream_job', "${UPSTREAM_PROJECT}"),
    new StringParameterValue
    ('APPLICATION_COMMIT',"${GIT_COMMIT}"),
    new StringParameterValue
    ('APPLICATION_BRANCH',"${BRANCH}"),
    new StringParameterValue
    ('APPLICATION_GIT_TAG',"${GIT_TAG_NAME}")
  ], 
  propagate: true,
  wait: false
}
