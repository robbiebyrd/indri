package game

// AssignSlot claims the first empty slot in teamId for userId.
func (s *PostgresStore) AssignSlot(id string, teamId string, userId string, displayName string) (string, error) {
	return assignSlot(s, id, teamId, userId, displayName)
}

// RemovePlayer empties a slot, keeping it in place for reassignment.
func (s *PostgresStore) RemovePlayer(id string, slotId string) error {
	return removePlayer(s, id, slotId)
}

// ConnectPlayer marks a slot's player as connected.
func (s *PostgresStore) ConnectPlayer(id string, slotId string) error {
	return setConnected(s, id, slotId, true)
}

// DisconnectPlayer marks a slot's player as disconnected.
func (s *PostgresStore) DisconnectPlayer(id string, slotId string) error {
	return setConnected(s, id, slotId, false)
}
