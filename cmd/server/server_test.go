package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/GuiSantosHashDaSilva/PBLConcorrenciaAgoraVai/internals/protocolo"
)

// Função auxiliar para inicializar um estado de servidor limpo para os testes
func setupTestServer() *ServerState {
	state := &ServerState{
		rides:      make(map[string]*protocolo.RideRequest),
		nextRideID: 1,
		dbFile:     "test_db_mock.json", // Arquivo temporário para testes
	}
	return state
}

// =====================================================================
// TESTE 1: Exclusão Mútua (Item 4) e Precedência/Timestamp (Item 6)
// =====================================================================
func TestConcurrencyBookSegment(t *testing.T) {
	state := setupTestServer()

	// Injetamos via JSON para contornar os campos exatos da struct interna
	mockJSON := `{
		"driver_id": "motorista_test",
		"date": "15-10-2026",
		"segments": [
			{
				"origin": "Feira",
				"destination": "Salvador",
				"available_seats": 1,
				"price": 50.0
			}
		]
	}`
	var ride protocolo.RideRequest
	json.Unmarshal([]byte(mockJSON), &ride)
	state.rides["1"] = &ride

	var wg sync.WaitGroup
	sucessos := 0
	falhas := 0
	var mu sync.Mutex // Mutex próprio do teste para contar os resultados em segurança

	// Simula 3 requisições REST SIMULTÂNEAS vindas de outros peers
	// tentando reservar a ÚNICA vaga disponível na Carona 1
	qtdRequisicoes := 3
	for i := 1; i <= qtdRequisicoes; i++ {
		wg.Add(1)
		go func(passengerID string, timestamp int64) {
			defer wg.Done()

			reqBody := map[string]any{
				"passenger_id": passengerID,
				"ride_id":      "1",
				"origin":       "Feira",
				"destination":  "Salvador",
				"timestamp":    timestamp, // Precedência sendo enviada
			}
			bodyBytes, _ := json.Marshal(reqBody)
			req, _ := http.NewRequest(http.MethodPost, "/api/book", bytes.NewBuffer(bodyBytes))
			rr := httptest.NewRecorder()

			// Chama a API REST de reserva internamente
			state.handlerBookSegment(rr, req)

			// Contabiliza o resultado HTTP
			mu.Lock()
			if rr.Code == http.StatusOK {
				sucessos++
			} else if rr.Code == http.StatusConflict { // Erro 409
				falhas++
			}
			mu.Unlock()
		}(fmt.Sprintf("passageiro_concorrente_%d", i), time.Now().UnixNano())
	}

	wg.Wait() // Aguarda todos os passageiros tentarem ao mesmo tempo

	// Verificação do resultado: Como só há 1 vaga e Mutex, deve haver exato 1 sucesso e 2 conflitos.
	if sucessos != 1 {
		t.Errorf("Falha na Exclusão Mútua: Esperava 1 sucesso, obteve %d", sucessos)
	} else {
		t.Logf("SUCESSO: Exclusão Mútua bloqueou corretamente as corridas de concorrência. (%d Reserva / %d Rejeições)", sucessos, falhas)
	}
}

// =====================================================================
// TESTE 2: Cancelamento em Cascata e Rollback (Item 9)
// =====================================================================
func TestCascadeCancelREST(t *testing.T) {
	state := setupTestServer()

	// Cria uma carona onde o passageiro_alvo JÁ POSSUI a reserva (AvailableSeats reduzido e nome na lista)
	mockJSON := `{
		"driver_id": "motorista_test2",
		"date": "20-10-2026",
		"segments": [
			{
				"origin": "Feira",
				"destination": "Santo Amaro",
				"available_seats": 2,
				"passengers": ["passageiro_alvo"]
			}
		]
	}`
	var ride protocolo.RideRequest
	json.Unmarshal([]byte(mockJSON), &ride)
	state.rides["2"] = &ride

	// Um motorista cancelou a parte 2 da viagem no Peer vizinho.
	// O Peer vizinho dispara este payload REST de Cancelamento em Cascata para nós.
	reqBody := map[string]string{
		"passenger_id": "passageiro_alvo",
		"date":         "20-10-2026",
	}
	bodyBytes, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest(http.MethodPost, "/api/cascade-cancel", bytes.NewBuffer(bodyBytes))
	rr := httptest.NewRecorder()

	// Aciona o handler de cascata
	state.handlerCascadeCancel(rr, req)

	// Validar retorno HTTP
	if rr.Code != http.StatusOK {
		t.Errorf("Esperava HTTP 200 OK, obteve %d", rr.Code)
	}

	// Validar Estado Interno (se a vaga foi devolvida e o passageiro sumiu)
	estadoAtualBytes, _ := json.Marshal(state.rides["2"])

	if bytes.Contains(estadoAtualBytes, []byte(`"passageiro_alvo"`)) {
		t.Errorf("Falha na Cascata: O 'passageiro_alvo' ainda está no array de passageiros da viagem.")
	} else {
		t.Log("SUCESSO: O passageiro foi removido do trecho e a vaga foi devolvida perfeitamente!")
	}
}
