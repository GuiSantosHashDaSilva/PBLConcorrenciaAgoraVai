package protocolo

type Servico string

const (
	SERVICO_LOGIN        Servico = "servico_login"
	BUSCAR_VIAGEM        Servico = "buscar_viagem"
	CONFIRMAR_VIAGEM     Servico = "confirmar_viagem"
	CONSULTAR_VIAGENS    Servico = "consultar_viagens"
	VISUALIZAR_HISTORICO Servico = "visualizar_historico" 
	PUBLISH_RIDE Servico = "PUBLISH_RIDE"
	BOOK_ROUTE   Servico = "BOOK_ROUTE"
)
