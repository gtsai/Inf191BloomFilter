FROM golang:1.8

WORKDIR /go/src/Inf191BloomFilter
COPY . .

CMD ["go", "run", "cmd/bloomFilterServer/bloomFilterServer.go", "1000000", "10"]



