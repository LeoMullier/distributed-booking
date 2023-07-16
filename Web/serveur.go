/*
** Déclaration du package
 */
package main

/*
** Importation des ressources
 */
import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	"github.com/gorilla/websocket"
)

/*
** Initialisation des variables globales
 */
var appSocket *websocket.Conn // communication entre le serveur web et l'application de base
var cnxClient *websocket.Conn // communication de base entre le serveur web et le client html

/*
** Traitement des messages échangés avec le client
 */
func do_websocket(w http.ResponseWriter, r *http.Request) {
	// Préparation de la communication
	var upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	var err error
	cnxClient, err = upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Println("upgrade:", err)
		return
	}

	// Boucle infini de réception des messages issus du client
	for {
		// Réception
		_, message, err := cnxClient.ReadMessage()
		if err != nil {
			log.Println("(!) Erreur lors de la réception du message reçu du client |", err)
			return
		}
		//log.Println("réception de client web  : " + string(message))
		//log.Println("ecriture vers app socket : " + string(message))
		err = appSocket.WriteMessage(websocket.TextMessage, message)
		if err != nil {
			log.Println("(!) Erreur lors de l'écriture du message reçu du client |", err)
			return
		}

		// Confirmation de réception et avis de traitement
		confMsg := []byte("Traitement en cours")
		//log.Println("ecriture vers client web : " + string(confMsg))
		err = cnxClient.WriteMessage(websocket.TextMessage, confMsg)
		if err != nil {
			log.Println("(!) Erreur lors de l'écriture de l'avis de traitement en cours sur au message reçu du client |", err)
			return
		}

	}
}

/*
** Démarrage et configuration du serveur web
 */
func do_webserver(w http.ResponseWriter, r *http.Request) {
	//fmt.Fprintf(w, "Bonjour depuis le serveur web en Go !")
	exePath, err := os.Executable()
	if err != nil {
		log.Fatal("(!) Erreur lors du démarrage du serveur web |", err)
	}

	// On récupère le chemin absolu du dossier contenant le programme et du fichier client.html du dossier courant
	exeDir := filepath.Dir(exePath)
	filePath := filepath.Join(exeDir, "client.html")
	file, err := os.Open(filePath)
	if err != nil {
		log.Fatal("(!) Erreur lors de la récupération du fichier client.html |", err)
	}
	defer file.Close()

	// On lit le contenu du du fichier puis on l'affiche
	content, err := ioutil.ReadAll(file)
	if err != nil {
		log.Fatal("(!) Erreur lors du parsing du fichier client.html |", err)
	}
	fmt.Fprint(w, string(content))
}

/*
** Traitements des messages échangés avec l'application de base
 */
func echangeApp(app_host *string, app_port *string) {
	// Préparation de la communication
	u := url.URL{Scheme: "ws", Host: *app_host + ":" + *app_port, Path: "/ws"}
	log.Println("Connextion en cours vers", u.String())
	var err error
	appSocket, _, err = websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Fatal("(!) Erreur lors de la communication avec AppSocket |", err)
	}
	defer appSocket.Close()

	// Boucle infini de réception des messages issus de l'application de base
	for {
		_, message, err := appSocket.ReadMessage()

		if err != nil {
			log.Println("(!) Erreur lors de la réception du message reçu de l'application de base |", err)
		}
		//fmt.Printf("SERVEUR : Message reçu : %s\n", message)

		// Et on transmet au client au client le message reçu
		cnxClient.WriteMessage(websocket.TextMessage, message)
	}

}

/*
** Fonction main() de serveur.go
 */
func main() {
	// Suppression des dates dans les logs
	log.SetFlags(log.Flags() &^ (log.Ldate | log.Ltime))

	// Gestion des arguments d'exécution du programme
	var port = flag.String("port", "4444", "n° de port")
	var addr = flag.String("addr", "localhost", "nom/adresse machine")
	var app_host = flag.String("appHost", "localhost", "hôte de l'app associée")
	var app_port = flag.String("appPort", "4410", "n° de port de l'app associée")
	flag.Parse()

	// Connexion à l'APP socket de l'application de base
	go echangeApp(app_host, app_port)

	// Lancement du serveur web pour la websocket
	u := url.URL{Scheme: "ws", Host: *addr + ":" + *port, Path: "/ws"}
	log.Println("Connexion en cours vers", u.String())

	// Lancement du serveur pour servir les clients web
	log.Println("Démarrage du serveur web", u.String())
	http.HandleFunc("/", do_webserver)

	log.Println("Démarrage du serveur web", u.String())
	http.HandleFunc("/ws", do_websocket)

	log.Println("Démarrage du serveur web", u.String())
	http.ListenAndServe(*addr+":"+*port, nil)
}
