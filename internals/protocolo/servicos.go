package protocolo

type Servico string

const (
	SERVICO_LOGIN        Servico = "servico_login"
	BUSCAR_VIAGEM        Servico = "buscar_viagem"
	CONFIRMAR_VIAGEM     Servico = "confirmar_viagem"
	CONSULTAR_VIAGENS    Servico = "consultar_viagens"
	VISUALIZAR_HISTORICO Servico = "visualziar_historico"
)
