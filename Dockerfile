FROM golang:1.22-alpine

WORKDIR /src

COPY go.mod go.sum* ./
RUN go mod download || true

COPY . .


RUN go build -o /usr/local/bin/server ./cmd/server/server.go
RUN go build -o /usr/local/bin/motorista ./cmd/motorista/main.go
RUN go build -o /usr/local/bin/passageiro ./cmd/passageiro/main.go

WORKDIR /data


CMD ["server"]