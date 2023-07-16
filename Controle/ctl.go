/*
** Déclaration du package
 */
package main

/*
** Importation des ressources
 */
import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

/*
** Initialisation des variables globales pour le contrôleur
 */
var nbPlacesTotal int
var nombre_instances int = 3

// nombre de site total
var horloge_locale int = 0                                              // horloge locale au site
var fileAttente []FileAttente = make([]FileAttente, nombre_instances+1) // on a i+1 occurence car un tableau commence à zéro

var siteId int = 0                    // identification du site actuel
var places_app []int = make([]int, 0) //
var mutex = &sync.Mutex{}             // exclusion mutuelle

/*
** Initialisation des variables utiles à la sauvegarde
 */
var couleur int = 0                                          // 0 pour blanc et 1 pour rouge
var initiateur bool = false                                  // site à l'initiative de la sauvegarde ?
var bilan int = 0                                            // = nombre d'émission - nombre de réception
var EG []Sauvegarde = make([]Sauvegarde, nombre_instances+1) // état de la sauvegarde EG = Etat Global
var horloge_vec []int = make([]int, nombre_instances+1)      // vecteur avec autant de composantes que de sites +1 (commence à zéro)
var NbEtatsAttendus int = 0                                  // états devant être reçus
var NbMsgAttendus int = 0                                    // messages devant être reçus

type FileAttente struct { // file d'attente des messages
	Type string `json:"type"`
	Date int    `json:"date"`
}

type Sauvegarde struct { // sauvegarde d'un site
	FileAttente  []FileAttente `json:"fileAttente"`
	Places       []int         `json:"place"`
	Horloge_vect []int         `json:"horlogeVectorielle"`
}

/*
** retourne le titre formaté
 */
func titleLog(title string) (f string) {
	format := "SITE:" + strconv.Itoa(siteId) + " -- %20s --"
	f = fmt.Sprintf(format, strings.ToUpper(title))
	return f
}

/*
** Mise en forme des logs avec les préfixes APP, nom de la fonction
 */
func runningFunc() (f string) {
	pc, _, _, _ := runtime.Caller(1)
	//pid := os.Getpid()
	funcName := runtime.FuncForPC(pc).Name()
	format := "(CONTROLE-%d)(%-20s) -- "
	f = fmt.Sprintf(format, siteId, funcName[5:])
	return f
}

/*
** Alimenter la structure fileAttente
 */
func dumpData() {
	buf := ""
	buf += "horloge_locale=" + strconv.Itoa(horloge_locale)
	buf += ", fileAttente:"
	for i := 1; i <= nombre_instances; i++ {
		buf += "[" + strconv.Itoa(i) + "]=" + strconv.Itoa(fileAttente[i].Date) + "," + fileAttente[i].Type + ";"
	}
}

/*
** Initialisation de la file d'attente
 */
func initialisation() {
	log.Println(runningFunc(), titleLog("Init"), "initialisation de la file d'attente")
	for i := 1; i <= nombre_instances; i++ {
		fileAttente[i] = FileAttente{"liberation", 0}
	}
	dumpData()
}

/*
** Envoyer d'un message périodique sur la sortie standard
 */
func sendperiodic(period time.Duration, type_message string) {
	if period == 0 {
		return
	}
	for {
		monlock()
		horloge_locale = horloge_locale + 1
		incrementer_horloge_vectorielle()
		monlocklear()
		log.Println(runningFunc(), titleLog("SEND PERIO"), "--   ENVOI   -- Horloge locale:", horloge_locale)
		fmt.Printf("~=sender=%d~=hlg=%d~=type=%s\n", siteId, horloge_locale, type_message)

		stdout := bufio.NewWriter(os.Stdout)
		stdout.Flush()

		time.Sleep(period)
	}
}

/*
** Traitements liés à la réception des messages sur stdin
 */
func reception(c chan<- int) {
	// Les messages entrant peuvent provenir d'un ctl ou de mon app
	// si toApp=1, alors le message est pour l'app, il n'est pas pour moi
	// si sender=0, alors le message provient de mon app, il est pour moi
	// si sender <> 0 et receiver=moi alors je traite le message car il provient d'un autre ctl et il est pour moi
	// sinon je fais suivre

	//Initialisations
	log.Println(runningFunc(), titleLog("DEBUT RECEPTION"), "Entrée dans la fonction reception")
	var message_recu string
	var horloge_recue int = 0
	var horloge_vect_recue []int = make([]int, 3)

	// Boucle infinie pour réceptionner successivement tous les messages
	for {
		fmt.Scanln(&message_recu)

		if len(message_recu) != 0 {
			log.Println(runningFunc(), titleLog("RECEPTION FIFO"), message_recu)
			var emetteur int

			// Découpage du string pour différencier tous les arguments du message
			tab_allkeyval := strings.Split(message_recu[1:], message_recu[0:1])
			desc_message := make(map[string]string) // tableau avec la clé et la valeur de chaque argument
			for _, keyval := range tab_allkeyval {
				tab_keyval := strings.Split(keyval[1:], keyval[0:1])
				desc_message[tab_keyval[0]] = tab_keyval[1]
			}

			// Vérification sur la validité du message
			emetteur, _ = strconv.Atoi(desc_message["sender"]) // Convertit la chaîne en entier
			if desc_message["sender"] == "" {
				log.Println(runningFunc(), titleLog("WARNING"), "(!) Le message reçu contient un champ sender vide")
			}
			tmpToApp, err := strconv.Atoi(desc_message["toApp"]) // Convertit la chaîne en entier
			if err != nil {
				panic(err)
			}
			if desc_message["toApp"] == "" {
				log.Println(runningFunc(), titleLog("WARNING"), "(!) Le message reçu contient un champ toApp vide")
			}

			if tmpToApp == 1 { // si =1 alors il s'agit d'un message à destination de l'app du site, le contrôleur ne traite pas et transmets le
				//log.Println(runningFunc(), titleLog("IGNORE"), "Message recu à destination d'une app, je ne traite pas")
			} else {

				/* Mise à jour des places de app */
				if desc_message["etatPlaces"] != "" {
					places_app = creation_tableau(desc_message["etatPlaces"])
				}

				// si je ne suis pas le receiver, je ne traite pas
				switch emetteur {
				// réception de message de mon APP

				case 0:

					switch desc_message["type"] {
					// Réception d’une demande de section critique de l’application de base :
					case "demandeSC":
						reception_demande_section_critique()
						// Réception d’une demande de section critique de l’application de base :

					// Réception fin de section critique de l’application de base
					case "finSC":
						reception_fin_section_critique()

					// message de mise à jour des places
					case "majPlacesReservees":
						log.Println(runningFunc(), titleLog("TRANSMISSION"), "Message majPlacesReservees reçu à faire suivre aux CTL:", desc_message)

						envoyer_a_tous_ctl("majPlacesReservees~=listePlaces=" + desc_message["listePlaces"])

					// message de mise à jour des places
					case "majPlacesLiberees":
						log.Println(runningFunc(), titleLog("TRANSMISSION"), "Message majPlacesLiberees reçu à faire suivre aux CTL:", desc_message)
						envoyer_a_tous_ctl("majPlacesLiberees~=listePlaces=" + desc_message["listePlaces"])

					// L'application demande a faire la sauvegarde
					case "debutInstantane":
						reception_debut_instantane(places_app)

					default:
						log.Println(runningFunc(), titleLog("ERROR"), "Message recu mais non compris :", desc_message["type"])
					}

				// reception de messages d'autre ctl
				default:
					var tmpRecv int
					if desc_message["receiver"] == "" {
						log.Println(runningFunc(), titleLog("WARNING"), "(!) Le message reçu contient un champ receiver vide")
					} else {
						tmpRecv, err = strconv.Atoi(desc_message["receiver"]) // Convertit la chaîne en entier
						if err != nil {
							panic(err)
						}
					}

					if desc_message["receiver"] != "" && tmpRecv != siteId {
						// le message n'est pas pour moi, je l'écris sur stdout pour le suivant
						log.Println(runningFunc(), titleLog("TRANSMISSION"), "Ce message n'est pas pour moi, je le transmets au suivant")
						fmt.Printf("%s\n", message_recu)
					} else {

						// Horloge
						horloge_recue, _ = strconv.Atoi(desc_message["hlg"])
						if desc_message["hlg"] == "" {
							log.Println(runningFunc(), titleLog("WARNING"), "(!) Le message reçu contient un champ hlg vide")
						}
						horloge_vect_recue = creation_tableau(desc_message["hlgvect"])
						if desc_message["hlgvect"] == "" {
							log.Println(runningFunc(), titleLog("WARNING"), "(!) Le message reçu contient un champ hlgvect vide")
						}

						if desc_message["type"] != "resetSauvegarde" && desc_message["type"] != "etat" && desc_message["type"] != "prepost" {

							// Le message est bien pour nous
							if !strings.Contains(desc_message["type"], "majPlacesReservees") && !strings.Contains(desc_message["type"], "majPlacesLiberees") {
								log.Println(runningFunc(), titleLog("BILAN"), "App veut envoyer, Bilan du site passe de", bilan, "à", bilan-1)
								monlock()
								bilan = bilan - 1
								monlocklear()
							}

							couleur_recu, _ := strconv.Atoi(desc_message["couleur"])
							if desc_message["couleur"] == "" {
								log.Println(runningFunc(), titleLog("WARNING"), "(!) Le message reçu contient un champ couleur vide")
							}

							//Première reception d'un message rouge => Si prend son instantané
							if couleur_recu == 1 && couleur == 0 {
								monlock()
								// Première réception d’un message rouge. Si prend son instantané.
								couleur = 1 // Rouge
								EG[siteId] = Sauvegarde{
									FileAttente:  fileAttente,
									Places:       places_app,
									Horloge_vect: horloge_vec,
								}
								monlocklear()
								envoyer_etat_a_ctl(EG, bilan, desc_message["sender"])
							}
							if couleur_recu == 0 && couleur == 1 {
								envoyer_prepost_a_ctl(desc_message, desc_message["sender"])
							}
						}

						// Traiter message
						switch desc_message["type"] {

						case "requete":
							// Réception d’un message de type requête
							reception_requete(emetteur, horloge_recue, horloge_vect_recue)

						case "liberation":
							//Réception d’un message de type libération
							reception_liberation(emetteur, horloge_recue, horloge_vect_recue)

						case "accuse":
							// Réception d’un message de type accusé
							reception_accuse(emetteur, horloge_recue, horloge_vect_recue)

						case "majPlacesReservees":
							// Message de mise à jour des places reçu d'un ctl, pour le raffraichissement
							listePlaces := creation_tableau(desc_message["listePlaces"])
							log.Println(runningFunc(), titleLog("Maj places réservées"), listePlaces)
							for _, p := range listePlaces {
								places_app[p] = 1
							}
							envoyer_a_app("majPlacesReservees~=listePlaces=" + desc_message["listePlaces"])

						case "majPlacesLiberees":
							// message de mise à jour des places
							listePlaces := creation_tableau(desc_message["listePlaces"])
							log.Println(runningFunc(), titleLog("Maj places libérées"), listePlaces)
							for _, p := range listePlaces {
								places_app[p] = 0
							}
							envoyer_a_app("majPlacesLiberees~=listePlaces=" + desc_message["listePlaces"])

						case "etat":
							// etat de l'instantané, si je suis l'initiateur
							if initiateur == true {
								var bilan_recu int
								var sauvegarde_recue []Sauvegarde
								if desc_message["bilan"] != "" {
									bilan_recu, _ = strconv.Atoi(desc_message["bilan"])
								}
								if desc_message["sauvegarde"] != "" {
									sauvegarde_recue = StringToSauvegardes(desc_message["sauvegarde"])
								}
								reception_etat(sauvegarde_recue, bilan_recu, desc_message["sender"])
							}

						case "prepost":
							if initiateur == true {
								reception_prepost(desc_message)
							}

						case "resetSauvegarde":
							couleur = 0
							bilan = 0

						//Envoie de la demande d'instantané
						case "envoi_instantane":
							log.Println(runningFunc(), titleLog("DEMANDE SNAPSHOT"), "Envoie de la demande d'instantané")

						default:
							log.Println(runningFunc(), titleLog("ERROR"), "Message recu mais non compris")
						}

					}
				}
			}
		}
	}
}

/*
** envoi d'un message avec sender=0 pour prévenir l'App
 */
func envoyer_a_app(type_message string) {
	// LOGGING
	log.Println(runningFunc(), titleLog("ENVOIE A APP"), "toApp=1, type="+type_message)
	fmt.Printf("~=sender=%d~=type=%s~=receiver=%s~=toApp=%s\n", siteId, type_message, "0", "1")
}

/*
** envoi d'un message à un site spécifique
 */
func envoyer_a_ctl(type_message string, destinataire int) {

	if type_message != "resetSauvegarde" && !strings.Contains(type_message, "majPlacesReservees") && !strings.Contains(type_message, "majPlacesLiberees") {
		log.Println(runningFunc(), titleLog("BILAN"), "CTRL va envoyer a", destinataire, "Bilan du site passe de", bilan, "à", bilan+1)
		bilan = bilan + 1
	}
	log.Println(runningFunc(), titleLog("ENVOIE A CTL"), fmt.Sprintf("message=~=sender=%d~=hlg=%d~=hlgvect=%s~=type=%s~=receiver=%s~=toApp=%s~=couleur=%d", siteId, horloge_locale, tableau_entier_vers_string(horloge_vec), type_message, strconv.Itoa(destinataire), "0", couleur))
	fmt.Printf("~=sender=%d~=hlg=%d~=type=%s~=hlgvect=%s~=receiver=%s~=toApp=%s~=couleur=%d\n", siteId, horloge_locale, type_message, tableau_entier_vers_string(horloge_vec), strconv.Itoa(destinataire), "0", couleur)
}

/*
** envoi d'un message à un site spécifique
 */
func envoyer_a_suivant_ctl(type_message string) {
	log.Println(runningFunc(), titleLog("ENVOIE A SUIV"), fmt.Sprintf("message=~=sender=%d~=hlg=%d~=hlgvect=%s~=type=%s~=receiver=%s~=toApp=%s~=couleur=%d\n", siteId, horloge_locale, tableau_entier_vers_string(horloge_vec), type_message, "", "0", couleur))
	fmt.Printf("~=sender=%d~=hlg=%d~=type=%s~=hlgvect=%s~=receiver=%s~=toApp=%s~=couleur=%d\n", siteId, horloge_locale, type_message, tableau_entier_vers_string(horloge_vec), "", "0", couleur)
}

/*
** Envoi du message à tous les sites différents de moi (site de base)
 */
func envoyer_a_tous_ctl(type_message string) {
	log.Println(runningFunc(), titleLog("ENVOIE A TOUS"))
	for i := 1; i <= nombre_instances; i++ {
		if i != siteId { // on envoie le message à toute les autres instance (celles != de moi)
			envoyer_a_ctl(type_message, i)
		}
	}
}

// Envoyer ETAT
func envoyer_etat_a_ctl(EG []Sauvegarde, bilan_recu int, destinataire string) {
	log.Println(runningFunc(), titleLog("BILAN"), "CTRL va envoyer etat a", destinataire, "Bilan du site passe de", bilan, "à", bilan+1)
	bilan_recu = bilan_recu + 1
	fmt.Printf("~=sender=%d~=hlg=%d~=type=%s~=hlgvect=%s~=receiver=%s~=toApp=%s~=bilan=%d~=sauvegarde=%s\n", siteId, horloge_locale, "etat", tableau_entier_vers_string(horloge_vec), destinataire, "0", bilan_recu, SauvegardeToString(EG))
}

// Envoyer PREPOST
func envoyer_prepost_a_ctl(desc_message map[string]string, destinataire string) {
	log.Printf(runningFunc(), titleLog("ENVOIE PREPOST"), "Envoi d'un message à", destinataire, "de type prepost")
	fmt.Printf("~=sender=%d~=hlg=%s~=hlgvect=%s~=type=%s~=receiver=%s~=toApp=%s~=bilan=%s\n", siteId, desc_message["hlg"], tableau_entier_vers_string(horloge_vec), "prepost", destinataire, desc_message["toApp"], desc_message["bilan"])
}

/*
Réception d’une demande de section critique de l’application de base :

	  Recevoir( [demandeSC] ) de l’application de base
	      hi <- hi + 1
		  Tabi[i] <- (requête, hi)
		  envoyer( [requête] hi ) à tous les autres sites
*/
func reception_demande_section_critique() {
	demande_section_critique()
}

func demande_section_critique() {
	monlock()
	horloge_locale++
	incrementer_horloge_vectorielle()
	fileAttente[siteId] = FileAttente{"requete", horloge_locale}
	envoyer_a_tous_ctl("requete")
	dumpData()
	monlocklear()
}

/*
*
Réception fin de section critique de l’application de base :

	recevoir( [finSC] ) de l’application de base
	    hi <- hi + 1
	    Tabi[i] <- (libération, hi)
	    envoyer( [libération] hi ) à tous les autres sites.
*/
func reception_fin_section_critique() {
	fin_section_critique()
}
func fin_section_critique() {
	monlock()
	horloge_locale++
	incrementer_horloge_vectorielle()
	fileAttente[siteId] = FileAttente{"liberation", horloge_locale}
	envoyer_a_tous_ctl("liberation")
	monlocklear()
}

/*
Réception d’un message de type libération :

	recevoir( [libération] h ) de Sj
	    hi <- max(hi, h) + 1
	    Tabi[j] ← (libération, h)
	    * L’arrivée du message pourrait permettre de satisfaire une éventuelle requête de Si.
	    si Tabi[i].type == requête et (Tabi[i].date, i) <2 (Tabi[k].date, k) pour tout k <> i alors Si est demandeur et sa requête est la plus ancienne.
	        envoyer( [débutSC] ) à l’application de base
	    fin si
*/
func reception_liberation(emetteur int, horloge_recue int, horloge_vect_recue []int) {
	monlock()
	// recevoir( [liberation] h ) de Sj
	// on met le max dans horloge locale
	horloge_locale = int(math.Max(float64(horloge_locale), float64(horloge_recue)) + 1)
	incrementer_horloge_vectorielle_et_emmeteur(horloge_vect_recue)

	si_demandeur := true
	// Tabi[j] ← (libération, h)
	fileAttente[emetteur] = FileAttente{"liberation", horloge_recue}
	monlocklear()

	//      si Tabi[i].type == requête et (Tabi[i].date, i) <2 (Tabi[k].date, k) pour tout k <> i alors Si est demandeur et sa requête est la plus ancienne.
	//          envoyer( [débutSC] ) à l’application de base

	for k := 1; k <= nombre_instances; k++ {
		if k != siteId { // pour tout k<> i
			// On vérifie si Si est demandeur et sa requête est la plus ancienne.
			if !(fileAttente[siteId].Type == "requete" && comparaison_rel_ordre_deux(
				fileAttente[siteId].Date,
				siteId,
				fileAttente[k].Date,
				k)) {
				si_demandeur = false
			}
		}
	}
	if si_demandeur {
		// envoyer( [débutSC] ) à l’application de base
		envoyer_a_app("debutSC")
	}

}

func monlocklear() {
	mutex.Unlock()
}
func monlock() {
	mutex.Lock()
}

/*
Réception d’un message de type requête :

	recevoir( [requête] h ) de Sj
	    hi <- max(hi, h) + 1
	    Tabi[j] <- (requête, h)
	    envoyer( [accusé] hi ) à Sj
	    *L’arrivée du message pourrait permettre de satisfaire une éventuelle demande de Si.
	    si Tabi[i].type == requête et (Tabi[i].date, i) <2 (Tabi[k].date, k) pour tout k <> i alors Si est demandeur et sa requête est la plus ancienne.
	      envoyer( [débutSC] ) à l’application de base
	    fin si
*/
func reception_requete(emetteur int, horloge_recue int, horloge_vect_recue []int) {
	monlock()

	demande_si_satisfaite := true

	horloge_locale = int(math.Max(float64(horloge_locale), float64(horloge_recue)) + 1)
	incrementer_horloge_vectorielle_et_emmeteur(horloge_vect_recue)

	//  Tabi[j] ← (requête, h)
	fileAttente[emetteur] = FileAttente{"requete", horloge_recue}

	monlocklear()
	// envoyer( [accusé] hi ) à Sj
	envoyer_a_ctl("accuse", emetteur)

	// si Tabi[i].type == requête et (Tabi[i].date, i) <2 (Tabi[k].date, k) pour tout k <> i alors Si est demandeur et sa requête est la plus ancienne.
	for k := 1; k <= nombre_instances; k++ {
		if k == siteId {
			if !(fileAttente[siteId].Type == "requete" && comparaison_rel_ordre_deux(
				fileAttente[siteId].Date,
				siteId,
				fileAttente[k].Date,
				k)) {
				demande_si_satisfaite = false
			}
		}
	}
	if demande_si_satisfaite {
		// envoyer( [débutSC] ) à l’application de base
		envoyer_a_app("debutSC")
	}

}

/*
Réception d’un message de type accusé :
  recevoir( [accusé] h ) de Sj
      hi <- max(hi, h) + 1
      si Tabi[j].type <> requête alors On n’écrase pas la date d’une requête par celle d’un accusé.
        Tabi[j] <- (accusé, h)
      fin si
    *L’arrivée du message pourrait permettre de satisfaire une éventuelle demande de Si.
      si Tabi[i].type == requête et (Tabi[i].date, i) <2 (Tabi[k].date, k) pour tout k <> i alors Si est demandeur et sa requête est la plus ancienne.
        envoyer( [débutSC] ) à l’application de base
      fin si
*/

func reception_accuse(emetteur int, horloge_recue int, horloge_vect_recue []int) {
	monlock()
	// recevoir( [accusé] h ) de Sj

	demande_si_satisfaite := true

	// hi← max(hi, h) + 1
	horloge_locale = int(math.Max(float64(horloge_locale), float64(horloge_recue)) + 1)
	incrementer_horloge_vectorielle_et_emmeteur(horloge_vect_recue)

	// si Tabi[j].type <> requête alors On n’écrase pas la date d’une requête par celle d’un accusé.

	if fileAttente[emetteur].Type != "requete" {
		// On n’écrase pas la date d’une requête par celle d’un accusé.
		fileAttente[emetteur] = FileAttente{"accuse", horloge_recue}
	}
	monlocklear()
	// si Tabi[i].type == requête et (Tabi[i].date, i) <2 (Tabi[k].date, k) pour tout k <> i alors Si est demandeur et sa requête est la plus ancienne.
	for k := 1; k <= nombre_instances; k++ {
		if k != siteId {
			if !(fileAttente[siteId].Type == "requete" && comparaison_rel_ordre_deux(
				fileAttente[siteId].Date,
				siteId,
				fileAttente[k].Date,
				k)) {
				demande_si_satisfaite = false
			}
		}
	}
	if demande_si_satisfaite {
		// envoyer( [débutSC] ) à l’application de base
		envoyer_a_app("debutSC")
	}

}

/*
Réception d'une action qui ne s'effectue que sur un site dur ordre de App
*/
func reception_debut_instantane(places []int) {
	monlock()
	couleur = 1 // rouge
	initiateur = true

	EG[siteId] = Sauvegarde{
		FileAttente:  fileAttente,
		Places:       places,
		Horloge_vect: horloge_vec,
	}

	NbEtatsAttendus = nombre_instances - 1
	NbMsgAttendus = bilan
	monlocklear()
	envoyer_a_tous_ctl("envoi_instantane")
}

/*
Réception d'un message de type état
*/
func reception_etat(etat []Sauvegarde, bilan_recu int, sender string) {
	monlock()
	EG_union(etat, sender)
	NbEtatsAttendus = NbEtatsAttendus - 1
	NbMsgAttendus = NbMsgAttendus + bilan_recu
	monlocklear()
	if NbEtatsAttendus == 0 && NbMsgAttendus == 0 {
		log.Println(runningFunc(), titleLog("Sauvegarde"), "Sauvegarde en cours dans sauvegarde.json...")
		formater_EG()
		sauvegarder("sauvegarde.json", SauvegardeToString(EG[1:]))
		monlock()
		couleur = 0
		initiateur = false
		bilan = 0
		monlocklear()
		envoyer_a_tous_ctl("resetSauvegarde")
	}

}

/*
Réception d'un message de type prépost
*/
func reception_prepost(desc_message map[string]string) {
	monlock()
	NbMsgAttendus = NbMsgAttendus - 1
	EG_union_message(desc_message["type"], desc_message["hlg"], desc_message["hlgvect"])
	monlocklear()
	if NbEtatsAttendus == 0 && NbMsgAttendus == 0 {
		log.Println(runningFunc(), titleLog("Sauvegarde"), "Sauvegarde en cours dans sauvegarde.json...")
		formater_EG()
		sauvegarder("sauvegarde.json", SauvegardeToString(EG[1:]))
		monlock()
		couleur = 0
		bilan = 0
		initiateur = false
		monlocklear()
		envoyer_a_tous_ctl("resetSauvegarde")
	}

}

func incrementer_horloge_vectorielle_et_emmeteur(horloge_vec_recue []int) {
	horloge_vec[siteId] = horloge_locale
	for i := 1; i <= 3; i++ {
		if i != siteId {
			if horloge_vec[i] < horloge_vec_recue[i] {
				horloge_vec[i] = horloge_vec_recue[i]
			}
		}
	}
}

func incrementer_horloge_vectorielle() {
	horloge_vec[siteId] += 1
}

func comparaison_rel_ordre_deux(i1 int, j1 int, i2 int, j2 int) bool {
	if i1 < i2 {
		return true
	} else if i1 > i2 {
		return false
	}
	// ICI : i1 = i2 (puisque on a traité le cas ou i1 < i2 ET i2 < i1)
	if j1 < j2 {
		return true
	}
	return false
}

func EG_union(U1 []Sauvegarde, sender string) {
	sender_int, _ := strconv.Atoi(sender)
	EG[sender_int] = U1[sender_int]
}

func EG_union_message(type_message string, horloge string, horloge_vect string) {
	EG[siteId].FileAttente[siteId].Date, _ = strconv.Atoi(horloge)
	EG[siteId].FileAttente[siteId].Type = type_message
	EG[siteId].Horloge_vect = creation_tableau(horloge_vect)
}

func SauvegardeToString(sauvegarde []Sauvegarde) string {
	instanceSauv, _ := json.Marshal(sauvegarde)
	return string(instanceSauv)
}

func StringToSauvegardes(jsonString string) []Sauvegarde {
	var sauvegardes []Sauvegarde
	json.Unmarshal([]byte(jsonString), &sauvegardes)
	return sauvegardes
}

func creation_tableau(str string) []int {

	str2 := strings.ReplaceAll(str, "[", "") // Supprime les crochets
	str2 = strings.ReplaceAll(str2, "]", "") // Supprime les crochets
	strs := strings.Split(str2, ",")         // Sépare les éléments // prendre un autre séparateur que le séparateur de champ
	arr := make([]int, 0)                    // Crée un tableau vide

	for _, s := range strs {
		num, _ := strconv.Atoi(strings.TrimSpace(s)) // Convertit la chaîne en entier
		arr = append(arr, num)

	}
	return arr
}

func formater_EG() {

	for i := 1; i < len(EG); i++ {
		EG[i-1].FileAttente = EG[i].FileAttente[1:]
		EG[i].Horloge_vect = EG[i].Horloge_vect[1:]
	}

}

func sauvegarder(filename string, data string) error {
	envoyer_a_app("confirmSauvegarde")
	// Ouvrir le fichier en mode append
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	// Créer un writer bufio pour écrire dans le fichier
	writer := bufio.NewWriter(file)

	// Écrire les données dans le fichier
	_, err = writer.WriteString(data)
	if err != nil {
		return err
	}

	// Vider le buffer pour s'assurer que toutes les données sont écrites dans le fichier
	err = writer.Flush()
	if err != nil {
		return err
	}

	return nil
}

func tableau_entier_vers_string(entiers []int) string {
	strArray := make([]string, len(entiers))
	for i, n := range entiers {
		strArray[i] = strconv.Itoa(n)
	}
	str := strings.Join(strArray, ",")

	str = "[" + str + "]"
	return str
}

/*
** Initialisation des places
 */
func initPlaces() {
	log.Println(runningFunc(), titleLog("Init"), "initialisation des places")
	for i := 0; i < nbPlacesTotal; i++ {
		places_app = append(places_app, 0)
	}
}

func main() {
	// Suppression des dates dans les logs
	log.SetFlags(log.Flags() &^ (log.Ldate | log.Ltime))

	// Variables
	var (
		c            chan int = make(chan int)
		period       int
		siteIdTemp   int
		nbPlacesTemp int
	)

	// Arguments
	flag.IntVar(&period, "n", 0, "Period of periodic message emission (in seconds)")
	flag.IntVar(&nbPlacesTemp, "nb", 50, "Nombre de places")
	flag.IntVar(&siteIdTemp, "s", 0, "Site ID")
	flag.Parse()

	// Definition numero site
	siteId = siteIdTemp
	//Définition nombre max de places
	nbPlacesTotal = nbPlacesTemp

	log.Println(runningFunc(), titleLog(""), ", period=", period)

	// Initialisation des places
	initPlaces()

	// On initialise la file d'attente
	initialisation()

	if period > 0 { // on ne fait des envois périodiques que si period > 0
		go sendperiodic(time.Duration(period)*time.Second, "TEST")
	}

	// FIN AJOUT

	// On attend les messages
	go reception(c)

	for {
		time.Sleep(time.Duration(60) * time.Second)
	} // Pour attendre la fin des goroutines...
}
