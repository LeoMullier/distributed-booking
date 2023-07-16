package main

import (
	"log"
	"net/url"

	"github.com/gorilla/websocket"
)

func main() {

	u := url.URL{Scheme: "ws", Host: "localhost:4443", Path: "/ws"}
	log.Printf("connecting to %s", u.String())

	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Fatal("dial:", err)
	}
	// defer c.Close()

	// Lit les messages du serveur WebSocket
	go func() {
		for {
			_, message, err := c.ReadMessage()
			if err != nil {
				log.Println("read:", err)
				return
			}
			log.Printf("received: %s", message)
		}
	}()

	message := []byte(",=type=demandeEtatPlaces")
	err = c.WriteMessage(websocket.TextMessage, message)
	if err != nil {
		log.Println("write:", err)
		return
	}

	message = []byte(",=type=demandeReservation,=places=[1;2;3]")
	err = c.WriteMessage(websocket.TextMessage, message)
	if err != nil {
		log.Println("write:", err)
		return
	}

	message = []byte(",=type=demandeEtatPlaces")
	err = c.WriteMessage(websocket.TextMessage, message)
	if err != nil {
		log.Println("write:", err)
		return
	}

	err = c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	if err != nil {
		log.Println("write close:", err)
		return
	}
}
