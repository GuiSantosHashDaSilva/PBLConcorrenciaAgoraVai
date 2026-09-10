package models

import "time"

type Viagem struct {
	Data	time.Time		
	Hora	int
	Origem	string
	Destino	string
	Trechos	[]Trecho
}