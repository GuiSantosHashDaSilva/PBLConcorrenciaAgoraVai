# 🚗 Vai Junto - Sistema Distribuído de Caronas (Consórcio)

Este projeto implementa um sistema de caronas verdadeiramente distribuído desenvolvido em **Go (Golang)**. Criado para a disciplina de Concorrência e Conectividade (TEC502 - Problema 2), o sistema simula um consórcio de empresas operando em rede. 

A arquitetura adota um modelo híbrido: comunicação via **Sockets TCP (JSON)** entre clientes e servidores, e uma **API REST (HTTP/JSON)** para a comunicação distribuída entre os servidores parceiros (peers).

---

## 🛠️ Tecnologias Utilizadas

- **Linguagem:** Go 1.22+
- **Comunicação Cliente-Servidor:** TCP/IP nativo com payload JSON
- **Comunicação Server-to-Server:** API REST (HTTP GET/POST)
- **Sincronização:** `sync.Mutex`, Goroutines, Timestamps
- **Testes:** `httptest` (Mocking de rede)
- **Infraestrutura/Emulação:** Docker e Docker Compose

---

## ⚙️ Arquitetura e Soluções Distribuídas (Barema)

O sistema foi arquitetado para lidar com os desafios clássicos de sistemas distribuídos:

- **Busca Distribuída (BFS):** Quando um cliente faz uma busca local, o servidor realiza requisições REST `GET /api/rides` para os servidores vizinhos, mescla os grafos em memória e executa uma Busca em Largura (BFS) para sugerir rotas intermunicipais operadas por empresas diferentes.
- **Reserva Atômica e Two-Phase Commit (2PC):** A reserva de um roteiro composto exige a alocação de vagas em múltiplos servidores. O servidor de origem atua como Coordenador, garantindo que o passageiro reserve tudo ou sofra *rollback* total (`POST /api/book/undo`) caso falte vaga num dos nós da rede.
- **Prevenção de Deadlock Distribuído:** Para evitar impasses onde Servidor A espera o B e o B espera o A (espera circular), o coordenador aplica ordenação lexicográfica aos recursos (IDs de trechos) antes de iniciar as requisições de travamento.
- **Exclusão Mútua e Precedência:** Condições de corrida na memória são mitigadas por `sync.Mutex`. Em caso de requisições simultâneas para o mesmo trecho, o coordenador envia um *Timestamp* (UnixNano) para garantir a precedência temporal.
- **Cancelamento em Cascata:** Se um motorista cancelar uma viagem no Servidor A, o sistema expulsa os passageiros afetados e dispara requisições REST `POST /api/cascade-cancel` limpando os trechos dependentes daquele passageiro no Servidor B.
- **Tolerância a Falhas:** As requisições entre servidores possuem *timeouts* restritos (3s). Se o Servidor B cair, o Servidor A não congela; ele aborta a transação e mantém o sistema operante. A persistência de dados JSON permite a recuperação do estado do Servidor B assim que ele for reiniciado.

---

## 🚀 Como Executar e Testar (Topologia do Laboratório)

O sistema está preparado para rodar em uma topologia física de 3 computadores, utilizando um único arquivo `docker-compose.yml`.

**Pré-requisitos:** Descubra os IPs locais (ex: `192.168.1.51` e `192.168.1.52`) das máquinas 1 e 2. Edite o arquivo `docker-compose.yml` preenchendo a variável `PEERS` com esses IPs.

### 1. Inicializando os Servidores (Consórcio Docker)

**No Computador 1 (Servidor Empresa A):**
Abra o terminal na raiz do projeto e inicie apenas o contêiner A:
```bash
docker compose up empresa_a --build
```

**No Computador 2 (Servidor Empresa B):**
Abra o terminal na raiz do projeto e inicie apenas o contêiner B:
```bash
docker compose up empresa_b --build
```

### 2. Conectando os Clientes

**No Computador 3 (Estação de Clientes):**
Não é necessário Docker. Abra múltiplos terminais e execute os clientes Go nativamente. Quando o programa solicitar o IP, aponte para as máquinas 1 e 2.

- **Terminal 1 (Motorista):** `go run cmd/motorista/main.go` -> Conecta em `192.168.1.51:8811`
- **Terminal 2 (Motorista):** `go run cmd/motorista/main.go` -> Conecta em `192.168.1.52:8812`
- **Terminal 3 (Passageiro):** `go run cmd/passageiro/main.go` -> Conecta em `192.168.1.51:8811`

### 3. Roteiro de Uso

1. **Motorista 1** publica uma carona de *Feira* para *Santo Amaro*.
2. **Motorista 2** publica de *Santo Amaro* para *Salvador*.
3. **Passageiro 1** busca uma viagem de *Feira* para *Salvador*. O sistema distribuído unirá os trechos e calculará o preço total.
4. O Passageiro seleciona a rota composta, disparando o processo atômico de reserva através da rede (REST).

---

## 🧪 Bateria de Testes Distribuídos

O projeto acompanha arquivos de testes robustos (`server_test.go`) para atestar as regras de concorrência e consistência através de *mocks* HTTP REST.

Para validar, navegue até a pasta do servidor e execute:
```bash
cd cmd/server
go test -v
```

Os testes validarão dois cenários cruciais:
1. **`TestConcurrencyBookSegment`:** Simula 3 requisições REST simultâneas disputando uma única vaga, atestando a robustez da `sync.Mutex` ao garantir 1 sucesso e 2 conflitos corretos.
2. **`TestCascadeCancelREST`:** Injeta um payload de *Cascade Cancel* simulando um vizinho da rede. Atesta se o servidor processa o aviso, expulsa o passageiro dependente, devolve a vaga à rede e salva os novos dados consistentes no disco.