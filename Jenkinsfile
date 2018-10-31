def label = "envoy-${UUID.randomUUID().toString()}"

podTemplate(label: label, containers: [
  containerTemplate(name: 'envoy', image: 'golang:1.11', ttyEnabled: true, command: 'cat')
  ], volumes: [
  persistentVolumeClaim(mountPath: '/home/jenkins/', claimName: 'envoy-repo', readOnly: false)
  ]) {

  node(label) {
    stage('Init') {
      container('envoy') {
          checkout scm
          sh 'make init'
      }
    }
    stage('test-report-junit') {
      container('envoy') {
          sh 'make test-report-junit'
          sh 'ls -lah'
      }
    }
  }
}
