package messages

type WSMessage interface {
	JSON() []byte
	JSONString() string
	String() string
}
