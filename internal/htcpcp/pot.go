package htcpcp

import (
	"fmt"
	"math"
	"sync"
	"time"
)

// Pot represents a virtual coffee or tea pot with a state machine.
// Each pot maintains its own temperature, fill level, brew state,
// and addition history. This is enterprise-grade beverage infrastructure.
type Pot struct {
	mu sync.RWMutex

	ID          int
	Type        PotType
	State       PotState
	Beverage    BeverageType
	TeaVariety  TeaVariety
	Temperature float64 // Celsius
	FillLevel   int     // 0-100 percent
	Additions   []Addition
	BrewStart   time.Time
	LastUpdate  time.Time
	MilkAmount  float64 // ml of milk added, for the WHEN jokes
}

// PotConfig defines the fleet of pots available.
// In a real deployment you'd read this from a ConfigMap. Obviously.
type PotConfig struct {
	ID   int
	Type PotType
	Name string
}

// DefaultPotConfigs returns the standard fleet configuration.
// pot-2 is always the teapot. This is load-bearing configuration.
var DefaultPotConfigs = []PotConfig{
	{ID: 0, Type: PotTypeCoffee, Name: "Primary Brew Unit"},
	{ID: 1, Type: PotTypeCoffee, Name: "Secondary Brew Unit"},
	{ID: 2, Type: PotTypeTeapot, Name: "Her Majesty's Teapot"},
	{ID: 3, Type: PotTypeCoffee, Name: "Emergency Backup Unit"},
}

// NewPot creates a new pot in idle state at room temperature.
func NewPot(cfg PotConfig) *Pot {
	return &Pot{
		ID:          cfg.ID,
		Type:        cfg.Type,
		State:       StateIdle,
		Temperature: 21.0, // Room temperature. Disappointing.
		FillLevel:   0,
		Additions:   []Addition{},
		LastUpdate:  time.Now(),
	}
}

// Status returns the current pot status for API responses.
func (p *Pot) Status() PotStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()

	p.simulateTemperature()

	status := PotStatus{
		ID:          p.ID,
		Type:        p.Type,
		State:       p.State,
		Beverage:    p.Beverage,
		TeaVariety:  p.TeaVariety,
		Temperature: math.Round(p.Temperature*10) / 10,
		TempLabel:   TemperatureLabel(p.Temperature),
		FillLevel:   p.FillLevel,
		Additions:   p.Additions,
		Safe:        "if-user-awake",
	}

	if !p.BrewStart.IsZero() {
		t := p.BrewStart
		status.BrewStarted = &t
		status.BrewElapsed = time.Since(p.BrewStart).Round(time.Second).String()
	}

	return status
}

// StartBrew begins the brewing process. Returns an error description if
// the pot cannot brew the requested beverage.
func (p *Pot) StartBrew(beverage BeverageType, variety TeaVariety, additions []Addition) (PotStatus, string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// THE critical check: teapots cannot brew coffee.
	// RFC 2324 Section 2.3.2
	if p.Type == PotTypeTeapot && (beverage == BeverageCoffee || beverage == BeverageEspresso) {
		return PotStatus{
			ID:    p.ID,
			Type:  p.Type,
			State: p.State,
			Safe:  "if-user-awake",
		}, "teapot"
	}

	// Coffee pots can't brew tea (they're not provisioned for it)
	if p.Type == PotTypeCoffee && beverage == BeverageTea {
		return PotStatus{
			ID:    p.ID,
			Type:  p.Type,
			State: p.State,
			Safe:  "if-user-awake",
		}, "not-tea-capable"
	}

	// Can't brew if already brewing
	if p.State != StateIdle && p.State != StateReady && p.State != StateCooling {
		return p.statusLocked(), "busy"
	}

	p.State = StateBrewing
	p.Beverage = beverage
	p.TeaVariety = variety
	p.Additions = additions
	p.BrewStart = time.Now()
	p.Temperature = 25.0 // Starting to heat
	p.FillLevel = 10
	p.MilkAmount = 0
	p.LastUpdate = time.Now()

	// Start a goroutine to simulate the brew lifecycle
	go p.brewLifecycle()

	return p.statusLocked(), ""
}

// StopBrew stops the current brew (e.g., "stop" command in request body).
func (p *Pot) StopBrew() PotStatus {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.State == StateBrewing || p.State == StatePouring {
		p.State = StateReady
		p.Temperature = 82.0
		p.FillLevel = 100
		p.LastUpdate = time.Now()
	}

	return p.statusLocked()
}

// SayWhen stops the addition of milk. "Enough? Say WHEN." -- RFC 2324 Section 2.1.4
func (p *Pot) SayWhen() (PotStatus, float64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	milkAdded := p.MilkAmount
	p.MilkAmount = 0
	p.LastUpdate = time.Now()

	return p.statusLocked(), milkAdded
}

// AddMilk adds milk to the pot. Called internally when brew has milk additions.
func (p *Pot) addMilk(amount float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.MilkAmount += amount
}

// brewLifecycle simulates the brewing process over time.
// In a real HTCPCP deployment this would control actual hardware.
// We don't have actual hardware. We have imagination and goroutines.
//
// Timeline:
//
//	Grinding: ~3s  (coffee only)
//	Brewing:  ~12s (6 ticks x 2s, temp rises, cup fills)
//	Pouring:  ~2s
//	Ready:    ~15s (enjoy it while it's hot)
//	Cooling:  ~30s (cools down to room temp)
//	Idle:     pot resets, janitor cleans up dynamic pots after ~60s
//
// Total lifecycle: about 60 seconds. Fast enough that
// the dashboard stays interesting, slow enough to watch.
func (p *Pot) brewLifecycle() {
	// Grinding phase (for coffee)
	if p.Beverage == BeverageCoffee || p.Beverage == BeverageEspresso {
		p.mu.Lock()
		p.State = StateGrinding
		p.LastUpdate = time.Now()
		p.mu.Unlock()
		time.Sleep(3 * time.Second)
	}

	// Brewing phase — temperature rises, cup fills
	p.mu.Lock()
	p.State = StateBrewing
	p.LastUpdate = time.Now()
	p.mu.Unlock()

	for i := 0; i < 6; i++ {
		time.Sleep(2 * time.Second)
		p.mu.Lock()
		p.Temperature = math.Min(96.0, p.Temperature+float64(10+i*2))
		p.FillLevel = min(100, p.FillLevel+15)
		p.LastUpdate = time.Now()
		p.mu.Unlock()
	}

	// Add milk if any milk-type additions
	hasMilk := false
	p.mu.RLock()
	for _, a := range p.Additions {
		if cat, ok := ValidAdditions[a]; ok && cat == "milk" {
			hasMilk = true
			break
		}
	}
	p.mu.RUnlock()

	if hasMilk {
		for i := 0; i < 10; i++ {
			p.addMilk(28.3)
			time.Sleep(500 * time.Millisecond)
			p.mu.RLock()
			milk := p.MilkAmount
			p.mu.RUnlock()
			if milk == 0 {
				break
			}
		}
	}

	// Pouring phase
	p.mu.Lock()
	p.State = StatePouring
	p.FillLevel = 100
	p.LastUpdate = time.Now()
	p.mu.Unlock()
	time.Sleep(2 * time.Second)

	// Ready — enjoy it while it's hot
	p.mu.Lock()
	p.State = StateReady
	p.Temperature = 88.0
	p.FillLevel = 100
	p.LastUpdate = time.Now()
	p.mu.Unlock()

	time.Sleep(15 * time.Second)

	// Cooling phase — drops to room temp
	p.mu.Lock()
	p.State = StateCooling
	p.LastUpdate = time.Now()
	p.mu.Unlock()

	for i := 0; i < 10; i++ {
		time.Sleep(3 * time.Second)
		p.mu.Lock()
		p.Temperature = math.Max(21.0, p.Temperature-7.0)
		p.FillLevel = max(0, p.FillLevel-10)
		p.LastUpdate = time.Now()
		if p.Temperature <= 21.0 {
			p.State = StateIdle
			p.Beverage = BeverageNone
			p.TeaVariety = TeaNone
			p.Additions = []Addition{}
			p.FillLevel = 0
			p.BrewStart = time.Time{}
			p.mu.Unlock()
			return
		}
		p.mu.Unlock()
	}

	// Force idle if cooling didn't finish
	p.mu.Lock()
	p.State = StateIdle
	p.Beverage = BeverageNone
	p.TeaVariety = TeaNone
	p.Additions = []Addition{}
	p.FillLevel = 0
	p.Temperature = 21.0
	p.BrewStart = time.Time{}
	p.mu.Unlock()
}

// simulateTemperature adjusts temperature based on time elapsed since last update.
// Called under read lock from Status(). Does not write -- temp changes happen in brewLifecycle.
func (p *Pot) simulateTemperature() {
	// Temperature simulation is handled by brewLifecycle goroutine.
	// This is just a placeholder for future ambient cooling calculations
	// that would require a PhD in thermodynamics and a coffee budget.
}

func (p *Pot) statusLocked() PotStatus {
	status := PotStatus{
		ID:          p.ID,
		Type:        p.Type,
		State:       p.State,
		Beverage:    p.Beverage,
		TeaVariety:  p.TeaVariety,
		Temperature: math.Round(p.Temperature*10) / 10,
		TempLabel:   TemperatureLabel(p.Temperature),
		FillLevel:   p.FillLevel,
		Additions:   p.Additions,
		Safe:        "if-user-awake",
	}
	if !p.BrewStart.IsZero() {
		t := p.BrewStart
		status.BrewStarted = &t
		status.BrewElapsed = time.Since(p.BrewStart).Round(time.Second).String()
	}
	return status
}

// PotFleet manages all pots in the deployment.
// Starts with 4 default pots. New pots are created dynamically via BREW /pot.
// Every ~5th dynamically created pot is a surprise teapot.
// Expired pots (idle for 5+ minutes) are cleaned up automatically.
type PotFleet struct {
	mu      sync.RWMutex
	pots    map[int]*Pot
	nextID  int
	created int // total pots ever created, for the teapot lottery
}

// NewPotFleet creates the fleet with the 4 default pots.
func NewPotFleet() *PotFleet {
	fleet := &PotFleet{
		pots:   make(map[int]*Pot),
		nextID: len(DefaultPotConfigs),
	}
	for _, cfg := range DefaultPotConfigs {
		fleet.pots[cfg.ID] = NewPot(cfg)
	}

	// Start the janitor goroutine to clean up expired pots.
	// Enterprise-grade garbage collection for beverage infrastructure.
	go fleet.janitor()

	return fleet
}

// GetPot returns a pot by ID, or nil if not found.
func (f *PotFleet) GetPot(id int) *Pot {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.pots[id]
}

// CreatePot dynamically creates a new pot and returns it.
// Every ~5th pot is a surprise teapot. You've been warned.
func (f *PotFleet) CreatePot() *Pot {
	f.mu.Lock()
	defer f.mu.Unlock()

	id := f.nextID
	f.nextID++
	f.created++

	// The teapot lottery: every 5th dynamically created pot is a teapot.
	// This is load-bearing randomness.
	potType := PotTypeCoffee
	coffeeNames := []string{
		"Dynamic Brew Unit",
		"Caffeination Station",
		"Bean Machine",
		"The Grind Chamber",
		"Drip Commander",
		"Espresso Express",
		"The Caffeine Cannon",
		"Brew Force One",
		"The Daily Grind",
		"Cup Constructor",
	}
	name := fmt.Sprintf("%s #%d", coffeeNames[id%len(coffeeNames)], id)
	if f.created%5 == 0 {
		potType = PotTypeTeapot
		teapotNames := []string{
			"Surprise Teapot",
			"Undercover Teapot",
			"Teapot in Disguise",
			"Definitely Not A Coffee Pot",
			"The Teapot Strikes Again",
			"Stealth Teapot",
			"Plot Twist Pot",
			"Her Majesty's Reserve",
			"The 418 Generator",
			"Chaos Teapot",
			"The RFC Enforcer",
			"Agent Teapot",
		}
		name = teapotNames[f.created/5%len(teapotNames)]
	}

	pot := NewPot(PotConfig{ID: id, Type: potType, Name: name})
	f.pots[id] = pot
	return pot
}

// AllStatus returns status for all pots, sorted by ID.
func (f *PotFleet) AllStatus() []PotStatus {
	f.mu.RLock()
	defer f.mu.RUnlock()

	// Collect IDs and sort
	ids := make([]int, 0, len(f.pots))
	for id := range f.pots {
		ids = append(ids, id)
	}
	sortInts(ids)

	statuses := make([]PotStatus, 0, len(ids))
	for _, id := range ids {
		statuses = append(statuses, f.pots[id].Status())
	}
	return statuses
}

// Count returns the total number of pots in the fleet.
func (f *PotFleet) Count() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return len(f.pots)
}

// janitor cleans up expired dynamic pots every 15 seconds.
// Removes dynamic pots (id >= 4) that are:
//   - idle for 60+ seconds, OR
//   - cooling for 60+ seconds (the brew is dead, let it go)
//
// The 4 original pots (0-3) are permanent infrastructure.
func (f *PotFleet) janitor() {
	for {
		time.Sleep(15 * time.Second)

		f.mu.Lock()
		now := time.Now()
		for id, pot := range f.pots {
			if id < len(DefaultPotConfigs) {
				continue // never remove the originals
			}
			pot.mu.RLock()
			state := pot.State
			age := now.Sub(pot.LastUpdate)
			pot.mu.RUnlock()

			remove := false
			switch state {
			case StateIdle:
				remove = age > 60*time.Second
			case StateCooling:
				remove = age > 60*time.Second
			}
			if remove {
				delete(f.pots, id)
			}
		}
		f.mu.Unlock()
	}
}

// sortInts sorts a slice of ints in ascending order.
// We could import sort, but this is a coffee pot server,
// not a computer science lecture.
func sortInts(a []int) {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j-1] > a[j]; j-- {
			a[j-1], a[j] = a[j], a[j-1]
		}
	}
}
