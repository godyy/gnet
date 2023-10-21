all: client_mac client_linux server_mac server_linux

client_mac:
	GOOS=darwin GOARCH=amd64 go build -o bin/iotest_client_mac client/client.go

client_linux:
	GOOS=linux GOARCH=amd64 go build -o bin/iotest_client_linux client/client.go

server_mac:
	GOOS=darwin GOARCH=amd64 go build -o bin/iotest_server_mac server/server.go

server_linux:
	GOOS=linux GOARCH=amd64 go build -o bin/iotest_server_linux server/server.go