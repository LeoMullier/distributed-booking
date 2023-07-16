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
	"log"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

/*
** Initialisation des variables globales au site
 */
var siteId int = 0                   // identification du site
var mutex = &sync.Mutex{}            // exclusion mutuelle
var nbPlacesTotal int                // nombre de places dans la salle
var places []int = make([]int, 0)    // tableau pour représenter l'ensemble des places
var cnxWithWebServer *websocket.Conn // websocket pour communiquer avec le serveur web
var couleur [3]int = [3]int{0, 0, 0} // couleur des sites pour la sauvegarde
var donneesSC DonneesSC              // données protégées par la section critique

type DonneesSC struct { // les données sauvegardées qui seront traitées en section critique
	TypeDemande string
	ListePlaces []int
}

/*
** retourne le titre formaté
 */
func titleLog(title string) (f string) {
	format := "SITE:" + strconv.Itoa(siteId) + " -- %20s --"
	f = fmt.Sprintf(format, strings.ToUpper(title))

	// Retour de la fonction
	return f
}

/*
** Mise en forme des logs avec les préfixes APP, nom de la fonction
 */
func runningFunc() (f string) {
	pc, _, _, _ := runtime.Caller(1)
	//pid := os.Getpid()
	funcName := runtime.FuncForPC(pc).Name()
	format := "(APP-%-6d)(%-20s) -- "
	f = fmt.Sprintf(format, siteId, funcName[5:])

	// Retour de la fonction
	return f
}

/*
** Traitements suite à la réception d'un messages du serveur web via la websocket
 */
func do_websocket(w http.ResponseWriter, r *http.Request) {
	// Préparation de la communication
	var upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	var err error
	cnxWithWebServer, err = upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(runningFunc(), titleLog("WARNING"), "(!) Erreur lors de l'upgrade de la websocket app <-> serveur web |", err)
		return
	}

	// Boucle infinie de réception des messages sur la websocket
	for {
		_, message, err := cnxWithWebServer.ReadMessage()
		if err != nil {
			log.Println(runningFunc(), titleLog("WARNING"), "(!) Erreur lors de la réception du message reçu du serveur web |", err)
		}
		log.Print(runningFunc(), " ", titleLog("RECEPTION WEBSOCKET"), " ", string(message))

		// Dans cette partie, il faut décoder le message pour savoir ce que le serveur web souhaite parmis
		// demandeReservation
		// demandeLiberation
		// etatPlaces

		if len(message) != 0 {
			// Découpage du string du message pour différencier les différents arguments
			strMessage := strings.ReplaceAll(string(message), "\n", "")     // on supprime les \n
			tab_allkeyval := strings.Split(strMessage[1:], strMessage[0:1]) // on distingue les couples clé+valeur
			desc_message := make(map[string]string)                         // on génère le tableau avec chaque argument du message
			for _, keyval := range tab_allkeyval {                          // on alimente le tableau pour tous les arguments du message
				tab_keyval := strings.Split(keyval[1:], keyval[0:1])
				desc_message[tab_keyval[0]] = tab_keyval[1]
			}

			// Dans le cas d'un transfert vers un module de contrôle, on ajoute les champs nécessaires à message
			msgToCtl := make(map[string]string)
			msgToCtl["sender"] = strconv.Itoa(0) // On écrit au module de contrôle avec sender=0
			msgToCtl["listePlaces"] = desc_message["listePlaces"]
			msgToCtl["type"] = desc_message["type"]
			msgToCtl["toApp"] = strconv.Itoa(0) // pour dire que le message n'est pas à destination d'une app mais d'un dtl

			// Sélection des traitements en fonction du type de message envoyé par le serveur web
			switch desc_message["type"] {
			case "demandeReservation":
				// Requête de réservation d'une liste de places
				tab, err := creation_tableau(desc_message["listePlaces"])
				if !placesLibres(tab) || err {
					// Si il y a un conflit entre les places demandées pour la réservation et l'état actuel des places
					log.Println(runningFunc(), titleLog("Demande Reservation"), "Toutes les places demandees ne sont pas libres !")
					sendToWebServer("~=demandeReservationKO")
				} else {
					// Sinon, on sait que les places sont libres, on sauvegarde les valeurs et on demande à entrer en section critique
					donneesSC.TypeDemande = desc_message["type"]
					donneesSC.ListePlaces = tab
					log.Println(runningFunc(), titleLog("Demande Reservation"), "Toutes les places demandees sont libres")
					reception_reserver_places(msgToCtl)
				}

			case "demandeLiberation":
				// Requête de libération d'une liste de places
				tab, err := creation_tableau(desc_message["listePlaces"])
				if !placesOccupees(tab) || err {
					// Si il y a un conflit entre les places demandées pour la libération et l'état actuel des places
					log.Println(runningFunc(), titleLog("Demande Libération"), "Toutes les places demandees ne sont pas occupées !")
					sendToWebServer("~=demandeLiberationKO")
				} else {
					// Sinon, on sait que les places sont occupées, on sauvegarde les valeurs et on demande à entrer en section critique
					donneesSC.TypeDemande = desc_message["type"]
					donneesSC.ListePlaces = tab
					log.Println(runningFunc(), titleLog("Demande Libération"), "Toutes les places demandees sont occupées, Libération des places")
					reception_liberer_places(msgToCtl)
				}

			case "demandeEtatPlaces":
				// Requête de récupération de l'atat de l'ensemble des places (libre ou occupé)
				log.Println(runningFunc(), titleLog("Demande etat places"), "Traitement...")

				// On écrit sur la websocket le message formaté
				sendToWebServer(formateMsgEtatPlaces())

			case "demandeSauvegarde":
				// Requête de lancement d'une sauvegarde (le site actuel sera considéré comme l'initiateur)
				receptionDemandeSauvegarde(msgToCtl)

			default:
				// Requête erronée ou indésirable
				log.Println(runningFunc(), titleLog("ERROR"), "Erreur de message")
			}
		}
	}
}

/*
** Traitements pour démarrer une sauvegarde du système
 */
func receptionDemandeSauvegarde(msg map[string]string) {
	log.Println(runningFunc(), titleLog("Demande de sauvegarde"), "~=sender=%s~=type=%s~=etatPlaces=%s~=toApp=%s\n", msg["sender"], "debutInstantane", tableau_entier_vers_string(places), msg["toApp"])

	// Envoi du message de demande de sauvegarde au module de contrôle du site local
	fmt.Printf("~=sender=%s~=type=%s~=etatPlaces=%s~=toApp=%s\n", msg["sender"], "debutInstantane", tableau_entier_vers_string(places), msg["toApp"])
}

/*
** Envoi d'un message vers le serveur Web grâce à la websocket
 */
func sendToWebServer(msg string) {
	log.Println(runningFunc(), titleLog("Envoi vers webServer"), msg)
	err := cnxWithWebServer.WriteMessage(websocket.TextMessage, []byte(msg))
	if err != nil {
		log.Println(runningFunc(), "sendToWebServer write:", err)
	}
}

/*
** Traitements pour réserver une liste de places
 */
func reception_reserver_places(msg map[string]string) {
	// Envoi du message de demande de section critique au module de contrôle du site local pour pouvoir modifier les données
	fmt.Printf("~=sender=%s~=hlg=%d~=type=%s~=listePlaces=%s~=toApp=%s\n", msg["sender"], msg["hlg"], "demandeSC", msg["listePlaces"], msg["toApp"])
}

/*
** Traitements pour libérer une liste de places
 */
func reception_liberer_places(msg map[string]string) {
	//Envoi du message de demande de section critique au module de contrôle du site local pour pouvoir modifier les données
	fmt.Printf("~=sender=%s~=hlg=%d~=type=%s~=listePlaces=%s~=toApp=%s\n", msg["sender"], msg["hlg"], "demandeSC", msg["listePlaces"], msg["toApp"])
}

/*
** Initialisation de la communication avec le serveur web
 */
func do_webserver(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Hell0 w0rld de l'application de base vers le serveur web en Go !")
}

/*
** Initialisation du tableau des places pour l'ensemble de l'exécution du système
 */
func initPlaces() {
	log.Println(runningFunc(), titleLog("init des places"))
	for i := 0; i < nbPlacesTotal; i++ {
		places = append(places, 0)
	}
}

/*
** Affichage du tableau des places dans la console
 */
func afficherPlaces() {
	buff := ""
	for i := 0; i < len(places); i++ {
		buff += strconv.Itoa(places[i]) + "" // 0 pour libre et 1 pour réservée
	}
	log.Println(runningFunc(), titleLog("Etat des places"), buff)
}

/*
** Vérification de la possibilité de réserver les places demandées
 */
func placesLibres(tab []int) bool {
	for i := 0; i < len(tab); i++ {
		if tab[i] < 0 || tab[i] > len(places)-1 || places[tab[i]] == 1 {
			return false // au moins une place est déjà occupée ou valeur erronée
		}
	}

	// Retour validé de la fonction
	return true
}

/*
** Vérification de la possibilité de libérer les places demandées
 */
func placesOccupees(tab []int) bool {
	for i := 0; i < len(tab); i++ {
		if tab[i] < 0 || tab[i] > len(places)-1 || places[tab[i]] == 0 {
			return false // au moins une place est déjà libérée ou valeur erronée
		}
	}

	// Retour validé de la fonction
	return true
}

/*
** Traitements suite à la réception d'un messages du module de contrôle via l'entrée standard STDIN
 */
func reception(c chan<- int) {
	var message_recu string

	// Boucle infinie de réception des messages sur l'entrée standard
	for {
		fmt.Scanln(&message_recu)
		if len(message_recu) != 0 {
			// Découpage du string du message pour différencier les différents arguments
			tab_allkeyval := strings.Split(message_recu[1:], message_recu[0:1]) // on distingue les couples clé+valeur
			desc_message := make(map[string]string)                             // on génère le tableau avec chaque argument du message
			for _, keyval := range tab_allkeyval {                              // on alimente le tableau pour tous les arguments du message
				tab_keyval := strings.Split(keyval[1:], keyval[0:1])
				desc_message[tab_keyval[0]] = tab_keyval[1]
			}

			// Récupération de l'émetteur
			emetteur, _ := strconv.Atoi(desc_message["sender"])

			// Sélection des traitements en fonction de toApp
			switch desc_message["toApp"] {
			case "1":
				// si =1, le message est adressé à cette application donc on le traite
				log.Println(runningFunc(), titleLog("RECEPTION FIFO"), message_recu)

				// L'émetteur du message est il le module de contrôle du site local ?
				if emetteur == siteId {
					// Sélection des traitements en fonction du type de message envoyé par le module de contrôle
					switch desc_message["type"] {
					case "debutSC":
						// Je suis content, je suis autorisé à modifier la valeur des places réservées :)
						log.Println(runningFunc(), titleLog("DEBUT SECTION CRITIQUE"), donneesSC)

						// Sélection des traitements en fonction du type de requête
						switch donneesSC.TypeDemande {
						case "demandeReservation":
							// Requête de réservation d'une liste de places
							reserver_places(donneesSC.ListePlaces)

						case "demandeLiberation":
							// Requête de libération d'une liste de places
							liberer_places(donneesSC.ListePlaces)

						default:
							// Requête erronée ou indésirable
							log.Println(runningFunc(), titleLog("ERROR"), "debutSC reçu mais donneesS.TypeDemande non compris:", donneesSC.TypeDemande)
						}

					case "majPlacesReservees":
						// Des places ont été mises à jour (réservation) par un autre application de base, il faut actualiser sa liste des places du site local
						tab, err := creation_tableau(desc_message["listePlaces"])

						if !err {
							majPlacesReservees(tab)
						}

					case "majPlacesLiberees":
						// Des places ont été mises à jour (libération) par un autre application de base, il faut actualiser sa liste des places du site local
						tab, err := creation_tableau(desc_message["listePlaces"])
						if !err {
							majPlacesLiberees(tab)
						}

					case "confirmSauvegarde":
						sendToWebServer("Sauvegarde réalisée avec succès !")

					default:
						// Requête erronée ou indésirable
						log.Println(runningFunc(), titleLog("ERROR"), "Message recu mais non compris :", desc_message["type"])
					}

				} else {
					// Erreur lors de la transmission du message
					log.Println(runningFunc(), titleLog("Transmission"), "Erreur de transmission")
				}

			default:
				// si =0 ou autre, le message n'est adressé à cette application donc on l'ignore
				//log.Println(runningFunc(), titleLog("Ignorer"), "Message n'est pas pour une app, je l'ignore")
			}
		}
	}
}

/*
** Traitements pour formater le message etatPlaces pour le transmettre au serveur web
 */
func formateMsgEtatPlaces() string {
	// on envoie au serveur un message contenant la liste des places
	// ~=etatPlaces=000011110000....
	msg := "~=type=etatPlaces~=etatPlaces="
	for place := range places {
		msg += strconv.Itoa(places[place])
	}

	// Retour de la fonction
	return msg
}

/*
** Traitements pour formater le message listePlaces pour le transmettre au module de contrôle
 */
func formateListeMajPlaces(listeDePlaces []int) string {
	// on envoie un message contenant la liste places des mises à jour (réservation)
	// [p,p,p,p,...]
	msg := "["
	for _, place := range listeDePlaces {
		msg += strconv.Itoa(place) + ","
	}
	msg = msg[0 : len(msg)-1]
	msg += "]"

	// Retour de la fonction
	return msg

}

/*
** Traitement de mise à jours des places (réservation)
 */
func majPlacesReservees(listePlaces []int) {
	// Des places ont été mises à jour (réservées) par un autre app, je mets à jour ma copie locale
	monlock()
	log.Println(runningFunc(), titleLog("Maj places réservées"), listePlaces)
	for _, p := range listePlaces {
		places[p] = 1
	}
	monlocklear()
	sendToWebServer(formateMsgEtatPlaces())
}

/*
** Traitement de mise à jours des places (libération)
 */
func majPlacesLiberees(listePlaces []int) {
	// Des places ont été mises à jour (libérées) par un autre app, je mets à jour ma copie locale
	monlock()
	log.Println(runningFunc(), titleLog("Maj places libérées"), listePlaces)
	for _, p := range listePlaces {
		places[p] = 0
	}
	monlocklear()
	sendToWebServer(formateMsgEtatPlaces())
}

/*
** Traitement de réservation d'une liste de places
 */
func reserver_places(listePlaces []int) {
	log.Println(runningFunc(), titleLog("Reserver les places"), listePlaces)
	monlock()
	for _, p := range listePlaces {
		places[p] = 1
	}
	monlocklear()

	// Renvoyer un message vers le serveur web et lui indiquer que la réservation de places est faite
	sendToWebServer(fmt.Sprintf("~=type=%s~=sender=%d\n", "reservationOK", siteId))

	// Avant de sortir de la section critique, on envoit au module de contrôle la confirmation de réservation
	fmt.Printf("~=sender=%d~=type=%s~=toApp=0~=receiver=%d~=listePlaces=%s\n",
		0,
		"majPlacesReservees",
		siteId,
		formateListeMajPlaces(listePlaces))

	// On formate le message de fin de section critique et on l'écrit sur la sortie standard STDOUT
	log.Println(runningFunc(), titleLog("Envoi Fin SC"), "J'envoie un message finSC à contrôle ")
	fmt.Printf("~=sender=%d~=type=%s~=toApp=0~=receiver=%d,~=etatPlaces=%s\n", 0, "finSC", siteId, tableau_entier_vers_string(places))
}

/*
** Traitement de libération d'une liste de places
 */
func liberer_places(listePlaces []int) {
	log.Println(runningFunc(), titleLog("Libérer les places"), listePlaces)
	monlock()
	for _, p := range listePlaces {
		places[p] = 0
	}
	monlocklear()

	// Renvoyer un message vers le serveur web et lui indiquer que la libération de places est faite
	sendToWebServer(fmt.Sprintf("~=type=%s~=sender=%d\n", "liberationOK", siteId))

	// Avant de sortir de section critique, on envoit au module de contrôle la confirmation de libération
	log.Println(runningFunc(), titleLog("Envoi maj"), "J'envoie un message majPlacesLiberees à contrôle")
	fmt.Printf("~=sender=%d~=type=%s~=toApp=0~=receiver=%d~=listePlaces=%s\n",
		0,
		"majPlacesLiberees",
		siteId,
		formateListeMajPlaces(listePlaces))

	// On formate le message de fin de section critique et on l'écrit sur la sortie standard STDOUT
	log.Println(runningFunc(), titleLog("Envoi Fin SC"), "J'envoie un message finSC à contrôle ")
	fmt.Printf("~=sender=%d~=type=%s~=toApp=0~=receiver=%d\n", 0, "finSC", siteId)
}

/*
** Verrouillage de l'exclusion mutuelle
 */
func monlock() {
	mutex.Lock()
}

/*
** Déverrouillage de l'exclusion mutuelle
 */
func monlocklear() {
	mutex.Unlock()
}

/*
** Création d'un tableau de places à partir d'une chaîne de caractères
 */
func creation_tableau(str string) ([]int, bool) {
	// Nettayage de la chaîne fournie
	str2 := strings.ReplaceAll(str, "[", "") // On supprime les crochets
	str2 = strings.ReplaceAll(str2, "]", "") // On supprime les crochets
	strs := strings.Split(str2, ",")         // On sépare les éléments (prendre un autre séparateur que le séparateur de champ)
	arr := make([]int, 0)                    // On génère un un tableau vide
	var isErreur = false                     // Pas d'erreur pour le moment

	// On parcourt tous les éléments de la chapine fournie
	for _, s := range strs {
		num, err := strconv.Atoi(strings.TrimSpace(s)) // On convertit la chaîne en entier
		if err != nil {
			isErreur = true
		}

		//Ajoute l'entier si et seulement si la valeur est comprise entre 0 et la longueur -1 de la liste de places
		if num >= 0 && num < len(places) {
			arr = append(arr, num)
		} else {
			isErreur = true
		}
	}

	// Retour de la fonction
	return arr, isErreur
}

/*
** Création d'une chaîne de caractères à partir d'un tableau de places
 */
func tableau_entier_vers_string(entiers []int) string {
	strArray := make([]string, len(entiers))
	for i, n := range entiers {
		strArray[i] = strconv.Itoa(n)
	}
	str := strings.Join(strArray, ",")
	str = "[" + str + "]"

	// Retour de la fonction
	return str
}

/*
** Fonction main() du programme app.go
 */
func main() {
	// Suppression des dates dans les logs
	log.SetFlags(log.Flags() &^ (log.Ldate | log.Ltime))

	// Initialisations
	var (
		c            chan int = make(chan int)
		siteIdTemp   int
		nbPlacesTemp int
	)

	// Gestion des arguments d'exécution du programme
	// flag.IntVar(&period, "n", 2, "Period of periodic message emission (in seconds)")
	// Définition dynamique des numéros de port, exemple 4440+siteId ?
	flag.IntVar(&siteIdTemp, "s", 0, "Site ID")
	flag.IntVar(&nbPlacesTemp, "nb", 50, "Nombre de places")
	var port = flag.String("port", "4443", "n° de port")
	var addr = flag.String("addr", "localhost", "nom/adresse machine")
	flag.Parse()

	siteId = siteIdTemp          // Définition du numéro de site
	nbPlacesTotal = nbPlacesTemp // Définition nombre max de places
	log.Println(runningFunc(), titleLog(""))

	// Initialisation des places
	initPlaces()
	afficherPlaces()

	// On attend les messages sur l'entrée standard STDIN
	go reception(c)

	// Démarrer le serveur Web dans une go-routine
	go func() {
		http.HandleFunc("/", do_webserver)
		log.Println(runningFunc(), titleLog("Webserver ouvert"))
		http.HandleFunc("/ws", do_websocket)
		log.Println(runningFunc(), titleLog("Websocket ouvert"))
		http.ListenAndServe(*addr+":"+*port, nil)
		log.Println(runningFunc(), titleLog("Listen and Serve !!"))
	}()

	// Pour attendre la fin des goroutines
	for {
		time.Sleep(time.Duration(60) * time.Second)
	}
}
