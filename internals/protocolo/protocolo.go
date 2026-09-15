package protocolo

import "encoding/json"

// Message representa a estrutura intermediária para comunicação TCP
type Message struct {
	Type    Servico         `json:"type"`    // Ex: "PUBLISH_RIDE", "SEARCH_ROUTE", "BOOK_ROUTE"
	Payload json.RawMessage `json:"payload"` // Dados específicos da requisição ou resposta
}

type RideRequest struct {
	DriverID string    `json:"driver_id"`
	Date     string    `json:"date"`
	Segments []Segment `json:"segments"`
}

type Segment struct {
	Origin         string   `json:"origin"`
	Destination    string   `json:"destination"`
	AvailableSeats int      `json:"available_seats"`
	Price          float64  `json:"price"`
	Passengers     []string `json:"passengers"`
}
