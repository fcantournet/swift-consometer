node('dockerHost_int0'){
  stage 'Clean workspace'
  deleteDir()

  stage 'update properties'  
  CopyArtifactPermissionProperty: '/Docker/swift-consometer_build'

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
 
  build job: 'Docker/swift-consometer_build',
  parameters: [
    new StringParameterValue
    ('upstream_job', "{env.JOB_NAME}"),
    new StringParameterValue
    ('APPLICATION_COMMIT',"${env.GIT_COMMIT}"),
    new StringParameterValue
    ('APPLICATION_BRANCH',"master"),
    new StringParameterValue
    ('APPLICATION_GIT_TAG',"${env.GIT_TAG_NAME}")
  ], 
  propagate: true, 
  wait: true
}
