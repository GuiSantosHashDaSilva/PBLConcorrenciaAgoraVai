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
	mu         sync.Mutex
	rides      map[string]*protocolo.RideRequest
	nextRideID int
}

func main() {

	state := &ServerState{
		rides:      make(map[string]*protocolo.RideRequest),
		nextRideID: 1,
	}

	// Interface de socket nativa do TCP/IP
	listener, err := net.Listen("tcp", ":8811")
	if err != nil {
		fmt.Println("Erro ao iniciar servidor:", err)
		return
	}
	defer listener.Close()
	fmt.Println("Servidor central VAIJUNTO aguardando conexões na porta 8811...")

	for {
		conn, err := listener.Accept()
		if err != nil {
			continue
		}
		// Goroutine para cada cliente, permite atendimento simultâneo
		go handleConnection(conn, state)
	}
}

func handleConnection(conn net.Conn, state *ServerState) {
	defer conn.Close()
	scanner := bufio.NewScanner(conn)

	fmt.Println("Novo cliente conectado:", conn.RemoteAddr())

	for scanner.Scan() {
		textoRecebido := scanner.Text()
		fmt.Println("-> Recebido do cliente:", textoRecebido) 

		var msg protocolo.Message
		if err := json.Unmarshal([]byte(textoRecebido), &msg); err != nil {
			fmt.Println("Erro no Unmarshal principal:", err)
			conn.Write([]byte(`{"status":"error", "message":"invalid format"}` + "\n"))
			continue
		}

		
		switch string(msg.Type) {
		case "LOGIN":
			var req struct {
				Login string `json:"login"`
				Senha string `json:"senha"`
			}

			if err := json.Unmarshal(msg.Payload, &req); err != nil {
				fmt.Println("Erro no Unmarshal do Login:", err)
				conn.Write([]byte(`{"status":"error", "message":"Payload de login inválido"}` + "\n"))
				continue
			}

			
			
			if req.Login != "" && req.Senha != "" {
				fmt.Printf("<- Motorista '%s' autenticado com sucesso!\n", req.Login)
				conn.Write([]byte(`{"status":"success", "message":"Login Efetuado com sucesso"}` + "\n"))
			} else {
				fmt.Println("<- Falha no login: credenciais vazias.")
				conn.Write([]byte(`{"status":"error", "message":"Login ou Senha incorreta"}` + "\n"))
			}
		
		case "PUBLISH_RIDE":
			var req protocolo.RideRequest

			if err := json.Unmarshal(msg.Payload, &req); err != nil {
				fmt.Println("Erro no Unmarshal do Payload:", err)
				conn.Write([]byte(`{"status":"error", "message":"invalid payload"}` + "\n"))
				continue
			}

			state.mu.Lock()
			rideID := fmt.Sprintf("%d", state.nextRideID)
			state.nextRideID++
			state.rides[rideID] = &req
			state.mu.Unlock()

			fmt.Println("<- Carona salva! Enviando sucesso.")
			conn.Write([]byte(`{"status":"success", "message":"Ride published"}` + "\n"))
		
		case "CONSULT_RIDES":
			var req struct {
				DriverID string `json:"driver_id"`
			}
			if err := json.Unmarshal(msg.Payload, &req); err != nil {
				conn.Write([]byte(`{"status":"error", "message":"Payload inválido"}` + "\n"))
				continue
			}

			// Trava o estado para leitura
			state.mu.Lock()
			var minhasCaronas []*protocolo.RideRequest
			for _, ride := range state.rides {
				if ride.DriverID == req.DriverID {
					minhasCaronas = append(minhasCaronas, ride)
				}
			}
			state.mu.Unlock() // Libera depois de copiar os dados

			// Transforma a lista de caronas em json e envia de volta
			respostaBytes, _ := json.Marshal(map[string]any{
				"status": "success",
				"rides":  minhasCaronas,
			})
			conn.Write(append(respostaBytes, '\n'))
			fmt.Printf("<- Enviando lista de caronas para o motorista '%s'\n", req.DriverID)

		case "CANCEL_RIDE":
			var req struct {
				RideID   string `json:"ride_id"`
				DriverID string `json:"driver_id"`
			}
			if err := json.Unmarshal(msg.Payload, &req); err != nil {
				conn.Write([]byte(`{"status":"error", "message":"Payload inválido"}` + "\n"))
				continue
			}

			// Trava o estado pra apagar
			state.mu.Lock()
			ride, existe := state.rides[req.RideID]

			// Só cancela se a carona existir e for de quem pediu
			if existe && ride.DriverID == req.DriverID {
				delete(state.rides, req.RideID)
				state.mu.Unlock()

				fmt.Printf("<- Carona %s cancelada com sucesso!\n", req.RideID)
				conn.Write([]byte(`{"status":"success", "message":"Carona cancelada com sucesso"}` + "\n"))
			} else {
				state.mu.Unlock()

				fmt.Printf("<- Falha ao cancelar: Carona %s não encontrada ou sem permissão.\n", req.RideID)
				conn.Write([]byte(`{"status":"error", "message":"Carona não encontrada ou permissão negada"}` + "\n"))
			}

		case "SEARCH_ROUTE":
			var req struct {
				Date        string `json:"date"`
				Origin      string `json:"origin"`
				Destination string `json:"destination"`
			}
			if err := json.Unmarshal(msg.Payload, &req); err != nil {
				conn.Write([]byte(`{"status":"error", "message":"Payload inválido"}` + "\n"))
				continue
			}

			type TrechoInfo struct {
				RideID         string  `json:"ride_id"`
				DriverID       string  `json:"driver_id"`
				Date           string  `json:"date"`
				Origin         string  `json:"origin"`
				Destination    string  `json:"destination"`
				AvailableSeats int     `json:"available_seats"`
				Price          float64 `json:"price"`
			}

			state.mu.Lock()
			grafo := make(map[string][]TrechoInfo)
			for rideID, ride := range state.rides {

				if ride.Date == req.Date {
					for _, seg := range ride.Segments {
						if seg.AvailableSeats > 0 {
							grafo[seg.Origin] = append(grafo[seg.Origin], TrechoInfo{
								RideID:         rideID,
								DriverID:       ride.DriverID,
								Date:           ride.Date,
								Origin:         seg.Origin,
								Destination:    seg.Destination,
								AvailableSeats: seg.AvailableSeats,
								Price:          seg.Price,
							})
						}
					}
				}
			}
			state.mu.Unlock()

			// Estrutura do BFS
			var rotasEncontradas [][]TrechoInfo // Armazena os caminhos completos

			type Path struct {
				Trechos     []TrechoInfo
				Visitados   map[string]bool
				CurrentNode string
			}
			var fila []Path

			// Inicializa a fila com as arestas que saem da origem
			for _, aresta := range grafo[req.Origin] {
				fila = append(fila, Path{
					Trechos:     []TrechoInfo{aresta},
					Visitados:   map[string]bool{req.Origin: true, aresta.Destination: true},
					CurrentNode: aresta.Destination,
				})
			}

			// execução bfs
			for len(fila) > 0 {
				// Desenfileira o primeiro caminho
				caminhoAtual := fila[0]
				fila = fila[1:]

				// Chegou no destino desejado? salva o caminho inteiro
				if caminhoAtual.CurrentNode == req.Destination {
					rotasEncontradas = append(rotasEncontradas, caminhoAtual.Trechos)
					continue
				}

				// Busca os próximos destinos a partir da cidade atual
				for _, vizinho := range grafo[caminhoAtual.CurrentNode] {
					if !caminhoAtual.Visitados[vizinho.Destination] {
						// Clona o mapa de visitados para não interferir em outras rotas
						novoVisitados := make(map[string]bool)
						for k, v := range caminhoAtual.Visitados {
							novoVisitados[k] = v
						}
						novoVisitados[vizinho.Destination] = true

						// Clona o caminho de trechos e adiciona o novo vizinho
						novosTrechos := make([]TrechoInfo, len(caminhoAtual.Trechos))
						copy(novosTrechos, caminhoAtual.Trechos)
						novosTrechos = append(novosTrechos, vizinho)

						// Coloca o novo caminho na fila
						fila = append(fila, Path{
							Trechos:     novosTrechos,
							Visitados:   novoVisitados,
							CurrentNode: vizinho.Destination,
						})
					}
				}
			}

			// retorno
			respostaBytes, _ := json.Marshal(map[string]any{
				"status": "success",
				"routes": rotasEncontradas, 
			})
			conn.Write(append(respostaBytes, '\n'))
			fmt.Printf("<- BFS Concluído: %d rota(s) encontrada(s) de %s para %s no dia %s\n", len(rotasEncontradas), req.Origin, req.Destination, req.Date)

		case "BOOK_ROUTE":
			var req struct {
				PassengerID string `json:"passenger_id"`
				Itinerary   []struct {
					RideID      string `json:"ride_id"`
					Origin      string `json:"origin"`
					Destination string `json:"destination"`
				} `json:"itinerary"`
			}
			if err := json.Unmarshal(msg.Payload, &req); err != nil {
				conn.Write([]byte(`{"status":"error", "message":"Payload inválido para reserva"}` + "\n"))
				continue
			}

			state.mu.Lock()

			// Verificação atomica
			podeReservar := true
			for _, trechoReq := range req.Itinerary {
				ride, existe := state.rides[trechoReq.RideID]
				if !existe {
					podeReservar = false 
					break
				}

				trechoValido := false
				for _, seg := range ride.Segments {
					if seg.Origin == trechoReq.Origin && seg.Destination == trechoReq.Destination {
						if seg.AvailableSeats < 1 { // Sem vagas nesse trecho 
							podeReservar = false
						}
						trechoValido = true
						break
					}
				}

				if !trechoValido || !podeReservar {
					podeReservar = false
					break
				}
			}

			// reservação atomica
			if podeReservar {
				for _, trechoReq := range req.Itinerary {
					ride := state.rides[trechoReq.RideID]
					for i, seg := range ride.Segments {
						if seg.Origin == trechoReq.Origin && seg.Destination == trechoReq.Destination {
							// Desconta a vaga
							ride.Segments[i].AvailableSeats--
							// adiciona o passageiro no trecho
							ride.Segments[i].Passengers = append(ride.Segments[i].Passengers, req.PassengerID)
							break
						}
					}
				}
				state.mu.Unlock()
				fmt.Printf("<- Reserva atômica concluída para o passageiro '%s'!\n", req.PassengerID)
				conn.Write([]byte(`{"status":"success", "message":"Itinerário confirmado!"}` + "\n"))
			} else {
				state.mu.Unlock() // Libera o acesso para outros clientes
				fmt.Println("<- Falha na reserva: Trechos indisponíveis.")
				conn.Write([]byte(`{"status":"error", "message":"Um ou mais trechos estão lotados ou não existem."}` + "\n"))
			}

		case "CONSULT_RESERVATIONS":
			var req struct {
				PassengerID string `json:"passenger_id"`
			}
			if err := json.Unmarshal(msg.Payload, &req); err != nil {
				conn.Write([]byte(`{"status":"error", "message":"Payload inválido"}` + "\n"))
				continue
			}

			// Trava o estado para buscar
			state.mu.Lock()

			// Estrutura para devolver os trechos que o passageiro comprou
			type Reserva struct {
				RideID      string  `json:"ride_id"`
				DriverID    string  `json:"driver_id"`
				Date        string  `json:"date"`
				Origin      string  `json:"origin"`
				Destination string  `json:"destination"`
				Price       float64 `json:"price"`
			}
			var minhasReservas []Reserva


			for rideID, ride := range state.rides {
				for _, seg := range ride.Segments {
					for _, passID := range seg.Passengers {
						if passID == req.PassengerID {
							minhasReservas = append(minhasReservas, Reserva{
								RideID:      rideID,
								DriverID:    ride.DriverID,
								Date:        ride.Date,
								Origin:      seg.Origin,
								Destination: seg.Destination,
								Price:       seg.Price,
							})
							break
						}
					}
				}
			}
			state.mu.Unlock()

			respostaBytes, _ := json.Marshal(map[string]any{
				"status":       "success",
				"reservations": minhasReservas,
			})
			conn.Write(append(respostaBytes, '\n'))
			fmt.Printf("<- Enviando reservas para o passageiro '%s'\n", req.PassengerID)

		case "CANCEL_RESERVATION":
			var req struct {
				PassengerID string `json:"passenger_id"`
				RideID      string `json:"ride_id"`
				Origin      string `json:"origin"`
				Destination string `json:"destination"`
			}
			if err := json.Unmarshal(msg.Payload, &req); err != nil {
				conn.Write([]byte(`{"status":"error", "message":"Payload inválido"}` + "\n"))
				continue
			}

			state.mu.Lock()
			ride, existe := state.rides[req.RideID]
			sucesso := false

			if existe {
				for i, seg := range ride.Segments {
					if seg.Origin == req.Origin && seg.Destination == req.Destination {
						for j, passID := range seg.Passengers {
							if passID == req.PassengerID {
								ride.Segments[i].Passengers = append(seg.Passengers[:j], seg.Passengers[j+1:]...)

								ride.Segments[i].AvailableSeats++

								sucesso = true
								break
							}
						}
						break
					}
				}
			}
			state.mu.Unlock()

			if sucesso {
				fmt.Printf("<- Reserva cancelada! Vaga devolvida. (Passageiro: %s | Trecho: %s -> %s)\n", req.PassengerID, req.Origin, req.Destination)
				conn.Write([]byte(`{"status":"success", "message":"Reserva cancelada com sucesso! Vaga devolvida."}` + "\n"))
			} else {
				fmt.Printf("<- Falha ao cancelar: Reserva de '%s' não encontrada.\n", req.PassengerID)
				conn.Write([]byte(`{"status":"error", "message":"Reserva não encontrada neste trecho."}` + "\n"))
			}

		default:
			fmt.Printf("<- TIPO DESCONHECIDO: '%s'\n", msg.Type)
			conn.Write([]byte(`{"status":"error", "message":"Unknown type"}` + "\n"))
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Printf("Erro ao ler da conexão %s: %v\n", conn.RemoteAddr(), err)
	} else {
		fmt.Println("Cliente desconectado limparmente:", conn.RemoteAddr())
	}

}
