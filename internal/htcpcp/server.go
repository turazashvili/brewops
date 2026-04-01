package htcpcp

import (
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/niko/brewops/internal/metrics"
)

// pick returns a random element from a string slice.
func pick(pool []string) string {
	return pool[rand.Intn(len(pool))]
}

// ── Rotating flavor text pools for plain-text responses ──

var flavorBrew = []string{
	"The brew is in progress. Patience is a virtue. Caffeine is better.",
	"Brewing has commenced. Stand by for caffeination.",
	"Your pot is warming up. So should you.",
	"The beans have been summoned. They respond.",
	"Brew initiated. This is the most productive thing you'll do today.",
	"Coffee is being constructed. Some assembly required.",
	"The grinder is grinding. The heater is heating. The future is caffeinated.",
	"You've committed to this brew. There is no rollback.",
	"The coffee gods have received your offering. They are pleased.",
	"Brew in progress. ETA: soon. Definition of 'soon': unclear.",
}

var flavor418 = []string{
	"The teapot stares at you. It is unmoved.",
	"You knew it was a teapot. Deep down, you knew.",
	"Somewhere, a coffee pot is lonely. Try that one instead.",
	"The teapot has been a teapot since it was manufactured. This hasn't changed.",
	"RFC 2324 predicted this moment. You walked right into it.",
	"This teapot has seen things. Mostly 418 requests.",
	"In a parallel universe, this teapot brews coffee. This is not that universe.",
	"The teapot would like you to know: it's not mad. Just disappointed.",
	"418. The teapot's favorite number. And now yours too.",
	"No coffee was harmed in the making of this error. Because no coffee was made.",
}

var flavorTea = []string{
	"The kettle is on. Keep calm.",
	"Tea: for when you want caffeine but with manners.",
	"The teapot was BORN for this. Literally.",
	"Steeping in progress. This is the teapot's time to shine.",
	"The British would be so proud right now.",
	"Tea: because sometimes coffee is too much commitment.",
	"The teapot purrs contentedly. This is its purpose.",
	"Finally, someone who reads the RFC and chooses tea.",
}

var flavorGet = []string{
	"A watched pot never boils. But we watch them anyway.",
	"Observability: the art of staring at things professionally.",
	"You checked on the pot. The pot appreciates it.",
	"Status: it's a pot. Beyond that, it's complicated.",
	"Monitoring complete. The pot remains a pot.",
	"The pot has been observed. Heisenberg would have concerns.",
}

var flavorWelcome = []string{
	"There is coffee all over the world.",
	"Welcome to the future of beverage infrastructure.",
	"You've found the world's most over-engineered coffee pot.",
	"RFC 2324 was a joke. We didn't get the memo.",
	"Where enterprise engineering meets caffeine addiction.",
	"The protocol nobody asked for. The server nobody needed. You're welcome.",
}

// Server is the HTCPCP/1.0 compliant server.
// RFC 2324 + RFC 7168 (TEA extension).
// Production-grade. Battle-tested*. Enterprise-ready**.
//
// * Not actually tested in battle.
// ** Not actually used by any enterprise. Or anyone.
type Server struct {
	Fleet   *PotFleet
	Metrics *metrics.Collector
}

// NewServer creates a new HTCPCP server with the default fleet.
func NewServer(fleet *PotFleet, collector *metrics.Collector) *Server {
	return &Server{
		Fleet:   fleet,
		Metrics: collector,
	}
}

// Handler returns the HTTP handler with CORS middleware.
// Routes are matched by path prefix — no mux needed for a protocol
// this important.
func (s *Server) Handler() http.Handler {
	return s.corsMiddleware(http.HandlerFunc(s.route))
}

// route dispatches requests based on URL path.
func (s *Server) route(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	switch {
	case path == "/pot" && (r.Method == "BREW" || r.Method == "POST"):
		s.handleNewPot(w, r)
	case strings.HasPrefix(path, "/pot-"):
		s.handlePot(w, r)
	case strings.HasPrefix(path, "/tea/") || path == "/tea":
		s.handleTeaVariety(w, r)
	case path == "/coffee":
		s.handleCoffeeRoot(w, r)
	case path == "/status":
		s.handleStatus(w, r)
	case path == "/events":
		s.handleSSE(w, r)
	case path == "/stats":
		s.handleStats(w, r)
	case path == "/health":
		s.handleHealth(w, r)
	default:
		// For the root path or unknown paths, return a welcome message
		s.handleCoffeeRoot(w, r)
	}
}

// corsMiddleware adds CORS headers for the dashboard.
func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, BREW, WHEN, PROPFIND, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept-Additions")
		w.Header().Set("Access-Control-Expose-Headers", "Safe, X-BrewOps-Version")

		// Always set the Safe header per RFC 2324 Section 2.2.1.1
		w.Header().Set("Safe", "if-user-awake")
		w.Header().Set("X-BrewOps-Version", "1.0.0")

		if r.Method == "OPTIONS" {
			w.WriteHeader(204)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// handlePot handles requests to /pot-{id} endpoints.
func (s *Server) handlePot(w http.ResponseWriter, r *http.Request) {
	// Parse pot ID from path
	potID, err := parsePotID(r.URL.Path)
	if err != nil {
		s.jsonError(w, r, http.StatusNotFound, "Pot not found. Check your pot-designator (RFC 2324 Section 3).")
		return
	}

	pot := s.Fleet.GetPot(potID)
	if pot == nil {
		s.respondErr(w, r, http.StatusNotFound, fmt.Sprintf("pot-%d does not exist. Fleet has %d pots. Use BREW /pot to create a new one.", potID, s.Fleet.Count()), nil)
		return
	}

	switch r.Method {
	case "BREW", "POST":
		s.handleBrew(w, r, pot)
	case "GET":
		s.handleGet(w, r, pot)
	case "WHEN":
		s.handleWhen(w, r, pot)
	case "PROPFIND":
		s.handlePropfind(w, r, pot)
	default:
		s.jsonError(w, r, http.StatusMethodNotAllowed, fmt.Sprintf("Method %s not recognized by HTCPCP/1.0. Try BREW, GET, WHEN, or PROPFIND.", r.Method))
	}
}

// handleNewPot creates a new pot dynamically via BREW /pot.
// Every ~5th pot is a surprise teapot. The fleet grows without bound.
// This is fine. Everything is fine.
func (s *Server) handleNewPot(w http.ResponseWriter, r *http.Request) {
	// ── DoCS (Denial of Coffee Service) detection ──
	// RFC 2324 §7: "Unmoderated access to unprotected coffee pots from
	// Internet users might lead to several kinds of denial of coffee
	// service attacks."
	isDoCS, brewCount, docsTotal := s.Metrics.CheckDoCS(r.RemoteAddr)
	if isDoCS {
		// Log the attack as a CRITICAL incident
		s.Metrics.Record("BREW", "/pot [DoCS]", 429, r.RemoteAddr, nil)
	}

	pot := s.Fleet.CreatePot()

	// Read body for start/stop
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1024))
	command := strings.TrimSpace(strings.ToLower(string(body)))
	if command == "" {
		command = "start"
	}

	// Parse additions
	additions := parseAdditions(r.Header.Get("Accept-Additions"))
	var validAdds []Addition
	var addStrings []string
	for _, a := range additions {
		if _, ok := ValidAdditions[a]; ok {
			validAdds = append(validAdds, a)
			addStrings = append(addStrings, string(a))
		}
	}

	// Determine beverage
	contentType := r.Header.Get("Content-Type")
	beverage, teaVariety := determineBeverage(contentType, r.URL.Path)

	// If the new pot is a teapot and they want coffee: surprise 418!
	if pot.Type == PotTypeTeapot && (beverage == BeverageCoffee || beverage == BeverageEspresso) {
		s.Metrics.Record(r.Method, r.URL.Path, 418, r.RemoteAddr, nil)
		s.respond418(w, r, pot)
		return
	}

	// Start brewing
	if command == "start" {
		status, errKind := pot.StartBrew(beverage, teaVariety, validAdds)
		if errKind != "" {
			s.Metrics.Record(r.Method, r.URL.Path, 500, r.RemoteAddr, nil)
			s.respondErr(w, r, 500, "Failed to start brew on new pot. The coffee gods are displeased.", nil)
			return
		}

		s.Metrics.Record(r.Method, r.URL.Path, 200, r.RemoteAddr, addStrings)
		host := inferHost(r)
		jsonData := map[string]interface{}{
			"status":  "brewing",
			"pot":     status,
			"message": fmt.Sprintf("New pot created! pot-%d (%s). Brew started.", pot.ID, pot.Type),
		}
		if isDoCS {
			jsonData["docs_warning"] = fmt.Sprintf("Denial of Coffee Service detected: %d brews in 30s", brewCount)
			jsonData["docs_attacks_total"] = docsTotal
		}
		s.respond(w, r, 200, func() string {
			potIcon := "☕"
			surprise := ""
			if pot.Type == PotTypeTeapot {
				potIcon = "🫖"
				surprise = "\n  Note: This pot is a teapot! It can only brew tea.\n"
			}
			addLine := ""
			if len(addStrings) > 0 {
				addLine = fmt.Sprintf("  Additions:   %s\n", strings.Join(addStrings, ", "))
			}
			docsWarning := ""
			if isDoCS {
				docsWarning = fmt.Sprintf(`
  ╔═══════════════════════════════════════════════════╗
  ║  ⚠  DENIAL OF COFFEE SERVICE ATTACK DETECTED  ⚠  ║
  ╚═══════════════════════════════════════════════════╝

  You have initiated %d brews in 30 seconds.
  This has been classified as a DoCS attack.

  RFC 2324 §7: "Unmoderated access to unprotected coffee
  pots from Internet users might lead to several kinds of
  denial of coffee service attacks."

  Your brew has been served anyway. This is a coffee pot,
  not a firewall. (Modern coffee pots do not use fire.)

  DoCS attacks total: %d

`, brewCount, docsTotal)
			}
			return fmt.Sprintf(`
  New Pot Created!
  ════════════════

       ( (
        ) )
      ........
      |      |]
      \      /
       '----'

  %s pot-%d has joined the fleet.

  Type:        %s
  Beverage:    %s
  State:       %s
  Temperature: %.1f°C (%s)
%s%s
  Safe:        if-user-awake

  %s

  Check status: curl %s/pot-%d
  Say WHEN:     curl -X WHEN %s/pot-%d
%s
`, potIcon, pot.ID, pot.Type, beverage, status.State,
				status.Temperature, status.TempLabel, addLine, surprise,
				pick(flavorBrew), host, pot.ID, host, pot.ID, docsWarning)
		}, jsonData)
		return
	}

	// Just created, nothing to stop
	s.respond(w, r, 200, func() string {
		return fmt.Sprintf("\n  pot-%d created but no brew started (command was '%s').\n\n", pot.ID, command)
	}, map[string]interface{}{
		"pot_id":  pot.ID,
		"type":    pot.Type,
		"message": fmt.Sprintf("pot-%d created. Send 'start' to brew.", pot.ID),
	})
}

// handleBrew processes BREW and POST requests.
// RFC 2324 Section 2.1.1: "A coffee pot server MUST accept both the
// BREW and POST method equivalently."
func (s *Server) handleBrew(w http.ResponseWriter, r *http.Request, pot *Pot) {
	// Read body (should be "start" or "stop")
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1024))
	command := strings.TrimSpace(strings.ToLower(string(body)))

	if command == "" {
		command = "start"
	}

	if command != "start" && command != "stop" {
		s.jsonError(w, r, http.StatusBadRequest,
			"coffee-message-body must be 'start' or 'stop' (RFC 2324 Section 4).")
		return
	}

	// Handle stop
	if command == "stop" {
		st := pot.StopBrew()
		s.Metrics.Record(r.Method, r.URL.Path, 200, r.RemoteAddr, nil)
		s.respond(w, r, 200, func() string {
			return fmt.Sprintf(`
  Brew Stopped
  ════════════

  pot-%d has been stopped.
  State:       %s
  Temperature: %.1f°C (%s)
  Fill:        %d%%
`, st.ID, st.State, st.Temperature, st.TempLabel, st.FillLevel)
		}, st)
		return
	}

	// Parse additions from Accept-Additions header
	additions := parseAdditions(r.Header.Get("Accept-Additions"))

	// Validate additions
	var validAdds []Addition
	var addStrings []string
	for _, a := range additions {
		if _, ok := ValidAdditions[a]; ok {
			validAdds = append(validAdds, a)
			addStrings = append(addStrings, string(a))
		} else {
			// 406 Not Acceptable
			s.Metrics.Record(r.Method, r.URL.Path, 406, r.RemoteAddr, nil)
			s.respondNotAcceptable(w, r, a)
			return
		}
	}

	// Determine beverage type from Content-Type and path
	contentType := r.Header.Get("Content-Type")
	beverage, teaVariety := determineBeverage(contentType, r.URL.Path)

	// If it's a teapot and you want coffee: 418.
	// This is THE moment. The whole reason this project exists.
	if pot.Type == PotTypeTeapot && (beverage == BeverageCoffee || beverage == BeverageEspresso) {
		s.Metrics.Record(r.Method, r.URL.Path, 418, r.RemoteAddr, nil)
		s.respond418(w, r, pot)
		return
	}

	// If it's a coffee pot and you want tea
	if pot.Type == PotTypeCoffee && beverage == BeverageTea {
		s.Metrics.Record(r.Method, r.URL.Path, 406, r.RemoteAddr, nil)
		s.jsonError(w, r, 406,
			"This is a coffee pot. It is not tea-capable. Per RFC 7168, tea requires a 'message/teapot' capable device.")
		return
	}

	// If it's a teapot brewing tea, check for tea variety
	if pot.Type == PotTypeTeapot && beverage == BeverageTea && teaVariety == TeaNone {
		// Return 300 Multiple Options with Alternates header (RFC 7168 Section 2.1.1)
		s.Metrics.Record(r.Method, r.URL.Path, 300, r.RemoteAddr, nil)
		s.respond300(w, r, pot)
		return
	}

	// Brew it!
	status, errKind := pot.StartBrew(beverage, teaVariety, validAdds)
	if errKind != "" {
		switch errKind {
		case "teapot":
			s.Metrics.Record(r.Method, r.URL.Path, 418, r.RemoteAddr, nil)
			s.respond418(w, r, pot)
		case "busy":
			s.Metrics.Record(r.Method, r.URL.Path, 503, r.RemoteAddr, nil)
			s.respondErr(w, r, 503, fmt.Sprintf("pot-%d is currently busy (%s). Please wait or try another pot.", pot.ID, pot.State), nil)
		default:
			s.Metrics.Record(r.Method, r.URL.Path, 500, r.RemoteAddr, nil)
			s.respondErr(w, r, 500, "Unexpected brew error. The coffee gods are displeased.", nil)
		}
		return
	}

	s.Metrics.Record(r.Method, r.URL.Path, 200, r.RemoteAddr, addStrings)
	jsonData := map[string]interface{}{
		"status":  "brewing",
		"pot":     status,
		"message": fmt.Sprintf("Brew started on pot-%d. Beverage: %s.", pot.ID, beverage),
	}
	s.respond(w, r, 200, func() string {
		addLine := ""
		if len(validAdds) > 0 {
			addLine = fmt.Sprintf("  Additions:   %s\n", strings.Join(addStrings, ", "))
		}
		return fmt.Sprintf(`
  Brew Started
  ════════════

       ( (
        ) )
      ........
      |      |]
      \      /
       '----'

  Pot:         pot-%d (%s)
  Beverage:    %s
  State:       %s
  Temperature: %.1f°C (%s)
%s
  Safe:        if-user-awake

  %s
  Check status: curl %s/pot-%d
  Say WHEN:     curl -X WHEN %s/pot-%d

`, pot.ID, pot.Type, beverage, status.State, status.Temperature,
			status.TempLabel, addLine, pick(flavorBrew), inferHost(r), pot.ID, inferHost(r), pot.ID)
	}, jsonData)
}

// handleGet returns the current status of a pot.
// RFC 2324 Section 2.1.2: "In HTCPCP, the resources associated with a
// coffee pot are physical, and not information resources."
func (s *Server) handleGet(w http.ResponseWriter, r *http.Request, pot *Pot) {
	st := pot.Status()
	s.Metrics.Record("GET", r.URL.Path, 200, r.RemoteAddr, nil)

	icon := "coffee-pot"
	if st.Type == PotTypeTeapot {
		icon = "teapot"
	}
	s.respond(w, r, 200, func() string {
		bevLine := ""
		if st.Beverage != "" {
			bevLine = fmt.Sprintf("  Beverage:    %s\n", st.Beverage)
		}
		if st.TeaVariety != "" {
			bevLine += fmt.Sprintf("  Tea Variety: %s\n", st.TeaVariety)
		}
		addLine := ""
		if len(st.Additions) > 0 {
			adds := make([]string, len(st.Additions))
			for i, a := range st.Additions {
				adds[i] = string(a)
			}
			addLine = fmt.Sprintf("  Additions:   %s\n", strings.Join(adds, ", "))
		}
		brewLine := ""
		if st.BrewElapsed != "" {
			brewLine = fmt.Sprintf("  Brew Time:   %s\n", st.BrewElapsed)
		}
		return fmt.Sprintf(`
  pot-%d Status
  ════════════

  Type:        %s
  State:       %s
  Temperature: %.1f°C (%s)
  Fill Level:  %d%%
%s%s%s  Safe:        %s

  %s

`, st.ID, icon, st.State, st.Temperature,
			st.TempLabel, st.FillLevel, bevLine, addLine, brewLine, st.Safe, pick(flavorGet))
	}, st)
}

// handleWhen handles the WHEN method.
// RFC 2324 Section 2.1.4: "When coffee is poured, and milk is offered,
// it is necessary for the holder of the recipient of milk to say 'when'
// at the time when sufficient milk has been introduced into the coffee.
// For this purpose, the 'WHEN' method has been added to HTCPCP.
// Enough? Say WHEN."
func (s *Server) handleWhen(w http.ResponseWriter, r *http.Request, pot *Pot) {
	st, milkAmount := pot.SayWhen()
	s.Metrics.Record("WHEN", r.URL.Path, 200, r.RemoteAddr, nil)

	jsonData := map[string]interface{}{
		"status":            st,
		"milk_dispensed_ml": milkAmount,
		"message":           fmt.Sprintf("WHEN acknowledged. %.0fml of milk dispensed before halt.", milkAmount),
		"rfc_reference":     "RFC 2324 Section 2.1.4: Enough? Say WHEN.",
	}
	s.respond(w, r, 200, func() string {
		return fmt.Sprintf(`
  WHEN — Milk Halted
  ══════════════════

  "Enough? Say WHEN." — RFC 2324 §2.1.4

  Pot:            pot-%d
  Milk Dispensed: %.0fml
  State:          %s
  Temperature:    %.1f°C (%s)

`, st.ID, milkAmount, st.State, st.Temperature, st.TempLabel)
	}, jsonData)
}

// handlePropfind returns metadata about the brew.
// RFC 2324 Section 2.1.3: "If a cup of coffee is data, metadata about
// the brewed resource is discovered using the PROPFIND method."
func (s *Server) handlePropfind(w http.ResponseWriter, r *http.Request, pot *Pot) {
	st := pot.Status()
	s.Metrics.Record("PROPFIND", r.URL.Path, 200, r.RemoteAddr, nil)

	props := map[string]interface{}{
		"pot":                 st,
		"protocol":            "HTCPCP/1.0",
		"rfc":                 "RFC 2324",
		"rfc_extension":       "RFC 7168 (TEA)",
		"author":              "Larry Masinter",
		"safe":                "if-user-awake",
		"content_type":        contentTypeForPot(pot.Type),
		"available_additions": availableAdditions(),
		"server":              "BrewOps/1.0.0",
	}

	s.respond(w, r, 200, func() string {
		return fmt.Sprintf(`
  PROPFIND — Brew Metadata
  ════════════════════════

  "If a cup of coffee is data, metadata about
   the brewed resource is discovered using the
   PROPFIND method." — RFC 2324 §2.1.3

  Pot:          pot-%d (%s)
  State:        %s
  Temperature:  %.1f°C (%s)
  Protocol:     HTCPCP/1.0
  Content-Type: %s
  Author:       Larry Masinter
  Server:       BrewOps/1.0.0
  Safe:         if-user-awake

  Additions Available:
    Milk:    Cream, Half-and-half, Whole-milk, Part-Skim, Skim, Non-Dairy
    Syrup:   Vanilla, Almond, Raspberry, Chocolate
    Alcohol: Whisky, Rum, Kahlua, Aquavit
    Sugar:   Sugar, Xylitol, Stevia

`, st.ID, st.Type, st.State, st.Temperature, st.TempLabel,
			contentTypeForPot(pot.Type))
	}, props)
}

// handleTeaVariety handles /tea/{variety} for RFC 7168 compliance.
func (s *Server) handleTeaVariety(w http.ResponseWriter, r *http.Request) {
	// Tea is brewed in the teapot (pot-2)
	pot := s.Fleet.GetPot(2)
	if pot == nil {
		s.jsonError(w, r, 500, "Teapot not found in fleet. This is a configuration catastrophe.")
		return
	}

	// Extract variety from path
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 2 {
		s.respond300(w, r, pot)
		return
	}

	variety := TeaVariety(parts[1])
	if !ValidTeaVarieties[variety] {
		s.jsonError(w, r, 404, fmt.Sprintf("Tea variety '%s' not found. Try: darjeeling, earl-grey, peppermint, green-tea, chamomile, oolong.", parts[1]))
		return
	}

	if r.Method != "BREW" && r.Method != "POST" {
		s.handleGet(w, r, pot)
		return
	}

	additions := parseAdditions(r.Header.Get("Accept-Additions"))
	var validAdds []Addition
	var addStrings []string
	for _, a := range additions {
		if _, ok := ValidAdditions[a]; ok {
			validAdds = append(validAdds, a)
			addStrings = append(addStrings, string(a))
		}
	}

	status, errKind := pot.StartBrew(BeverageTea, variety, validAdds)
	if errKind == "busy" {
		s.Metrics.Record(r.Method, r.URL.Path, 503, r.RemoteAddr, nil)
		s.jsonError(w, r, 503, "Teapot is busy. Patience is a virtue, especially with tea.")
		return
	}

	s.Metrics.Record(r.Method, r.URL.Path, 200, r.RemoteAddr, addStrings)
	jsonData := map[string]interface{}{
		"status":  "steeping",
		"pot":     status,
		"variety": variety,
		"message": fmt.Sprintf("Now steeping %s in Her Majesty's Teapot.", variety),
	}
	s.respond(w, r, 200, func() string {
		return fmt.Sprintf(`
  Tea Steeping
  ════════════

        ╭───╮
        │ ~ │
        │~~~│
    ╭───╯   ╰─╮
    │  TEAPOT  │
    ╰──────╯───╯

  Variety:     %s
  Pot:         pot-%d (Her Majesty's Teapot)
  State:       steeping
  Temperature: %.1f°C (%s)

  %s

`, variety, status.ID, status.Temperature, status.TempLabel, pick(flavorTea))
	}, jsonData)
}

// handleCoffeeRoot handles the coffee: URI scheme root.
func (s *Server) handleCoffeeRoot(w http.ResponseWriter, r *http.Request) {
	s.Metrics.Record(r.Method, r.URL.Path, 200, r.RemoteAddr, nil)
	jsonData := map[string]interface{}{
		"protocol":   "HTCPCP/1.0",
		"uri_scheme": "coffee:",
		"reference":  "RFC 2324 Section 3",
		"pots":       s.Fleet.AllStatus(),
		"message":    "Welcome to BrewOps. The world's first production-grade HTCPCP/1.0 implementation.",
	}
	s.respond(w, r, 200, func() string {
		host := inferHost(r)
		pots := s.Fleet.AllStatus()
		potLines := ""
		for _, p := range pots {
			icon := "☕"
			if p.Type == PotTypeTeapot {
				icon = "🫖"
			}
			potLines += fmt.Sprintf("    %s pot-%d  %-12s  %-10s  %.1f°C\n",
				icon, p.ID, p.Type, p.State, p.Temperature)
		}
		return fmt.Sprintf(`
  BrewOps — HTCPCP/1.0
  ═════════════════════

  %s

  Protocol:  HTCPCP/1.0 (RFC 2324 + RFC 7168)
  Server:    BrewOps/1.0.0

  Fleet:
%s
  Try:
    curl -X BREW %s/pot -H 'Content-Type: message/coffeepot' -d 'start'
    curl -X BREW %s/pot-2 -H 'Content-Type: message/coffeepot' -d 'start'
    curl -X BREW %s/tea/earl-grey -H 'Content-Type: message/teapot' -d 'start'

  BREW /pot auto-creates a new pot for you. No ID needed.
  (Every 5th pot is a surprise teapot.)

  Dashboard: %s/dashboard

`, pick(flavorWelcome), potLines, host, host, host, host)
	}, jsonData)
}

// handleStatus returns status for all pots.
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	s.Metrics.Record("GET", "/status", 200, r.RemoteAddr, nil)
	stats := s.Metrics.Stats()
	pots := s.Fleet.AllStatus()
	jsonData := map[string]interface{}{
		"pots":  pots,
		"stats": stats,
	}
	s.respond(w, r, 200, func() string {
		potLines := ""
		for _, p := range pots {
			icon := "☕"
			if p.Type == PotTypeTeapot {
				icon = "🫖"
			}
			potLines += fmt.Sprintf("    %s pot-%d  %-12s  %-10s  %5.1f°C  %-35s  Fill: %d%%\n",
				icon, p.ID, p.Type, p.State, p.Temperature, p.TempLabel, p.FillLevel)
		}
		return fmt.Sprintf(`
  Fleet Status
  ════════════

%s
  Global Stats
  ────────────
  Total Brews:      %d
  Total 418s:       %d
  418 Rate:         %.1f%%
  Unique Brewers:   %d
  Caffeine (mg):    %.0f
  Popular Addition: %s
  Brew Uptime:      %.2f%%
  Spills/Quarter:   %d
  DoCS Attacks:     %d

`, potLines, stats.TotalBrews, stats.Total418s, stats.Rate418,
			stats.UniqueBrewers, stats.CaffeineDispensed, stats.MostPopularAdd,
			stats.BrewUptime, stats.SpillsThisQuarter, stats.DoCSAttacks)
	}, jsonData)
}

// handleSSE streams events to the dashboard via Server-Sent Events.
func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", 500)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Send recent events first
	recent := s.Metrics.RecentEvents(30)
	for _, event := range recent {
		data, _ := json.Marshal(event)
		fmt.Fprintf(w, "data: %s\n\n", data)
	}

	// Send current stats
	statsData := s.Metrics.StatsJSON()
	fmt.Fprintf(w, "event: stats\ndata: %s\n\n", statsData)

	// Send current pot status
	potData, _ := json.Marshal(s.Fleet.AllStatus())
	fmt.Fprintf(w, "event: pots\ndata: %s\n\n", potData)

	flusher.Flush()

	// Subscribe to new events
	ch := s.Metrics.Subscribe()
	defer s.Metrics.Unsubscribe(ch)

	// Also send periodic stats/pot updates
	ticker := newSafeTimer(3 * 1000) // Every 3 seconds
	defer ticker.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-ch:
			if !ok {
				return
			}
			data, _ := json.Marshal(event)
			fmt.Fprintf(w, "data: %s\n\n", data)

			// Also send updated stats
			statsData := s.Metrics.StatsJSON()
			fmt.Fprintf(w, "event: stats\ndata: %s\n\n", statsData)

			flusher.Flush()
		case <-ticker.C:
			// Periodic pot status update
			potData, _ := json.Marshal(s.Fleet.AllStatus())
			fmt.Fprintf(w, "event: pots\ndata: %s\n\n", potData)

			statsData := s.Metrics.StatsJSON()
			fmt.Fprintf(w, "event: stats\ndata: %s\n\n", statsData)

			flusher.Flush()
		}
	}
}

// handleStats returns current global stats.
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	s.jsonResponse(w, 200, s.Metrics.Stats())
}

// handleHealth is a health check endpoint for deployment platforms.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	fmt.Fprintf(w, `{"status":"brewing","protocol":"HTCPCP/1.0","uptime":"99.97%%"}`)
}

// ======================================================================
// Response helpers
// ======================================================================

// respond418 sends the legendary 418 I'm a Teapot response.
// This is the crown jewel. The reason we're all here.
// Returns plain text so the ASCII teapot renders properly in terminals.
// Per RFC 2324 §2.3.2: "The resulting entity body MAY be short and stout."
func (s *Server) respond418(w http.ResponseWriter, r *http.Request, pot *Pot) {
	// If the client wants JSON (e.g., the dashboard JS), give them JSON.
	accept := r.Header.Get("Accept")
	if strings.Contains(accept, "application/json") {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Teapot-Mood", "unimpressed")
		w.WriteHeader(418)
		body := map[string]interface{}{
			"error":      418,
			"status":     "I'm a teapot",
			"message":    "The requested pot is a teapot and cannot brew coffee.",
			"rfc":        "RFC 2324 Section 2.3.2",
			"pot_id":     pot.ID,
			"pot_type":   pot.Type,
			"suggestion": "Try pot-0, pot-1, or pot-3 for coffee. Or embrace tea with BREW /tea/earl-grey",
		}
		out, _ := json.MarshalIndent(body, "", "  ")
		w.Write(out)
		w.Write([]byte("\n"))
		return
	}

	// For everyone else (curl, browsers, etc.): beautiful plain text.
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Teapot-Mood", "unimpressed")
	w.WriteHeader(418)

	fmt.Fprintf(w, `
  418 I'm a Teapot
  ════════════════

        ╭───╮
        │   │
        │   │
    ╭───╯   ╰─╮
    │  TEAPOT  │
    ╰──────╯───╯

  "I'm short and stout.
   Here is my handle, here is my spout.
   Tip me over, pour me out."

  The requested pot (pot-%d) is a teapot and CANNOT brew coffee.

  %s

  RFC 2324 §2.3.2:
    "Any attempt to brew coffee with a teapot should result in
     the error code '418 I'm a teapot'. The resulting entity
     body MAY be short and stout."

  Suggestion: Try BREW /pot to get a new pot (might be a teapot too though).
              Or embrace tea: curl -X BREW %s/tea/earl-grey \
                -H 'Content-Type: message/teapot' -d 'start'

`, pot.ID, pick(flavor418), inferHost(r))
}

// respond300 sends Multiple Options with Alternates header per RFC 7168.
func (s *Server) respond300(w http.ResponseWriter, r *http.Request, pot *Pot) {
	alternates := `{"/tea/darjeeling" {type message/teapot}}, ` +
		`{"/tea/earl-grey" {type message/teapot}}, ` +
		`{"/tea/peppermint" {type message/teapot}}, ` +
		`{"/tea/green-tea" {type message/teapot}}, ` +
		`{"/tea/chamomile" {type message/teapot}}, ` +
		`{"/tea/oolong" {type message/teapot}}`

	w.Header().Set("Alternates", alternates)

	body := map[string]interface{}{
		"status":  300,
		"message": "Multiple tea varieties available. Please select one.",
		"rfc":     "RFC 7168 Section 2.1.1",
		"varieties": []map[string]string{
			{"uri": "/tea/darjeeling", "name": "Darjeeling", "origin": "India"},
			{"uri": "/tea/earl-grey", "name": "Earl Grey", "origin": "England"},
			{"uri": "/tea/peppermint", "name": "Peppermint", "origin": "Herbal"},
			{"uri": "/tea/green-tea", "name": "Green Tea", "origin": "China/Japan"},
			{"uri": "/tea/chamomile", "name": "Chamomile", "origin": "Herbal"},
			{"uri": "/tea/oolong", "name": "Oolong", "origin": "China"},
		},
	}

	host := inferHost(r)
	s.respond(w, r, 300, func() string {
		return fmt.Sprintf(`
  300 Multiple Options — Tea Menu
  ═══════════════════════════════

  RFC 7168 §2.1.1: Please select a tea variety.

    Darjeeling   — curl -X BREW %s/tea/darjeeling  -H 'Content-Type: message/teapot' -d 'start'
    Earl Grey    — curl -X BREW %s/tea/earl-grey    -H 'Content-Type: message/teapot' -d 'start'
    Peppermint   — curl -X BREW %s/tea/peppermint   -H 'Content-Type: message/teapot' -d 'start'
    Green Tea    — curl -X BREW %s/tea/green-tea    -H 'Content-Type: message/teapot' -d 'start'
    Chamomile    — curl -X BREW %s/tea/chamomile    -H 'Content-Type: message/teapot' -d 'start'
    Oolong       — curl -X BREW %s/tea/oolong       -H 'Content-Type: message/teapot' -d 'start'

`, host, host, host, host, host, host)
	}, body)
}

// respondNotAcceptable sends 406 for invalid additions.
func (s *Server) respondNotAcceptable(w http.ResponseWriter, r *http.Request, addition Addition) {
	body := map[string]interface{}{
		"error":               406,
		"status":              "Not Acceptable",
		"message":             fmt.Sprintf("Addition '%s' is not recognized by HTCPCP/1.0.", addition),
		"rfc":                 "RFC 2324 Section 2.3.1",
		"available_additions": availableAdditions(),
	}

	s.respondErr(w, r, 406, fmt.Sprintf(`Addition '%s' is not recognized by HTCPCP/1.0.

  RFC 2324 §2.3.1: "In practice, most automated coffee pots
  cannot currently provide additions."

  Available Additions:
    Milk:    Cream, Half-and-half, Whole-milk, Part-Skim, Skim, Non-Dairy
    Syrup:   Vanilla, Almond, Raspberry, Chocolate
    Alcohol: Whisky, Rum, Kahlua, Aquavit
    Sugar:   Sugar, Xylitol, Stevia`, addition), body)
}

// wantsJSON returns true if the client explicitly asks for JSON.
// Browsers and the dashboard JS send Accept: application/json.
// curl and terminals don't, so they get pretty plain text.
func wantsJSON(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	return strings.Contains(accept, "application/json")
}

func (s *Server) jsonResponse(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	out, _ := json.MarshalIndent(data, "", "  ")
	w.Write(out)
	w.Write([]byte("\n"))
}

func (s *Server) jsonError(w http.ResponseWriter, r *http.Request, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	out, _ := json.MarshalIndent(map[string]interface{}{
		"error":   status,
		"message": message,
	}, "", "  ")
	w.Write(out)
	w.Write([]byte("\n"))
}

// textResponse writes a pretty plain-text response for terminal users.
func (s *Server) textResponse(w http.ResponseWriter, status int, text string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	w.Write([]byte(text))
}

// respond sends either plain text or JSON depending on the client.
func (s *Server) respond(w http.ResponseWriter, r *http.Request, status int, textFn func() string, data interface{}) {
	if wantsJSON(r) {
		s.jsonResponse(w, status, data)
		return
	}
	s.textResponse(w, status, textFn())
}

// respondErr sends either plain text or JSON error depending on the client.
func (s *Server) respondErr(w http.ResponseWriter, r *http.Request, status int, textMsg string, data interface{}) {
	if wantsJSON(r) {
		if data != nil {
			s.jsonResponse(w, status, data)
		} else {
			s.jsonError(w, r, status, textMsg)
		}
		return
	}
	s.textResponse(w, status, "\n  "+fmt.Sprintf("%d", status)+" Error\n  "+strings.Repeat("─", len(textMsg)+2)+"\n\n  "+textMsg+"\n\n")
}

// ======================================================================
// Parsing helpers
// ======================================================================

func parsePotID(path string) (int, error) {
	// Handle /pot-0, /pot-1, /pot-0/something, etc.
	path = strings.TrimPrefix(path, "/pot-")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) == 0 {
		return 0, fmt.Errorf("no pot ID")
	}
	return strconv.Atoi(parts[0])
}

func inferHost(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if fwd := r.Header.Get("X-Forwarded-Proto"); fwd != "" {
		scheme = fwd
	}
	return scheme + "://" + r.Host
}

func parseAdditions(header string) []Addition {
	if header == "" {
		return nil
	}
	var additions []Addition
	parts := strings.Split(header, ",")
	for _, part := range parts {
		// Handle parameters like "Cream;q=0.9"
		name := strings.TrimSpace(strings.SplitN(part, ";", 2)[0])
		if name != "" && name != "*" {
			additions = append(additions, Addition(name))
		}
	}
	return additions
}

func determineBeverage(contentType, path string) (BeverageType, TeaVariety) {
	// Check content type first
	ct := strings.ToLower(strings.TrimSpace(contentType))
	if ct == "message/teapot" {
		// Check path for variety
		for variety := range ValidTeaVarieties {
			if strings.Contains(path, string(variety)) {
				return BeverageTea, variety
			}
		}
		return BeverageTea, TeaNone
	}

	// Check path for hints
	pathLower := strings.ToLower(path)
	if strings.Contains(pathLower, "espresso") {
		return BeverageEspresso, TeaNone
	}
	if strings.Contains(pathLower, "tea") {
		for variety := range ValidTeaVarieties {
			if strings.Contains(pathLower, string(variety)) {
				return BeverageTea, variety
			}
		}
		return BeverageTea, TeaNone
	}

	// Default: coffee
	return BeverageCoffee, TeaNone
}

func contentTypeForPot(potType PotType) string {
	if potType == PotTypeTeapot {
		return "message/teapot"
	}
	return "message/coffeepot"
}

func availableAdditions() map[string][]string {
	return map[string][]string{
		"milk":    {"Cream", "Half-and-half", "Whole-milk", "Part-Skim", "Skim", "Non-Dairy"},
		"syrup":   {"Vanilla", "Almond", "Raspberry", "Chocolate"},
		"alcohol": {"Whisky", "Rum", "Kahlua", "Aquavit"},
		"sugar":   {"Sugar", "Xylitol", "Stevia"},
	}
}

func teapotASCII() string {
	return `
        ╭───╮
        │   │    I'm short and stout.
        │   │    Here is my handle,
    ╭───╯   ╰─╮ here is my spout.
    │  TEAPOT  │
    ╰──────╯───╯ Tip me over, pour me out.
`
}

// safeTimer wraps time.Ticker for SSE periodic updates.
type safeTimer struct {
	C    <-chan time.Time
	stop func()
}

func newSafeTimer(ms int) *safeTimer {
	ticker := time.NewTicker(time.Duration(ms) * time.Millisecond)
	return &safeTimer{
		C:    ticker.C,
		stop: ticker.Stop,
	}
}

func (t *safeTimer) Stop() {
	t.stop()
}
