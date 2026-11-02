package protocolo

type Servico string

const (
	SERVICO_LOGIN        Servico = "servico_login"
	BUSCAR_VIAGEM        Servico = "buscar_viagem"
	CONFIRMAR_VIAGEM     Servico = "confirmar_viagem"
	CONSULTAR_VIAGENS    Servico = "consultar_viagens"
	VISUALIZAR_HISTORICO Servico = "visualizar_historico"
	PEER_FETCH_RIDES     Servico = "PEER_FETCH_RIDES"
	PEER_BOOK_SEGMENT    Servico = "PEER_BOOK_SEGMENT" // Usa para mandar um peer reservar o trecho dele
	PEER_UNDO_SEGMENT    Servico = "PEER_UNDO_SEGMENT" // Usa para cancelar (rollback) se algo der errado
	PUBLISH_RIDE         Servico = "PUBLISH_RIDE"
	BOOK_ROUTE           Servico = "BOOK_ROUTE"
)
