package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"

	"github.com/GuiSantosHashDaSilva/PBLConcorrenciaAgoraVai/internals/protocolo"
)

func main() {
	// 1. Conexão ao servidor central
	conn, err := net.Dial("tcp", "localhost:8080")
	if err != nil {
		fmt.Println("Erro ao conectar ao servidor:", err)
		return
	}
	defer conn.Close()

	// 2. Montando o payload usando as structs do seu protocolo
	req := protocolo.RideRequest{
		DriverID: "motorista_123",
		Date:     "2026-10-15",
		Segments: []protocolo.Segment{
			{
				Origin:         "Salvador",
				Destination:    "Feira de Santana",
				AvailableSeats: 4,
				Price:          35.00,
			},
			{
				Origin:         "Feira de Santana",
				Destination:    "Vitória da Conquista",
				AvailableSeats: 4,
				Price:          80.00,
			},
		},
	}

	payloadBytes, _ := json.Marshal(req)

	// 3. Empacotando na mensagem principal
	msg := protocolo.Message{
		Type:    protocolo.PUBLISH_RIDE,
		Payload: payloadBytes,
	}

	msgBytes, _ := json.Marshal(msg)

	// 4. Enviando via socket (com quebra de linha para o Scanner do servidor ler corretamente)
	fmt.Fprintf(conn, string(msgBytes)+"\n")

	// 5. Lendo a resposta
	response, _ := bufio.NewReader(conn).ReadString('\n')
	fmt.Print("Resposta do servidor: ", response)
}
