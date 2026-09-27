package game

// HasHost reports whether the game has a host.
func (s *MemoryStore) HasHost(id string) bool {
	return hasHost(s, id)
}

// PlayerIsHost reports whether the player hosts the game.
func (s *MemoryStore) PlayerIsHost(id string, playerId string) bool {
	return playerIsHost(s, id, playerId)
}

// SetPlayerAsHost makes the player the game's sole host.
func (s *MemoryStore) SetPlayerAsHost(id string, playerId string) error {
	return setPlayerAsHost(s, id, playerId)
}

// UnsetHost clears the host flag on every player.
func (s *MemoryStore) UnsetHost(id string) error {
	return unsetHost(s, id)
}
