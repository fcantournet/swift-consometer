node('dockerHost_int0'){
  //Add job properties
  properties ([[$class: 'ParametersDefinitionProperty',
                parameterDefinitions: [
                  [$class: 'StringParameterDefinition',
                    defaultValue: 'git@git.corp.cloudwatt.com:applications/swift-consometer.git',
                    description: 'Swift-Consometer git URL',
                    name: 'GITURL'],
                  [$class: 'StringParameterDefinition',
                    defaultValue: 'master',
                    description: 'Swift-Consometer Application BranchName/Tag/HashCommit',
                    name: 'BRANCH']]],
             [$class: 'jenkins.model.BuildDiscarderProperty',
                strategy: [$class : 'LogRotator', numToKeepStr : '10', daysToKeepStr: '30', artifactNumToKeepStr: '5']]
  ])

  cloudwatt.init_noshallow()

  stage 'build'
  withCredentials([[$class: 'StringBinding', credentialsId: 'd6b8e14d-b18e-46e2-bec5-81c11ced1517', variable: 'NEXUS_DEPLOYMENT_PASSWORD']]) {
    sh 'make docker-publish'
  }

  stage 'build docker'
  cloudwatt.trigger_parameterizedjob_nexusartifacts('Docker/swift-consometer_build')

}
