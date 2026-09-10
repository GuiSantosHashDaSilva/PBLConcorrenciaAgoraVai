package models

type Pessoa struct {
	Nome  string `json:"nome"`
	Login string `json:"login"`
	Senha string `json:"senha"`
}

func (p Pessoa) autenticar(nome_login, senha_login string) (bool, string) {
	if nome_login == p.Nome && p.Senha == senha_login {
		return true, "Login Efetuado com sucesso"
	}

	return false, "Nome ou Senha incorreta"
}
