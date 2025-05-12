# monitor
Custom Nload Copy Made By TCP

# SETUP

1. apt-get update
2. apt install golang-go 
3. apt install git
4. apt install screen
5. git clone https://github.com/TCPTHEGOAT/monitor
6. cd monitor
7. go install github.com/eiannone/keyboard@latest
8. go get github.com/eiannone/keyboard
9. go mod init github.com/eiannone/keyboard
10. go mod init monitor or go mod init
11. go mod tidy 
12. go build nload.go
13. chmod 777 * (only if compiling)

# RUNNING THE SCRIPT

1. (manual non compiled) go run nload.go [optional parameters: -t timeout]
2. (manual compiled) go run ./nload [optional parameters: -t timeout]
3. (auto compiled) screen go run ./nload [optional parameters: -t timeout]
4. (auto non compiled) screen go run nload.go [optional parameters: -t timeout]



