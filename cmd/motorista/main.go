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
		enderecoServidor = "localhost:8811"
	}

	conn, err := net.Dial("tcp", enderecoServidor)
	if err != nil {
		fmt.Println("Erro ao conectar ao servidor:", err)
		return
	}
	defer conn.Close()

	
	leitorRede := bufio.NewReader(conn)
	scannerTeclado := bufio.NewScanner(os.Stdin)
	var driverID string

	for {
		fmt.Println("\n=== VAIJUNTO: PAINEL DO MOTORISTA ===")
		if driverID == "" {
			fmt.Println("Status: Não autenticado")
			fmt.Println("1. Autenticar (Login)")
		} else {
			fmt.Printf("Status: Autenticado como '%s'\n", driverID)
			fmt.Println("2. Publicar Carona")
			fmt.Println("3. Consultar Minhas Caronas")
			fmt.Println("4. Cancelar Carona")
		}
		fmt.Println("0. Sair")
		fmt.Print("Escolha uma opção: ")

		scannerTeclado.Scan()
		opcao := scannerTeclado.Text()

		if opcao == "0" {
			break
		}

		if driverID == "" && opcao == "1" {
			// Passa o leitorRede para a função
			driverID = autenticar(conn, scannerTeclado, leitorRede)
		} else if driverID != "" {
			switch opcao {
			case "2":
				publicarCarona(conn, scannerTeclado, leitorRede, driverID)
			case "3":
				consultarCaronas(conn, leitorRede, driverID)
			case "4":
				cancelarCarona(conn, scannerTeclado, leitorRede, driverID)
			default:
				fmt.Println("Opção inválida.")
			}
		}
	}
}

// atualiza as funções pra receber o leitor
func autenticar(conn net.Conn, scanner *bufio.Scanner, leitor *bufio.Reader) string {
	fmt.Print("\n--- Login ---\nDigite seu Login (ID): ")
	scanner.Scan()
	login := scanner.Text()

	fmt.Print("Digite sua Senha: ")
	scanner.Scan()
	senha := scanner.Text()

	req := map[string]string{"login": login, "senha": senha}
	enviarRequisicao(conn, "LOGIN", req)

	// Usa o leitor único aqui
	resposta, _ := leitor.ReadString('\n')
	fmt.Println("Resposta do servidor:", strings.TrimSpace(resposta))

	if strings.Contains(resposta, "success") {
		return login
	}
	return ""
}

func publicarCarona(conn net.Conn, scanner *bufio.Scanner, leitor *bufio.Reader, driverID string) {
	fmt.Print("\n--- Publicar Carona ---\nData da viagem (ex: 15-10-2026): ")
	scanner.Scan()
	data := scanner.Text()

	fmt.Print("Quantos trechos terá a viagem? ")
	scanner.Scan()
	qtd, _ := strconv.Atoi(scanner.Text())

	var segmentos []map[string]any
	for i := 0; i < qtd; i++ {
		fmt.Print("Origem: ")
		scanner.Scan()
		origem := scanner.Text()

		fmt.Print("Destino: ")
		scanner.Scan()
		destino := scanner.Text()

		fmt.Print("Quantidade de Assentos: ")
		scanner.Scan()
		assentos, _ := strconv.Atoi(scanner.Text())

		fmt.Print("Preço: ")
		scanner.Scan()
		preco, _ := strconv.ParseFloat(scanner.Text(), 64)

		segmentos = append(segmentos, map[string]any{
			"origin": origem, "destination": destino, "available_seats": assentos, "price": preco,
		})
	}

	req := map[string]any{"driver_id": driverID, "date": data, "segments": segmentos}
	enviarRequisicao(conn, "PUBLISH_RIDE", req)

	resposta, _ := leitor.ReadString('\n')
	fmt.Println("Resposta do servidor:", strings.TrimSpace(resposta))
}

func consultarCaronas(conn net.Conn, leitor *bufio.Reader, driverID string) {
	fmt.Println("\n--- Minhas Caronas ---")
	req := map[string]string{"driver_id": driverID}
	enviarRequisicao(conn, "CONSULT_RIDES", req)

	resposta, _ := leitor.ReadString('\n')

	// Estrutura para ler e formatar os dados que vêm do servidor
	var respData struct {
		Status string `json:"status"`
		Rides  []struct {
			Date     string `json:"date"`
			Segments []struct {
				Origin         string   `json:"origin"`
				Destination    string   `json:"destination"`
				AvailableSeats int      `json:"available_seats"`
				Price          float64  `json:"price"`
				Passengers     []string `json:"passengers"`
			} `json:"segments"`
		} `json:"rides"`
	}

	json.Unmarshal([]byte(resposta), &respData)

	if len(respData.Rides) == 0 {
		fmt.Println("Nenhuma carona publicada ou encontrada.")
		return
	}

	for i, ride := range respData.Rides {
		fmt.Printf("\n[Viagem %d] - Data: %s\n", i+1, ride.Date)
		for j, seg := range ride.Segments {
			fmt.Printf("  Trecho %d: %s -> %s\n", j+1, seg.Origin, seg.Destination)
			fmt.Printf("  Assentos Restantes: %d | Preço: R$ %.2f\n", seg.AvailableSeats, seg.Price)

			// Mostra os passageiros se tiver algum
			if len(seg.Passengers) > 0 {
				fmt.Printf("  Passageiros Confirmados: %s\n", strings.Join(seg.Passengers, ", "))
			} else {
				fmt.Println("  Passageiros Confirmados: Nenhum")
			}
		}
	}
}

func cancelarCarona(conn net.Conn, scanner *bufio.Scanner, leitor *bufio.Reader, driverID string) {
	fmt.Print("\n--- Cancelar Carona ---\nDigite o ID da viagem que deseja cancelar (ex: 1, 2): ")
	scanner.Scan()
	rideID := strings.TrimSpace(scanner.Text())

	// Agora enviamos o ID simples diretamente na requisição
	req := map[string]string{"ride_id": rideID, "driver_id": driverID}

	enviarRequisicao(conn, "CANCEL_RIDE", req)

	resposta, _ := leitor.ReadString('\n')
	fmt.Println("Resposta do servidor:", strings.TrimSpace(resposta))
}

func enviarRequisicao(conn net.Conn, tipo string, payload any) {
	payloadBytes, _ := json.Marshal(payload)
	msg := Message{Type: tipo, Payload: payloadBytes}
	msgBytes, _ := json.Marshal(msg)
	fmt.Fprintf(conn, string(msgBytes)+"\n")
}
