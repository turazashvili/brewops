package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	defaultServer = "http://localhost:8418"
	version       = "1.0.0"
)

const helpText = `
brew-ctl — HTCPCP/1.0 Command Line Client
         RFC 2324 + RFC 7168 (TEA)

USAGE:
    brew-ctl <command> [options]

COMMANDS:
    status              Show all pot statuses
    brew <type>         Brew a beverage (coffee, espresso)
    tea <variety>       Brew tea (earl-grey, darjeeling, peppermint, etc.)
    get <pot-id>        Get status of a specific pot
    when <pot-id>       Say "when" to stop milk (RFC 2324 §2.1.4)
    additions <pot-id>  Add additions to current brew
    props <pot-id>      PROPFIND — get brew metadata
    logs                Stream live incident log
    help                Show this help message

OPTIONS:
    --server <url>      Server URL (default: http://localhost:8418)
    --pot <id>          Pot ID (default: 0)
    --additions <list>  Comma-separated additions (e.g., Cream,Whisky)

EXAMPLES:
    brew-ctl status
    brew-ctl brew coffee --pot 0
    brew-ctl brew espresso --pot 1 --additions Cream,Vanilla
    brew-ctl brew coffee --pot 2              # 418!
    brew-ctl tea earl-grey
    brew-ctl when 0
    brew-ctl logs --server https://brewops.fly.dev
`

const teapotASCII = `
        ╭───╮
        │   │    I'm short and stout.
        │   │    Here is my handle,
    ╭───╯   ╰─╮ here is my spout.
    │  TEAPOT  │
    ╰──────╯───╯ Tip me over, pour me out.
`

const coffeeASCII = `
       ( (
        ) )
      ........
      |      |]
      \      /
       '----'
`

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Println(helpText)
		return
	}

	server := defaultServer
	potID := "0"
	additions := ""

	// Parse flags
	var positional []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--server":
			if i+1 < len(args) {
				server = args[i+1]
				i++
			}
		case "--pot":
			if i+1 < len(args) {
				potID = args[i+1]
				i++
			}
		case "--additions":
			if i+1 < len(args) {
				additions = args[i+1]
				i++
			}
		case "--help", "-h":
			fmt.Println(helpText)
			return
		case "--version", "-v":
			fmt.Printf("brew-ctl v%s (HTCPCP/1.0)\n", version)
			return
		default:
			positional = append(positional, args[i])
		}
	}

	if len(positional) == 0 {
		fmt.Println(helpText)
		return
	}

	cmd := positional[0]

	switch cmd {
	case "status":
		cmdStatus(server)
	case "brew":
		bevType := "coffee"
		if len(positional) > 1 {
			bevType = positional[1]
		}
		cmdBrew(server, potID, bevType, additions)
	case "tea":
		variety := "earl-grey"
		if len(positional) > 1 {
			variety = positional[1]
		}
		cmdTea(server, variety, additions)
	case "get":
		pid := potID
		if len(positional) > 1 {
			pid = positional[1]
		}
		cmdGet(server, pid)
	case "when":
		pid := potID
		if len(positional) > 1 {
			pid = positional[1]
		}
		cmdWhen(server, pid)
	case "props", "propfind":
		pid := potID
		if len(positional) > 1 {
			pid = positional[1]
		}
		cmdPropfind(server, pid)
	case "logs":
		cmdLogs(server)
	case "help":
		fmt.Println(helpText)
	default:
		fmt.Printf("\033[31mUnknown command: %s\033[0m\n", cmd)
		fmt.Println(helpText)
		os.Exit(1)
	}
}

func cmdStatus(server string) {
	printHeader("FLEET STATUS")

	resp, err := http.Get(server + "/status")
	if err != nil {
		printError("Failed to connect to BrewOps server at %s: %v", server, err)
		return
	}
	defer resp.Body.Close()

	var data struct {
		Pots  []map[string]interface{} `json:"pots"`
		Stats map[string]interface{}   `json:"stats"`
	}
	json.NewDecoder(resp.Body).Decode(&data)

	for _, pot := range data.Pots {
		id := pot["id"]
		potType := fmt.Sprint(pot["type"])
		state := fmt.Sprint(pot["state"])
		temp := pot["temperature_celsius"]
		tempLabel := fmt.Sprint(pot["temperature_label"])
		fill := pot["fill_level_percent"]

		icon := "☕"
		if potType == "teapot" {
			icon = "🫖"
		}

		stateColor := "\033[32m" // green
		switch state {
		case "brewing", "grinding":
			stateColor = "\033[33m" // yellow
		case "cooling":
			stateColor = "\033[36m" // cyan
		case "idle":
			stateColor = "\033[37m" // white
		}

		fmt.Printf("  %s \033[1mpot-%v\033[0m  %-12s  %s%-10s\033[0m  %.1f°C %-35s  Fill: %v%%\n",
			icon, id, potType, stateColor, state, temp, tempLabel, fill)
	}

	if data.Stats != nil {
		stats := data.Stats
		fmt.Println()
		printHeader("GLOBAL STATS")
		fmt.Printf("  Total Brews:     %v\n", stats["total_brews"])
		fmt.Printf("  Total 418s:      \033[31m%v\033[0m\n", stats["total_418s"])
		fmt.Printf("  Unique Brewers:  %v\n", stats["unique_brewers"])
		fmt.Printf("  Caffeine (mg):   %v\n", stats["caffeine_dispensed_mg"])
		fmt.Printf("  418 Rate:        \033[31m%.1f%%\033[0m\n", stats["rate_418_percent"])
		fmt.Printf("  Brew Uptime:     \033[32m%.2f%%\033[0m\n", stats["brew_uptime_percent"])
	}
	fmt.Println()
}

func cmdBrew(server, potID, bevType, additions string) {
	printHeader(fmt.Sprintf("BREW %s in pot-%s", strings.ToUpper(bevType), potID))

	url := fmt.Sprintf("%s/pot-%s", server, potID)

	req, _ := http.NewRequest("BREW", url, bytes.NewBufferString("start"))
	req.Header.Set("Content-Type", "message/coffeepot")
	if additions != "" {
		req.Header.Set("Accept-Additions", additions)
	}

	// Show progress animation
	fmt.Printf("\n  Initiating BREW request...\n")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		printError("Failed to connect: %v", err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	switch resp.StatusCode {
	case 418:
		// THE MOMENT. The whole reason this project exists.
		fmt.Printf("\n  \033[1;31mERROR 418: I'm a Teapot\033[0m\n")
		fmt.Printf("  \033[33m%s\033[0m\n", teapotASCII)
		fmt.Printf("  \033[31mThe requested pot (pot-%s) is a teapot and cannot brew coffee.\033[0m\n", potID)
		fmt.Printf("  Per RFC 2324 §2.3.2: \"Any attempt to brew coffee with a teapot\n")
		fmt.Printf("  should result in the error code '418 I'm a teapot'.\"\n\n")
		fmt.Printf("  \033[33mSuggestion:\033[0m Try pot-0, pot-1, or pot-3 for coffee.\n")
		fmt.Printf("             Or embrace tea: brew-ctl tea earl-grey\n\n")

	case 200:
		fmt.Printf("  \033[32m%s\033[0m\n", coffeeASCII)
		fmt.Printf("  \033[1;32mBREW STARTED\033[0m\n\n")
		prettyPrintJSON(body)

	case 406:
		fmt.Printf("\n  \033[33mERROR 406: Not Acceptable\033[0m\n")
		prettyPrintJSON(body)

	case 503:
		fmt.Printf("\n  \033[33mERROR 503: Pot Busy\033[0m\n")
		prettyPrintJSON(body)

	default:
		fmt.Printf("\n  Response (%d):\n", resp.StatusCode)
		prettyPrintJSON(body)
	}
}

func cmdTea(server, variety, additions string) {
	printHeader(fmt.Sprintf("BREW TEA: %s", variety))

	url := fmt.Sprintf("%s/tea/%s", server, variety)

	req, _ := http.NewRequest("BREW", url, bytes.NewBufferString("start"))
	req.Header.Set("Content-Type", "message/teapot")
	if additions != "" {
		req.Header.Set("Accept-Additions", additions)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		printError("Failed to connect: %v", err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == 200 {
		fmt.Printf("\n  🫖 \033[1;32mTea steeping: %s\033[0m\n\n", variety)
	} else if resp.StatusCode == 300 {
		fmt.Printf("\n  🫖 \033[33mMultiple tea varieties available:\033[0m\n\n")
	}

	prettyPrintJSON(body)
}

func cmdGet(server, potID string) {
	url := fmt.Sprintf("%s/pot-%s", server, potID)

	resp, err := http.Get(url)
	if err != nil {
		printError("Failed to connect: %v", err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	printHeader(fmt.Sprintf("POT-%s STATUS", potID))
	prettyPrintJSON(body)
}

func cmdWhen(server, potID string) {
	url := fmt.Sprintf("%s/pot-%s", server, potID)

	req, _ := http.NewRequest("WHEN", url, nil)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		printError("Failed to connect: %v", err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	printHeader("WHEN — Milk Halted")
	fmt.Printf("  \"Enough? Say WHEN.\" — RFC 2324 §2.1.4\n\n")
	prettyPrintJSON(body)
}

func cmdPropfind(server, potID string) {
	url := fmt.Sprintf("%s/pot-%s", server, potID)

	req, _ := http.NewRequest("PROPFIND", url, nil)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		printError("Failed to connect: %v", err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	printHeader(fmt.Sprintf("PROPFIND pot-%s — Brew Metadata", potID))
	prettyPrintJSON(body)
}

func cmdLogs(server string) {
	printHeader("LIVE INCIDENT LOG")
	fmt.Printf("  Streaming from %s/events...\n", server)
	fmt.Printf("  Press Ctrl+C to stop.\n\n")

	resp, err := http.Get(server + "/events")
	if err != nil {
		printError("Failed to connect: %v", err)
		return
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")

		var event struct {
			Timestamp string `json:"timestamp"`
			Severity  string `json:"severity"`
			Method    string `json:"method"`
			Path      string `json:"path"`
			Status    int    `json:"status"`
			Message   string `json:"message"`
			IP        string `json:"ip"`
		}

		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		if event.Timestamp == "" {
			continue
		}

		sevColor := "\033[37m"
		switch event.Severity {
		case "CRITICAL":
			sevColor = "\033[1;31m"
		case "WARNING":
			sevColor = "\033[33m"
		case "SUCCESS":
			sevColor = "\033[32m"
		case "INFO":
			sevColor = "\033[36m"
		}

		fmt.Printf("  \033[90m[%s]\033[0m %s%-8s\033[0m \033[90m%-15s\033[0m %-6s %-15s %s\n",
			event.Timestamp, sevColor, event.Severity, event.IP,
			event.Method, event.Path, event.Message)
	}
}

// ======================================================================
// Output helpers
// ======================================================================

func printHeader(title string) {
	line := strings.Repeat("─", len(title)+4)
	fmt.Printf("\n  \033[1;33m┌%s┐\033[0m\n", line)
	fmt.Printf("  \033[1;33m│  %s  │\033[0m\n", title)
	fmt.Printf("  \033[1;33m└%s┘\033[0m\n\n", line)
}

func printError(format string, args ...interface{}) {
	fmt.Printf("  \033[1;31mERROR:\033[0m "+format+"\n\n", args...)
	os.Exit(1)
}

func prettyPrintJSON(data []byte) {
	var out bytes.Buffer
	if err := json.Indent(&out, data, "  ", "  "); err != nil {
		fmt.Printf("  %s\n", string(data))
		return
	}
	fmt.Printf("  %s\n\n", out.String())
}
