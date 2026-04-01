package htcpcp

import "time"

// PotState represents the current state of a coffee/tea pot.
// RFC 2324 does not define explicit states, but a production-grade
// implementation obviously needs a finite state machine. Obviously.
type PotState string

const (
	StateIdle     PotState = "idle"
	StateGrinding PotState = "grinding"
	StateBrewing  PotState = "brewing"
	StatePouring  PotState = "pouring"
	StateReady    PotState = "ready"
	StateCooling  PotState = "cooling"
)

// BeverageType represents what's being brewed.
type BeverageType string

const (
	BeverageNone     BeverageType = ""
	BeverageCoffee   BeverageType = "coffee"
	BeverageEspresso BeverageType = "espresso"
	BeverageTea      BeverageType = "tea"
)

// TeaVariety represents available tea varieties per RFC 7168.
type TeaVariety string

const (
	TeaNone       TeaVariety = ""
	TeaDarjeeling TeaVariety = "darjeeling"
	TeaEarlGrey   TeaVariety = "earl-grey"
	TeaPeppermint TeaVariety = "peppermint"
	TeaGreenTea   TeaVariety = "green-tea"
	TeaChamomile  TeaVariety = "chamomile"
	TeaOolong     TeaVariety = "oolong"
)

var ValidTeaVarieties = map[TeaVariety]bool{
	TeaDarjeeling: true,
	TeaEarlGrey:   true,
	TeaPeppermint: true,
	TeaGreenTea:   true,
	TeaChamomile:  true,
	TeaOolong:     true,
}

// PotType determines whether the pot is a coffee pot or a teapot.
// Per RFC 2324 Section 2.3.2: "Any attempt to brew coffee with a
// teapot should result in the error code 418 I'm a teapot."
type PotType string

const (
	PotTypeCoffee PotType = "coffee-pot"
	PotTypeTeapot PotType = "teapot"
)

// Addition types per RFC 2324 Section 2.2.2.1 and RFC 7168 Section 2.2.1.
type Addition string

// Milk types
const (
	AdditionCream       Addition = "Cream"
	AdditionHalfAndHalf Addition = "Half-and-half"
	AdditionWholeMilk   Addition = "Whole-milk"
	AdditionPartSkim    Addition = "Part-Skim"
	AdditionSkim        Addition = "Skim"
	AdditionNonDairy    Addition = "Non-Dairy"
)

// Syrup types
const (
	AdditionVanilla   Addition = "Vanilla"
	AdditionAlmond    Addition = "Almond"
	AdditionRaspberry Addition = "Raspberry"
	AdditionChocolate Addition = "Chocolate"
)

// Alcohol types
const (
	AdditionWhisky  Addition = "Whisky"
	AdditionRum     Addition = "Rum"
	AdditionKahlua  Addition = "Kahlua"
	AdditionAquavit Addition = "Aquavit"
)

// Sugar types (RFC 7168)
const (
	AdditionSugar   Addition = "Sugar"
	AdditionXylitol Addition = "Xylitol"
	AdditionStevia  Addition = "Stevia"
)

// ValidAdditions is the complete set of RFC-compliant additions.
var ValidAdditions = map[Addition]string{
	AdditionCream:       "milk",
	AdditionHalfAndHalf: "milk",
	AdditionWholeMilk:   "milk",
	AdditionPartSkim:    "milk",
	AdditionSkim:        "milk",
	AdditionNonDairy:    "milk",
	AdditionVanilla:     "syrup",
	AdditionAlmond:      "syrup",
	AdditionRaspberry:   "syrup",
	AdditionChocolate:   "syrup",
	AdditionWhisky:      "alcohol",
	AdditionRum:         "alcohol",
	AdditionKahlua:      "alcohol",
	AdditionAquavit:     "alcohol",
	AdditionSugar:       "sugar",
	AdditionXylitol:     "sugar",
	AdditionStevia:      "sugar",
}

// BrewRequest represents a parsed BREW/POST request.
type BrewRequest struct {
	PotID       int
	Command     string // "start" or "stop"
	ContentType string // "message/coffeepot" or "message/teapot"
	Additions   []Addition
	TeaVariety  TeaVariety
	Beverage    BeverageType
}

// PotStatus represents the current state of a pot for API responses.
type PotStatus struct {
	ID          int          `json:"id"`
	Type        PotType      `json:"type"`
	State       PotState     `json:"state"`
	Beverage    BeverageType `json:"beverage,omitempty"`
	TeaVariety  TeaVariety   `json:"tea_variety,omitempty"`
	Temperature float64      `json:"temperature_celsius"`
	TempLabel   string       `json:"temperature_label"`
	FillLevel   int          `json:"fill_level_percent"`
	Additions   []Addition   `json:"additions,omitempty"`
	BrewStarted *time.Time   `json:"brew_started,omitempty"`
	BrewElapsed string       `json:"brew_elapsed,omitempty"`
	Safe        string       `json:"safe"`
}

// TemperatureLabel returns a human-readable (and legally concerning)
// temperature description.
// TemperatureLabel returns a human-readable (and legally concerning)
// temperature description. Multiple options per range for variety.
func TemperatureLabel(tempC float64) string {
	switch {
	case tempC < 20:
		return pickLabel([]string{
			"Room Temperature (Disappointing)",
			"Ambient (Why Did You Even Brew)",
			"Cold (This Is Just Sad)",
		})
	case tempC < 40:
		return pickLabel([]string{
			"Tepid (Why Bother)",
			"Lukewarm (Commitment Issues)",
			"Barely Warm (Try Harder)",
		})
	case tempC < 60:
		return pickLabel([]string{
			"Warm (Acceptable)",
			"Getting There (Patience)",
			"Warm-ish (Could Be Warmer)",
		})
	case tempC < 75:
		return pickLabel([]string{
			"Hot (Careful)",
			"Hot (Don't Be A Hero)",
			"Properly Hot (Finally)",
		})
	case tempC < 85:
		return pickLabel([]string{
			"Very Hot (Caution Advised)",
			"Very Hot (Sip, Don't Gulp)",
			"Dangerously Drinkable",
		})
	case tempC < 95:
		return pickLabel([]string{
			"Scalding (Lawsuit Pending)",
			"Scalding (Legal Has Questions)",
			"Extremely Hot (Regret Imminent)",
		})
	default:
		return pickLabel([]string{
			"Lawsuit Hot (Legal Has Been Notified)",
			"Surface of the Sun (Approximately)",
			"Beyond Hot (Insurance Voided)",
			"Thermonuclear (Handle At Own Risk)",
		})
	}
}

func pickLabel(labels []string) string {
	// Use time-based selection for variety without importing math/rand
	idx := int(time.Now().UnixNano()/1000) % len(labels)
	return labels[idx]
}
