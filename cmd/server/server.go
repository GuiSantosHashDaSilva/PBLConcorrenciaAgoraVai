package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"

	"github.com/GuiSantosHashDaSilva/PBLConcorrenciaAgoraVai/internals/protocolo"
)

type ServerState struct {
	mu         sync.Mutex
	rides      map[string]*protocolo.RideRequest
	nextRideID int
	peers      []string
	dbFile     string
}

func main() {

	// Lê a porta da variável de ambiente; se não existir, usa 8811
	porta := os.Getenv("PORT")
	if porta == "" {
		porta = "8811"
	}
	state := &ServerState{
		rides:      make(map[string]*protocolo.RideRequest),
		dbFile:     "db_" + porta + ".json",
		nextRideID: 1,
	}
	state.CarregarDados()

	peersEnv := os.Getenv("PEERS")
	if peersEnv != "" {
		state.peers = strings.Split(peersEnv, ",")
		fmt.Println("Servidor configurado com os peers:", state.peers)
	} else {
		fmt.Println("Nenhum peer configurado. Rodando em modo isolado.")
	}

	// Interface de socket nativa do TCP/IP com porta dinâmica
	listener, err := net.Listen("tcp", ":"+porta)
	if err != nil {
		fmt.Println("Erro ao iniciar servidor na porta", porta, ":", err)
		return
	}
	defer listener.Close()
	fmt.Printf("Servidor central VAIJUNTO aguardando conexões na porta %s...\n", porta)

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
			state.SalvarDados()
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
			state.mu.Unlock() // libera depois de copiar os dados

			// transforma a lista de caronas em json e envia de volta
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
				state.SalvarDados()
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
				ServerAddr     string  `json:"server_addr,omitempty"`
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

			// carrega viagens dos servers peers
			reqPeer := protocolo.Message{Type: protocolo.PEER_FETCH_RIDES}
			for _, peerAddr := range state.peers {
				resp, err := state.EnviarParaPeer(peerAddr, reqPeer)
				if err == nil && resp.Type == "SUCCESS" {
					var viagensDoPeer map[string]*protocolo.RideRequest
					if err := json.Unmarshal(resp.Payload, &viagensDoPeer); err == nil {

						// Filtra e joga os trechos do vizinho no grafo
						for rideID, ride := range viagensDoPeer {
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
											ServerAddr:     peerAddr, // Marca que este trecho é de outro servidor
										})
									}
								}
							}
						}
					}
				}
			}

			// BFS
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
					ServerAddr  string `json:"server_addr"`
				} `json:"itinerary"`
			}
			if err := json.Unmarshal(msg.Payload, &req); err != nil {
				conn.Write([]byte(`{"status":"error", "message":"Payload inválido para reserva"}` + "\n"))
				continue
			}

			// registra quais peers já confirmaram a reserva caso dê Rollback
			sucessoPeers := []struct{ Addr, RideID, Orig, Dest string }{}
			sucessoLocais := []struct{ RideID, Orig, Dest string }{}

			podeReservar := true

			for _, trechoReq := range req.Itinerary {
				if trechoReq.ServerAddr != "" {
					// pede para o Peer reservar
					payloadPeer, _ := json.Marshal(map[string]string{
						"passenger_id": req.PassengerID,
						"ride_id":      trechoReq.RideID,
						"origin":       trechoReq.Origin,
						"destination":  trechoReq.Destination,
					})

					resp, err := state.EnviarParaPeer(trechoReq.ServerAddr, protocolo.Message{
						Type:    protocolo.PEER_BOOK_SEGMENT,
						Payload: payloadPeer,
					})

					if err != nil || resp.Type != "SUCCESS" {
						podeReservar = false
						break // falhou num peer, para tudo
					}

					// peer confirmou, guarda para caso precise fazer rollback
					sucessoPeers = append(sucessoPeers, struct{ Addr, RideID, Orig, Dest string }{
						Addr: trechoReq.ServerAddr, RideID: trechoReq.RideID, Orig: trechoReq.Origin, Dest: trechoReq.Destination,
					})

				} else {
					state.mu.Lock()
					ride, existe := state.rides[trechoReq.RideID]
					trechoReservado := false

					if existe {
						for i, seg := range ride.Segments {
							if seg.Origin == trechoReq.Origin && seg.Destination == trechoReq.Destination {
								if seg.AvailableSeats > 0 {
									ride.Segments[i].AvailableSeats--
									ride.Segments[i].Passengers = append(ride.Segments[i].Passengers, req.PassengerID)
									trechoReservado = true
									// guarda para rollback local se der erro depois
									sucessoLocais = append(sucessoLocais, struct{ RideID, Orig, Dest string }{
										RideID: trechoReq.RideID, Orig: trechoReq.Origin, Dest: trechoReq.Destination,
									})
									state.SalvarDados()
								}
								break
							}
						}
					}
					state.mu.Unlock()

					if !trechoReservado {
						podeReservar = false
						break // falhou localmente, cancela tudo
					}
				}
			}

			if podeReservar {
				fmt.Printf("<- Reserva distribuída concluída para o passageiro '%s'!\n", req.PassengerID)
				conn.Write([]byte(`{"status":"success", "message":"Itinerário confirmado!"}` + "\n"))
			} else {
				// rollback
				for _, p := range sucessoPeers {
					payloadUndo, _ := json.Marshal(map[string]string{
						"passenger_id": req.PassengerID, "ride_id": p.RideID, "origin": p.Orig, "destination": p.Dest,
					})
					state.EnviarParaPeer(p.Addr, protocolo.Message{Type: protocolo.PEER_UNDO_SEGMENT, Payload: payloadUndo})
				}

				state.mu.Lock()
				for _, l := range sucessoLocais {
					ride := state.rides[l.RideID]
					for i, seg := range ride.Segments {
						if seg.Origin == l.Orig && seg.Destination == l.Dest {
							ride.Segments[i].AvailableSeats++ // devolve a vaga
							// remove o passageiro da lista
							ride.Segments[i].Passengers = ride.Segments[i].Passengers[:len(ride.Segments[i].Passengers)-1]
							break
						}
					}
				}
				state.SalvarDados()
				state.mu.Unlock()

				fmt.Println("<- Falha na reserva distribuída: Rollback executado.")
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

			type Reserva struct {
				RideID      string  `json:"ride_id"`
				DriverID    string  `json:"driver_id"`
				Date        string  `json:"date"`
				Origin      string  `json:"origin"`
				Destination string  `json:"destination"`
				Price       float64 `json:"price"`
			}
			var minhasReservas []Reserva

			// 1. Busca no servidor LOCAL
			state.mu.Lock()
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
							break // Vai pro próximo segmento
						}
					}
				}
			}
			state.mu.Unlock()

			// 2. Busca nos servidores VIZINHOS (Peers)
			reqPeer := protocolo.Message{
				Type:    protocolo.PEER_FETCH_PASSENGER_RIDES,
				Payload: msg.Payload, // Repassa o JSON com o PassengerID
			}
			for _, peerAddr := range state.peers {
				resp, err := state.EnviarParaPeer(peerAddr, reqPeer)
				if err == nil && resp.Type == "SUCCESS" {
					var reservasDoPeer []Reserva
					if err := json.Unmarshal(resp.Payload, &reservasDoPeer); err == nil {
						// Junta as reservas do vizinho na lista final
						minhasReservas = append(minhasReservas, reservasDoPeer...)
					}
				}
			}

			// 3. Envolve a lista combinada e envia ao cliente
			respostaBytes, _ := json.Marshal(map[string]any{
				"status":       "success",
				"reservations": minhasReservas,
			})
			conn.Write(append(respostaBytes, '\n'))
			fmt.Printf("<- Enviando reservas globais para o passageiro '%s'\n", req.PassengerID)

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

			// 1. TENTA CANCELAR LOCALMENTE PRIMEIRO
			state.mu.Lock()
			ride, existe := state.rides[req.RideID]
			sucesso := false

			if existe {
				for i, seg := range ride.Segments {
					if seg.Origin == req.Origin && seg.Destination == req.Destination {
						for j, passID := range seg.Passengers {
							if passID == req.PassengerID {
								// Remove o passageiro
								ride.Segments[i].Passengers = append(seg.Passengers[:j], seg.Passengers[j+1:]...)
								// Devolve a vaga
								ride.Segments[i].AvailableSeats++

								state.SalvarDados() // <-- Muito importante salvar o estado atualizado!
								sucesso = true
								break
							}
						}
						break
					}
				}
			}
			state.mu.Unlock()

			// 2. SE NÃO ENCONTROU LOCALMENTE, PERGUNTA AOS PEERS (REDE DISTRIBUÍDA)
			if !sucesso {
				// Reutilizamos a requisição de Rollback que criamos no Two-Phase Commit
				reqPeer := protocolo.Message{
					Type:    protocolo.PEER_UNDO_SEGMENT,
					Payload: msg.Payload, // O payload atual já tem os dados perfeitos (PassengerID, RideID, etc)
				}

				for _, peerAddr := range state.peers {
					resp, err := state.EnviarParaPeer(peerAddr, reqPeer)
					if err == nil && resp.Type == "SUCCESS" {
						sucesso = true
						break // O peer confirmou que a viagem era dele e cancelou. Fim da busca!
					}
				}
			}

			// 3. DEVOLVE A RESPOSTA FINAL AO CLIENTE
			if sucesso {
				fmt.Printf("<- Reserva cancelada! Vaga devolvida. (Passageiro: %s | Trecho: %s -> %s)\n", req.PassengerID, req.Origin, req.Destination)
				conn.Write([]byte(`{"status":"success", "message":"Reserva cancelada com sucesso! Vaga devolvida."}` + "\n"))
			} else {
				fmt.Printf("<- Falha ao cancelar: Reserva de '%s' não encontrada em nenhum servidor.\n", req.PassengerID)
				conn.Write([]byte(`{"status":"error", "message":"Reserva não encontrada em nenhum trecho na rede."}` + "\n"))
			}

		case string(protocolo.PEER_FETCH_RIDES):
			state.mu.Lock()
			// Transforma o mapa local num JSON para enviar ao peer solicitante
			payload, _ := json.Marshal(state.rides)
			state.mu.Unlock()

			resposta := protocolo.Message{
				Type:    "SUCCESS",
				Payload: payload,
			}
			json.NewEncoder(conn).Encode(resposta)

		case string(protocolo.PEER_BOOK_SEGMENT):
			// decodifica o pedido que veio do outro servidor
			var req struct {
				PassengerID string `json:"passenger_id"`
				RideID      string `json:"ride_id"`
				Origin      string `json:"origin"`
				Destination string `json:"destination"`
			}
			json.Unmarshal(msg.Payload, &req)

			// tenta fazer a reserva localmente
			state.mu.Lock()
			sucesso := false
			if ride, existe := state.rides[req.RideID]; existe {
				for i, seg := range ride.Segments {
					if seg.Origin == req.Origin && seg.Destination == req.Destination {
						if seg.AvailableSeats > 0 { // Confirma se ainda tem vaga
							ride.Segments[i].AvailableSeats--
							ride.Segments[i].Passengers = append(ride.Segments[i].Passengers, req.PassengerID)
							state.SalvarDados()
							sucesso = true
						}
						break
					}
				}
			}
			state.mu.Unlock()

			// responde ao servidor cordenador
			resp := protocolo.Message{Type: "SUCCESS"}
			if !sucesso {
				resp.Type = "ERROR"
			}
			json.NewEncoder(conn).Encode(resp)

		case string(protocolo.PEER_FETCH_PASSENGER_RIDES):
			var req struct {
				PassengerID string `json:"passenger_id"`
			}
			json.Unmarshal(msg.Payload, &req)

			type Reserva struct {
				RideID      string  `json:"ride_id"`
				DriverID    string  `json:"driver_id"`
				Date        string  `json:"date"`
				Origin      string  `json:"origin"`
				Destination string  `json:"destination"`
				Price       float64 `json:"price"`
			}
			var reservasPeer []Reserva

			// Varre a memória do peer
			state.mu.Lock()
			for rideID, ride := range state.rides {
				for _, seg := range ride.Segments {
					for _, passID := range seg.Passengers {
						if passID == req.PassengerID {
							reservasPeer = append(reservasPeer, Reserva{
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

			// Converte a lista para JSON e manda de volta como payload
			payloadResp, _ := json.Marshal(reservasPeer)
			conn.Write(append([]byte(`{"type":"SUCCESS", "payload":`+string(payloadResp)+`}`), '\n'))

		case string(protocolo.PEER_UNDO_SEGMENT):
			// 1. Decodifica o pedido de Rollback
			var req struct {
				PassengerID string `json:"passenger_id"`
				RideID      string `json:"ride_id"`
				Origin      string `json:"origin"`
				Destination string `json:"destination"`
			}
			json.Unmarshal(msg.Payload, &req)

			// 2. Desfaz a reserva (devolve a vaga e remove o passageiro)
			state.mu.Lock()
			if ride, existe := state.rides[req.RideID]; existe {
				for i, seg := range ride.Segments {
					if seg.Origin == req.Origin && seg.Destination == req.Destination {
						ride.Segments[i].AvailableSeats++ // Devolve a vaga

						// Busca o passageiro na lista e o remove
						for j, p := range ride.Segments[i].Passengers {
							if p == req.PassengerID {
								ride.Segments[i].Passengers = append(ride.Segments[i].Passengers[:j], ride.Segments[i].Passengers[j+1:]...)
								break
							}
						}
						state.SalvarDados()
						break
					}
				}
			}
			state.mu.Unlock()

			// Confirma que o desfazimento foi feito
			json.NewEncoder(conn).Encode(protocolo.Message{Type: "SUCCESS"})

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

// lê o arquivo JSON e joga para a memória ao iniciar o servidor
func (s *ServerState) CarregarDados() {
	dados, err := os.ReadFile(s.dbFile)
	if err != nil {
		fmt.Println("[DB] Arquivo de dados não encontrado. Iniciando base limpa.")
		return
	}

	err = json.Unmarshal(dados, &s.rides)
	if err != nil {
		fmt.Println("[DB] Erro ao decodificar o arquivo JSON:", err)
		return
	}
	fmt.Println("[DB] Viagens anteriores carregadas com sucesso!")
}

// escreve o estado atual da memória no arquivo JSON
func (s *ServerState) SalvarDados() {
	// Usa o MarshalIndent para o JSON ficar formatado e legível
	dados, err := json.MarshalIndent(s.rides, "", "  ")
	if err != nil {
		fmt.Println("[DB] Erro ao serializar dados para salvamento:", err)
		return
	}

	err = os.WriteFile(s.dbFile, dados, 0644)
	if err != nil {
		fmt.Println("[DB] Erro ao escrever no arquivo JSON:", err)
	}
}

// envia uma requisição TCP para um servidor vizinho e retorna a resposta
func (s *ServerState) EnviarParaPeer(peerAddr string, req protocolo.Message) (protocolo.Message, error) {
	conn, err := net.Dial("tcp", peerAddr)
	if err != nil {
		return protocolo.Message{}, fmt.Errorf("falha ao conectar no peer %s: %v", peerAddr, err)
	}
	defer conn.Close()

	// Codifica a requisição
	encoder := json.NewEncoder(conn)
	if err := encoder.Encode(req); err != nil {
		return protocolo.Message{}, fmt.Errorf("falha ao enviar para %s: %v", peerAddr, err)
	}

	// Lê a resposta do peer
	var resp protocolo.Message
	decoder := json.NewDecoder(conn)
	if err := decoder.Decode(&resp); err != nil {
		return protocolo.Message{}, fmt.Errorf("falha ao ler resposta de %s: %v", peerAddr, err)
	}

	return resp, nil
}
