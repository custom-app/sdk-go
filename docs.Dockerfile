FROM golang:1.17

RUN go install golang.org/x/tools/cmd/godoc@latest
WORKDIR /usr/src/go/sdk
COPY . .

ENTRYPOINT ["godoc", "-goroot=/usr/src/go", "-http=:6060"]
