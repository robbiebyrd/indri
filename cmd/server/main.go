package main

import (
	"github.com/olahol/melody"
	"indri/internal/entrypoints"
	"indri/internal/handlers/message"
	gameService "indri/internal/services/game"
)

func main() {
	m := melody.New()

	gs := gameService.NewService()
	gameId := "123"
	gs.New(&gameId)
	return

	//ur := user.NewService()
	//
	//email := "me@robbiebyrd.com"
	//password := "password"
	//score := 0
	//
	//a := models.User{
	//	Email:    &email,
	//	Name:     "Robbie Byrd",
	//	Password: &password,
	//	Score:    &score,
	//}
	//
	//b, err := ur.New(&a)
	//if err != nil {
	//	fmt.Println(err)
	//}
	//
	//fmt.Println(b)

	m.HandleConnect(func(s *melody.Session) {
		entrypoints.HandleConnect(s, m)
	})

	m.HandleDisconnect(func(s *melody.Session) {
		entrypoints.HandleDisconnect(s, m)
	})

	m.HandleMessage(func(s *melody.Session, msg []byte) {
		message.HandleMessage(s, m, msg)
	})

	entrypoints.Serve(m)
}
