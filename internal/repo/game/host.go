package game

// HasHost checks to see if the game has a host already.
func (s *Repo) HasHost(id string) bool {
	g, err := s.Get(id)
	if err != nil {
		return false
	}

	for _, p := range g.Players {
		if p.Host {
			return true
		}
	}

	return false
}

// PlayerIsHost checks to see if a player is currently the host of the game.
func (s *Repo) PlayerIsHost(id string, playerId string) bool {
	g, err := s.Get(id)
	if err != nil {
		return false
	}

	return g.Players[playerId].Host
}

func (s *Repo) UnsetHost(id string) error {
	g, err := s.Get(id)
	if err != nil {
		return err
	}

	for pId := range g.Players {
		err = s.UpdateField(id, "players."+pId+".host", true)
		if err != nil {
			return err
		}
	}

	return nil
}

// SetPlayerAsHost checks to see if a player is currently the host of the game.
func (s *Repo) SetPlayerAsHost(id string, playerId string) error {
	if !s.HasHost(id) {
		err := s.UnsetHost(id)
		if err != nil {
			return err
		}
	}

	err := s.UpdateField(id, "players."+playerId+".host", true)
	if err != nil {
		return err
	}

	return nil
}
