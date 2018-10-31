podTemplate(
    label: 'envoy',
    containers: [
        containerTemplate(
            name: 'envoy',
            image: 'golang:1.11',
            ttyEnabled: true,
            command: 'cat',
        )
    ]
){
    node(envoy) {
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
