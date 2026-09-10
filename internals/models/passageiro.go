package models

type Passageiro struct {
	ViagensFeitas []Viagem
}

func (p Passageiro) buscarViagem() ([]Viagem, string) {
	return nil, "Ainda não implementado"
}

func (p Passageiro) confirmarReserva() (bool, string) {
	return true, "Ainda não implementado"
}

func (p Passageiro) cancelarReserva() (bool, string) {
	return true, "Ainda não implementado"
}
