package models

type Motorista struct {
	HistoricoDeViagens []Viagem
}

func (p Motorista) buscarViagem() ([]Viagem, string) {
	return nil, "Ainda não implementado"
}

func (p Motorista) confirmarReserva() (bool, string) {
	return true, "Ainda não implementado"
}

func (p Motorista) cancelarReserva() (bool, string) {
	return true, "Ainda não implementado"
}

func (p Motorista) consultarViagem() (bool, string) {
	return true, "Ainda não implementado"
}
