package messages

type WSMessage interface {
	New() string
}

type AuthMessage struct {
	Token *string `json:"token,omitempty"`

}
