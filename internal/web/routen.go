package web

import (
	"net/http"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// route is one row of the route table: what is served, what it demands of
// the role, and who answers.
type route struct {
	Methode  string
	Pfad     string
	Anspruch Anspruch
	Handler  http.HandlerFunc
}

// routen is the one list of the server's routes. NewMux registers every row
// (withDB around the Gate of its Anspruch), the route tests read the same
// table. A row with a reason for its Anspruch carries it in a comment.
func routen(z *Zugang, version, buildDate string) []route {
	return []route{
		{http.MethodGet, "/", Offen, handleIndex()},
		// Offen: logging in and out is for the visitor who has no role yet.
		{http.MethodPost, "/login", Offen, z.login.HandleLogin()},
		{http.MethodPost, "/logout", Offen, z.login.HandleLogout()},

		{http.MethodGet, "/ablesungen", Braucht(store.RechtAblesungenAnsehen), handleAblesungenListe(z)},
		{http.MethodGet, "/ablesungen/export.csv", NurVermieter, handleExportCSV()},
		// Creating a reading is a Recht of its own, so a Mieter or a visitor
		// can be allowed to record a reading (Ticket #129).
		{http.MethodGet, "/ablesungen/neu", Braucht(store.RechtAblesungAnlegen), handleWizardForm(z)},
		{http.MethodPost, "/ablesungen", Braucht(store.RechtAblesungAnlegen), handleCreateAblesung()},
		{http.MethodPost, "/ablesungen/import", NurVermieter, handleImportCSV()},
		{http.MethodGet, "/ablesungen/{id}", Braucht(store.RechtAblesungenAnsehen), handleAblesungDetail(z)},
		// Completing a Teilstand needs only the Recht to create a reading,
		// just as creating it did (Ticket #130). A complete reading stays
		// for the Vermieter. A missing id answers 404 before the check.
		{http.MethodGet, "/ablesungen/{id}/bearbeiten", AnlegenOderVermieter, handleEditWizardForm(z)},
		{http.MethodPost, "/ablesungen/{id}", AnlegenOderVermieter, handleUpdateAblesung()},
		{http.MethodPost, "/ablesungen/{id}/loeschen", NurVermieter, handleDeleteAblesung()},

		// Offen: every role sees its own Dashboard, what the cards show
		// is decided by the Zugriff. The same goes for the static
		// explanation and the update check.
		{http.MethodGet, "/dashboard", Offen, handleDashboard(version, buildDate, z)},
		{http.MethodGet, "/update-check", Offen, handleUpdateCheck(version)},
		{http.MethodGet, "/berechnungslogik", Offen, handleBerechnungslogik(z)},
		{http.MethodGet, "/abrechnung", NurVermieter, handleAbrechnung(z)},

		{http.MethodGet, "/stammdaten", Braucht(store.RechtStammdatenAnsehen), handleStammdatenForm(z)},
		{http.MethodPost, "/stammdaten", NurVermieter, handleUpdateStammdaten()},
		{http.MethodPost, "/stammdaten/sortierung", NurVermieter, handleMoveKostenposition()},

		{http.MethodGet, "/fixkosten", Braucht(store.RechtFixkostenAnsehen), handleFixkostenListe(z)},
		{http.MethodGet, "/fixkosten/export.csv", NurVermieter, handleExportFixkostenCSV()},
		{http.MethodGet, "/fixkosten/neu", NurVermieter, handleFixkostenForm(z)},
		{http.MethodPost, "/fixkosten", NurVermieter, handleCreateFixkosten()},
		{http.MethodPost, "/fixkosten/import", NurVermieter, handleImportFixkostenCSV()},
		{http.MethodGet, "/fixkosten/{id}", Braucht(store.RechtFixkostenAnsehen), handleFixkostenDetail(z)},
		{http.MethodGet, "/fixkosten/{id}/bearbeiten", NurVermieter, handleFixkostenEditForm(z)},
		{http.MethodPost, "/fixkosten/{id}", NurVermieter, handleUpdateFixkosten()},
		{http.MethodPost, "/fixkosten/{id}/loeschen", NurVermieter, handleDeleteFixkosten()},
	}
}
