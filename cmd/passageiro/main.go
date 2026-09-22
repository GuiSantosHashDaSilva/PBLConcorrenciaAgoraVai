package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

type Message struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

func main() {
	enderecoServidor := os.Getenv("SERVER_ADDR")
	if enderecoServidor == "" {
		enderecoServidor = "localhost:8811" //pra rodar fora do Docker
	}

	conn, err := net.Dial("tcp", enderecoServidor)
	if err != nil {
		fmt.Println("Erro ao conectar ao servidor:", err)
		return
	}
	defer conn.Close()

	leitorRede := bufio.NewReader(conn)
	scannerTeclado := bufio.NewScanner(os.Stdin)
	var passengerID string // Variável para controlar a sessão do passageiro

	for {
		fmt.Println("\n=== VAIJUNTO: PAINEL DO PASSAGEIRO ===")
		if passengerID == "" {
			fmt.Println("Status: Não autenticado")
			fmt.Println("1. Autenticar (Login)")
		} else {
			fmt.Printf("Status: Autenticado como '%s'\n", passengerID)
			fmt.Println("2. Buscar e Reservar Viagens")
			fmt.Println("3. Consultar Minhas Reservas")
			fmt.Println("4. Cancelar uma Reserva")
		}
		fmt.Println("0. Sair")
		fmt.Print("Escolha uma opção: ")

		scannerTeclado.Scan()
		opcao := strings.TrimSpace(scannerTeclado.Text())

		if opcao == "0" {
			fmt.Println("Encerrando cliente...")
			break
		}

		if passengerID == "" && opcao == "1" {
			passengerID = autenticar(conn, scannerTeclado, leitorRede)
		} else if passengerID != "" {
			switch opcao {
			case "2":
				buscarEReservar(conn, scannerTeclado, leitorRede, passengerID)
			case "3":
				consultarReservas(conn, leitorRede, passengerID)
			case "4":
				cancelarReserva(conn, scannerTeclado, leitorRede, passengerID)
			default:
				fmt.Println("Opção inválida.")
			}
		} else {
			fmt.Println("Você precisa se autenticar primeiro.")
		}
	}
}


func autenticar(conn net.Conn, scanner *bufio.Scanner, leitor *bufio.Reader) string {
	fmt.Print("\n--- Login ---\nDigite seu Login (ID): ")
	scanner.Scan()
	login := strings.TrimSpace(scanner.Text())

	fmt.Print("Digite sua Senha: ")
	scanner.Scan()
	senha := strings.TrimSpace(scanner.Text())

	req := map[string]string{"login": login, "senha": senha}
	enviarRequisicao(conn, "LOGIN", req)

	resposta, _ := leitor.ReadString('\n')
	fmt.Println("Resposta do servidor:", strings.TrimSpace(resposta))

	if strings.Contains(resposta, "success") {
		return login
	}
	return ""
}


func buscarEReservar(conn net.Conn, scanner *bufio.Scanner, leitor *bufio.Reader, passengerID string) {
	fmt.Print("\n--- Buscar Viagens ---\n")

	fmt.Print("Data da viagem (ex: 15-10-2026): ")
	scanner.Scan()
	dataViagem := strings.TrimSpace(scanner.Text())

	fmt.Print("Origem: ")
	scanner.Scan()
	origem := strings.TrimSpace(scanner.Text())

	fmt.Print("Destino: ")
	scanner.Scan()
	destino := strings.TrimSpace(scanner.Text())

	// Requisição de busca
	reqBusca := map[string]string{
		"date":        dataViagem,
		"origin":      origem,
		"destination": destino,
	}
	enviarRequisicao(conn, "SEARCH_ROUTE", reqBusca)

	resposta, _ := leitor.ReadString('\n')

	// Estrutura para ler o roteiro
	var respData struct {
		Status string `json:"status"`
		Routes [][]struct {
			RideID      string  `json:"ride_id"`
			DriverID    string  `json:"driver_id"`
			Origin      string  `json:"origin"`
			Destination string  `json:"destination"`
			Price       float64 `json:"price"`
		} `json:"routes"`
	}

	json.Unmarshal([]byte(resposta), &respData)

	if len(respData.Routes) == 0 {
		fmt.Println("\nNenhuma viagem encontrada.")
		return
	}

	fmt.Printf("\n=== %d ROTEIRO(S) ENCONTRADO(S) ===\n", len(respData.Routes))
	for i, rota := range respData.Routes {
		fmt.Printf("\n[Opção %d]\n", i+1)
		var precoTotal float64
		for _, trecho := range rota {
			fmt.Printf("  -> %s para %s (Carona ID: %s | Motorista: %s | R$ %.2f)\n",
				trecho.Origin, trecho.Destination, trecho.RideID, trecho.DriverID, trecho.Price)
			precoTotal += trecho.Price
		}
		fmt.Printf("  * PREÇO TOTAL: R$ %.2f\n", precoTotal)
	}

	fmt.Print("\nDigite o número da opção que deseja reservar (ou 0 para cancelar/voltar): ")
	scanner.Scan()
	escolhaStr := strings.TrimSpace(scanner.Text())

	escolha, err := strconv.Atoi(escolhaStr)
	if err != nil || escolha == 0 {
		fmt.Println("Operação cancelada. Voltando ao menu...")
		return
	}

	if escolha > 0 && escolha <= len(respData.Routes) {
		rotaEscolhida := respData.Routes[escolha-1]
		var itinerario []map[string]string

		for _, trecho := range rotaEscolhida {
			itinerario = append(itinerario, map[string]string{
				"ride_id":     trecho.RideID,
				"origin":      trecho.Origin,
				"destination": trecho.Destination,
			})
		}

		// Requisição de reserva
		reqReserva := map[string]any{
			"passenger_id": passengerID,
			"itinerary":    itinerario,
		}
		enviarRequisicao(conn, "BOOK_ROUTE", reqReserva)

		respReserva, _ := leitor.ReadString('\n')
		fmt.Println("\nStatus da Reserva:", strings.TrimSpace(respReserva))
	} else {
		fmt.Println("Opção inválida. Voltando ao menu...")
	}
}

func consultarReservas(conn net.Conn, leitor *bufio.Reader, passengerID string) {
	fmt.Println("\n--- Minhas Reservas ---")
	req := map[string]string{"passenger_id": passengerID}
	enviarRequisicao(conn, "CONSULT_RESERVATIONS", req)

	resposta, _ := leitor.ReadString('\n')

	// Estrutura para ler o array de reservas devolvido pelo servidor
	var respData struct {
		Status       string `json:"status"`
		Reservations []struct {
			RideID      string  `json:"ride_id"`
			DriverID    string  `json:"driver_id"`
			Date        string  `json:"date"`
			Origin      string  `json:"origin"`
			Destination string  `json:"destination"`
			Price       float64 `json:"price"`
		} `json:"reservations"`
	}

	json.Unmarshal([]byte(resposta), &respData)

	if len(respData.Reservations) == 0 {
		fmt.Println("Você ainda não possui nenhuma reserva confirmada.")
		return
	}

	fmt.Printf("\nVocê possui %d trecho(s) reservado(s):\n", len(respData.Reservations))
	for i, res := range respData.Reservations {
		fmt.Printf("\n[Reserva %d]\n", i+1)
		fmt.Printf("  Data da Viagem: %s (Carona ID: %s)\n", res.Date, res.RideID)
		fmt.Printf("  Motorista: %s\n", res.DriverID)
		fmt.Printf("  Trecho: %s -> %s\n", res.Origin, res.Destination)
		fmt.Printf("  Valor: R$ %.2f\n", res.Price)
	}
}

func cancelarReserva(conn net.Conn, scanner *bufio.Scanner, leitor *bufio.Reader, passengerID string) {
	fmt.Print("\n--- Cancelar Reserva ---\n")
	
	fmt.Print("Digite o ID numérico da Carona (ex: 1, 2): ")
	scanner.Scan()
	rideID := strings.TrimSpace(scanner.Text())

	fmt.Print("Origem do trecho que deseja cancelar: ")
	scanner.Scan()
	origem := strings.TrimSpace(scanner.Text())

	fmt.Print("Destino do trecho que deseja cancelar: ")
	scanner.Scan()
	destino := strings.TrimSpace(scanner.Text())

	req := map[string]string{
		"passenger_id": passengerID,
		"ride_id":      rideID,
		"origin":       origem,
		"destination":  destino,
	}

	enviarRequisicao(conn, "CANCEL_RESERVATION", req)

	resposta, _ := leitor.ReadString('\n')
	fmt.Println("\nResposta do servidor:", strings.TrimSpace(resposta))
}

func enviarRequisicao(conn net.Conn, tipo string, payload any) {
	payloadBytes, _ := json.Marshal(payload)
	msg := Message{Type: tipo, Payload: payloadBytes}
	msgBytes, _ := json.Marshal(msg)
	fmt.Fprintf(conn, string(msgBytes)+"\n")
}
