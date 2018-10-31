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
){
    node(podlabel) {
        stage('Checkout') {
            checkout scm
        }
        container('envoy') {
            ansiColor('xterm') {
                stage("Init") {
                    sh ('''
				        ls -lah
            	        make init
            	        ls -lah
                    ''')
                }
            }
        }
    }
}
