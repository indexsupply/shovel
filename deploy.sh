GOOS=linux GOARCH=amd64 go build -o ethping ./cmd/ethping
tar -cvzf ethping.tar.gz ethping
scp ethping.tar.gz ubuntu@jumper:~
ssh ubuntu@jumper 'tar -xzf ethping.tar.gz'
rm ethping.tar.gz ethping
