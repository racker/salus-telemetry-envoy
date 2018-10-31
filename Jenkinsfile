def label = "envoy-${UUID.randomUUID().toString()}"

podTemplate(label: label, containers: [
  containerTemplate(name: 'envoy', image: 'golang:1.11', ttyEnabled: true, command: 'cat')
  ], volumes: [
  persistentVolumeClaim(mountPath: '/home/jenkins/', claimName: 'envoy-repo', readOnly: false)
  ]) {

  node(label) {
    stage('Init') {
      git 'https://github.com/jenkinsci/kubernetes-plugin.git'
      container('envoy') {
          sh 'make ini'
      }
    }
  }
}
