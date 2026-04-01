package metrics

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"strings"
	"sync"
	"time"
)

// Severity levels for brew incidents. Because every coffee event
// deserves an incident severity classification.
type Severity string

const (
	SeverityCritical Severity = "CRITICAL"
	SeverityWarning  Severity = "WARNING"
	SeverityInfo     Severity = "INFO"
	SeveritySuccess  Severity = "SUCCESS"
)

// Event represents a single incident in the BrewOps timeline.
type Event struct {
	ID        int      `json:"id"`
	Timestamp string   `json:"timestamp"`
	Severity  Severity `json:"severity"`
	IP        string   `json:"ip"`
	Method    string   `json:"method"`
	Path      string   `json:"path"`
	Status    int      `json:"status"`
	Message   string   `json:"message"`
}

// GlobalStats holds the aggregate metrics across all operations.
type GlobalStats struct {
	TotalBrews        int     `json:"total_brews"`
	Total418s         int     `json:"total_418s"`
	TotalRequests     int     `json:"total_requests"`
	UniqueBrewers     int     `json:"unique_brewers"`
	CaffeineDispensed float64 `json:"caffeine_dispensed_mg"`
	MostPopularAdd    string  `json:"most_popular_addition"`
	TeapotIncidents   int     `json:"teapot_incidents_today"`
	BrewUptime        float64 `json:"brew_uptime_percent"`
	SpillsThisQuarter int     `json:"spills_this_quarter"`
	DoCSAttacks       int     `json:"docs_attacks"`
	Rate418           float64 `json:"rate_418_percent"`
	StartedAt         string  `json:"started_at"`
}

// Collector gathers metrics and manages the event ring buffer.
type Collector struct {
	mu sync.RWMutex

	events    []Event
	maxEvents int
	eventID   int

	totalBrews    int
	total418s     int
	totalRequests int
	uniqueIPs     map[string]bool
	additionCount map[string]int
	caffeineTotal float64
	startedAt     time.Time
	docsAttacks   int

	// Rate tracking for DoCS (Denial of Coffee Service) detection.
	// Maps masked IP -> list of BREW timestamps in the last 30 seconds.
	brewTimes map[string][]time.Time

	// SSE subscribers
	subscribers map[chan Event]bool
	subMu       sync.RWMutex
}

const (
	docsWindowSec = 30 // seconds
	docsThreshold = 10 // brews per window to trigger DoCS
)

// NewCollector creates a new metrics collector.
func NewCollector() *Collector {
	return &Collector{
		events:        make([]Event, 0, 200),
		maxEvents:     200,
		uniqueIPs:     make(map[string]bool),
		additionCount: make(map[string]int),
		brewTimes:     make(map[string][]time.Time),
		subscribers:   make(map[chan Event]bool),
		startedAt:     time.Now(),
	}
}

// Record logs a new event and broadcasts it to all SSE subscribers.
func (c *Collector) Record(method, path string, status int, ip string, additions []string) {
	c.mu.Lock()

	c.eventID++
	c.totalRequests++

	maskedIP := maskIP(ip)

	severity, message := c.generateMessage(method, path, status, additions)

	if status == 200 && (method == "BREW" || method == "POST") {
		c.totalBrews++
		// Average espresso: 63mg, coffee: 95mg, tea: 47mg
		if strings.Contains(path, "espresso") || strings.Contains(message, "Espresso") {
			c.caffeineTotal += 63
		} else if strings.Contains(path, "tea") || strings.Contains(message, "tea") {
			c.caffeineTotal += 47
		} else {
			c.caffeineTotal += 95
		}
		for _, a := range additions {
			c.additionCount[a]++
		}
	}

	if status == 418 {
		c.total418s++
	}

	c.uniqueIPs[maskedIP] = true

	event := Event{
		ID:        c.eventID,
		Timestamp: time.Now().UTC().Format("15:04:05"),
		Severity:  severity,
		IP:        maskedIP,
		Method:    method,
		Path:      path,
		Status:    status,
		Message:   message,
	}

	// Ring buffer
	if len(c.events) >= c.maxEvents {
		c.events = c.events[1:]
	}
	c.events = append(c.events, event)

	c.mu.Unlock()

	// Broadcast to SSE subscribers
	c.subMu.RLock()
	for ch := range c.subscribers {
		select {
		case ch <- event:
		default:
			// Subscriber too slow, drop event
		}
	}
	c.subMu.RUnlock()
}

// Subscribe returns a channel that receives new events.
func (c *Collector) Subscribe() chan Event {
	ch := make(chan Event, 50)
	c.subMu.Lock()
	c.subscribers[ch] = true
	c.subMu.Unlock()
	return ch
}

// Unsubscribe removes a subscriber channel.
func (c *Collector) Unsubscribe(ch chan Event) {
	c.subMu.Lock()
	delete(c.subscribers, ch)
	c.subMu.Unlock()
	close(ch)
}

// RecentEvents returns the last n events for initial page load.
func (c *Collector) RecentEvents(n int) []Event {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if n > len(c.events) {
		n = len(c.events)
	}
	result := make([]Event, n)
	copy(result, c.events[len(c.events)-n:])
	return result
}

// Stats returns the current global statistics.
func (c *Collector) Stats() GlobalStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	mostPopular := "None yet"
	maxCount := 0
	for add, count := range c.additionCount {
		if count > maxCount {
			maxCount = count
			mostPopular = add
		}
	}

	rate418 := 0.0
	if c.totalRequests > 0 {
		rate418 = float64(c.total418s) / float64(c.totalRequests) * 100
	}

	return GlobalStats{
		TotalBrews:        c.totalBrews,
		Total418s:         c.total418s,
		TotalRequests:     c.totalRequests,
		UniqueBrewers:     len(c.uniqueIPs),
		CaffeineDispensed: c.caffeineTotal,
		MostPopularAdd:    mostPopular,
		TeapotIncidents:   c.total418s,
		BrewUptime:        99.97, // Always 99.97%. We're that good.
		SpillsThisQuarter: 3,     // Exactly 3. Always 3.
		DoCSAttacks:       c.docsAttacks,
		Rate418:           rate418,
		StartedAt:         c.startedAt.UTC().Format(time.RFC3339),
	}
}

// StatsJSON returns stats as JSON bytes.
func (c *Collector) StatsJSON() []byte {
	stats := c.Stats()
	data, _ := json.Marshal(stats)
	return data
}

// CheckDoCS checks if the given IP is mounting a Denial of Coffee Service
// attack (more than 10 BREW requests in 30 seconds). Records a BREW timestamp
// and returns (isAttack, brewCount, docsTotal).
//
// RFC 2324 §7: "Unmoderated access to unprotected coffee pots from Internet
// users might lead to several kinds of denial of coffee service attacks."
func (c *Collector) CheckDoCS(ip string) (bool, int, int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	masked := maskIP(ip)
	now := time.Now()
	cutoff := now.Add(-time.Duration(docsWindowSec) * time.Second)

	// Prune old timestamps
	times := c.brewTimes[masked]
	pruned := make([]time.Time, 0, len(times))
	for _, t := range times {
		if t.After(cutoff) {
			pruned = append(pruned, t)
		}
	}
	pruned = append(pruned, now)
	c.brewTimes[masked] = pruned

	count := len(pruned)
	isAttack := count > docsThreshold

	if isAttack {
		c.docsAttacks++
	}

	return isAttack, count, c.docsAttacks
}

// DoCSCount returns the current DoCS attack count.
func (c *Collector) DoCSCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.docsAttacks
}

// maskIP replaces the last two octets for privacy.
// Because even in a joke protocol, privacy matters.
func maskIP(ip string) string {
	// Handle ip:port format
	host, _, err := net.SplitHostPort(ip)
	if err != nil {
		host = ip
	}

	// Handle IPv6 loopback
	if host == "::1" || host == "127.0.0.1" || host == "localhost" {
		return "127.0.xx.xx"
	}

	parts := strings.Split(host, ".")
	if len(parts) == 4 {
		return parts[0] + "." + parts[1] + ".xx.xx"
	}

	// IPv6 or weird format, just mask the end
	if len(host) > 8 {
		return host[:8] + "::xxxx"
	}
	return host
}

// ======================================================================
// Funny message templates. The soul of BrewOps.
// ======================================================================

// All templates use {COUNT}, {VARIETY}, {ADDITIONS}, {TOD}, {POTID} as placeholders.

var messages418 = []string{
	// Classic incident reports
	"Incident #{COUNT}. Teapot remains unimpressed.",
	"This is attempt #{COUNT} globally. The teapot is not going to change its mind.",
	"Incident report filed. Category: operator error. Subcategory: teapot confusion.",
	"Filing JIRA ticket TEA-{COUNT}: 'Pot refuses to brew coffee.' Status: Won't Fix.",
	"Another one. Paging the SRE on-call... oh wait, it's just a teapot.",
	// RFC references
	"Per RFC 2324 \u00a72.3.2: 'The resulting entity body MAY be short and stout.' It is.",
	"RFC 2324 is very clear on this. Section 2.3.2. Read it. Weep.",
	"Compliance check: 418 correctly returned per RFC 2324. The system works.",
	// Deadpan
	"Escalating to On-Brew Engineer. ETA: never. It's a teapot.",
	"Postmortem scheduled. Root cause: it's a teapot. Resolution: use a coffee pot.",
	"Have you tried turning the teapot off and on again? Still a teapot.",
	"The teapot has spoken. Appeal denied.",
	// Existential
	"The teapot was a teapot yesterday. It's a teapot today. It'll be a teapot tomorrow.",
	"You can't brew coffee in a teapot. This is not a limitation. It's an identity.",
	"Some pots are coffee pots. Some pots are teapots. This one knows what it is.",
	"The teapot doesn't dream of being a coffee pot. It's at peace.",
	// Dramatic
	"A hush fell over the data center. Someone tried to brew coffee in a teapot. Again.",
	"Breaking: Local pot refuses coffee request. Cites RFC compliance. More at 11.",
	"The audacity. The sheer, unbridled audacity. It's a teapot.",
	"Somewhere, Larry Masinter smiles. The protocol is working as intended.",
	// Technical
	"Stack trace: main() -> brew() -> validatePot() -> TEAPOT. That's it. That's the trace.",
	"Return code 418: not a bug, it's a feature. Literally. It's in the RFC.",
	"Error budget: infinite. You can 418 this teapot all day. It doesn't care.",
	"Rollback rejected. The pot has always been a teapot. There is no previous state.",
}

var messagesSuccess = []string{
	// Production metrics
	"Caffeine payload deployed. Productivity impact: imminent.",
	"This is global brew #{COUNT} served by BrewOps worldwide.",
	"Another successful brew. Uptime maintained. SLA intact.",
	"Brew dispensed successfully. The internet runs on coffee.",
	// Legal humor
	"Brew complete. Handle with caution and a lawyer present.",
	"Hot liquid dispensed. Liability waiver implicit in the BREW request.",
	"Coffee served. Any resemblance to actual good coffee is coincidental.",
	// Decaf shade
	"Decaf was not offered as an option. What's the point? (RFC 2324 \u00a72.2.3)",
	"Full-caffeine confirmed. We don't do half measures.",
	// Nerdy
	"Brew #{COUNT}. The coffee must flow. (Dune, but for developers.)",
	"200 OK. The happiest status code, especially when coffee is involved.",
	"Deploying caffeine to production. No rollback plan. None needed.",
	"Coffee compiled successfully. Zero warnings. Zero tests. Ship it.",
	// Time-aware
	"Brew complete. The human requires fuel. The fuel has been provided.",
	"Another day, another brew. The cycle continues.",
	"Coffee: turning mass into energy since whenever E=mc2 was discovered.",
}

var messagesSuccessAdditions = []string{
	"{ADDITIONS} detected. Bold choice for {TOD}.",
	"Brew with {ADDITIONS}. Living dangerously, are we?",
	"Adding {ADDITIONS} at {TOD}. No judgment. (Some judgment.)",
	"{ADDITIONS} requested. The barista raises an eyebrow but complies.",
	"{ADDITIONS} during {TOD}? Respect.",
	"Customized brew with {ADDITIONS}. You know what you want.",
	"{ADDITIONS} added. This is a cry for help disguised as a coffee order.",
	"Brew configured: {ADDITIONS}. Your commit message should be this decisive.",
	"{ADDITIONS}. Interesting. Very interesting. The pot has opinions but keeps them.",
}

var messagesWhen = []string{
	"WHEN received. Milk addition halted.",
	"Milk flow terminated. The pouring has ceased.",
	"Enough? WHEN. (RFC 2324 \u00a72.1.4)",
	"User said 'when'. Restraint noted.",
	"WHEN acknowledged. The milk heard you. It stopped.",
	"Milk halted mid-pour. Dramatic, but effective.",
	"You said when. The milk respects boundaries.",
	"WHEN processed. Dairy operations suspended.",
	"The milk has been told. It knows when to stop now.",
	"Pour interrupted. Saving the rest for the next person with less restraint.",
}

var messagesGet = []string{
	"Someone is watching pot-{POTID}. A watched pot never boils.",
	"Status check requested. All systems nominal... probably.",
	"Monitoring request logged. Pot-{POTID} appreciates the attention.",
	"Health check: pot-{POTID} is alive. Define 'alive' for a coffee pot.",
	"Pot-{POTID} status pulled. It didn't ask to be observed, but here we are.",
	"Checking on pot-{POTID}. It's doing its best.",
	"Pot-{POTID} is being monitored. It feels perceived.",
	"GET request for pot-{POTID}. The pot has no secrets. (It's a pot.)",
	"Observability achieved. Pot-{POTID} has been thoroughly observed.",
	"Status: pot-{POTID} exists. Beyond that, define 'status'.",
	"Pot-{POTID} pinged. Response: it's a pot. What did you expect?",
}

var messages406 = []string{
	"Requested additions not available. We're a coffee pot, not a cocktail bar.",
	"The combination requested has been deemed an affront to coffee.",
	"406: Not Acceptable. Neither is that combination of additions.",
	"RFC 2324 \u00a72.3.1: 'In practice, most automated coffee pots cannot currently provide additions.'",
	"That addition doesn't exist in our inventory. Or in nature.",
	"Not Acceptable. The pot has standards. Low standards, but standards.",
	"The additions engine rejected your request. It's not personal. (It's a little personal.)",
	"406: We checked the RFC. That addition isn't in it. Sorry. (Not sorry.)",
}

var messagesTea = []string{
	"Tea brewing initiated. Very civilized.",
	"Steeping {VARIETY}. The British would approve.",
	"{VARIETY} selected. A refined choice.",
	"Kettle on. Tea in progress. Keep calm.",
	"Steeping {VARIETY}. The teapot is in its element.",
	"{VARIETY}: a beverage of culture. Unlike those coffee heathens.",
	"Tea time. The teapot was born for this moment.",
	"Brewing {VARIETY}. Pinky up, please.",
	"The teapot hums contentedly. This is what it was made for.",
	"{VARIETY} steeping. Estimated enjoyment: very high.",
	"Tea requested. Finally, someone with taste.",
	"Steeping {VARIETY}. No rush. Tea doesn't believe in sprints.",
}

func (c *Collector) generateMessage(method, path string, status int, additions []string) (Severity, string) {
	replacer := func(msg string, extra map[string]string) string {
		for k, v := range extra {
			msg = strings.ReplaceAll(msg, "{"+k+"}", v)
		}
		return msg
	}

	switch {
	case status == 429:
		// Denial of Coffee Service attack detected
		msgs := []string{
			"DoCS ATTACK DETECTED. Engaging coffee countermeasures. Decoy pots deployed.",
			"DENIAL OF COFFEE SERVICE. This is not a drill. (Well, it is. But still.)",
			"DoCS alert! Rapid brewing detected. RFC 2324 \u00a77 warned us about this.",
			"Coffee supply under siege. Activating emergency reserves.",
			"Hostile brewing activity detected. The coffee must flow.",
			"DoCS incident #{COUNT}. Paging the On-Brew SRE team.",
			"We're under attack. Someone really wants coffee. Can you blame them?",
			"Trojan grounds detected in the plumbing. (RFC 2324 \u00a77)",
			"DoCS #{COUNT}: Brew velocity exceeds safe threshold. Deploying rate limiters. Just kidding.",
			"The coffee pots are overwhelmed. Send help. Send beans.",
			"Someone is stress-testing our beverage infrastructure. Bold.",
			"Alert: caffeine throughput exceeds human consumption capacity.",
			"DoCS detected. Countermeasures: none. This is a coffee pot, not Fort Knox.",
			"Brew flood detected. The pots are multiplying. This is fine.",
			"RFC 2324 \u00a77: 'The improper use of filtration devices might admit trojan grounds.'",
			"DoCS #{COUNT}. The On-Brew Engineer has left the building. And the country.",
		}
		msg := msgs[rand.Intn(len(msgs))]
		return SeverityCritical, replacer(msg, map[string]string{
			"COUNT": fmt.Sprintf("%d", c.docsAttacks),
		})

	case status == 418:
		msg := messages418[rand.Intn(len(messages418))]
		count := fmt.Sprintf("%d", c.total418s+1)
		return SeverityCritical, replacer(msg, map[string]string{"COUNT": count})

	case status == 406:
		msg := messages406[rand.Intn(len(messages406))]
		return SeverityWarning, msg

	case status == 300:
		return SeverityInfo, "Tea menu requested. Alternates header sent with available varieties."

	case (method == "BREW" || method == "POST") && status == 200:
		if strings.Contains(path, "tea") || strings.Contains(path, "darjeeling") ||
			strings.Contains(path, "earl-grey") || strings.Contains(path, "peppermint") ||
			strings.Contains(path, "green-tea") || strings.Contains(path, "chamomile") ||
			strings.Contains(path, "oolong") {
			msg := messagesTea[rand.Intn(len(messagesTea))]
			variety := "tea"
			for _, v := range []string{"darjeeling", "earl-grey", "peppermint", "green-tea", "chamomile", "oolong"} {
				if strings.Contains(path, v) {
					variety = v
					break
				}
			}
			return SeveritySuccess, replacer(msg, map[string]string{"VARIETY": variety})
		}
		if len(additions) > 0 {
			msg := messagesSuccessAdditions[rand.Intn(len(messagesSuccessAdditions))]
			addStr := strings.Join(additions, ", ")
			return SeveritySuccess, replacer(msg, map[string]string{"ADDITIONS": addStr, "TOD": timeOfDay()})
		}
		msg := messagesSuccess[rand.Intn(len(messagesSuccess))]
		count := fmt.Sprintf("%d", c.totalBrews+1)
		return SeveritySuccess, replacer(msg, map[string]string{"COUNT": count})

	case method == "WHEN":
		msg := messagesWhen[rand.Intn(len(messagesWhen))]
		return SeverityWarning, msg

	case method == "GET":
		potID := "0"
		if idx := strings.Index(path, "/pot-"); idx >= 0 {
			rest := path[idx+5:]
			for i, ch := range rest {
				if ch < '0' || ch > '9' {
					rest = rest[:i]
					break
				}
			}
			if rest != "" {
				potID = rest
			}
		}
		msg := messagesGet[rand.Intn(len(messagesGet))]
		return SeverityInfo, replacer(msg, map[string]string{"POTID": potID})

	case method == "PROPFIND":
		return SeverityInfo, "PROPFIND: Metadata requested. The brew has properties. Who knew."

	default:
		return SeverityInfo, fmt.Sprintf("%s %s -> %d", method, path, status)
	}
}

func timeOfDay() string {
	h := time.Now().Hour()
	switch {
	case h < 6:
		return "the ungodly hours"
	case h < 9:
		return "the morning rush"
	case h < 12:
		return "mid-morning"
	case h < 14:
		return "the post-lunch slump"
	case h < 17:
		return "the afternoon"
	case h < 20:
		return "the evening (caffeine after 5pm?)"
	default:
		return "this late at night"
	}
}
