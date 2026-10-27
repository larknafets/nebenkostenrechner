package web

import "net/http"

// navData is every page's shared nav facts: Base, the Zugriff of the request
// and the login/demo entries, computed once per request by Zugang.NavData.
// Embedded (anonymous field) in every page's template-data struct:
// text/template promotes embedded-struct fields for dot-access, so layout.html
// reads .Base/.Vermieter/.IsDemoSession/.ShowLoginEntry/.Kann* unchanged,
// whether they come directly from a struct or via this embed. Aktuell (the
// active nav tab) deliberately stays out - each handler names its own page, a
// constructor here can't derive that for it.
type navData struct {
	Zugriff
	Base            string
	IsDemoSession   bool
	ShowLoginEntry  bool
	ShowLogoutEntry bool
	LoginOptionen   []loginOption
	// HatMieter is true when the Mieter login exists (see mieterAktiv).
	HatMieter bool
}

// NavData builds navData for the current request - the single place the
// Zugriff, requestBase and the login entries are put together.
//
// The login entry shows for everybody but the Vermieter session and the demo
// session, so a Mieter can switch to the Vermieter and the (otherwise open)
// app keeps a visible path to the demo login (Issue #122). The logout entry
// needs a real session to end: with no Vermieter password everyone counts as
// Vermieter, but there is no session to log out of.
func (z *Zugang) NavData(r *http.Request) navData {
	zg := z.Zugriff(r)
	return navData{
		Zugriff:         zg,
		Base:            requestBase(r),
		IsDemoSession:   zg.Demo,
		ShowLoginEntry:  !zg.HatSitzung || !zg.Vermieter,
		ShowLogoutEntry: zg.HatSitzung,
		LoginOptionen:   z.login.loginOptionen(),
		HatMieter:       z.login.mieterAktiv(),
	}
}
