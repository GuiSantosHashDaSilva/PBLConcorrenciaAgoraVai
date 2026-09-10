package main

import (
	"bufio"
	"fmt"
	"net"
)

func main() {
	// Conexão ao servidor central
	conn, err := net.Dial("tcp", "localhost:8080")
	if err != nil {
		fmt.Println("Erro ao conectar ao servidor:", err)
		return
	}
	defer conn.Close()


	

	response, _ := bufio.NewReader(conn).ReadString('\n')
	fmt.Println("Resposta do servidor:", response)
}
