package messages

type AuthMessage struct {
	Token *string `json:"token,omitempty"`
}

func AuthSuccess(int, ...string) map[string]interface{} {
	return nil
}

func AuthError(int, ...string) map[string]interface{} {
	return nil
}
