podlabel = "envoy-${UUID.randomUUID().toString()}"

podTemplate(
    label: podlabel,
    containers: [
        containerTemplate(
            name: 'envoy',
            image: 'golang:1.11',
            ttyEnabled: true,
            command: 'cat',
        )
    ]
    // TODO: Add persistentVolumeClaim
){
    node(podlabel) {
        stage('Checkout') {
            checkout scm
        }
        container('envoy') {
            ansiColor('xterm') {
                stage("Init") {
                    sh ('''
            	        make init
                    ''')
                }
                stage("Test-Report-JUnit") {
                    sh ('''
            	        make test-report-junit
                        cat test-results/report.xml
                    ''')
                }
            }
        }
    }
}
