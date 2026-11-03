package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

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
	// olha a porta TCP da variável de ambiente; se não existir, usa 8811
	porta := os.Getenv("PORT")
	if porta == "" {
		porta = "8811"
	}

	// calcula a porta REST (ex: se TCP for 8811, REST vai ser 9811)
	portaInt, _ := strconv.Atoi(porta)
	portaREST := strconv.Itoa(portaInt + 1000)

	state := &ServerState{
		rides:      make(map[string]*protocolo.RideRequest),
		dbFile:     "db_" + porta + ".json",
		nextRideID: 1,
	}
	state.CarregarDados()

	peersEnv := os.Getenv("PEERS")
	if peersEnv != "" {
		state.peers = strings.Split(peersEnv, ",")
		fmt.Println("Servidor configurado com os peers TCP:", state.peers)
	} else {
		fmt.Println("Nenhum peer configurado. Rodando em modo isolado.")
	}

	// inicio do server http peer
	go func() {
		mux := http.NewServeMux()
		// rotas
		mux.HandleFunc("/api/rides", state.handlerGetRides)
		mux.HandleFunc("/api/book", state.handlerBookSegment)
		mux.HandleFunc("/api/book/undo", state.handlerUndoSegment)
		mux.HandleFunc("/api/reservations", state.handlerGetReservations)
		mux.HandleFunc("/api/cascade-cancel", state.handlerCascadeCancel)

		fmt.Printf("Servidor REST escutando na porta %s...\n", portaREST)
		if err := http.ListenAndServe(":"+portaREST, mux); err != nil {
			fmt.Println("Erro no servidor REST:", err)
		}
	}()

	// interface de socket TCP/IP para clientes
	listener, err := net.Listen("tcp", ":"+porta)
	if err != nil {
		fmt.Println("Erro ao iniciar servidor TCP na porta", porta, ":", err)
		return
	}
	defer listener.Close()
	fmt.Printf("Servidor TCP esperando clientes na porta %s...\n", porta)

	for {
		conn, err := listener.Accept()
		if err != nil {
			continue
		}
		// goroutine para cada cliente, atendimento simultâneo
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

			state.mu.Lock()
			ride, existe := state.rides[req.RideID]

			if existe && ride.DriverID == req.DriverID {
				dataDaViagem := ride.Date

				// identifica todos os passageiros que perdem a viagem
				passageirosAfetados := make(map[string]bool)
				for _, seg := range ride.Segments {
					for _, p := range seg.Passengers {
						passageirosAfetados[p] = true
					}
				}

				// exclui a carona cancelada
				delete(state.rides, req.RideID)

				// remove os mesmos passageiros de outras caronas no mesmo servidor e data
				for pID := range passageirosAfetados {
					for _, outrasRides := range state.rides {
						if outrasRides.Date == dataDaViagem {
							for i, seg := range outrasRides.Segments {
								for j, passID := range seg.Passengers {
									if passID == pID {
										outrasRides.Segments[i].Passengers = append(seg.Passengers[:j], seg.Passengers[j+1:]...)
										outrasRides.Segments[i].AvailableSeats++
										break
									}
								}
							}
						}
					}
				}
				state.SalvarDados()
				state.mu.Unlock()

				// avisa os Peers via REST para cancelarem o resto da viagem
				for pID := range passageirosAfetados {
					payload := map[string]string{
						"passenger_id": pID,
						"date":         dataDaViagem,
					}
					for _, peerAddr := range state.peers {
						// manda a requisição para limpar os nós vizinhos
						state.RequestPeerREST(peerAddr, "/api/cascade-cancel", http.MethodPost, payload)
					}
				}

				fmt.Printf("<- Carona %s cancelada com sucesso. Itinerários dependentes foram desfeitos.\n", req.RideID)
				conn.Write([]byte(`{"status":"success", "message":"Carona cancelada e passageiros notificados."}` + "\n"))
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

			// carrega viagens dos servers peers pela API REST
			for _, peerAddr := range state.peers {
				respBytes, err := state.RequestPeerREST(peerAddr, "/api/rides", http.MethodGet, nil)
				if err == nil {
					var viagensDoPeer map[string]*protocolo.RideRequest
					if err := json.Unmarshal(respBytes, &viagensDoPeer); err == nil {
						// Filtra e junta os trechos do vizinho no grafo
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
											ServerAddr:     peerAddr, // marca que este trecho é de outro servidor
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

			// gera uma marca de tempo pra ter precedência
			// captura o momento exato em que o server coordenador começa a transação
			timestampTransacao := time.Now().UnixNano()

			// ordenação global pra evitar deadlock
			// oredena os trechos e garante que todos os servidores tentem travar os recursos sempre na mesma ordem
			sort.Slice(req.Itinerary, func(i, j int) bool {
				strI := req.Itinerary[i].ServerAddr + req.Itinerary[i].RideID
				strJ := req.Itinerary[j].ServerAddr + req.Itinerary[j].RideID
				return strI < strJ
			})

			// registra qual peers já confirmou a reserva caso dê rollback
			sucessoPeers := []struct{ Addr, RideID, Orig, Dest string }{}
			sucessoLocais := []struct{ RideID, Orig, Dest string }{}

			podeReservar := true

			for _, trechoReq := range req.Itinerary {
				if trechoReq.ServerAddr != "" {
					// pede para o Peer reservar pelo POST (REST)
					payloadPeer := map[string]any{
						"passenger_id": req.PassengerID,
						"ride_id":      trechoReq.RideID,
						"origin":       trechoReq.Origin,
						"destination":  trechoReq.Destination,
						"timestamp":    timestampTransacao, // manda o critério de precedência
					}

					_, err := state.RequestPeerREST(trechoReq.ServerAddr, "/api/book", http.MethodPost, payloadPeer)
					if err != nil {
						// o erro pega tanto falhas de timeout quanto o conflito 409 do HTTP qnd n tem vagas
						podeReservar = false
						break // falhou num peer cancela tudo
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
				// rollback via REST
				for _, p := range sucessoPeers {
					payloadUndo := map[string]string{
						"passenger_id": req.PassengerID, "ride_id": p.RideID, "origin": p.Orig, "destination": p.Dest,
					}
					state.RequestPeerREST(p.Addr, "/api/book/undo", http.MethodPost, payloadUndo)
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

			// busca no servidor local
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
							break // vai pro próximo segmento
						}
					}
				}
			}
			state.mu.Unlock()

			// busca nos server peers via GET (REST) com query parameter
			for _, peerAddr := range state.peers {
				endpoint := fmt.Sprintf("/api/reservations?passenger_id=%s", req.PassengerID)
				respBytes, err := state.RequestPeerREST(peerAddr, endpoint, http.MethodGet, nil)

				if err == nil {
					var reservasDoPeer []Reserva
					if err := json.Unmarshal(respBytes, &reservasDoPeer); err == nil {
						// junta as reservas do vizinho na lista final
						minhasReservas = append(minhasReservas, reservasDoPeer...)
					}
				}
			}

			// manda a lista combinada pra o cliente
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

			// tenta cancelar localmente
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

			// verifica se ta nos peers via REST
			if !sucesso {
				payloadUndo := map[string]string{
					"passenger_id": req.PassengerID,
					"ride_id":      req.RideID,
					"origin":       req.Origin,
					"destination":  req.Destination,
				}

				for _, peerAddr := range state.peers {
					_, err := state.RequestPeerREST(peerAddr, "/api/book/undo", http.MethodPost, payloadUndo)
					if err == nil {
						sucesso = true
						break // o peer confirmou que a viagem era dele e cancelou
					}
				}
			}

			// devolve a resposta
			if sucesso {
				fmt.Printf("<- Reserva cancelada! Vaga devolvida. (Passageiro: %s | Trecho: %s -> %s)\n", req.PassengerID, req.Origin, req.Destination)
				conn.Write([]byte(`{"status":"success", "message":"Reserva cancelada com sucesso! Vaga devolvida."}` + "\n"))
			} else {
				fmt.Printf("<- Falha ao cancelar: Reserva de '%s' não encontrada em nenhum servidor.\n", req.PassengerID)
				conn.Write([]byte(`{"status":"error", "message":"Reserva não encontrada em nenhum trecho na rede."}` + "\n"))
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

// envia uma requisição REST para um servidor vizinho
func (s *ServerState) RequestPeerREST(peerAddr string, endpoint string, method string, reqBody interface{}) ([]byte, error) {
	// O peerAddr recebido é o TCP (ex: "localhost:8812"). Vamos converter para a porta REST (9812).
	partes := strings.Split(peerAddr, ":")
	host := "localhost"
	portaTCP := "8812"
	if len(partes) == 2 {
		host = partes[0]
		portaTCP = partes[1]
	}
	pInt, _ := strconv.Atoi(portaTCP)
	portaREST := strconv.Itoa(pInt + 1000)
	restAddr := fmt.Sprintf("%s:%s", host, portaREST)

	url := fmt.Sprintf("http://%s%s", restAddr, endpoint)

	var reqBytes []byte
	var err error
	if reqBody != nil {
		reqBytes, err = json.Marshal(reqBody)
		if err != nil {
			return nil, err
		}
	}

	req, err := http.NewRequest(method, url, bytes.NewBuffer(reqBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	// Cliente HTTP com limite de tempo. Se o Peer falhar ou demorar, a requisição é cancelada e o erro é tratado.
	client := &http.Client{
		Timeout: 3 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("falha ao contactar peer %s: %v", restAddr, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("peer %s retornou código de erro HTTP %d", restAddr, resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

// handler API REST

func (s *ServerState) handlerGetRides(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
		return
	}

	s.mu.Lock()
	payload, err := json.Marshal(s.rides)
	s.mu.Unlock()

	if err != nil {
		http.Error(w, "Erro ao serializar dados", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(payload)
}

func (s *ServerState) handlerBookSegment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		PassengerID string `json:"passenger_id"`
		RideID      string `json:"ride_id"`
		Origin      string `json:"origin"`
		Destination string `json:"destination"`
		Timestamp   int64  `json:"timestamp"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Payload inválido", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	sucesso := false
	if ride, existe := s.rides[req.RideID]; existe {
		for i, seg := range ride.Segments {
			if seg.Origin == req.Origin && seg.Destination == req.Destination {
				if seg.AvailableSeats > 0 {
					s.rides[req.RideID].Segments[i].AvailableSeats--
					s.rides[req.RideID].Segments[i].Passengers = append(seg.Passengers, req.PassengerID)
					s.SalvarDados()
					sucesso = true
				}
				break
			}
		}
	}
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	if sucesso {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"SUCCESS"}`))
	} else {
		// retorna 409 Conflict se a vaga já foi ocupada
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte(`{"status":"ERROR"}`))
	}
}

func (s *ServerState) handlerUndoSegment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		PassengerID string `json:"passenger_id"`
		RideID      string `json:"ride_id"`
		Origin      string `json:"origin"`
		Destination string `json:"destination"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	s.mu.Lock()
	if ride, existe := s.rides[req.RideID]; existe {
		for i, seg := range ride.Segments {
			if seg.Origin == req.Origin && seg.Destination == req.Destination {
				s.rides[req.RideID].Segments[i].AvailableSeats++
				for j, p := range seg.Passengers {
					if p == req.PassengerID {
						s.rides[req.RideID].Segments[i].Passengers = append(seg.Passengers[:j], seg.Passengers[j+1:]...)
						break
					}
				}
				s.SalvarDados()
				break
			}
		}
	}
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"SUCCESS"}`))
}

func (s *ServerState) handlerGetReservations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
		return
	}

	// o id do passageiro vem na URL
	passengerID := r.URL.Query().Get("passenger_id")

	type Reserva struct {
		RideID      string  `json:"ride_id"`
		DriverID    string  `json:"driver_id"`
		Date        string  `json:"date"`
		Origin      string  `json:"origin"`
		Destination string  `json:"destination"`
		Price       float64 `json:"price"`
	}
	var reservasPeer []Reserva

	s.mu.Lock()
	for rideID, ride := range s.rides {
		for _, seg := range ride.Segments {
			for _, passID := range seg.Passengers {
				if passID == passengerID {
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
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(reservasPeer)
}

func (s *ServerState) handlerCascadeCancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		PassengerID string `json:"passenger_id"`
		Date        string `json:"date"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Payload inválido", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	for _, ride := range s.rides {
		if ride.Date == req.Date {
			for i, seg := range ride.Segments {
				for j, passID := range seg.Passengers {
					if passID == req.PassengerID {
						// Remove o passageiro e devolve a vaga à carona de terceiros
						ride.Segments[i].Passengers = append(seg.Passengers[:j], seg.Passengers[j+1:]...)
						ride.Segments[i].AvailableSeats++
						break
					}
				}
			}
		}
	}
	s.SalvarDados()
	s.mu.Unlock()

	w.WriteHeader(http.StatusOK)
}
