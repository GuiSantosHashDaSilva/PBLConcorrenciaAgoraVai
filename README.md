# PBLConcorrenciaAgoraVai

# 🚗 Vai Junto - Sistema de Caronas Distribuído

Este projeto é um sistema distribuído de caronas (estilo "BlaBlaCar") desenvolvido em **Go (Golang)** utilizando comunicação via **Sockets TCP** e troca de mensagens no formato **JSON**. 

O sistema implementa uma arquitetura Cliente-Servidor robusta, gerenciamento de concorrência com exclusão mútua (`sync.Mutex`), reserva atômica de assentos e roteamento inteligente utilizando teoria dos grafos (Busca em Largura - BFS).

---

## 🛠️ Tecnologias Utilizadas

- **Linguagem:** Go 1.21+
- **Comunicação:** TCP/IP nativo (`net` package)
- **Protocolo:** JSON customizado sobre TCP
- **Infraestrutura:** Docker e Docker Compose

---

## ⚙️ Funcionalidades e Arquitetura

### 🖥️ Servidor (Nó Central)
- **Roteamento em Grafo (BFS):** Conecta trechos de diferentes motoristas para formar rotas complexas caso não exista uma viagem direta.
- **Reserva Atômica (Tudo ou Nada):** Garante que um passageiro só consiga reservar uma rota com múltiplos trechos se **todas** as vagas estiverem disponíveis.
- **Controle de Concorrência (Mutex):** Impede condições de corrida (Race Conditions) ao descontar vagas concorridas por múltiplos usuários no mesmo milissegundo.

### 🚘 Cliente Motorista
- Autenticação de sessão.
- Publicação de viagens com múltiplos trechos (definindo origem, destino, vagas e preço).
- Consulta do histórico de viagens (exibindo os IDs dos passageiros confirmados).
- Cancelamento de caronas publicadas.

### 🧍 Cliente Passageiro
- Autenticação de sessão.
- Busca inteligente de viagens com cálculo automático de rotas compostas e preço total.
- Reserva direta pelo menu de busca.
- Consulta de reservas ativas.
- Cancelamento de trechos reservados (devolvendo a vaga para o sistema).

---

## 📡 Protocolo de Comunicação (Exemplos)

Toda mensagem trafegada via TCP utiliza um padrão universal contendo o `type` da requisição e o `payload` (dados). O servidor devolve um JSON contendo o `status` (success/error).

**1. Exemplo de Login (`LOGIN`)**
*Cliente envia:*
```json
{
  "type": "LOGIN",
  "payload": {
    "login": "motorista_01",
    "senha": "123"
  }
}
```

*Servidor responde:*
```json
    {"status": "success", "message": "Login Efetuado com sucesso"}
```