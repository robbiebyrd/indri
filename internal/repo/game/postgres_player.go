package game

// AssignSlot claims the first empty slot in teamId for userId.
func (s *PostgresStore) AssignSlot(id string, teamId string, userId string, displayName string) (string, error) {
	return assignSlot(s, id, teamId, userId, displayName)
}

// RemovePlayer empties userId's slot, keeping it in place for reassignment.
func (s *PostgresStore) RemovePlayer(id string, slotId string, userId string) error {
	return removePlayer(s, id, slotId, userId)
}

// ConnectPlayer marks userId, in slotId, as connected.
func (s *PostgresStore) ConnectPlayer(id string, slotId string, userId string) error {
	return setConnected(s, id, slotId, userId, true)
}

// DisconnectPlayer marks userId, in slotId, as disconnected.
func (s *PostgresStore) DisconnectPlayer(id string, slotId string, userId string) error {
	return setConnected(s, id, slotId, userId, false)
}
