# Usa uma imagem oficial e leve do Go
FROM golang:1.21-alpine

# Define o diretório de trabalho dentro do contêiner
WORKDIR /app

# Copia todos os arquivos do seu projeto para o contêiner
COPY . .

# Compila o servidor e os clientes
# (Assumindo que eles estão na raiz. O Go cuida das dependências locais como o pacote 'protocolo')
RUN go build -o vaijunto_server cmd/server/server.go
RUN go build -o vaijunto_motorista cmd/motorista/main.go
RUN go build -o vaijunto_passageiro cmd/passageiro/main.go

# Por padrão, quando a imagem rodar, iniciará o servidor
CMD ["./vaijunto_server"]