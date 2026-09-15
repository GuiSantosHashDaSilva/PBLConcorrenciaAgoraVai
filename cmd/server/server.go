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
		// Goroutine para cada cliente conectado, garantindo atendimento simultâneo
		go handleConnection(conn, state)
	}
}

func handleConnection(conn net.Conn, state *ServerState) {
	defer conn.Close()
	scanner := bufio.NewScanner(conn)

	fmt.Println("Novo cliente conectado:", conn.RemoteAddr())

	for scanner.Scan() {
		textoRecebido := scanner.Text()
		fmt.Println("-> Recebido do cliente:", textoRecebido) // LOG CRUCIAL: Mostra o JSON exato

		var msg protocolo.Message
		if err := json.Unmarshal([]byte(textoRecebido), &msg); err != nil {
			fmt.Println("Erro no Unmarshal principal:", err)
			conn.Write([]byte(`{"status":"error", "message":"invalid format"}` + "\n"))
			continue
		}

		// Convertendo para string para garantir que o switch funcione independentemente do tipo Servico
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

			// Lógica de autenticação (simplificada para o protótipo)
			// Em um sistema real, aqui você buscaria a 'Pessoa' em um mapa ou banco de dados
			if req.Login != "" && req.Senha != "" {
				fmt.Printf("<- Motorista '%s' autenticado com sucesso!\n", req.Login)
				// Concatenação correta do \n fora das crases
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
			rideID := fmt.Sprintf("%s-%s", req.DriverID, req.Date)
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

			// Trava o estado para leitura segura
			state.mu.Lock()
			var minhasCaronas []*protocolo.RideRequest
			for _, ride := range state.rides {
				if ride.DriverID == req.DriverID {
					minhasCaronas = append(minhasCaronas, ride)
				}
			}
			state.mu.Unlock() // Libera imediatamente após copiar os dados

			// Transforma a lista de caronas em JSON e envia de volta
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

			// Trava o estado para deleção segura
			state.mu.Lock()
			ride, existe := state.rides[req.RideID]

			// Só cancela se a carona existir e pertencer a quem pediu
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
				Origin      string `json:"origin"`
				Destination string `json:"destination"`
			}
			if err := json.Unmarshal(msg.Payload, &req); err != nil {
				conn.Write([]byte(`{"status":"error", "message":"Payload inválido"}` + "\n"))
				continue
			}

			// Estrutura auxiliar para o grafo
			type TrechoInfo struct {
				RideID         string  `json:"ride_id"`
				DriverID       string  `json:"driver_id"`
				Date           string  `json:"date"`
				Origin         string  `json:"origin"`
				Destination    string  `json:"destination"`
				AvailableSeats int     `json:"available_seats"`
				Price          float64 `json:"price"`
			}

			// 1. BLOQUEIO RÁPIDO: Monta a Lista de Adjacência e libera o acesso
			state.mu.Lock()
			grafo := make(map[string][]TrechoInfo)
			for rideID, ride := range state.rides {
				for _, seg := range ride.Segments {
					if seg.AvailableSeats > 0 { // Só cria aresta se houver vaga
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
			state.mu.Unlock() // Mutex liberado! O BFS roda sem travar o servidor.

			// 2. ESTRUTURAS DO ALGORITMO BFS
			var rotasEncontradas [][]TrechoInfo // Armazena caminhos completos

			type Path struct {
				Trechos     []TrechoInfo
				Visitados   map[string]bool // Evita ciclos infinitos (ex: A -> B -> A)
				CurrentNode string
			}
			var fila []Path

			// Inicializa a fila com as arestas que partem da Origem
			for _, aresta := range grafo[req.Origin] {
				fila = append(fila, Path{
					Trechos:     []TrechoInfo{aresta},
					Visitados:   map[string]bool{req.Origin: true, aresta.Destination: true},
					CurrentNode: aresta.Destination,
				})
			}

			// 3. EXECUÇÃO DO BFS
			for len(fila) > 0 {
				// Desenfileira o primeiro caminho (Pop)
				caminhoAtual := fila[0]
				fila = fila[1:]

				// Chegamos ao destino desejado? Salva a rota completa!
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

						// Enfileira o novo caminho estendido
						fila = append(fila, Path{
							Trechos:     novosTrechos,
							Visitados:   novoVisitados,
							CurrentNode: vizinho.Destination,
						})
					}
				}
			}

			// 4. RETORNO AO PASSAGEIRO
			respostaBytes, _ := json.Marshal(map[string]any{
				"status": "success",
				"routes": rotasEncontradas, // Devolve uma lista de roteiros, onde cada roteiro é uma lista de trechos
			})
			conn.Write(append(respostaBytes, '\n'))
			fmt.Printf("<- BFS Concluído: %d rota(s) encontrada(s) de %s para %s\n", len(rotasEncontradas), req.Origin, req.Destination)

		case "BOOK_ROUTE":
			// 1. Atualizamos a estrutura para ler o PassengerID
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

			// 1. FASE DE VERIFICAÇÃO (Tudo ou nada)
			podeReservar := true
			for _, trechoReq := range req.Itinerary {
				ride, existe := state.rides[trechoReq.RideID]
				if !existe {
					podeReservar = false // Carona não existe
					break
				}

				trechoValido := false
				for _, seg := range ride.Segments {
					if seg.Origin == trechoReq.Origin && seg.Destination == trechoReq.Destination {
						if seg.AvailableSeats < 1 { // Sem vagas neste trecho específico
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

			// 2. FASE DE EFETIVAÇÃO (Atômica)
			if podeReservar {
				for _, trechoReq := range req.Itinerary {
					ride := state.rides[trechoReq.RideID]
					for i, seg := range ride.Segments {
						if seg.Origin == trechoReq.Origin && seg.Destination == trechoReq.Destination {
							// Desconta a vaga
							ride.Segments[i].AvailableSeats--
							// ADICIONA O PASSAGEIRO NA LISTA DESTE TRECHO ESPECÍFICO
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

			// Trava o estado para buscar com segurança
			state.mu.Lock()

			// Estrutura para devolver os trechos exatos que o passageiro comprou
			type Reserva struct {
				RideID      string  `json:"ride_id"`
				DriverID    string  `json:"driver_id"`
				Date        string  `json:"date"`
				Origin      string  `json:"origin"`
				Destination string  `json:"destination"`
				Price       float64 `json:"price"`
			}
			var minhasReservas []Reserva

			// Varre todas as caronas, todos os trechos e todos os passageiros
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
							break // Já achou neste trecho, vai pro próximo
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
				// Procura o trecho correto
				for i, seg := range ride.Segments {
					if seg.Origin == req.Origin && seg.Destination == req.Destination {
						// Procura o passageiro na lista deste trecho
						for j, passID := range seg.Passengers {
							if passID == req.PassengerID {
								// 1. Remove o passageiro da lista (fatiando o slice)
								ride.Segments[i].Passengers = append(seg.Passengers[:j], seg.Passengers[j+1:]...)

								// 2. Devolve a vaga para o carro
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
			// SE O SERVIDOR NÃO RECONHECER O TIPO, ELE AVISA (EVITANDO O DEADLOCK)
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
