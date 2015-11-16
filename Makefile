
build:
	gb build

deploy: build
	scp bin/swift-consometer d-bstinf-0000.adm.lab0.aub.cloudwatt.net:~
	
