package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Message struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

func main() {
	fmt.Println("=== INICIANDO TESTE DE CONCORRÊNCIA (MUTEX) ===")

	// 1. O Script Motorista publica uma carona com 5 vagas
	prepararCarona()
	time.Sleep(1 * time.Second) // Dá um tempo para o servidor processar

	// 50 passageiros tentam reservar ao mesmo tempo
	var wg sync.WaitGroup
	var sucessos int32
	var falhas int32
	totalTentativas := 50

	fmt.Printf("\nDisparando %d requisições simultâneas para uma carona com 5 vagas...\n", totalTentativas)

	for i := 0; i < totalTentativas; i++ {
		wg.Add(1)
		passengerID := fmt.Sprintf("passageiro_teste_%d", i)

		go func(pID string) {
			defer wg.Done()
			sucesso := tentarReservar(pID)
			if sucesso {
				atomic.AddInt32(&sucessos, 1)
			} else {
				atomic.AddInt32(&falhas, 1)
			}
		}(passengerID)
	}

	wg.Wait() // Espera todas as 50 goroutines terminarem

	// Resultado
	fmt.Println("\n=== RESULTADO DO TESTE ===")
	fmt.Printf("Sucessos (Vagas preenchidas): %d\n", sucessos)
	fmt.Printf("Falhas (Vagas esgotadas): %d\n", falhas)

	if sucessos == 5 && falhas == 45 {
		fmt.Println("passou: O Mutex funcionou, nenhuma vaga foi vendida a mais e não houve corrida")
	} else {
		fmt.Println("falhou: Houve vazamento de vagas ou bloqueio inesperado.")
	}
}

// Função auxiliar que cria a carona alvo do teste
func prepararCarona() {
	conn, err := net.Dial("tcp", "localhost:8811")
	if err != nil {
		fmt.Println("Erro: O servidor precisa estar rodando para o teste.")
		return
	}
	defer conn.Close()

	segmentos := []map[string]any{
		{"origin": "A", "destination": "B", "available_seats": 5, "price": 10.0},
	}
	req := map[string]any{
		"driver_id": "motorista_teste",
		"date":      "2026-12-31",
		"segments":  segmentos,
	}

	enviar(conn, "PUBLISH_RIDE", req)
	leitor := bufio.NewReader(conn)
	leitor.ReadString('\n')
	fmt.Println("Carona de teste publicada com sucesso (ID: motorista_teste-2026-12-31 | 5 Vagas).")
}

// Função q cada goroutine executa
func tentarReservar(passengerID string) bool {
	conn, err := net.Dial("tcp", "localhost:8811")
	if err != nil {
		return false
	}
	defer conn.Close()

	req := map[string]any{
		"passenger_id": passengerID,
		"itinerary": []map[string]string{
			{"ride_id": "1", "origin": "A", "destination": "B"},
		},
	}

	enviar(conn, "BOOK_ROUTE", req)
	leitor := bufio.NewReader(conn)
	resposta, _ := leitor.ReadString('\n')

	// Se a resposta for "success", deu certo
	return strings.Contains(resposta, "success")
}

func enviar(conn net.Conn, tipo string, payload any) {
	payloadBytes, _ := json.Marshal(payload)
	msg := Message{Type: tipo, Payload: payloadBytes}
	msgBytes, _ := json.Marshal(msg)
	fmt.Fprintf(conn, string(msgBytes)+"\n")
}
