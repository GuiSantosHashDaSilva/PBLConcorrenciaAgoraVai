# 🚗 Vai Junto - Sistema de Caronas Distribuído

Este projeto é um sistema distribuído de caronas desenvolvido em **Go (Golang)** utilizando comunicação via **Sockets TCP** e troca de mensagens no formato **JSON**. 

Desenvolvido como trabalho acadêmico, o sistema implementa uma arquitetura Cliente-Servidor e lida com problemas de sistemas distribuídos, como concorrência e gerenciamento de estado.

---

## 🛠️ Tecnologias Utilizadas

- **Linguagem:** Go 1.21+
- **Comunicação:** TCP/IP nativo
- **Protocolo:** JSON customizado sobre TCP
- **Infraestrutura:** Docker e Docker Compose

---

## ⚙️ Funcionalidades e Arquitetura

- **Roteamento em Grafo (Busca em Largura - BFS):** O servidor conecta trechos de diferentes motoristas para formar rotas. Se o João vai de A para B, e a Maria vai de B para C, o sistema sugere a rota completa de A para C.
- **Reserva Atômica:** Garante que um passageiro só consiga reservar uma rota composta se todas as vagas de todos os trechos estiverem disponíveis simultaneamente.
- **Controle de Concorrência:** Utiliza `sync.Mutex` no servidor para impedir condições de corrida, garantindo que múltiplas reservas no mesmo tempo não resultem em vagas negativas.
- **Painel do Motorista:** Publicação e cancelamento de viagens, além da visualização de histórico com a identificação dos passageiros confirmados.
- **Painel do Passageiro:** Busca inteligente com soma de preços, reserva atômica direta e cancelamento de trechos (devolvendo a vaga).

---

## 📡 Protocolo de Comunicação (Exemplos)

Toda mensagem trafegada via TCP utiliza um padrão universal contendo o `type` da requisição e o `payload` (dados). O servidor sempre devolve um JSON contendo o `status` (success/error) e uma `message`.

**1. Exemplo de Publicação de Carona (`PUBLISH_RIDE`)**
*Cliente envia:*
```json
{
  "type": "PUBLISH_RIDE",
  "payload": {
    "driver_id": "motorista_01",
    "date": "15-10-2026",
    "segments": [
      {
        "origin": "Salvador",
        "destination": "Feira de Santana",
        "available_seats": 4,
        "price": 40.00
      }
    ]
  }
}
```
*Servidor responde:*
```json
{"status": "success", "message": "Carona publicada com sucesso!"}
```

**2. Exemplo de Reserva Atômica (`BOOK_ROUTE`)**
*Cliente envia um array de trechos (que podem pertencer a motoristas distintos). O Servidor processa tudo em bloco usando `sync.Mutex`:*
```json
{
  "type": "BOOK_ROUTE",
  "payload": {
    "passenger_id": "passageiro_01",
    "itinerary": [
      {
        "ride_id": "motorista_01-15-10-2025",
        "origin": "Salvador",
        "destination": "Feira de Santana"
      }
    ]
  }
}
```
*Servidor responde (em caso de sucesso):*
```json
{"status": "success", "message": "Itinerário confirmado!"}
```

---

## Como Executar o Sistema no Terminal

Você pode executar o sistema utilizando Docker ou localmente via Go.

### Opção A: Execução via Docker Compose
*Pré-requisito: Docker e Docker Compose instalados.*

Abra **três terminais diferentes** na raiz do projeto e siga a ordem:

1. **Terminal 1 (Servidor):**
   ```bash
   docker-compose up -d server
   docker-compose logs -f server
   ```
   *(Este terminal ficará aberto exibindo os logs e conexões da rede em tempo real).*

2. **Terminal 2 (Motorista):**
   ```bash
   docker-compose run motorista
   ```

3. **Terminal 3 (Passageiro):**
   ```bash
   docker-compose run passageiro
   ```

*(Para desligar tudo ao final do uso, execute: `docker-compose down`)*

### Opção B: Execução Local Sem Docker
*Pré-requisito: Go 1.21+ instalado na máquina.*

Abra **três terminais diferentes** na raiz do projeto:

1. **Terminal 1:** `go run server.go`
2. **Terminal 2:** `go run cliente_motorista.go`
3. **Terminal 3:** `go run cliente_passageiro.go`

---

## Como Usar o Sistema

Para testar todas as funcionalidades do sistema, siga este roteiro de uso com os painéis abertos:

### Passo 1: Publicando uma Carona (Painel do Motorista)
1. No terminal do motorista, escolha a opção **1** para fazer o Login (ex: Login: `joao`, Senha: `123`).
2. Escolha a opção **2 (Publicar Carona)**.
3. Preencha os dados.


### Passo 2: Buscando e Reservando (Painel do Passageiro)
1. No terminal do passageiro, escolha a opção **1** para fazer o Login (ex: Login: `carlos`, Senha: `123`).
2. Escolha a opção **2 (Buscar e Reservar Viagens)**.
3. Digite a Origem (`Salvador`) e Destino (`Feira`).
4. O sistema (usando BFS) retornará as opções disponíveis com o preço total calculado.
5. Digite o **número da opção** desejada para efetuar a reserva. O servidor confirmará a reserva e descontará a vaga.

### Passo 3: Consultando o Histórico
- **No Motorista:** Escolha a opção **3 (Consultar Minhas Caronas)**. Você verá a carona publicada, as vagas restantes e o ID do passageiro `carlos` confirmado.
- **No Passageiro:** Escolha a opção **3 (Consultar Minhas Reservas)** para visualizar os detalhes do trecho adquirido.

### Passo 4: Cancelamento (Opcional)
- O **Passageiro** pode usar a opção **4** para cancelar a reserva, o que devolve a vaga para o motorista automaticamente.
- O **Motorista** pode usar a opção **4** para cancelar a viagem inteira.

---

## Teste de Concorrência e Proteção Mutex

O projeto acompanha um script de estresse para provar o funcionamento da exclusão mútua (`sync.Mutex`). O teste publica uma carona com apenas 5 vagas e lança **50 passageiros concorrentes** tentando reservar exatamente no mesmo instante.

Para executar e validar:
1. Certifique-se de que o Servidor está rodando (Terminal 1).
2. Abra um novo terminal e execute:
   ```bash
   go run cmd/teste_concorrencia.go
   ```
**Resultado Esperado:** O sistema garantirá que apenas 5 requisições tenham sucesso e 45 falhem, exibindo a mensagem: `passou: O Mutex funcionou, nenhuma vaga foi vendida a mais e não houve corrida`. Nenhuma vaga extra será criada ou vendida.