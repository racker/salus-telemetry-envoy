def label = "envoy-${UUID.randomUUID().toString()}"

podTemplate(label: label, containers: [
  containerTemplate(name: 'envoy', image: 'golang:1.11', ttyEnabled: true, command: 'cat')
  ], volumes: [
  persistentVolumeClaim(mountPath: '/home/jenkins/', claimName: 'envoy-repo', readOnly: false)
  ]) {

  node(label) {
    container('envoy') {
      stage('Init') {
          sh 'ls -lah'
          checkout scm
          sh 'make init'
          sh 'ls -lah'
      }
      stage('test-report-junit') {
          sh 'make test-report-junit'
          sh 'ls -lah'
      }
    }
  }
}
