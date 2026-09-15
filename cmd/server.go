package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"sync"

	"github.com/GuiSantosHashDaSilva/PBLConcorrenciaAgoraVai/internals/protocolo"
)

type ServerState struct {
	mu sync.Mutex
	// O mapa agora espera corretamente o tipo do seu pacote protocolo
	rides map[string]*protocolo.RideRequest
}

func main() {

	state := &ServerState{
		rides: make(map[string]*protocolo.RideRequest),
	}

	// Interface de socket nativa do TCP/IP
	listener, err := net.Listen("tcp", ":8080")
	if err != nil {
		fmt.Println("Erro ao iniciar servidor:", err)
		return
	}
	defer listener.Close()
	fmt.Println("Servidor central VAIJUNTO aguardando conexões na porta 8080...")

	for {
		conn, err := listener.Accept()
		if err != nil {
			continue
		}
		// Goroutine para cada cliente conectado, garantindo atendimento simultâneo
		go handleConnection(conn, state)
	}
}

func handleConnection(conn net.Conn, state *ServerState) {
	defer conn.Close()
	scanner := bufio.NewScanner(conn)

	for scanner.Scan() {
		var msg protocolo.Message
		// O receptor valida e descarta mensagens malformadas (removida a duplicação)
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			conn.Write([]byte(`{"status":"error", "message":"invalid format"}\n`))
			continue
		}

		switch msg.Type {
		case "PUBLISH_RIDE":
			// Usando a estrutura correta do seu pacote protocolo
			var req protocolo.RideRequest

			// Converte o payload JSON para a estrutura RideRequest
			json.Unmarshal(msg.Payload, &req)

			// Trava o Mutex para salvar a carona na memória de forma segura
			state.mu.Lock()
			rideID := fmt.Sprintf("%s-%s", req.DriverID, req.Date)
			state.rides[rideID] = &req
			state.mu.Unlock()

			conn.Write([]byte(`{"status":"success", "message":"Ride published"}\n`))

		case "BOOK_ROUTE":
			// A confirmação atômica ocorre aqui dentro do Lock()
			state.mu.Lock()
			// 1. Verificar se TODOS os trechos solicitados têm assentos disponíveis
			// 2. Se sim, subtrair 1 de 'AvailableSeats' em cada trecho
			// 3. Se não, retornar erro sem alterar nada (Atomicidade)
			state.mu.Unlock()
		}
	}
}
