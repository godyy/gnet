all: clean client_linux client_macos client_windows server_linux server_macos server_windows

clean:
	rm -rf bin

client_linux:
	GOOS=linux GOARCH=amd64 go build -o ./bin/chat_client_linux client/main/main.go

client_macos:
	GOOS=darwin GOARCH=amd64 go build -o ./bin/chat_client_macos client/main/main.go

client_windows:
	GOOS=windows GOARCH=amd64 go build -o ./bin/chat_client_windows.exe client/main/main.go

server_linux:
	GOOS=linux GOARCH=amd64 go build -o ./bin/chat_server_linux server/main/main.go

server_macos:
	GOOS=darwin GOARCH=amd64 go build -o ./bin/chat_server_macos server/main/main.go

server_windows:
	GOOS=windows GOARCH=amd64 go build -o ./bin/chat_server_windows.exe server/main/main.go