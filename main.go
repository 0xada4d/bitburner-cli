package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"text/tabwriter"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"golang.org/x/net/websocket"
)

// ── RPC wire types ────────────────────────────────────────────────────────────

type rpcReq struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
	ID      int64  `json:"id"`
}

type rpcResp struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   json.RawMessage `json:"error,omitempty"`
}

type rpcResult struct {
	data json.RawMessage
	err  string
}

// ── Response payload types ────────────────────────────────────────────────────

type serverInfo struct {
	Hostname             string  `json:"hostname"`
	IP                   string  `json:"ip"`
	OrganizationName     string  `json:"organizationName"`
	CPUCores             int     `json:"cpuCores"`
	MaxRam               float64 `json:"maxRam"`
	RamUsed              float64 `json:"ramUsed"`
	HasAdminRights       bool    `json:"hasAdminRights"`
	PurchasedByPlayer    bool    `json:"purchasedByPlayer"`
	BackdoorInstalled    bool    `json:"backdoorInstalled"`
	RequiredHacking      int     `json:"requiredHackingSkill"`
	NumOpenPortsRequired int     `json:"numOpenPortsRequired"`
	MoneyAvailable       float64 `json:"moneyAvailable"`
	MoneyMax             float64 `json:"moneyMax"`
	MinDifficulty        float64 `json:"minDifficulty"`
	HackDifficulty       float64 `json:"hackDifficulty"`
	HackingChance        float64 `json:"hackingChance"`
	HackingTime          float64 `json:"hackingTime"`
	SSHPortOpen          bool    `json:"sshPortOpen"`
	FTPPortOpen          bool    `json:"ftpPortOpen"`
	SMTPPortOpen         bool    `json:"smtpPortOpen"`
	HTTPPortOpen         bool    `json:"httpPortOpen"`
	SQLPortOpen          bool    `json:"sqlPortOpen"`
}

type runningScript struct {
	PID               int     `json:"pid"`
	Filename          string  `json:"filename"`
	Args              []any   `json:"args"`
	Threads           int     `json:"threads"`
	RamUsage          float64 `json:"ramUsage"`
	Server            string  `json:"server"`
	OnlineRunningTime float64 `json:"onlineRunningTime"`
	OnlineMoneyMade   float64 `json:"onlineMoneyMade"`
}

type fileContent struct {
	Filename string `json:"filename"`
	Content  string `json:"content"`
}

type terminalAction struct {
	Action    string  `json:"action"`
	TimeLeft  float64 `json:"timeLeft"`
	TotalTime float64 `json:"totalTime"`
}

type darkwebItem struct {
	Name        string  `json:"name"`
	Price       float64 `json:"price"`
	Description string  `json:"description"`
	Owned       bool    `json:"owned"`
}

type scriptRamEntry struct {
	Name string  `json:"name"`
	Type string  `json:"type"`
	Cost float64 `json:"cost"`
}

type scriptRamResult struct {
	RamUsage float64          `json:"ramUsage"`
	Threads  int              `json:"threads"`
	Entries  []scriptRamEntry `json:"entries"`
}

type scanEntry struct {
	Hostname       string `json:"hostname"`
	IP             string `json:"ip"`
	HasAdminRights bool   `json:"hasAdminRights"`
}

type scanNode struct {
	Hostname        string     `json:"hostname"`
	HasAdminRights  bool       `json:"hasAdminRights"`
	RequiredHacking int        `json:"requiredHackingSkill"`
	NumOpenPorts    int        `json:"numOpenPortsRequired"`
	MaxRam          float64    `json:"maxRam"`
	Children        []scanNode `json:"children"`
}

type cityInfoResult struct {
	City       string  `json:"city"`
	TravelCost float64 `json:"travelCost"`
}

type cityLocationEntry struct {
	Name  string   `json:"name"`
	Types []string `json:"types"`
}

type companyInfo struct {
	Rep             float64  `json:"rep"`
	Favor           float64  `json:"favor"`
	CurrentJob      *string  `json:"currentJob"`
	AvailableFields []string `json:"availableFields"`
}

type crimeInfoEntry struct {
	Name          string  `json:"name"`
	Money         float64 `json:"money"`
	Time          float64 `json:"time"` // milliseconds
	SuccessChance float64 `json:"successChance"`
}

type companyJobEntry struct {
	Name               string  `json:"name"`
	Field              string  `json:"field"`
	IsStartingJob      bool    `json:"isStartingJob"`
	RequiredHacking    int     `json:"requiredHacking"`
	RequiredStrength   int     `json:"requiredStrength"`
	RequiredDefense    int     `json:"requiredDefense"`
	RequiredDexterity  int     `json:"requiredDexterity"`
	RequiredAgility    int     `json:"requiredAgility"`
	RequiredCharisma   int     `json:"requiredCharisma"`
	RequiredReputation float64 `json:"requiredReputation"`
	MoneySec           float64 `json:"moneySec"`
	RepSec             float64 `json:"repSec"`
}

type locationInfoResult struct {
	Name        string       `json:"name"`
	Types       []string     `json:"types"`
	CompanyInfo *companyInfo `json:"companyInfo,omitempty"`
}

type rfaServer struct {
	Hostname          string `json:"hostname"`
	HasAdminRights    bool   `json:"hasAdminRights"`
	PurchasedByPlayer bool   `json:"purchasedByPlayer"`
}

type techVendorServer struct {
	Hostname string  `json:"hostname"`
	Ram      float64 `json:"ram"`
}

type ramCostEntry struct {
	Ram  float64 `json:"ram"`
	Cost float64 `json:"cost"`
}

type techVendorInfo struct {
	MinRam        float64            `json:"minRam"`
	MaxRam        float64            `json:"maxRam"`
	ServerLimit   int                `json:"serverLimit"`
	ServerCount   int                `json:"serverCount"`
	Servers       []techVendorServer `json:"servers"`
	CostsByRam    []ramCostEntry     `json:"costsByRam"`
	HomeRam       float64            `json:"homeRam"`
	HomeRamCost   float64            `json:"homeRamCost"`
	HomeCores     int                `json:"homeCores"`
	HomeCoresCost float64            `json:"homeCoresCost"`
	HasTor        bool               `json:"hasTor"`
	TorCost       float64            `json:"torCost"`
}

type specialLocationInfo struct {
	Location           string  `json:"location"`
	CanCreateCorp      bool    `json:"canCreateCorp"`
	HasCorp            bool    `json:"hasCorp"`
	CanJoinBladeburner bool    `json:"canJoinBladeburner"`
	HasBladeburner     bool    `json:"hasBladeburner"`
	BBReq              string  `json:"bladeburnerRequirements"`
	CanAcceptGift      bool    `json:"canAcceptStaneksGift"`
	HasGift            bool    `json:"hasStaneksGift"`
	HasDarkscape       bool    `json:"hasDarkscape"`
	DarkscapeCost      float64 `json:"darkscapeCost"`
	Message            string  `json:"message"`
}

type casinoResult struct {
	Bet    float64 `json:"bet"`
	Won    bool    `json:"won"`
	Result string  `json:"result"`
	Payout float64 `json:"payout"`
}

type slotsPayline struct {
	Name   string  `json:"name"`
	Symbol string  `json:"symbol"`
	Count  int     `json:"count"`
	Gain   float64 `json:"gain"`
}

type slotsResult struct {
	Bet      float64        `json:"bet"`
	Grid     [][]string     `json:"grid"`
	Paylines []slotsPayline `json:"paylines"`
	NetGain  float64        `json:"netGain"`
}

type rouletteResult struct {
	Bet          float64 `json:"bet"`
	Strategy     string  `json:"strategy"`
	LandedNumber int     `json:"landedNumber"`
	LandedColor  string  `json:"landedColor"`
	Won          bool    `json:"won"`
	Gain         float64 `json:"gain"`
}

type blackjackCard struct {
	Value  string `json:"value"`
	Suit   string `json:"suit"`
	Hidden bool   `json:"hidden"`
}

type blackjackState struct {
	Status      string          `json:"status"`
	PlayerHand  []blackjackCard `json:"playerHand"`
	PlayerValue int             `json:"playerValue"`
	DealerHand  []blackjackCard `json:"dealerHand"`
	DealerValue int             `json:"dealerValue"`
	Bet         float64         `json:"bet"`
	Gain        *float64        `json:"gain"`
}

type graftableAugInfo struct {
	Name         string  `json:"name"`
	Cost         float64 `json:"cost"`
	Time         float64 `json:"time"`
	HasPrereqs   bool    `json:"hasPrereqs"`
	AlreadyOwned bool    `json:"alreadyOwned"`
}

type goGameState struct {
	Board         []string `json:"board"`
	CurrentPlayer string   `json:"currentPlayer"`
	BlackScore    float64  `json:"blackScore"`
	WhiteScore    float64  `json:"whiteScore"`
	Komi          float64  `json:"komi"`
	Opponent      string   `json:"opponent"`
	BoardSize     int      `json:"boardSize"`
	PreviousMove  *[2]int  `json:"previousMove"`
}

type goMoveResult struct {
	Type          string   `json:"type"`
	X             *int     `json:"x"`
	Y             *int     `json:"y"`
	Board         []string `json:"board"`
	CurrentPlayer string   `json:"currentPlayer"`
	BlackScore    float64  `json:"blackScore"`
	WhiteScore    float64  `json:"whiteScore"`
	Message       string   `json:"message"`
}

type hacknetNodeInfo struct {
	Name            string   `json:"name"`
	Level           int      `json:"level"`
	Ram             float64  `json:"ram"`
	Cores           int      `json:"cores"`
	Production      float64  `json:"production"`
	TimeOnline      float64  `json:"timeOnline"`
	TotalProduction float64  `json:"totalProduction"`
	IsServer        bool     `json:"isServer"`
	Cache           *int     `json:"cache"`
	HashCapacity    *float64 `json:"hashCapacity"`
	RamUsed         *float64 `json:"ramUsed"`
}

type hacknetStatusResult struct {
	IsServer     bool              `json:"isServer"`
	NumNodes     int               `json:"numNodes"`
	NextNodeCost float64           `json:"nextNodeCost"`
	Nodes        []hacknetNodeInfo `json:"nodes"`
}

type hashUpgradeInfo struct {
	Name  string  `json:"name"`
	Level int     `json:"level"`
	Cost  float64 `json:"cost"`
}

type hashStatusResult struct {
	Hashes   float64           `json:"hashes"`
	Capacity float64           `json:"capacity"`
	Upgrades []hashUpgradeInfo `json:"upgrades"`
}

type stockInfo struct {
	Symbol            string  `json:"symbol"`
	Name              string  `json:"name"`
	Price             float64 `json:"price"`
	AskPrice          float64 `json:"askPrice"`
	BidPrice          float64 `json:"bidPrice"`
	MaxShares         int     `json:"maxShares"`
	PlayerShares      int     `json:"playerShares"`
	PlayerAvgPx       float64 `json:"playerAvgPx"`
	PlayerShortShares int     `json:"playerShortShares"`
	PlayerAvgShortPx  float64 `json:"playerAvgShortPx"`
	Forecast          float64 `json:"forecast"`   // 0 means no 4S data
	Volatility        float64 `json:"volatility"` // 0 means no 4S data
}

type stockMarketData struct {
	HasWseAccount   bool        `json:"hasWseAccount"`
	HasTixApi       bool        `json:"hasTixApi"`
	Has4SData       bool        `json:"has4SData"`
	Has4STixApi     bool        `json:"has4STixApi"`
	WseCost         float64     `json:"wseCost"`
	TixApiCost      float64     `json:"tixApiCost"`
	FourSDataCost   float64     `json:"fourSDataCost"`
	FourSTixApiCost float64     `json:"fourSTixApiCost"`
	Commission      float64     `json:"commission"`
	Stocks          []stockInfo `json:"stocks"`
}

type stockTxResult struct {
	Symbol     string  `json:"symbol"`
	Shares     int     `json:"shares"`
	TotalCost  float64 `json:"totalCost"`
	Commission float64 `json:"commission"`
	Position   string  `json:"position"`
	Action     string  `json:"action"`
}

type playerStats struct {
	HP          float64 `json:"hp"`
	MaxHP       float64 `json:"maxHp"`
	Hacking     int     `json:"hacking"`
	Strength    int     `json:"strength"`
	Defense     int     `json:"defense"`
	Dexterity   int     `json:"dexterity"`
	Agility     int     `json:"agility"`
	Charisma    int     `json:"charisma"`
	Money       float64 `json:"money"`
	CurrentWork *string `json:"currentWork"`
}

type creatableProgramInfo struct {
	Name        string  `json:"name"`
	ReqLevel    int     `json:"reqLevel"`
	Time        float64 `json:"time"`
	Description string  `json:"description"`
	Owned       bool    `json:"owned"`
	ReqMet      bool    `json:"reqMet"`
	Progress    float64 `json:"progress"`
}

type programProgressResult struct {
	ProgramName string  `json:"programName"`
	Progress    float64 `json:"progress"`
}

type serverScriptGroup struct {
	Hostname string          `json:"hostname"`
	RamUsed  float64         `json:"ramUsed"`
	RamMax   float64         `json:"ramMax"`
	Scripts  []runningScript `json:"scripts"`
}

type saveFileResult struct {
	Identifier string `json:"identifier"`
	Binary     bool   `json:"binary"`
	Save       string `json:"save"`
}

type factionAugInfo struct {
	Name        string  `json:"name"`
	RepCost     float64 `json:"repCost"`
	MoneyCost   float64 `json:"moneyCost"`
	Owned       bool    `json:"owned"`
	Queued      bool    `json:"queued"`
	CanAfford   bool    `json:"canAfford"`
	HasRep      bool    `json:"hasRep"`
	HasPrereqs  bool    `json:"hasPrereqs"`
}

type factionInfoResult struct {
	Name               string           `json:"name"`
	Rep                float64          `json:"rep"`
	Favor              float64          `json:"favor"`
	OfferHackingWork   bool             `json:"offerHackingWork"`
	OfferFieldWork     bool             `json:"offerFieldWork"`
	OfferSecurityWork  bool             `json:"offerSecurityWork"`
	Augmentations      []factionAugInfo `json:"augmentations"`
}

type ownedAugEntry struct {
	Name  string `json:"name"`
	Level int    `json:"level"`
}

type sourceFileEntry struct {
	N     int `json:"n"`
	Level int `json:"level"`
}

type augmentationsStatus struct {
	Installed   []ownedAugEntry    `json:"installed"`
	Queued      []ownedAugEntry    `json:"queued"`
	NFGLevel    int                `json:"nfgLevel"`
	Entropy     int                `json:"entropy"`
	SourceFiles []sourceFileEntry  `json:"sourceFiles"`
	Mults       map[string]float64 `json:"mults"`
}

// ── Global state ──────────────────────────────────────────────────────────────

var (
	idCounter atomic.Int64
	mu        sync.Mutex
	pending   = map[int64]string{}
	respChans = map[int64]chan rpcResult{}
	cmdChan   = make(chan string, 16)

	activeWSMu sync.RWMutex
	activeWS   *websocket.Conn

	// true when city/location sub-mode is active; keeps "exit" from quitting the process
	inSubMode atomic.Bool
)

func getActiveWS() *websocket.Conn {
	activeWSMu.RLock()
	defer activeWSMu.RUnlock()
	return activeWS
}

// ── Aliases ───────────────────────────────────────────────────────────────────

func aliasFilePath() string {
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		if !strings.HasPrefix(dir, os.TempDir()) {
			return filepath.Join(dir, "aliases.json")
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		return filepath.Join(cwd, "aliases.json")
	}
	return "aliases.json"
}

// aliasStore holds per-repl alias maps. Keys: "global", "city", "location", "hacknet", "cloud".
var aliasStore = loadAliases()

func loadAliases() map[string]map[string]string {
	data, err := os.ReadFile(aliasFilePath())
	if err != nil {
		return map[string]map[string]string{"global": {}}
	}
	// Support old flat format (map[string]string) — migrate to new structure.
	var flat map[string]string
	if json.Unmarshal(data, &flat) == nil {
		// Check it isn't the new format by seeing if any value is itself a JSON object.
		isNew := false
		for _, v := range flat {
			if strings.HasPrefix(strings.TrimSpace(v), "{") {
				isNew = true
				break
			}
		}
		if !isNew {
			return map[string]map[string]string{"global": flat}
		}
	}
	var m map[string]map[string]string
	if json.Unmarshal(data, &m) != nil || m == nil {
		m = map[string]map[string]string{}
	}
	if m["global"] == nil {
		m["global"] = map[string]string{}
	}
	return m
}

func saveAliases() {
	data, _ := json.MarshalIndent(aliasStore, "", "  ")
	_ = os.WriteFile(aliasFilePath(), data, 0644)
}

// replAliases returns the alias map for the given repl scope, creating it if needed.
func replAliases(repl string) map[string]string {
	if aliasStore[repl] == nil {
		aliasStore[repl] = map[string]string{}
	}
	return aliasStore[repl]
}

// ── Session ───────────────────────────────────────────────────────────────────

// cityHandler is non-nil when in city sub-mode. It receives the raw input line
// and returns false to signal that city mode should exit.
type cityHandlerFn func(ws *websocket.Conn, out io.Writer, sess *session, line string) bool

// cityNames is package-level so the autocomplete function can access it.
var cityNames = []string{"Aevum", "Chongqing", "Sector-12", "New Tokyo", "Ishima", "Volhaven"}

type session struct {
	currentServer    string
	currentCity      string
	app              *tview.Application
	input            *tview.InputField
	out              io.Writer // ANSIWriter wrapping the output TextView
	outputView       *tview.TextView
	contextView      *tview.TextView
	statusBar        *tview.TextView
	contextRefreshFn func(ws *websocket.Conn) // nil = show server info
	seenInvites      map[string]bool          // faction invites already notified
	readInvites      map[string]bool          // faction invites dismissed from badge
	currentRepl      string                   // active repl scope for aliases: "global","city","location","hacknet","cloud"
	cityHandler      cityHandlerFn
	serverFiles      []string      // cached filenames for the current server
	serverNames      []string      // cached hostnames of all known servers
	neighbourNames   []string      // cached 1-hop reachable hostnames from current server
	cityLocations    []string      // cached location names for the current city
	programNames     []string      // cached creatable (not yet owned) program names
	subCmds          []string      // nil = top-level; set to available cmds while in a sub-repl
	tabActive        bool          // true for one autocomplete call when Tab is pressed
	dropdownOpen     bool          // true while the autocomplete dropdown is visible
	serverRefreshCh  chan struct{} // triggers an immediate server-panel refresh
	history          []string
	historyPos       int    // -1 when not browsing
	historyDraft     string // text saved when we started browsing history
}

var historyFile = func() string {
	if exe, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exe), "history.txt")
	}
	return "history.txt"
}()

func newSession(app *tview.Application, input *tview.InputField, out io.Writer, outputView, contextView, statusBar *tview.TextView) *session {
	s := &session{
		currentServer:   "home",
		currentRepl:     "global",
		app:             app,
		input:           input,
		out:             out,
		outputView:      outputView,
		contextView:     contextView,
		statusBar:       statusBar,
		serverRefreshCh: make(chan struct{}, 1),
		historyPos:      -1,
	}
	s.loadHistory()
	return s
}

func (s *session) setServer(hostname string) {
	s.currentServer = hostname
	s.serverFiles = nil
	s.updatePrompt()
	select {
	case s.serverRefreshCh <- struct{}{}:
	default:
	}
}

func (s *session) setCity(city string) {
	s.currentCity = city
	s.updatePrompt()
}

func (s *session) clearCity() {
	s.currentCity = ""
	s.updatePrompt()
}

func (s *session) updatePrompt() {
	var label string
	if s.currentCity != "" {
		label = fmt.Sprintf("[cyan][[white]%s[cyan]][-] [green]>[-] ", s.currentCity)
	} else {
		label = fmt.Sprintf("[yellow][[white]%s[yellow]][-] [green]>[-] ", s.currentServer)
	}
	s.app.QueueUpdateDraw(func() { s.input.SetLabel(label) })
}

// setPromptStr sets an arbitrary prompt string (used by sub-replsand hacknet).
func (s *session) setPromptStr(label string) {
	s.app.QueueUpdateDraw(func() { s.input.SetLabel(label) })
}

// setSubCompleter marks the session as being in a sub-repl with the given command set.
func (s *session) setSubCompleter(cmds []string) {
	s.subCmds = cmds
}

// restoreCompleter clears the sub-repl command set (back to top-level).
func (s *session) restoreCompleter() {
	s.subCmds = nil
}

// setContextRefresh sets the right-panel refresh function and title, then triggers an immediate refresh.
func (s *session) setContextRefresh(title string, fn func(ws *websocket.Conn)) {
	s.contextRefreshFn = fn
	s.app.QueueUpdateDraw(func() { s.contextView.SetTitle(fmt.Sprintf(" %s ", title)) })
	select {
	case s.serverRefreshCh <- struct{}{}:
	default:
	}
}

// clearContextRefresh reverts the right panel back to server-info mode.
func (s *session) clearContextRefresh() {
	s.contextRefreshFn = nil
	s.app.QueueUpdateDraw(func() { s.contextView.SetTitle(" Server ") })
	select {
	case s.serverRefreshCh <- struct{}{}:
	default:
	}
}

// refreshFileCompleter is now a no-op; autocomplete reads serverFiles dynamically.
func (s *session) refreshFileCompleter() {}

// ── History ───────────────────────────────────────────────────────────────────

func (s *session) loadHistory() {
	f, err := os.Open(historyFile)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if line := sc.Text(); line != "" {
			s.history = append(s.history, line)
		}
	}
	if len(s.history) > 1000 {
		s.history = s.history[len(s.history)-1000:]
	}
}

func (s *session) saveHistory() {
	lines := s.history
	if len(lines) > 1000 {
		lines = lines[len(lines)-1000:]
	}
	f, err := os.Create(historyFile)
	if err != nil {
		return
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	for _, l := range lines {
		fmt.Fprintln(w, l)
	}
	_ = w.Flush()
}

func (s *session) addHistory(line string) {
	if len(s.history) == 0 || s.history[len(s.history)-1] != line {
		s.history = append(s.history, line)
	}
	s.historyPos = -1
	s.historyDraft = ""
	s.saveHistory()
}

func (s *session) historyBack() {
	if len(s.history) == 0 {
		return
	}
	if s.historyPos == -1 {
		s.historyDraft = s.input.GetText()
		s.historyPos = len(s.history) - 1
	} else if s.historyPos > 0 {
		s.historyPos--
	} else {
		return
	}
	s.input.SetText(s.history[s.historyPos])
}

func (s *session) historyForward() {
	if s.historyPos == -1 {
		return
	}
	s.historyPos++
	if s.historyPos >= len(s.history) {
		s.historyPos = -1
		s.input.SetText(s.historyDraft)
	} else {
		s.input.SetText(s.history[s.historyPos])
	}
}

// ── Autocomplete ──────────────────────────────────────────────────────────────

var fileArgCmds = map[string]bool{
	"cat": true, "run": true, "tail": true, "check": true,
	"mem": true, "rm": true, "cp": true, "mv": true,
	"vim": true, "download": true,
}

// autocomplete is the tview SetAutocompleteFunc callback. It only returns
// completions when Tab was pressed (tabActive flag), so the dropdown never
// pops up while the user is typing normally.
func (s *session) autocomplete(text string) []string {
	if !s.tabActive {
		return nil
	}
	s.tabActive = false
	return s.computeCompletions(text)
}

// computeCompletions returns candidate completions for the current input text.
func (s *session) computeCompletions(text string) []string {
	trailingSpace := len(text) > 0 && text[len(text)-1] == ' '
	fields := strings.Fields(text)

	// The token we're completing (empty string if trailing space).
	var prefix string
	if !trailingSpace && len(fields) > 0 {
		prefix = fields[len(fields)-1]
	}

	var candidates []string

	completingFirstWord := len(fields) == 0 || (len(fields) == 1 && !trailingSpace)
	if completingFirstWord {
		if s.subCmds != nil {
			candidates = s.subCmds
		} else {
			for _, c := range commands {
				candidates = append(candidates, c.name)
			}
			candidates = append(candidates, "exit")
			// Also include alias names as first-word candidates.
			for k := range replAliases(s.currentRepl) {
				candidates = append(candidates, k)
			}
		}
	} else {
		cmd := fields[0]
		// Resolve alias to the underlying command for argument completion.
		if expanded, ok := replAliases(s.currentRepl)[cmd]; ok {
			if ep := strings.Fields(expanded); len(ep) > 0 {
				cmd = ep[0]
			}
		}
		// argIdx: 1-based index of the argument being completed
		argIdx := len(fields)
		if !trailingSpace {
			argIdx--
		}

		// interact takes a multi-word location name; match against the full arg suffix.
		if cmd == "interact" {
			// Everything after "interact " is the prefix to match against.
			argPrefix := ""
			if idx := strings.Index(text, " "); idx >= 0 {
				argPrefix = text[idx+1:]
			}
			pfxLower := strings.ToLower(argPrefix)
			var result []string
			for _, loc := range s.cityLocations {
				if strings.HasPrefix(strings.ToLower(loc), pfxLower) && loc != argPrefix {
					result = append(result, loc)
				}
			}
			return result
		}

		switch {
		case fileArgCmds[cmd] && argIdx == 1:
			candidates = s.serverFiles
		case cmd == "connect" && argIdx == 1:
			candidates = s.neighbourNames
		case (cmd == "travel" || cmd == "city") && argIdx == 1:
			candidates = cityNames
		case cmd == "upgrade" && argIdx == 2:
			candidates = []string{"level", "ram", "core", "cache"}
		case cmd == "go" && argIdx == 1:
			candidates = []string{"new", "board", "play", "pass"}
		case cmd == "graft" && argIdx == 1:
			candidates = []string{"list"}
		case cmd == "bj" && argIdx == 1:
			candidates = []string{"deal", "hit", "stand"}
		case cmd == "create" && argIdx == 1:
			candidates = s.programNames
		}
	}

	// Filter by prefix (case-insensitive).
	pfxLower := strings.ToLower(prefix)
	var result []string
	for _, c := range candidates {
		if strings.HasPrefix(strings.ToLower(c), pfxLower) && c != prefix {
			result = append(result, c)
		}
	}
	return result
}

// autocompleteSelected replaces the last token in the input with the selected entry.
// source is one of AutocompletedNavigate/Tab/Enter/Click.
func (s *session) autocompleteSelected(entry string, index int, source int) bool {
	if source == tview.AutocompletedNavigate {
		return false // keep dropdown open while browsing with arrow keys
	}
	// Tab, Enter, or Click: replace last token with the selected entry.
	s.dropdownOpen = false
	current := s.input.GetText()
	trailingSpace := len(current) > 0 && current[len(current)-1] == ' '
	fields := strings.Fields(current)
	if !trailingSpace && len(fields) > 0 {
		fields[len(fields)-1] = entry
	} else {
		fields = append(fields, entry)
	}
	s.input.SetText(strings.Join(fields, " ") + " ")
	return true // close dropdown
}

// fetchAndShowFiles fetches filenames for the session's current server,
// prints them, caches them on the session, and refreshes the completer.
func fetchAndShowFiles(ws *websocket.Conn, out io.Writer, sess *session) {
	fmt.Fprintf(out, "connected to %s\n", sess.currentServer)
	raw, err := sendMsgSync(ws, "getFileNames", map[string]string{"server": sess.currentServer})
	if err != nil {
		return // silent — ls failure shouldn't block connect
	}
	var files []string
	if err := json.Unmarshal(raw, &files); err != nil {
		return
	}
	sess.serverFiles = files
	sess.refreshFileCompleter()
	if len(files) == 0 {
		fmt.Fprintln(out, "(no files)")
		return
	}
	for _, f := range files {
		fmt.Fprintln(out, f)
	}
}

// refreshFilesIfCurrent silently re-fetches filenames when the mutated server
// is the one currently connected, then updates the completer cache.
func refreshFilesIfCurrent(ws *websocket.Conn, sess *session, mutatedServer string) {
	if mutatedServer != sess.currentServer {
		return
	}
	raw, err := sendMsgSync(ws, "getFileNames", map[string]string{"server": sess.currentServer})
	if err != nil {
		return
	}
	var files []string
	if err := json.Unmarshal(raw, &files); err != nil {
		return
	}
	sess.serverFiles = files
	sess.refreshFileCompleter()
}

// ── Formatting helpers ────────────────────────────────────────────────────────

func formatRAM(gb float64) string {
	if gb >= 1 {
		return fmt.Sprintf("%.2f GB", gb)
	}
	return fmt.Sprintf("%.0f MB", gb*1024)
}

func formatMoney(m float64) string {
	switch {
	case m >= 1e9:
		return fmt.Sprintf("$%.3fb", m/1e9)
	case m >= 1e6:
		return fmt.Sprintf("$%.3fm", m/1e6)
	case m >= 1e3:
		return fmt.Sprintf("$%.3fk", m/1e3)
	default:
		return fmt.Sprintf("$%.2f", m)
	}
}

func formatTime(s float64) string {
	switch {
	case s >= 3600:
		return fmt.Sprintf("%.1fh", s/3600)
	case s >= 60:
		return fmt.Sprintf("%.1fm", s/60)
	default:
		return fmt.Sprintf("%.1fs", s)
	}
}

// ── Context panel helpers ─────────────────────────────────────────────────────

// helpTable formats a list of [cmd, description] pairs into a two-column table.
// An empty cmd acts as a section divider (prints the description as a header line).
// A cmd of "-" prints a blank line.
func helpTable(out io.Writer, title string, rows [][2]string) {
	col := 28 // width of the left column
	fmt.Fprintf(out, "\n%s\n", title)
	fmt.Fprintf(out, "%s\n", strings.Repeat("─", col+20))
	for _, r := range rows {
		if r[0] == "-" {
			fmt.Fprintln(out)
			continue
		}
		if r[0] == "" {
			fmt.Fprintf(out, "  [%s]\n", r[1])
			continue
		}
		padding := col - len(r[0])
		if padding < 1 {
			padding = 1
		}
		fmt.Fprintf(out, "  %s%s%s\n", r[0], strings.Repeat(" ", padding), r[1])
	}
	fmt.Fprintln(out)
}

// ctxSep is a full-width divider for the 26-char-inner context panel.
const ctxSep = " [darkgray]──────────────────────[-]\n"

// ctxRAMBar returns two lines: a colored 12-block bar with %, then "used / max".
func ctxRAMBar(used, max float64) string {
	pct := 0.0
	if max > 0 {
		pct = used / max * 100
	}
	filled := int(pct / 100 * 12)
	if filled > 12 {
		filled = 12
	}
	col := "green"
	if pct >= 80 {
		col = "red"
	} else if pct >= 50 {
		col = "yellow"
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", 12-filled)
	return fmt.Sprintf(" [%s]%s[-] %4.0f%%\n %s / %s\n", col, bar, pct, formatRAM(used), formatRAM(max))
}

// ctxHeader returns a section divider with a title embedded: " ── Title ─────".
// Total visual width stays at 25 chars (fitting inside the 26-char inner panel).
func ctxHeader(title string) string {
	n := 20 - len(title)
	if n < 2 {
		n = 2
	}
	return fmt.Sprintf(" [darkgray]── %s %s[-]\n", title, strings.Repeat("─", n))
}

// ── RPC helpers ───────────────────────────────────────────────────────────────

func parseScriptArg(s string) any {
	if s == "true" {
		return true
	}
	if s == "false" {
		return false
	}
	if n, err := strconv.ParseFloat(s, 64); err == nil {
		return n
	}
	return s
}

func unmarshalErrMsg(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return string(raw)
}

// ── Math expression evaluator ─────────────────────────────────────────────────

type exprEval struct {
	s   string
	pos int
}

func evalMath(s string) (float64, error) {
	clean := strings.Map(func(r rune) rune {
		if strings.ContainsRune("0123456789.+-*/%()eE", r) {
			return r
		}
		return -1
	}, strings.ReplaceAll(s, " ", ""))
	e := &exprEval{s: clean}
	v, err := e.add()
	if err != nil {
		return 0, err
	}
	if e.pos < len(e.s) {
		return 0, fmt.Errorf("unexpected %q", string(e.s[e.pos]))
	}
	return v, nil
}

func (e *exprEval) add() (float64, error) {
	v, err := e.mul()
	if err != nil {
		return 0, err
	}
	for e.pos < len(e.s) && (e.s[e.pos] == '+' || e.s[e.pos] == '-') {
		op := e.s[e.pos]
		e.pos++
		r, err := e.mul()
		if err != nil {
			return 0, err
		}
		if op == '+' {
			v += r
		} else {
			v -= r
		}
	}
	return v, nil
}

func (e *exprEval) mul() (float64, error) {
	v, err := e.unary()
	if err != nil {
		return 0, err
	}
	for e.pos < len(e.s) && (e.s[e.pos] == '*' || e.s[e.pos] == '/' || e.s[e.pos] == '%') {
		op := e.s[e.pos]
		e.pos++
		r, err := e.unary()
		if err != nil {
			return 0, err
		}
		switch op {
		case '*':
			v *= r
		case '/':
			if r == 0 {
				return 0, fmt.Errorf("division by zero")
			}
			v /= r
		case '%':
			v = math.Mod(v, r)
		}
	}
	return v, nil
}

func (e *exprEval) unary() (float64, error) {
	if e.pos < len(e.s) && e.s[e.pos] == '-' {
		e.pos++
		v, err := e.primary()
		return -v, err
	}
	return e.primary()
}

func (e *exprEval) primary() (float64, error) {
	if e.pos >= len(e.s) {
		return 0, fmt.Errorf("unexpected end of expression")
	}
	if e.s[e.pos] == '(' {
		e.pos++
		v, err := e.add()
		if err != nil {
			return 0, err
		}
		if e.pos >= len(e.s) || e.s[e.pos] != ')' {
			return 0, fmt.Errorf("missing closing parenthesis")
		}
		e.pos++
		return v, nil
	}
	start := e.pos
	for e.pos < len(e.s) {
		c := e.s[e.pos]
		if c >= '0' && c <= '9' || c == '.' {
			e.pos++
		} else if (c == 'e' || c == 'E') && e.pos > start {
			e.pos++
			if e.pos < len(e.s) && (e.s[e.pos] == '+' || e.s[e.pos] == '-') {
				e.pos++
			}
		} else {
			break
		}
	}
	if e.pos == start {
		return 0, fmt.Errorf("expected number at position %d", e.pos)
	}
	return strconv.ParseFloat(e.s[start:e.pos], 64)
}

func printScanTree(out io.Writer, nodes []scanNode, prefix string, parentLast bool) {
	for i, node := range nodes {
		last := i == len(nodes)-1
		connector := "┣ "
		if last {
			connector = "┗ "
		}
		fmt.Fprintf(out, "%s%s%s\n", prefix, connector, node.Hostname)

		infoPrefix := prefix
		if last {
			infoPrefix += "    "
		} else {
			infoPrefix += "┃   "
		}

		root := "NO"
		if node.HasAdminRights {
			root = "YES"
		}
		fmt.Fprintf(out, "%sRoot: %s  Hack: %d  Ports: %d  RAM: %s\n",
			infoPrefix, root, node.RequiredHacking, node.NumOpenPorts, formatRAM(node.MaxRam))

		if len(node.Children) > 0 {
			childPrefix := prefix
			if last {
				childPrefix += "    "
			} else {
				childPrefix += "┃ "
			}
			printScanTree(out, node.Children, childPrefix, last)
		}
	}
}

var actionLabels = map[string]string{
	"h": "hack",
	"g": "grow",
	"w": "weaken",
	"b": "backdoor",
	"a": "analyze",
	"c": "crack",
}

// waitForAction polls getTerminalAction until the in-game action completes,
// showing a live progress bar, then prints the last terminal output lines.
func waitForAction(ws *websocket.Conn, out io.Writer) {
	time.Sleep(300 * time.Millisecond)

	spinner := []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'}
	tick := 0
	const barWidth = 20

	for {
		raw, err := sendMsgSync(ws, "getTerminalAction", nil)
		if err != nil {
			fmt.Fprintf(out, "\r%-70s\r", "")
			fmt.Fprintf(out, "poll error: %v\n", err)
			return
		}

		if string(raw) == "null" {
			break
		}

		var act terminalAction
		if err := json.Unmarshal(raw, &act); err != nil {
			break
		}

		progress := 0.0
		if act.TotalTime > 0 {
			progress = (act.TotalTime - act.TimeLeft) / act.TotalTime
		}
		filled := int(progress * barWidth)
		bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)
		label := actionLabels[act.Action]
		if label == "" {
			label = act.Action
		}

		fmt.Fprintf(out, "\r%c [%s] [%s] %3.0f%%  %s left  ",
			spinner[tick%len(spinner)], label, bar, progress*100, formatTime(act.TimeLeft))
		tick++
		time.Sleep(250 * time.Millisecond)
	}

	fmt.Fprintf(out, "\r%-70s\r", "")

	raw, err := sendMsgSync(ws, "getTerminalOutput", map[string]int{"count": 8})
	if err != nil {
		return
	}
	var lines []string
	if err := json.Unmarshal(raw, &lines); err != nil {
		return
	}
	for _, l := range lines {
		if l != "" {
			fmt.Fprintln(out, l)
		}
	}
}

// sendMsg fires a message and prints the request line (async; response is printed by receiver).
func sendMsg(ws *websocket.Conn, out io.Writer, method string, params any) {
	id := idCounter.Add(1)
	mu.Lock()
	pending[id] = method
	mu.Unlock()

	data, _ := json.Marshal(rpcReq{JSONRPC: "2.0", Method: method, Params: params, ID: id})
	if err := websocket.Message.Send(ws, string(data)); err != nil {
		fmt.Fprintf(out, "send error: %v\n", err)
		return
	}
	fmt.Fprintf(out, ">> %s\n", data)
}

// sendMsgSync sends a message and blocks until the response arrives (or times out).
// The response is NOT printed to the terminal; the caller formats it.
func sendMsgSync(ws *websocket.Conn, method string, params any) (json.RawMessage, error) {
	id := idCounter.Add(1)
	ch := make(chan rpcResult, 1)
	mu.Lock()
	pending[id] = method
	respChans[id] = ch
	mu.Unlock()

	data, _ := json.Marshal(rpcReq{JSONRPC: "2.0", Method: method, Params: params, ID: id})
	if err := websocket.Message.Send(ws, string(data)); err != nil {
		mu.Lock()
		delete(pending, id)
		delete(respChans, id)
		mu.Unlock()
		return nil, fmt.Errorf("send error: %v", err)
	}

	select {
	case r := <-ch:
		if r.err != "" {
			return nil, fmt.Errorf("%s", r.err)
		}
		return r.data, nil
	case <-time.After(10 * time.Second):
		mu.Lock()
		delete(pending, id)
		delete(respChans, id)
		mu.Unlock()
		return nil, fmt.Errorf("timeout waiting for %s response", method)
	}
}

// ── Commands ──────────────────────────────────────────────────────────────────

type cmdFn func(ws *websocket.Conn, out io.Writer, sess *session, parts []string)

type command struct {
	name string
	run  cmdFn
}

var commands []command

func init() {
	commands = []command{
		// ── Raw RFA methods ──────────────────────────────────────────────────────

		{"getFileNames", func(ws *websocket.Conn, out io.Writer, sess *session, parts []string) {
			server := sess.currentServer
			if len(parts) > 1 {
				server = parts[1]
			}
			sendMsg(ws, out, "getFileNames", map[string]string{"server": server})
		}},
		{"getFile", func(ws *websocket.Conn, out io.Writer, sess *session, parts []string) {
			if len(parts) < 3 {
				fmt.Fprintln(out, "usage: getFile <server> <filename>")
				return
			}
			sendMsg(ws, out, "getFile", map[string]string{"server": parts[1], "filename": parts[2]})
		}},
		{"getFileMetadata", func(ws *websocket.Conn, out io.Writer, sess *session, parts []string) {
			if len(parts) < 3 {
				fmt.Fprintln(out, "usage: getFileMetadata <server> <filename>")
				return
			}
			sendMsg(ws, out, "getFileMetadata", map[string]string{"server": parts[1], "filename": parts[2]})
		}},
		{"getAllFiles", func(ws *websocket.Conn, out io.Writer, sess *session, parts []string) {
			server := sess.currentServer
			if len(parts) > 1 {
				server = parts[1]
			}
			sendMsg(ws, out, "getAllFiles", map[string]string{"server": server})
		}},
		{"getAllFileMetadata", func(ws *websocket.Conn, out io.Writer, sess *session, parts []string) {
			server := sess.currentServer
			if len(parts) > 1 {
				server = parts[1]
			}
			sendMsg(ws, out, "getAllFileMetadata", map[string]string{"server": server})
		}},
		{"getDefinitionFile", func(ws *websocket.Conn, out io.Writer, sess *session, parts []string) {
			sendMsg(ws, out, "getDefinitionFile", nil)
		}},
		{"getAllServers", func(ws *websocket.Conn, out io.Writer, sess *session, parts []string) {
			sendMsg(ws, out, "getAllServers", nil)
		}},
		{"getSaveFile", func(ws *websocket.Conn, out io.Writer, sess *session, parts []string) {
			sendMsg(ws, out, "getSaveFile", nil)
		}},
		{"export_game", func(ws *websocket.Conn, out io.Writer, sess *session, parts []string) {
			raw, err := sendMsgSync(ws, "getSaveFile", nil)
			if err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
				return
			}
			var result saveFileResult
			if err := json.Unmarshal(raw, &result); err != nil {
				fmt.Fprintf(out, "error parsing save data: %v\n", err)
				return
			}
			ext := "json"
			if result.Binary {
				ext = "json.gz"
			}
			filename := fmt.Sprintf("bitburnerSave_%d.%s", time.Now().Unix(), ext)
			var data []byte
			if result.Binary {
				// each char in Save encodes one raw byte of the gzip stream
				data = make([]byte, len(result.Save))
				for i, ch := range result.Save {
					data[i] = byte(ch)
				}
			} else {
				data = []byte(result.Save)
			}
			if err := os.WriteFile(filename, data, 0644); err != nil {
				fmt.Fprintf(out, "error writing file: %v\n", err)
				return
			}
			fmt.Fprintf(out, "saved to %s (%d bytes)\n", filename, len(data))
		}},
		{"calculateRam", func(ws *websocket.Conn, out io.Writer, sess *session, parts []string) {
			if len(parts) < 3 {
				fmt.Fprintln(out, "usage: calculateRam <server> <filename>")
				return
			}
			sendMsg(ws, out, "calculateRam", map[string]string{"server": parts[1], "filename": parts[2]})
		}},
		{"pushFile", func(ws *websocket.Conn, out io.Writer, sess *session, parts []string) {
			if len(parts) < 4 {
				fmt.Fprintln(out, "usage: pushFile <server> <filename> <content...>")
				return
			}
			if _, err := sendMsgSync(ws, "pushFile", map[string]string{
				"server":   parts[1],
				"filename": parts[2],
				"content":  strings.Join(parts[3:], " "),
			}); err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
				return
			}
			refreshFilesIfCurrent(ws, sess, parts[1])
		}},
		{"deleteFile", func(ws *websocket.Conn, out io.Writer, sess *session, parts []string) {
			if len(parts) < 3 {
				fmt.Fprintln(out, "usage: deleteFile <server> <filename>")
				return
			}
			if _, err := sendMsgSync(ws, "deleteFile", map[string]string{"server": parts[1], "filename": parts[2]}); err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
				return
			}
			refreshFilesIfCurrent(ws, sess, parts[1])
		}},

		// ── Navigation ───────────────────────────────────────────────────────────

		{"connect", func(ws *websocket.Conn, out io.Writer, sess *session, parts []string) {
			if len(parts) < 2 {
				fmt.Fprintln(out, "usage: connect <hostname>")
				return
			}
			sess.setServer(parts[1])
			fetchAndShowFiles(ws, out, sess)
		}},
		{"home", func(ws *websocket.Conn, out io.Writer, sess *session, parts []string) {
			sess.setServer("home")
			fetchAndShowFiles(ws, out, sess)
		}},

		{"city", func(ws *websocket.Conn, out io.Writer, sess *session, parts []string) {
			raw, err := sendMsgSync(ws, "getPlayerCity", nil)
			if err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
				return
			}
			var info cityInfoResult
			if err := json.Unmarshal(raw, &info); err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
				return
			}
			sess.setCity(info.City)
			fmt.Fprintf(out, "City: %s  (travel costs %s)\n", info.City, formatMoney(info.TravelCost))
			fmt.Fprintf(out, "Commands: ls, travel <city>, info, exit\n")

			setCityPrompt := func() {
				sess.setPromptStr(fmt.Sprintf("[cyan][[white]city:%s[cyan]][-] [green]>[-] ", sess.currentCity))
			}
			setCityPrompt()
			inSubMode.Store(true)

			sess.setSubCompleter([]string{"ls", "travel", "info", "interact", "help", "alias", "unalias", "exit"})
			sess.currentRepl = "city"

			refreshCityContext := func() {
				sess.setContextRefresh(fmt.Sprintf("City: %s", sess.currentCity), func(ws *websocket.Conn) {
					raw, err := sendMsgSync(ws, "getCityLocations", map[string]string{"city": sess.currentCity})
					if err != nil {
						return
					}
					var locs []cityLocationEntry
					if json.Unmarshal(raw, &locs) != nil {
						return
					}
					names := make([]string, 0, len(locs))
					for _, loc := range locs {
						names = append(names, loc.Name)
					}
					sess.cityLocations = names
					byType := map[string][]string{}
					typeOrder := []string{}
					for _, loc := range locs {
						t := "Other"
						if len(loc.Types) > 0 {
							t = loc.Types[0]
						}
						if _, seen := byType[t]; !seen {
							typeOrder = append(typeOrder, t)
						}
						byType[t] = append(byType[t], loc.Name)
					}
					var sb strings.Builder
					for _, t := range typeOrder {
						sb.WriteString(ctxHeader(t))
						for _, name := range byType[t] {
							fmt.Fprintf(&sb, "  %s\n", name)
						}
					}
					text := sb.String()
					sess.app.QueueUpdateDraw(func() { sess.contextView.SetText(text) })
				})
			}
			refreshCityContext()

			sess.cityHandler = func(ws *websocket.Conn, out io.Writer, sess *session, line string) bool {
				cityParts := strings.Fields(line)
				if len(cityParts) == 0 {
					return true
				}
				switch cityParts[0] {
				case "exit", "quit":
					sess.clearContextRefresh()
					sess.clearCity()
					sess.cityLocations = nil
					sess.currentRepl = "global"
					return false
				case "help":
					helpTable(out, "City", [][2]string{
						{"ls", "List locations in current city"},
						{"travel <city>", "Travel to another city (" + strings.Join(cityNames, ", ") + ")"},
						{"info", "Show current city and travel cost"},
						{"interact <location>", "Enter a location sub-repl"},
						{"-", ""},
						{"alias [name=\"value\"]", "Define an alias"},
						{"unalias <name>", "Remove an alias"},
						{"help", "Show this help"},
						{"exit", "Return to top-level"},
					})
					return true
				case "info":
					r2, err := sendMsgSync(ws, "getPlayerCity", nil)
					if err != nil {
						fmt.Fprintf(out, "error: %v\n", err)
						return true
					}
					var ci cityInfoResult
					if err := json.Unmarshal(r2, &ci); err != nil {
						fmt.Fprintf(out, "error: %v\n", err)
						return true
					}
					sess.setCity(ci.City)
					setCityPrompt()
					fmt.Fprintf(out, "City: %s  (travel costs %s)\n", ci.City, formatMoney(ci.TravelCost))
				case "travel":
					if len(cityParts) < 2 {
						fmt.Fprintf(out, "usage: travel <%s>\n", strings.Join(cityNames, "|"))
						return true
					}
					dest := strings.Join(cityParts[1:], " ")
					r2, err := sendMsgSync(ws, "travelToCity", map[string]string{"city": dest})
					if err != nil {
						fmt.Fprintf(out, "error: %v\n", err)
						return true
					}
					var newCity string
					if err := json.Unmarshal(r2, &newCity); err != nil {
						fmt.Fprintf(out, "error: %v\n", err)
						return true
					}
					sess.setCity(newCity)
					setCityPrompt()
					refreshCityContext()
					fmt.Fprintf(out, "Traveled to %s\n", newCity)
				case "ls":
					r2, err := sendMsgSync(ws, "getCityLocations", map[string]string{"city": sess.currentCity})
					if err != nil {
						fmt.Fprintf(out, "error: %v\n", err)
						return true
					}
					var locs []cityLocationEntry
					if err := json.Unmarshal(r2, &locs); err != nil {
						fmt.Fprintf(out, "error: %v\n", err)
						return true
					}
					byType := map[string][]string{}
					typeOrder := []string{}
					for _, loc := range locs {
						t := "Other"
						if len(loc.Types) > 0 {
							t = loc.Types[0]
						}
						if _, seen := byType[t]; !seen {
							typeOrder = append(typeOrder, t)
						}
						byType[t] = append(byType[t], loc.Name)
					}
					fmt.Fprintf(out, "%s\n", sess.currentCity)
					for _, t := range typeOrder {
						fmt.Fprintf(out, "  %s:\n", t)
						for _, name := range byType[t] {
							fmt.Fprintf(out, "    %s\n", name)
						}
					}
				case "stats":
					r2, err := sendMsgSync(ws, "getPlayerStats", nil)
					if err != nil {
						fmt.Fprintf(out, "error: %v\n", err)
						return true
					}
					var ps playerStats
					if err := json.Unmarshal(r2, &ps); err != nil {
						fmt.Fprintf(out, "error: %v\n", err)
						return true
					}
					fmt.Fprintf(out, "HP:        %.0f / %.0f\n", ps.HP, ps.MaxHP)
					fmt.Fprintf(out, "Money:     %s\n", formatMoney(ps.Money))
					fmt.Fprintf(out, "Hacking:   %d\n", ps.Hacking)
					fmt.Fprintf(out, "Strength:  %d\n", ps.Strength)
					fmt.Fprintf(out, "Defense:   %d\n", ps.Defense)
					fmt.Fprintf(out, "Dexterity: %d\n", ps.Dexterity)
					fmt.Fprintf(out, "Agility:   %d\n", ps.Agility)
					fmt.Fprintf(out, "Charisma:  %d\n", ps.Charisma)
					if ps.CurrentWork != nil {
						fmt.Fprintf(out, "Working:   %s\n", *ps.CurrentWork)
					}
				case "stopwork":
					r2, err := sendMsgSync(ws, "stopWork", nil)
					if err != nil {
						fmt.Fprintf(out, "error: %v\n", err)
						return true
					}
					var msg string
					_ = json.Unmarshal(r2, &msg)
					fmt.Fprintln(out, msg)
				case "interact":
					if len(cityParts) < 2 {
						fmt.Fprintln(out, "usage: interact <location name>")
						return true
					}
					locName := strings.Join(cityParts[1:], " ")
					r2, err := sendMsgSync(ws, "getLocationInfo", map[string]any{"location": locName})
					if err != nil {
						fmt.Fprintf(out, "error: %v\n", err)
						return true
					}
					var locInfo locationInfoResult
					if err := json.Unmarshal(r2, &locInfo); err != nil {
						fmt.Fprintf(out, "error: %v\n", err)
						return true
					}
					hasLocType := func(t string) bool {
						for _, typ := range locInfo.Types {
							if typ == t {
								return true
							}
						}
						return false
					}

					printLocHelp := func() {
						title := fmt.Sprintf("%s  [%s]", locInfo.Name, strings.Join(locInfo.Types, ", "))
						rows := [][2]string{}
						anyInteraction := false

						// ── Gym ───────────────────────────────────────────────
						if hasLocType("Gym") {
							anyInteraction = true
							workRates := map[string]float64{}
							if rawWR, err2 := sendMsgSync(ws, "getWorkRates", map[string]string{"location": locInfo.Name}); err2 == nil {
								_ = json.Unmarshal(rawWR, &workRates)
							}
							rate := func(k string) string {
								if r, ok := workRates[k]; ok && r != 0 {
									return fmt.Sprintf("Train %s (%s/sec)", strings.ToUpper(k), formatMoney(r))
								}
								return "Train " + strings.ToUpper(k)
							}
							rows = append(rows, [2]string{"", "Gym"})
							rows = append(rows, [2]string{"train str", rate("str")})
							rows = append(rows, [2]string{"train def", rate("def")})
							rows = append(rows, [2]string{"train dex", rate("dex")})
							rows = append(rows, [2]string{"train agi", rate("agi")})
						}

						// ── University ────────────────────────────────────────
						if hasLocType("University") {
							anyInteraction = true
							workRates := map[string]float64{}
							if rawWR, err2 := sendMsgSync(ws, "getWorkRates", map[string]string{"location": locInfo.Name}); err2 == nil {
								_ = json.Unmarshal(rawWR, &workRates)
							}
							courses := []struct{ key, label string }{
								{"computer science", "Computer Science"},
								{"data structures", "Data Structures"},
								{"networks", "Networks"},
								{"algorithms", "Algorithms"},
								{"management", "Management"},
								{"leadership", "Leadership"},
							}
							rows = append(rows, [2]string{"", "University"})
							for _, c := range courses {
								desc := c.label
								if r, ok := workRates[c.key]; ok && r != 0 {
									desc = fmt.Sprintf("%s (%s/sec)", c.label, formatMoney(r))
								}
								rows = append(rows, [2]string{"study " + c.label, desc})
							}
						}

						// ── Company ───────────────────────────────────────────
						if hasLocType("Company") {
							anyInteraction = true
							if ci := locInfo.CompanyInfo; ci != nil {
								job := "(none)"
								if ci.CurrentJob != nil {
									job = *ci.CurrentJob
								}
								rows = append(rows, [2]string{"", fmt.Sprintf("Company  rep:%s  favor:%.0f  job:%s", formatMoney(ci.Rep), ci.Favor, job)})
							}
							if rawJobs, err2 := sendMsgSync(ws, "getCompanyJobInfo", map[string]string{"location": locInfo.Name}); err2 == nil {
								var jobs []companyJobEntry
								if json.Unmarshal(rawJobs, &jobs) == nil {
									curField := ""
									for _, j := range jobs {
										if j.Field != curField {
											curField = j.Field
											rows = append(rows, [2]string{"", curField})
										}
										reqs := []string{}
										if j.RequiredHacking > 0    { reqs = append(reqs, fmt.Sprintf("hack≥%d", j.RequiredHacking)) }
										if j.RequiredStrength > 0   { reqs = append(reqs, fmt.Sprintf("str≥%d", j.RequiredStrength)) }
										if j.RequiredDefense > 0    { reqs = append(reqs, fmt.Sprintf("def≥%d", j.RequiredDefense)) }
										if j.RequiredDexterity > 0  { reqs = append(reqs, fmt.Sprintf("dex≥%d", j.RequiredDexterity)) }
										if j.RequiredAgility > 0    { reqs = append(reqs, fmt.Sprintf("agi≥%d", j.RequiredAgility)) }
										if j.RequiredCharisma > 0   { reqs = append(reqs, fmt.Sprintf("cha≥%d", j.RequiredCharisma)) }
										if j.RequiredReputation > 0 { reqs = append(reqs, fmt.Sprintf("rep≥%s", formatMoney(j.RequiredReputation))) }
										desc := fmt.Sprintf("%s/sec  +%.2f rep/sec", formatMoney(j.MoneySec), j.RepSec)
										if len(reqs) > 0 {
											desc += "  [" + strings.Join(reqs, " ") + "]"
										}
										rows = append(rows, [2]string{"apply " + j.Field, desc})
									}
								}
							}
							rows = append(rows, [2]string{"work", "Start working (requires a job)"})
						}

						// ── Tech Vendor ───────────────────────────────────────
						if hasLocType("Tech Vendor") {
							anyInteraction = true
							rows = append(rows, [2]string{"", "Tech Vendor"})
							if r2tv, err2 := sendMsgSync(ws, "getTechVendorInfo", map[string]any{"location": locInfo.Name}); err2 == nil {
								var tvi techVendorInfo
								if json.Unmarshal(r2tv, &tvi) == nil {
									rows = append(rows, [2]string{"servers", fmt.Sprintf("List servers (%d/%d, %.0f–%.0fGB)", tvi.ServerCount, tvi.ServerLimit, tvi.MinRam, tvi.MaxRam)})
									rows = append(rows, [2]string{"buy <name> <ram>", "Purchase a server"})
									rows = append(rows, [2]string{"upgrade <name> <ram>", "Upgrade a server's RAM"})
									torStr := formatMoney(tvi.TorCost)
									if tvi.HasTor { torStr = "owned" }
									rows = append(rows, [2]string{"tor", fmt.Sprintf("TOR router (%s)", torStr)})
									rows = append(rows, [2]string{"ram", fmt.Sprintf("Upgrade home RAM  %.0fGB → %s", tvi.HomeRam, formatMoney(tvi.HomeRamCost))})
									rows = append(rows, [2]string{"cores", fmt.Sprintf("Upgrade home cores  %d → %s", tvi.HomeCores, formatMoney(tvi.HomeCoresCost))})
								}
							} else {
								rows = append(rows, [2]string{"servers", "List owned servers & costs"})
								rows = append(rows, [2]string{"buy <name> <ram>", "Purchase a server"})
								rows = append(rows, [2]string{"upgrade <name> <ram>", "Upgrade a server's RAM"})
								rows = append(rows, [2]string{"ram", "Upgrade home RAM"})
								rows = append(rows, [2]string{"cores", "Upgrade home CPU cores"})
								rows = append(rows, [2]string{"tor", "Purchase TOR router"})
							}
						}

						// ── Hospital ──────────────────────────────────────────
						if hasLocType("Hospital") {
							anyInteraction = true
							rows = append(rows, [2]string{"", "Hospital"})
							rows = append(rows, [2]string{"heal", "Hospitalize (restore HP)"})
						}

						// ── Slums ─────────────────────────────────────────────
						if hasLocType("Slums") {
							anyInteraction = true
							rows = append(rows, [2]string{"", "Crimes"})
							if rawCI, err2 := sendMsgSync(ws, "getCrimeInfo", nil); err2 == nil {
								var crimes []crimeInfoEntry
								if json.Unmarshal(rawCI, &crimes) == nil {
									for _, c := range crimes {
										desc := fmt.Sprintf("%s  %.1fs  %.0f%% success", formatMoney(c.Money), c.Time/1000, c.SuccessChance*100)
										rows = append(rows, [2]string{"crime " + c.Name, desc})
									}
								}
							} else {
								rows = append(rows, [2]string{"crime <type>", "Commit a crime"})
							}
						}

						// ── Travel Agency ─────────────────────────────────────
						if hasLocType("Travel Agency") {
							anyInteraction = true
							rows = append(rows, [2]string{"", "Travel Agency"})
							rows = append(rows, [2]string{"travel <city>", "Use the city-level travel command"})
						}

						// ── Stock Market ──────────────────────────────────────
						if hasLocType("Stock Market") {
							anyInteraction = true
							rows = append(rows, [2]string{"", "Stock Market"})
							if r2sm, err2 := sendMsgSync(ws, "getStockMarketData", nil); err2 == nil {
								var smd stockMarketData
								if json.Unmarshal(r2sm, &smd) == nil {
									if !smd.HasWseAccount {
										rows = append(rows, [2]string{"wse", fmt.Sprintf("Purchase WSE account (%s)", formatMoney(smd.WseCost))})
									} else {
										rows = append(rows, [2]string{"stocks", "List all stocks and your positions"})
										rows = append(rows, [2]string{"buy <SYM> <n>", "Buy n shares (long)"})
										rows = append(rows, [2]string{"sell <SYM> <n|-1>", "Sell n shares long (-1=all)"})
										rows = append(rows, [2]string{"short <SYM> <n>", "Buy short position (SF8 required)"})
										rows = append(rows, [2]string{"cover <SYM> <n|-1>", "Sell short position"})
										if !smd.Has4SData {
											rows = append(rows, [2]string{"4s", fmt.Sprintf("Buy 4S market data (%s)", formatMoney(smd.FourSDataCost))})
										}
										if !smd.Has4STixApi && smd.HasTixApi {
											rows = append(rows, [2]string{"4sapi", fmt.Sprintf("Buy 4S TIX API (%s)", formatMoney(smd.FourSTixApiCost))})
										}
									}
								}
							}
						}

						// ── Casino ────────────────────────────────────────────
						if hasLocType("Casino") {
							anyInteraction = true
							rows = append(rows, [2]string{"", "Casino"})
							rows = append(rows, [2]string{"coinflip <heads|tails> <bet>", "Coin flip (50/50, max $10k)"})
							rows = append(rows, [2]string{"slots <bet>", "Spin the slot machine"})
							rows = append(rows, [2]string{"roulette <strategy> <bet>", "red/black/odd/even/high/low/1-12/…/<n>"})
							rows = append(rows, [2]string{"bj deal <bet>", "Start a blackjack hand"})
							rows = append(rows, [2]string{"bj hit", "Take a card"})
							rows = append(rows, [2]string{"bj stand", "Dealer plays out"})
						}

						// ── Special ───────────────────────────────────────────
						if hasLocType("Special") {
							anyInteraction = true
							rows = append(rows, [2]string{"", "Special"})
							if r2sp, err2 := sendMsgSync(ws, "getSpecialInfo", map[string]any{"location": locInfo.Name}); err2 == nil {
								var spi specialLocationInfo
								if json.Unmarshal(r2sp, &spi) == nil {
									if spi.CanCreateCorp {
										rows = append(rows, [2]string{"corporation <name>", "Create corporation (self-funded)"})
										rows = append(rows, [2]string{"corporation <name> seed", "Create corporation (seed-funded)"})
									} else if spi.HasCorp {
										rows = append(rows, [2]string{"corporation", "(already exists)"})
									}
									if spi.CanJoinBladeburner {
										rows = append(rows, [2]string{"bladeburner", "Join the Bladeburner division"})
									} else if spi.HasBladeburner {
										rows = append(rows, [2]string{"bladeburner", "(already joined)"})
									} else if spi.BBReq != "" {
										rows = append(rows, [2]string{"bladeburner", spi.BBReq})
									}
									if spi.CanAcceptGift {
										rows = append(rows, [2]string{"stanek", "Accept Stanek's Gift"})
									} else if spi.HasGift {
										rows = append(rows, [2]string{"stanek", "(already accepted)"})
									}
									if spi.DarkscapeCost > 0 {
										if !spi.HasDarkscape {
											rows = append(rows, [2]string{"darkscape", fmt.Sprintf("Buy Darkscape Navigator (%s)", formatMoney(spi.DarkscapeCost))})
										} else {
											rows = append(rows, [2]string{"darkscape", "(already owned)"})
										}
									}
									if spi.Message != "" {
										rows = append(rows, [2]string{"", spi.Message})
									}
								}
							}
							if locInfo.Name == "Noodle Bar" {
								rows = append(rows, [2]string{"noodles", "Eat noodles"})
							}
							if locInfo.Name == "VitaLife" {
								rows = append(rows, [2]string{"graft list", "List available augmentations"})
								rows = append(rows, [2]string{"graft <augName>", "Start grafting an augmentation"})
							}
							if locInfo.Name == "CIA" || locInfo.Name == "DefComm" {
								if r2go, goErr := sendMsgSync(ws, "goGetBoard", nil); goErr == nil {
									var gs goGameState
									if json.Unmarshal(r2go, &gs) == nil && gs.BoardSize > 0 {
										rows = append(rows, [2]string{"", fmt.Sprintf("Active game: %s %dx%d  B:%.0f W:%.0f", gs.Opponent, gs.BoardSize, gs.BoardSize, gs.BlackScore, gs.WhiteScore)})
									}
								}
								rows = append(rows, [2]string{"go new <size> [opp]", "Start IPvGO game (size: 5/7/9/13)"})
								rows = append(rows, [2]string{"go board", "Show current board"})
								rows = append(rows, [2]string{"go play <x> <y>", "Place a router"})
								rows = append(rows, [2]string{"go pass", "Pass your turn"})
							}
							if locInfo.Name == "Arcade" {
								rows = append(rows, [2]string{"", "Megabyte Burner 2000 requires the game UI"})
							}
						}

						if !anyInteraction {
							rows = append(rows, [2]string{"", "No interactions available"})
						}

						// ── Always-available ──────────────────────────────────
						rows = append(rows, [2]string{"-", ""})
						rows = append(rows, [2]string{"info", "Refresh location info"})
						rows = append(rows, [2]string{"alias [name=\"value\"]", "Define an alias"})
						rows = append(rows, [2]string{"unalias <name>", "Remove an alias"})
						rows = append(rows, [2]string{"help", "Show this help"})
						rows = append(rows, [2]string{"exit", "Return to city"})

						helpTable(out, title, rows)
					} // end printLocHelp
					printLocHelp()

					prevHandler := sess.cityHandler
					sess.setPromptStr(fmt.Sprintf("[magenta][[white]%s[magenta]][-] [green]>[-] ", locInfo.Name))
					sess.currentRepl = "location"
					sess.setSubCompleter([]string{
						"info", "help", "exit", "alias", "unalias",
						"train", "study", "apply", "work",
						"servers", "buy", "upgrade", "ram", "cores", "tor",
						"heal", "crimes", "crime",
						"wse", "stocks", "sell", "short", "cover", "4s", "4sapi",
						"coinflip", "slots", "roulette", "bj",
						"corporation", "bladeburner", "stanek", "darkscape", "noodles",
						"graft", "go",
					})

					// Set context panel for this location.
					capturedLocInfo := locInfo
					sess.setContextRefresh(locInfo.Name, func(ws *websocket.Conn) {
						raw, err := sendMsgSync(ws, "getLocationInfo", map[string]any{"location": capturedLocInfo.Name})
						if err != nil {
							return
						}
						var li locationInfoResult
						if json.Unmarshal(raw, &li) != nil {
							return
						}
						hasType := func(t string) bool {
							for _, typ := range li.Types {
								if typ == t {
									return true
								}
							}
							return false
						}
						var sb strings.Builder
						if len(li.Types) > 0 {
							fmt.Fprintf(&sb, " [darkgray]%s[-]\n", strings.Join(li.Types, " · "))
						}
						if li.CompanyInfo != nil {
							ci := li.CompanyInfo
							job := "[darkgray]none[-]"
							if ci.CurrentJob != nil {
								job = *ci.CurrentJob
							}
							sb.WriteString(ctxHeader("Stats"))
							fmt.Fprintf(&sb, " [white]Rep[-]    [cyan]%s[-]\n", formatMoney(ci.Rep))
							fmt.Fprintf(&sb, " [white]Favor[-]  [cyan]%.0f[-]\n", ci.Favor)
							sb.WriteString(ctxHeader("Job"))
							fmt.Fprintf(&sb, "  %s\n", job)
							if len(ci.AvailableFields) > 0 {
								sb.WriteString(ctxHeader("Fields"))
								for _, f := range ci.AvailableFields {
									fmt.Fprintf(&sb, "  %s\n", f)
								}
							}
						} else {
							if hasType("Gym") {
								sb.WriteString(ctxHeader("Train"))
								fmt.Fprintf(&sb, "  str  def\n  dex  agi\n")
							}
							if hasType("University") {
								sb.WriteString(ctxHeader("Study"))
								for _, c := range []string{"Computer Science", "Data Structures", "Networks", "Algorithms", "Management", "Leadership"} {
									fmt.Fprintf(&sb, "  %s\n", c)
								}
							}
							if hasType("Hospital") {
								sb.WriteString(ctxHeader("Hospital"))
								fmt.Fprintf(&sb, "  heal\n")
							}
							if hasType("Slums") {
								sb.WriteString(ctxHeader("Crimes"))
								fmt.Fprintf(&sb, "  crimes\n  crime <type>\n")
							}
							if hasType("Casino") {
								sb.WriteString(ctxHeader("Games"))
								fmt.Fprintf(&sb, "  coinflip\n  slots\n  roulette\n  bj\n")
							}
							if hasType("Tech Vendor") {
								sb.WriteString(ctxHeader("Tech Vendor"))
								fmt.Fprintf(&sb, "  buy  upgrade\n  ram  cores  tor\n")
							}
							if hasType("Stock Market") {
								sb.WriteString(ctxHeader("Market"))
								fmt.Fprintf(&sb, "  stocks  buy  sell\n  short  cover\n")
							}
						}
						text := sb.String()
						sess.app.QueueUpdateDraw(func() { sess.contextView.SetText(text) })
					})

					sess.cityHandler = func(ws *websocket.Conn, out io.Writer, sess *session, line string) bool {
						lp := strings.Fields(line)
						if len(lp) == 0 {
							return true
						}
						if lp[0] == "exit" || lp[0] == "quit" {
							sess.cityHandler = prevHandler
							sess.currentRepl = "city"
							sess.setSubCompleter([]string{"ls", "travel", "info", "interact", "help", "alias", "unalias", "exit"})
							setCityPrompt()
							refreshCityContext()
							return true
						}
						rpcStr := func(method string, params map[string]any) {
							r2, err := sendMsgSync(ws, method, params)
							if err != nil {
								fmt.Fprintf(out, "error: %v\n", err)
								return
							}
							var s string
							_ = json.Unmarshal(r2, &s)
							fmt.Fprintln(out, s)
						}
						switch lp[0] {
						case "help":
							printLocHelp()
							return true
						// ── Gym ───────────────────────────────────────────────────────────
						case "train":
							if !hasLocType("Gym") {
								fmt.Fprintln(out, "not a gym")
								return true
							}
							if len(lp) < 2 {
								fmt.Fprintln(out, "usage: train <str|def|dex|agi>")
								return true
							}
							rpcStr("trainAtGym", map[string]any{"location": locInfo.Name, "stat": lp[1]})
						// ── University ────────────────────────────────────────────────────
						case "study":
							if !hasLocType("University") {
								fmt.Fprintln(out, "not a university")
								return true
							}
							if len(lp) < 2 {
								fmt.Fprintln(out, "usage: study <course>")
								return true
							}
							rpcStr("studyAtUniversity", map[string]any{"location": locInfo.Name, "course": strings.Join(lp[1:], " ")})
						// ── Company ───────────────────────────────────────────────────────
						case "info":
							if !hasLocType("Company") {
								fmt.Fprintln(out, "not a company")
								return true
							}
							r2, err := sendMsgSync(ws, "getCompanyInfo", map[string]any{"company": locInfo.Name})
							if err != nil {
								fmt.Fprintf(out, "error: %v\n", err)
								return true
							}
							var ci companyInfo
							if err := json.Unmarshal(r2, &ci); err != nil {
								fmt.Fprintf(out, "error: %v\n", err)
								return true
							}
							job := "(none — use 'apply' to get one)"
							if ci.CurrentJob != nil {
								job = *ci.CurrentJob
							}
							fmt.Fprintf(out, "Rep:     %s\nFavor:   %.0f\nJob:     %s\n", formatMoney(ci.Rep), ci.Favor, job)
							if len(ci.AvailableFields) > 0 {
								fmt.Fprintf(out, "Fields:  %s\n", strings.Join(ci.AvailableFields, ", "))
							}
						case "apply":
							if !hasLocType("Company") {
								fmt.Fprintln(out, "not a company")
								return true
							}
							if len(lp) < 2 {
								fmt.Fprintln(out, "usage: apply <field>")
								return true
							}
							rpcStr("applyAtCompany", map[string]any{"company": locInfo.Name, "field": strings.Join(lp[1:], " ")})
						case "work":
							if !hasLocType("Company") {
								fmt.Fprintln(out, "not a company")
								return true
							}
							rpcStr("workAtCompany", map[string]any{"company": locInfo.Name})
						// ── Tech Vendor ───────────────────────────────────────────────────
						case "servers":
							if !hasLocType("Tech Vendor") {
								fmt.Fprintln(out, "not a tech vendor")
								return true
							}
							r2, err := sendMsgSync(ws, "getTechVendorInfo", map[string]any{"location": locInfo.Name})
							if err != nil {
								fmt.Fprintf(out, "error: %v\n", err)
								return true
							}
							var tvi techVendorInfo
							if err := json.Unmarshal(r2, &tvi); err != nil {
								fmt.Fprintf(out, "error: %v\n", err)
								return true
							}
							fmt.Fprintf(out, "Owned: %d / %d\n", tvi.ServerCount, tvi.ServerLimit)
							if len(tvi.Servers) > 0 {
								fmt.Fprintf(out, "%-22s  RAM\n", "Hostname")
								for _, s := range tvi.Servers {
									fmt.Fprintf(out, "  %-20s  %.0fGB\n", s.Hostname, s.Ram)
								}
							}
							fmt.Fprintf(out, "\nAvailable RAM (this vendor):\n")
							for _, e := range tvi.CostsByRam {
								fmt.Fprintf(out, "  %6.0fGB  %s\n", e.Ram, formatMoney(e.Cost))
							}
							fmt.Fprintf(out, "\nHome upgrades:\n")
							fmt.Fprintf(out, "  RAM:   %.0fGB  → upgrade: %s\n", tvi.HomeRam, formatMoney(tvi.HomeRamCost))
							fmt.Fprintf(out, "  Cores: %d       → upgrade: %s\n", tvi.HomeCores, formatMoney(tvi.HomeCoresCost))
							if tvi.HasTor {
								fmt.Fprintf(out, "  TOR:   owned\n")
							} else {
								fmt.Fprintf(out, "  TOR:   %s\n", formatMoney(tvi.TorCost))
							}
						case "buy":
							if hasLocType("Stock Market") {
								if len(lp) < 3 {
									fmt.Fprintln(out, "usage: buy <SYMBOL> <shares>")
									return true
								}
								r2, err := sendMsgSync(ws, "buyStockLong", map[string]any{"stat": lp[1], "field": lp[2]})
								if err != nil {
									fmt.Fprintf(out, "error: %v\n", err)
									return true
								}
								var tx stockTxResult
								if err := json.Unmarshal(r2, &tx); err != nil {
									var s string
									_ = json.Unmarshal(r2, &s)
									fmt.Fprintln(out, s)
									return true
								}
								fmt.Fprintf(out, "Bought %d shares of %s for %s (commission %s)\n",
									tx.Shares, tx.Symbol, formatMoney(tx.TotalCost), formatMoney(tx.Commission))
							} else if hasLocType("Tech Vendor") {
								if len(lp) < 3 {
									fmt.Fprintln(out, "usage: buy <hostname> <ram>")
									return true
								}
								rpcStr("buyServer", map[string]any{"location": lp[1], "stat": lp[2]})
							} else {
								fmt.Fprintln(out, "no 'buy' available at this location")
							}
						case "upgrade":
							if !hasLocType("Tech Vendor") {
								fmt.Fprintln(out, "not a tech vendor")
								return true
							}
							if len(lp) < 3 {
								fmt.Fprintln(out, "usage: upgrade <hostname> <ram>")
								return true
							}
							rpcStr("upgradeServer", map[string]any{"location": lp[1], "stat": lp[2]})
						case "ram":
							if !hasLocType("Tech Vendor") {
								fmt.Fprintln(out, "not a tech vendor")
								return true
							}
							rpcStr("upgradeHomeRam", nil)
						case "cores":
							if !hasLocType("Tech Vendor") {
								fmt.Fprintln(out, "not a tech vendor")
								return true
							}
							rpcStr("upgradeHomeCores", nil)
						case "tor":
							if !hasLocType("Tech Vendor") {
								fmt.Fprintln(out, "not a tech vendor")
								return true
							}
							rpcStr("buyTor", nil)
						// ── Hospital ──────────────────────────────────────────────────────
						case "heal":
							if !hasLocType("Hospital") {
								fmt.Fprintln(out, "not a hospital")
								return true
							}
							rpcStr("hospitalize", nil)
						// ── Slums ─────────────────────────────────────────────────────────
						case "crimes":
							if !hasLocType("Slums") {
								fmt.Fprintln(out, "not the slums")
								return true
							}
							fmt.Fprintln(out, "Shoplift, Rob Store, Mug, Larceny, Deal Drugs, Bond Forgery,")
							fmt.Fprintln(out, "Traffick Arms, Homicide, Grand Theft Auto, Kidnap, Assassination, Heist")
						case "crime":
							if !hasLocType("Slums") {
								fmt.Fprintln(out, "not the slums")
								return true
							}
							if len(lp) < 2 {
								fmt.Fprintln(out, "usage: crime <type>")
								return true
							}
							rpcStr("commitCrime", map[string]any{"crime": strings.Join(lp[1:], " ")})
						// ── Stock Market ─────────────────────────────────────────────────
						case "wse":
							if !hasLocType("Stock Market") {
								fmt.Fprintln(out, "not the stock exchange")
								return true
							}
							rpcStr("buyWseAccount", nil)
						case "stocks":
							if !hasLocType("Stock Market") {
								fmt.Fprintln(out, "not the stock exchange")
								return true
							}
							r2, err := sendMsgSync(ws, "getStockMarketData", nil)
							if err != nil {
								fmt.Fprintf(out, "error: %v\n", err)
								return true
							}
							var smd stockMarketData
							if err := json.Unmarshal(r2, &smd); err != nil {
								fmt.Fprintf(out, "error: %v\n", err)
								return true
							}
							if !smd.HasWseAccount {
								fmt.Fprintln(out, "No WSE account. Use 'wse' to purchase access.")
								return true
							}
							has4S := smd.Has4SData
							if has4S {
								fmt.Fprintf(out, "%-5s  %-30s  %12s  %12s  %8s  %s\n", "SYM", "Name", "Price", "Position", "Forecast", "Volatility")
							} else {
								fmt.Fprintf(out, "%-5s  %-30s  %12s  %12s\n", "SYM", "Name", "Price", "Position")
							}
							for _, s := range smd.Stocks {
								pos := ""
								if s.PlayerShares > 0 {
									profit := (s.BidPrice - s.PlayerAvgPx) * float64(s.PlayerShares)
									pos = fmt.Sprintf("L %d @ %s (%+s)", s.PlayerShares, formatMoney(s.PlayerAvgPx), formatMoney(profit))
								} else if s.PlayerShortShares > 0 {
									profit := (s.PlayerAvgShortPx - s.AskPrice) * float64(s.PlayerShortShares)
									pos = fmt.Sprintf("S %d @ %s (%+s)", s.PlayerShortShares, formatMoney(s.PlayerAvgShortPx), formatMoney(profit))
								}
								if has4S {
									fmt.Fprintf(out, "%-5s  %-30s  %12s  %-30s  %7.1f%%  %.1f%%\n",
										s.Symbol, s.Name, formatMoney(s.Price), pos, s.Forecast*100, s.Volatility)
								} else {
									fmt.Fprintf(out, "%-5s  %-30s  %12s  %s\n", s.Symbol, s.Name, formatMoney(s.Price), pos)
								}
							}
						case "sell":
							if !hasLocType("Stock Market") {
								fmt.Fprintln(out, "not the stock exchange")
								return true
							}
							if len(lp) < 3 {
								fmt.Fprintln(out, "usage: sell <SYMBOL> <shares|-1>")
								return true
							}
							r2, err := sendMsgSync(ws, "sellStockLong", map[string]any{"stat": lp[1], "field": lp[2]})
							if err != nil {
								fmt.Fprintf(out, "error: %v\n", err)
								return true
							}
							var tx stockTxResult
							if err := json.Unmarshal(r2, &tx); err != nil {
								var s string
								_ = json.Unmarshal(r2, &s)
								fmt.Fprintln(out, s)
								return true
							}
							fmt.Fprintf(out, "Sold %d shares of %s for %s (commission %s)\n",
								tx.Shares, tx.Symbol, formatMoney(tx.TotalCost), formatMoney(tx.Commission))
						case "short":
							if !hasLocType("Stock Market") {
								fmt.Fprintln(out, "not the stock exchange")
								return true
							}
							if len(lp) < 3 {
								fmt.Fprintln(out, "usage: short <SYMBOL> <shares>")
								return true
							}
							r2, err := sendMsgSync(ws, "buyStockShort", map[string]any{"stat": lp[1], "field": lp[2]})
							if err != nil {
								fmt.Fprintf(out, "error: %v\n", err)
								return true
							}
							var tx stockTxResult
							if err := json.Unmarshal(r2, &tx); err != nil {
								var s string
								_ = json.Unmarshal(r2, &s)
								fmt.Fprintln(out, s)
								return true
							}
							fmt.Fprintf(out, "Shorted %d shares of %s for %s (commission %s)\n",
								tx.Shares, tx.Symbol, formatMoney(tx.TotalCost), formatMoney(tx.Commission))
						case "cover":
							if !hasLocType("Stock Market") {
								fmt.Fprintln(out, "not the stock exchange")
								return true
							}
							if len(lp) < 3 {
								fmt.Fprintln(out, "usage: cover <SYMBOL> <shares|-1>")
								return true
							}
							r2, err := sendMsgSync(ws, "sellStockShort", map[string]any{"stat": lp[1], "field": lp[2]})
							if err != nil {
								fmt.Fprintf(out, "error: %v\n", err)
								return true
							}
							var tx stockTxResult
							if err := json.Unmarshal(r2, &tx); err != nil {
								var s string
								_ = json.Unmarshal(r2, &s)
								fmt.Fprintln(out, s)
								return true
							}
							fmt.Fprintf(out, "Covered %d shares of %s, received %s (commission %s)\n",
								tx.Shares, tx.Symbol, formatMoney(tx.TotalCost), formatMoney(tx.Commission))
						case "4s":
							if !hasLocType("Stock Market") {
								fmt.Fprintln(out, "not the stock exchange")
								return true
							}
							rpcStr("buy4SData", nil)
						case "4sapi":
							if !hasLocType("Stock Market") {
								fmt.Fprintln(out, "not the stock exchange")
								return true
							}
							rpcStr("buy4STixApi", nil)
						// ── Casino ────────────────────────────────────────────────────────
						case "coinflip":
							if !hasLocType("Casino") {
								fmt.Fprintln(out, "not a casino")
								return true
							}
							if len(lp) < 3 {
								fmt.Fprintln(out, "usage: coinflip <heads|tails> <bet>")
								return true
							}
							side := strings.ToLower(lp[1])
							if side != "heads" && side != "tails" {
								fmt.Fprintln(out, "usage: coinflip <heads|tails> <bet>")
								return true
							}
							r2, err := sendMsgSync(ws, "playCoinFlip", map[string]any{"stat": side, "field": lp[2]})
							if err != nil {
								fmt.Fprintf(out, "error: %v\n", err)
								return true
							}
							var cr casinoResult
							if err := json.Unmarshal(r2, &cr); err != nil {
								fmt.Fprintf(out, "error: %v\n", err)
								return true
							}
							if cr.Won {
								fmt.Fprintf(out, "Landed: %s — You WIN! +%s\n", cr.Result, formatMoney(cr.Payout))
							} else {
								fmt.Fprintf(out, "Landed: %s — You lose. -%s\n", cr.Result, formatMoney(cr.Bet))
							}
						case "slots":
							if !hasLocType("Casino") {
								fmt.Fprintln(out, "not a casino")
								return true
							}
							if len(lp) < 2 {
								fmt.Fprintln(out, "usage: slots <bet>")
								return true
							}
							r2, err := sendMsgSync(ws, "playSlots", map[string]any{"field": lp[1]})
							if err != nil {
								fmt.Fprintf(out, "error: %v\n", err)
								return true
							}
							var sr slotsResult
							if err := json.Unmarshal(r2, &sr); err != nil {
								var s string
								_ = json.Unmarshal(r2, &s)
								fmt.Fprintln(out, s)
								return true
							}
							for _, row := range sr.Grid {
								fmt.Fprintf(out, "  | %s |\n", strings.Join(row, " | "))
							}
							if len(sr.Paylines) == 0 {
								fmt.Fprintf(out, "No winning lines. Lost %s\n", formatMoney(sr.Bet))
							} else {
								for _, pl := range sr.Paylines {
									fmt.Fprintf(out, "%s: %dx %s  +%s\n", pl.Name, pl.Count, pl.Symbol, formatMoney(pl.Gain))
								}
								fmt.Fprintf(out, "Net: %+s\n", formatMoney(sr.NetGain))
							}
						case "roulette":
							if !hasLocType("Casino") {
								fmt.Fprintln(out, "not a casino")
								return true
							}
							if len(lp) < 3 {
								fmt.Fprintln(out, "usage: roulette <strategy> <bet>")
								fmt.Fprintln(out, "  strategies: red black odd even high low 1-12 13-24 25-36 0-36")
								return true
							}
							r2, err := sendMsgSync(ws, "playRoulette", map[string]any{"stat": lp[1], "field": lp[2]})
							if err != nil {
								fmt.Fprintf(out, "error: %v\n", err)
								return true
							}
							var rr rouletteResult
							if err := json.Unmarshal(r2, &rr); err != nil {
								var s string
								_ = json.Unmarshal(r2, &s)
								fmt.Fprintln(out, s)
								return true
							}
							colorStr := rr.LandedColor
							if rr.Won {
								fmt.Fprintf(out, "Landed: %d (%s) — Bet: %s, strategy: %s — WIN +%s\n",
									rr.LandedNumber, colorStr, formatMoney(rr.Bet), rr.Strategy, formatMoney(rr.Gain))
							} else {
								fmt.Fprintf(out, "Landed: %d (%s) — Bet: %s, strategy: %s — LOSS -%s\n",
									rr.LandedNumber, colorStr, formatMoney(rr.Bet), rr.Strategy, formatMoney(rr.Bet))
							}
						case "bj":
							if !hasLocType("Casino") {
								fmt.Fprintln(out, "not a casino")
								return true
							}
							if len(lp) < 2 {
								fmt.Fprintln(out, "usage: bj <deal <bet>|hit|stand>")
								return true
							}
							printBJState := func(bs blackjackState) {
								fmtHand := func(cards []blackjackCard) string {
									parts := make([]string, len(cards))
									for i, c := range cards {
										if c.Hidden {
											parts[i] = "[?]"
										} else {
											parts[i] = c.Value + string([]rune(c.Suit)[0])
										}
									}
									return strings.Join(parts, " ")
								}
								fmt.Fprintf(out, "Player: %s  (%d)\n", fmtHand(bs.PlayerHand), bs.PlayerValue)
								fmt.Fprintf(out, "Dealer: %s  (%d)\n", fmtHand(bs.DealerHand), bs.DealerValue)
								switch bs.Status {
								case "player_turn":
									fmt.Fprintln(out, "Your turn: bj hit | bj stand")
								case "player_won":
									fmt.Fprintf(out, "You WIN! +%s\n", formatMoney(bs.Bet))
								case "player_won_blackjack":
									fmt.Fprintf(out, "BLACKJACK! +%s\n", formatMoney(bs.Bet))
								case "dealer_won", "dealer_won_blackjack":
									fmt.Fprintf(out, "Dealer wins. -%s\n", formatMoney(bs.Bet))
								case "tie":
									fmt.Fprintln(out, "Push (tie).")
								}
							}
							var method string
							var params map[string]any
							switch strings.ToLower(lp[1]) {
							case "deal":
								if len(lp) < 3 {
									fmt.Fprintln(out, "usage: bj deal <bet>")
									return true
								}
								method = "blackjackDeal"
								params = map[string]any{"field": lp[2]}
							case "hit":
								method = "blackjackHit"
								params = nil
							case "stand":
								method = "blackjackStand"
								params = nil
							default:
								fmt.Fprintf(out, "unknown bj command: %s\n", lp[1])
								return true
							}
							r2, err := sendMsgSync(ws, method, params)
							if err != nil {
								fmt.Fprintf(out, "error: %v\n", err)
								return true
							}
							var bs blackjackState
							if err := json.Unmarshal(r2, &bs); err != nil {
								var s string
								_ = json.Unmarshal(r2, &s)
								fmt.Fprintln(out, s)
								return true
							}
							printBJState(bs)
						// ── Special ───────────────────────────────────────────────────────
						case "corporation":
							if !hasLocType("Special") {
								fmt.Fprintln(out, "not a special location")
								return true
							}
							if len(lp) < 2 {
								fmt.Fprintln(out, "usage: corporation <name> [seed]")
								return true
							}
							name := lp[1]
							field := "selfFund"
							if len(lp) >= 3 && strings.EqualFold(lp[2], "seed") {
								field = "seed"
							}
							rpcStr("createCorporation", map[string]any{"stat": name, "field": field})
						case "bladeburner":
							if !hasLocType("Special") {
								fmt.Fprintln(out, "not a special location")
								return true
							}
							rpcStr("joinBladeburner", nil)
						case "stanek":
							if !hasLocType("Special") {
								fmt.Fprintln(out, "not a special location")
								return true
							}
							rpcStr("acceptStaneksGift", nil)
						case "darkscape":
							if !hasLocType("Special") {
								fmt.Fprintln(out, "not a special location")
								return true
							}
							rpcStr("buyDarkscape", nil)
						case "noodles":
							if !hasLocType("Special") {
								fmt.Fprintln(out, "not a special location")
								return true
							}
							rpcStr("eatNoodles", nil)
						// ── Grafting (VitaLife) ───────────────────────────────────────────
						case "graft":
							if !hasLocType("Special") {
								fmt.Fprintln(out, "not a special location")
								return true
							}
							if len(lp) < 2 {
								fmt.Fprintln(out, "usage: graft list | graft <augName>")
								return true
							}
							if strings.EqualFold(lp[1], "list") {
								raw, err := sendMsgSync(ws, "listGraftableAugs", nil)
								if err != nil {
									fmt.Fprintf(out, "error: %v\n", err)
									return true
								}
								var augs []graftableAugInfo
								if err := json.Unmarshal(raw, &augs); err != nil {
									var s string
									_ = json.Unmarshal(raw, &s)
									fmt.Fprintln(out, s)
									return true
								}
								if len(augs) == 0 {
									fmt.Fprintln(out, "no augmentations available for grafting")
									return true
								}
								w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
								fmt.Fprintln(w, "Name\tCost\tTime\tReqs Met\tOwned")
								for _, a := range augs {
									mins := a.Time / 60000
									fmt.Fprintf(w, "%s\t%s\t%.1fm\t%v\t%v\n",
										a.Name, formatMoney(a.Cost), mins, a.HasPrereqs, a.AlreadyOwned)
								}
								w.Flush()
							} else {
								augName := strings.Join(lp[1:], " ")
								raw, err := sendMsgSync(ws, "startGrafting", map[string]any{"stat": augName})
								if err != nil {
									fmt.Fprintf(out, "error: %v\n", err)
									return true
								}
								var s string
								_ = json.Unmarshal(raw, &s)
								fmt.Fprintln(out, s)
							}
						// ── IPvGO (CIA / DefComm) ─────────────────────────────────────────
						case "go":
							if !hasLocType("Special") {
								fmt.Fprintln(out, "not a special location")
								return true
							}
							if len(lp) < 2 {
								fmt.Fprintln(out, "usage: go new <size> [opponent] | go board | go play <x> <y> | go pass")
								return true
							}
							printGoBoard := func(board []string) {
								size := len(board)
								fmt.Fprintf(out, "  ")
								for c := 0; c < size; c++ {
									fmt.Fprintf(out, "%2d", c)
								}
								fmt.Fprintln(out)
								for r := 0; r < size; r++ {
									fmt.Fprintf(out, "%2d", r)
									for c := 0; c < size; c++ {
										ch := '.'
										if r < len(board[c]) {
											switch board[c][r] {
											case 'X':
												ch = 'X'
											case 'O':
												ch = 'O'
											case '#':
												ch = '#'
											}
										}
										fmt.Fprintf(out, " %c", ch)
									}
									fmt.Fprintln(out)
								}
							}
							switch strings.ToLower(lp[1]) {
							case "new":
								if len(lp) < 3 {
									fmt.Fprintln(out, "usage: go new <size> [opponent]")
									return true
								}
								params := map[string]any{"stat": lp[2]}
								if len(lp) >= 4 {
									params["field"] = strings.Join(lp[3:], " ")
								}
								raw, err := sendMsgSync(ws, "goNewGame", params)
								if err != nil {
									fmt.Fprintf(out, "error: %v\n", err)
									return true
								}
								var gs goGameState
								if err := json.Unmarshal(raw, &gs); err != nil {
									var s string
									_ = json.Unmarshal(raw, &s)
									fmt.Fprintln(out, s)
									return true
								}
								fmt.Fprintf(out, "New game vs %s (%dx%d). You are Black (X).\n", gs.Opponent, gs.BoardSize, gs.BoardSize)
								printGoBoard(gs.Board)
							case "board":
								raw, err := sendMsgSync(ws, "goGetBoard", nil)
								if err != nil {
									fmt.Fprintf(out, "error: %v\n", err)
									return true
								}
								var gs goGameState
								if err := json.Unmarshal(raw, &gs); err != nil {
									var s string
									_ = json.Unmarshal(raw, &s)
									fmt.Fprintln(out, s)
									return true
								}
								fmt.Fprintf(out, "vs %s  Black: %.0f  White: %.0f  Komi: %.1f  Turn: %s\n",
									gs.Opponent, gs.BlackScore, gs.WhiteScore, gs.Komi, gs.CurrentPlayer)
								printGoBoard(gs.Board)
							case "play":
								if len(lp) < 4 {
									fmt.Fprintln(out, "usage: go play <x> <y>")
									return true
								}
								raw, err := sendMsgSync(ws, "goMakeMove", map[string]any{"stat": lp[2], "field": lp[3]})
								if err != nil {
									fmt.Fprintf(out, "error: %v\n", err)
									return true
								}
								var mr goMoveResult
								if err := json.Unmarshal(raw, &mr); err != nil {
									var s string
									_ = json.Unmarshal(raw, &s)
									fmt.Fprintln(out, s)
									return true
								}
								if mr.Message != "" {
									fmt.Fprintln(out, mr.Message)
								}
								if mr.Type == "gameOver" {
									fmt.Fprintf(out, "Game over. Black: %.0f  White: %.0f\n", mr.BlackScore, mr.WhiteScore)
								} else {
									printGoBoard(mr.Board)
									fmt.Fprintf(out, "Black: %.0f  White: %.0f  Turn: %s\n", mr.BlackScore, mr.WhiteScore, mr.CurrentPlayer)
								}
							case "pass":
								raw, err := sendMsgSync(ws, "goPassTurn", nil)
								if err != nil {
									fmt.Fprintf(out, "error: %v\n", err)
									return true
								}
								var mr goMoveResult
								if err := json.Unmarshal(raw, &mr); err != nil {
									var s string
									_ = json.Unmarshal(raw, &s)
									fmt.Fprintln(out, s)
									return true
								}
								if mr.Message != "" {
									fmt.Fprintln(out, mr.Message)
								}
								if mr.Type == "gameOver" {
									fmt.Fprintf(out, "Game over. Black: %.0f  White: %.0f\n", mr.BlackScore, mr.WhiteScore)
								} else {
									printGoBoard(mr.Board)
									fmt.Fprintf(out, "Black: %.0f  White: %.0f  Turn: %s\n", mr.BlackScore, mr.WhiteScore, mr.CurrentPlayer)
								}
							default:
								fmt.Fprintf(out, "unknown go command: %s\n", lp[1])
							}
						case "alias":
							am := replAliases(sess.currentRepl)
							if len(lp) < 2 {
								if len(am) == 0 {
									fmt.Fprintln(out, "no aliases defined")
								} else {
									keys := make([]string, 0, len(am))
									for k := range am {
										keys = append(keys, k)
									}
									sort.Strings(keys)
									for _, k := range keys {
										fmt.Fprintf(out, "alias %s=%q\n", k, am[k])
									}
								}
								return true
							}
							name := lp[1]
							value := strings.Join(lp[2:], " ")
							if value == "" && strings.Contains(name, "=") {
								idx := strings.IndexByte(name, '=')
								value = name[idx+1:]
								name = name[:idx]
							}
							am[name] = value
							saveAliases()
							fmt.Fprintf(out, "alias %s=%q\n", name, value)
						case "unalias":
							if len(lp) < 2 {
								fmt.Fprintln(out, "usage: unalias <name>")
								return true
							}
							delete(replAliases(sess.currentRepl), lp[1])
							saveAliases()
						default:
							fmt.Fprintf(out, "unknown: %s\n", lp[0])
						}
						return true
					}
				case "alias":
					am := replAliases(sess.currentRepl)
					if len(cityParts) < 2 {
						if len(am) == 0 {
							fmt.Fprintln(out, "no aliases defined")
						} else {
							keys := make([]string, 0, len(am))
							for k := range am {
								keys = append(keys, k)
							}
							sort.Strings(keys)
							for _, k := range keys {
								fmt.Fprintf(out, "alias %s=%q\n", k, am[k])
							}
						}
						return true
					}
					name := cityParts[1]
					value := strings.Join(cityParts[2:], " ")
					if value == "" && strings.Contains(name, "=") {
						idx := strings.IndexByte(name, '=')
						value = name[idx+1:]
						name = name[:idx]
					}
					am[name] = value
					saveAliases()
					fmt.Fprintf(out, "alias %s=%q\n", name, value)
				case "unalias":
					if len(cityParts) < 2 {
						fmt.Fprintln(out, "usage: unalias <name>")
						return true
					}
					delete(replAliases(sess.currentRepl), cityParts[1])
					saveAliases()
				default:
					fmt.Fprintf(out, "unknown command: %s\n", cityParts[0])
				}
				return true
			}
		}},

		{"scan", func(ws *websocket.Conn, out io.Writer, sess *session, parts []string) {
			server := sess.currentServer
			if len(parts) > 1 {
				server = parts[1]
			}
			raw, err := sendMsgSync(ws, "scanServer", map[string]string{"server": server})
			if err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
				return
			}
			var entries []scanEntry
			if err := json.Unmarshal(raw, &entries); err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
				return
			}
			if len(entries) == 0 {
				fmt.Fprintln(out, "no connections")
				return
			}
			maxHost := len("Hostname")
			maxIP := len("IP")
			for _, e := range entries {
				if len(e.Hostname) > maxHost {
					maxHost = len(e.Hostname)
				}
				if len(e.IP) > maxIP {
					maxIP = len(e.IP)
				}
			}
			fmt.Fprintf(out, "%-*s  %-*s  Root\n", maxHost, "Hostname", maxIP, "IP")
			for _, e := range entries {
				root := "N"
				if e.HasAdminRights {
					root = "Y"
				}
				fmt.Fprintf(out, "%-*s  %-*s  %s\n", maxHost, e.Hostname, maxIP, e.IP, root)
			}
		}},
		{"scan-analyze", func(ws *websocket.Conn, out io.Writer, sess *session, parts []string) {
			depth := 1
			all := false
			for i := 1; i < len(parts); i++ {
				switch parts[i] {
				case "-a":
					all = true
				default:
					if n, err := strconv.Atoi(parts[i]); err == nil && n > 0 {
						depth = n
					}
				}
			}
			raw, err := sendMsgSync(ws, "scanAnalyze", map[string]any{"depth": depth, "all": all})
			if err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
				return
			}
			var nodes []scanNode
			if err := json.Unmarshal(raw, &nodes); err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
				return
			}
			fmt.Fprintf(out, "[%s]\n", sess.currentServer)
			printScanTree(out, nodes, "", false)
		}},

		// ── File operations ───────────────────────────────────────────────────────

		{"ls", func(ws *websocket.Conn, out io.Writer, sess *session, parts []string) {
			server := sess.currentServer
			remote := len(parts) > 1
			if remote {
				server = parts[1]
			}
			raw, err := sendMsgSync(ws, "getFileNames", map[string]string{"server": server})
			if err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
				return
			}
			var files []string
			if err := json.Unmarshal(raw, &files); err != nil {
				fmt.Fprintf(out, "error parsing response: %v\n", err)
				return
			}
			if !remote {
				sess.serverFiles = files
				sess.refreshFileCompleter()
			}
			for _, f := range files {
				fmt.Fprintln(out, f)
			}
		}},
		{"cat", func(ws *websocket.Conn, out io.Writer, sess *session, parts []string) {
			if len(parts) < 2 {
				fmt.Fprintln(out, "usage: cat <filename>")
				return
			}
			raw, err := sendMsgSync(ws, "getFile", map[string]string{"server": sess.currentServer, "filename": parts[1]})
			if err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
				return
			}
			var content string
			if err := json.Unmarshal(raw, &content); err != nil {
				fmt.Fprintf(out, "error parsing response: %v\n", err)
				return
			}
			fmt.Fprintln(out, content)
		}},
		{"cp", func(ws *websocket.Conn, out io.Writer, sess *session, parts []string) {
			if len(parts) < 3 {
				fmt.Fprintln(out, "usage: cp <src> <dst>")
				return
			}
			raw, err := sendMsgSync(ws, "getFile", map[string]string{"server": sess.currentServer, "filename": parts[1]})
			if err != nil {
				fmt.Fprintf(out, "error reading %s: %v\n", parts[1], err)
				return
			}
			var content string
			if err := json.Unmarshal(raw, &content); err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
				return
			}
			_, err = sendMsgSync(ws, "pushFile", map[string]string{"server": sess.currentServer, "filename": parts[2], "content": content})
			if err != nil {
				fmt.Fprintf(out, "error writing %s: %v\n", parts[2], err)
				return
			}
			fmt.Fprintf(out, "%s -> %s\n", parts[1], parts[2])
			refreshFilesIfCurrent(ws, sess, sess.currentServer)
		}},
		{"mv", func(ws *websocket.Conn, out io.Writer, sess *session, parts []string) {
			if len(parts) < 3 {
				fmt.Fprintln(out, "usage: mv <src> <dst>")
				return
			}
			raw, err := sendMsgSync(ws, "getFile", map[string]string{"server": sess.currentServer, "filename": parts[1]})
			if err != nil {
				fmt.Fprintf(out, "error reading %s: %v\n", parts[1], err)
				return
			}
			var content string
			if err := json.Unmarshal(raw, &content); err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
				return
			}
			if _, err = sendMsgSync(ws, "pushFile", map[string]string{"server": sess.currentServer, "filename": parts[2], "content": content}); err != nil {
				fmt.Fprintf(out, "error writing %s: %v\n", parts[2], err)
				return
			}
			if _, err = sendMsgSync(ws, "deleteFile", map[string]string{"server": sess.currentServer, "filename": parts[1]}); err != nil {
				fmt.Fprintf(out, "warning: could not delete %s: %v\n", parts[1], err)
			}
			fmt.Fprintf(out, "%s -> %s\n", parts[1], parts[2])
			refreshFilesIfCurrent(ws, sess, sess.currentServer)
		}},
		{"rm", func(ws *websocket.Conn, out io.Writer, sess *session, parts []string) {
			if len(parts) < 2 {
				fmt.Fprintln(out, "usage: rm <filename>")
				return
			}
			if _, err := sendMsgSync(ws, "deleteFile", map[string]string{"server": sess.currentServer, "filename": parts[1]}); err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
				return
			}
			refreshFilesIfCurrent(ws, sess, sess.currentServer)
		}},
		{"scp", func(ws *websocket.Conn, out io.Writer, sess *session, parts []string) {
			if len(parts) < 3 {
				fmt.Fprintln(out, "usage: scp <file1> [file2...] <dst_server>")
				return
			}
			files := parts[1 : len(parts)-1]
			dst := parts[len(parts)-1]
			for _, f := range files {
				raw, err := sendMsgSync(ws, "getFile", map[string]string{"server": sess.currentServer, "filename": f})
				if err != nil {
					fmt.Fprintf(out, "error reading %s: %v\n", f, err)
					continue
				}
				var content string
				if err := json.Unmarshal(raw, &content); err != nil {
					fmt.Fprintf(out, "error: %v\n", err)
					continue
				}
				if _, err = sendMsgSync(ws, "pushFile", map[string]string{"server": dst, "filename": f, "content": content}); err != nil {
					fmt.Fprintf(out, "error writing %s to %s: %v\n", f, dst, err)
					continue
				}
				fmt.Fprintf(out, "%s -> %s:%s\n", f, dst, f)
			}
		}},
		{"vim", func(ws *websocket.Conn, out io.Writer, sess *session, parts []string) {
			if len(parts) < 2 {
				fmt.Fprintln(out, "usage: vim <scriptname>")
				return
			}
			if _, err := exec.LookPath("code"); err != nil {
				fmt.Fprintln(out, "error: 'code' not found in PATH — is VS Code installed?")
				return
			}

			filename := parts[1]
			base := filepath.Base(filename)
			server := sess.currentServer

			tmpDir := filepath.Join(os.TempDir(), "bb-remote-vim")
			if err := os.MkdirAll(tmpDir, 0755); err != nil {
				fmt.Fprintf(out, "error creating temp dir: %v\n", err)
				return
			}
			tmpFile := filepath.Join(tmpDir, base)

			raw, err := sendMsgSync(ws, "getFile", map[string]string{
				"server": server, "filename": filename,
			})
			var initial string
			if err == nil {
				_ = json.Unmarshal(raw, &initial)
			}
			if err := os.WriteFile(tmpFile, []byte(initial), 0644); err != nil {
				fmt.Fprintf(out, "error writing temp file: %v\n", err)
				return
			}

			info, _ := os.Stat(tmpFile)
			lastMod := info.ModTime()

			upload := func() {
				content, err := os.ReadFile(tmpFile)
				if err != nil {
					fmt.Fprintf(out, "vim: read error: %v\n", err)
					return
				}
				conn := getActiveWS()
				if conn == nil {
					fmt.Fprintf(out, "vim: no active connection — %s not uploaded\n", filename)
					return
				}
				if _, err := sendMsgSync(conn, "pushFile", map[string]string{
					"server": server, "filename": filename, "content": string(content),
				}); err != nil {
					fmt.Fprintf(out, "vim: upload error: %v\n", err)
				} else {
					fmt.Fprintf(out, "uploaded %s to %s\n", filename, server)
					refreshFilesIfCurrent(conn, sess, server)
				}
			}

			fmt.Fprintf(out, "opening %s in VS Code (auto-upload on save)...\n", filename)

			go func() {
				done := make(chan struct{})
				go func() {
					ticker := time.NewTicker(500 * time.Millisecond)
					defer ticker.Stop()
					for {
						select {
						case <-done:
							return
						case <-ticker.C:
							info, err := os.Stat(tmpFile)
							if err != nil || !info.ModTime().After(lastMod) {
								continue
							}
							lastMod = info.ModTime()
							upload()
						}
					}
				}()

				_ = exec.Command("code", "--wait", tmpFile).Run()
				close(done)
				upload() // final upload on tab close
				_ = os.Remove(tmpFile)
				fmt.Fprintf(out, "vim: closed %s\n", filename)
			}()
		}},
		{"grep", func(ws *websocket.Conn, out io.Writer, sess *session, parts []string) {
			if len(parts) < 2 {
				fmt.Fprintln(out, "usage: grep <pattern> [file...]")
				return
			}
			re, err := regexp.Compile(parts[1])
			if err != nil {
				fmt.Fprintf(out, "invalid pattern: %v\n", err)
				return
			}
			raw, err := sendMsgSync(ws, "getAllFiles", map[string]string{"server": sess.currentServer})
			if err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
				return
			}
			var allFiles []fileContent
			if err := json.Unmarshal(raw, &allFiles); err != nil {
				fmt.Fprintf(out, "error parsing response: %v\n", err)
				return
			}
			filter := map[string]bool{}
			for _, f := range parts[2:] {
				filter[f] = true
			}
			for _, f := range allFiles {
				if len(filter) > 0 && !filter[f.Filename] {
					continue
				}
				for i, line := range strings.Split(f.Content, "\n") {
					if re.MatchString(line) {
						fmt.Fprintf(out, "%s:%d: %s\n", f.Filename, i+1, line)
					}
				}
			}
		}},
		{"download", func(ws *websocket.Conn, out io.Writer, sess *session, parts []string) {
			if len(parts) < 2 {
				fmt.Fprintln(out, "usage: download <filename>")
				return
			}
			raw, err := sendMsgSync(ws, "getFile", map[string]string{"server": sess.currentServer, "filename": parts[1]})
			if err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
				return
			}
			var content string
			if err := json.Unmarshal(raw, &content); err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
				return
			}
			// Use last path component as local filename
			local := parts[1]
			if i := strings.LastIndexAny(parts[1], "/\\"); i >= 0 {
				local = parts[1][i+1:]
			}
			if err := os.WriteFile(local, []byte(content), 0644); err != nil {
				fmt.Fprintf(out, "error writing file: %v\n", err)
				return
			}
			fmt.Fprintf(out, "saved %s (%d bytes)\n", local, len(content))
		}},

		// ── Server info ───────────────────────────────────────────────────────────

		{"free", func(ws *websocket.Conn, out io.Writer, sess *session, parts []string) {
			server := sess.currentServer
			if len(parts) > 1 {
				server = parts[1]
			}
			raw, err := sendMsgSync(ws, "getServerInfo", map[string]string{"server": server})
			if err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
				return
			}
			var info serverInfo
			if err := json.Unmarshal(raw, &info); err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
				return
			}
			pct := 0.0
			if info.MaxRam > 0 {
				pct = (info.RamUsed / info.MaxRam) * 100
			}
			fmt.Fprintf(out, "RAM:    %s / %s used (%.1f%%)\n", formatRAM(info.RamUsed), formatRAM(info.MaxRam), pct)
			fmt.Fprintf(out, "Cores:  %d\n", info.CPUCores)
		}},
		{"hostname", func(ws *websocket.Conn, out io.Writer, sess *session, parts []string) {
			server := sess.currentServer
			if len(parts) > 1 {
				server = parts[1]
			}
			raw, err := sendMsgSync(ws, "getServerInfo", map[string]string{"server": server})
			if err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
				return
			}
			var info serverInfo
			if err := json.Unmarshal(raw, &info); err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
				return
			}
			fmt.Fprintln(out, info.Hostname)
		}},
		{"ipaddr", func(ws *websocket.Conn, out io.Writer, sess *session, parts []string) {
			server := sess.currentServer
			if len(parts) > 1 {
				server = parts[1]
			}
			raw, err := sendMsgSync(ws, "getServerInfo", map[string]string{"server": server})
			if err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
				return
			}
			var info serverInfo
			if err := json.Unmarshal(raw, &info); err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
				return
			}
			fmt.Fprintln(out, info.IP)
		}},
		{"lscpu", func(ws *websocket.Conn, out io.Writer, sess *session, parts []string) {
			server := sess.currentServer
			if len(parts) > 1 {
				server = parts[1]
			}
			raw, err := sendMsgSync(ws, "getServerInfo", map[string]string{"server": server})
			if err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
				return
			}
			var info serverInfo
			if err := json.Unmarshal(raw, &info); err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
				return
			}
			fmt.Fprintf(out, "CPU(s): %d\n", info.CPUCores)
		}},
		{"sudov", func(ws *websocket.Conn, out io.Writer, sess *session, parts []string) {
			server := sess.currentServer
			if len(parts) > 1 {
				server = parts[1]
			}
			raw, err := sendMsgSync(ws, "getServerInfo", map[string]string{"server": server})
			if err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
				return
			}
			var info serverInfo
			if err := json.Unmarshal(raw, &info); err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
				return
			}
			if info.HasAdminRights {
				fmt.Fprintln(out, "You have ROOT access to this computer.")
			} else {
				fmt.Fprintln(out, "You do NOT have root access to this computer.")
			}
		}},
		{"analyze", func(ws *websocket.Conn, out io.Writer, sess *session, parts []string) {
			server := sess.currentServer
			if len(parts) > 1 {
				server = parts[1]
			}
			raw, err := sendMsgSync(ws, "getServerInfo", map[string]string{"server": server})
			if err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
				return
			}
			var info serverInfo
			if err := json.Unmarshal(raw, &info); err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
				return
			}
			boolStr := func(b bool) string {
				if b {
					return "YES"
				}
				return "NO"
			}
			openPorts := []string{}
			if info.SSHPortOpen {
				openPorts = append(openPorts, "SSH")
			}
			if info.FTPPortOpen {
				openPorts = append(openPorts, "FTP")
			}
			if info.SMTPPortOpen {
				openPorts = append(openPorts, "SMTP")
			}
			if info.HTTPPortOpen {
				openPorts = append(openPorts, "HTTP")
			}
			if info.SQLPortOpen {
				openPorts = append(openPorts, "SQL")
			}
			portsStr := "none"
			if len(openPorts) > 0 {
				portsStr = strings.Join(openPorts, ", ")
			}
			fmt.Fprintf(out, "Hostname:          %s\n", info.Hostname)
			fmt.Fprintf(out, "Organization:      %s\n", info.OrganizationName)
			fmt.Fprintf(out, "IP:                %s\n", info.IP)
			fmt.Fprintf(out, "Root access:       %s\n", boolStr(info.HasAdminRights))
			fmt.Fprintf(out, "Backdoor:          %s\n", boolStr(info.BackdoorInstalled))
			fmt.Fprintf(out, "RAM:               %s / %s\n", formatRAM(info.RamUsed), formatRAM(info.MaxRam))
			fmt.Fprintf(out, "CPU Cores:         %d\n", info.CPUCores)
			fmt.Fprintf(out, "Req. Hacking:      %d\n", info.RequiredHacking)
			fmt.Fprintf(out, "Ports required:    %d (open: %s)\n", info.NumOpenPortsRequired, portsStr)
			fmt.Fprintf(out, "Security:          %.2f (min %.2f)\n", info.HackDifficulty, info.MinDifficulty)
			fmt.Fprintf(out, "Hack chance:       %.1f%%\n", info.HackingChance*100)
			fmt.Fprintf(out, "Hack time:         %.1fs\n", info.HackingTime/1000)
			fmt.Fprintf(out, "Money:             %s / %s\n", formatMoney(info.MoneyAvailable), formatMoney(info.MoneyMax))
		}},

		// ── Script management ─────────────────────────────────────────────────────

		{"ps", func(ws *websocket.Conn, out io.Writer, sess *session, parts []string) {
			server := sess.currentServer
			if len(parts) > 1 {
				server = parts[1]
			}
			raw, err := sendMsgSync(ws, "getRunningScripts", map[string]string{"server": server})
			if err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
				return
			}
			var scripts []runningScript
			if err := json.Unmarshal(raw, &scripts); err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
				return
			}
			if len(scripts) == 0 {
				fmt.Fprintln(out, "no scripts running")
				return
			}
			fmt.Fprintf(out, "%-8s  %-4s  %-8s  %s\n", "PID", "T", "RAM", "SCRIPT")
			for _, s := range scripts {
				args := ""
				if len(s.Args) > 0 {
					b, _ := json.Marshal(s.Args)
					args = " " + string(b)
				}
				fmt.Fprintf(out, "%-8d  %-4d  %-8s  %s%s\n", s.PID, s.Threads, formatRAM(s.RamUsage*float64(s.Threads)), s.Filename, args)
			}
		}},
		{"top", func(ws *websocket.Conn, out io.Writer, sess *session, parts []string) {
			server := sess.currentServer
			if len(parts) > 1 {
				server = parts[1]
			}
			raw, err := sendMsgSync(ws, "getRunningScripts", map[string]string{"server": server})
			if err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
				return
			}
			var scripts []runningScript
			if err := json.Unmarshal(raw, &scripts); err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
				return
			}
			sort.Slice(scripts, func(i, j int) bool {
				return scripts[i].RamUsage*float64(scripts[i].Threads) > scripts[j].RamUsage*float64(scripts[j].Threads)
			})
			if len(scripts) == 0 {
				fmt.Fprintln(out, "no scripts running")
				return
			}
			fmt.Fprintf(out, "%-8s  %-4s  %-8s  %-12s  %s\n", "PID", "T", "RAM", "RUNNING", "SCRIPT")
			for _, s := range scripts {
				args := ""
				if len(s.Args) > 0 {
					b, _ := json.Marshal(s.Args)
					args = " " + string(b)
				}
				fmt.Fprintf(out, "%-8d  %-4d  %-8s  %-12s  %s%s\n",
					s.PID, s.Threads, formatRAM(s.RamUsage*float64(s.Threads)),
					formatTime(s.OnlineRunningTime), s.Filename, args)
			}
		}},
		{"kill", func(ws *websocket.Conn, out io.Writer, sess *session, parts []string) {
			if len(parts) < 2 {
				fmt.Fprintln(out, "usage: kill <pid>  OR  kill <script> [args...]")
				return
			}
			params := map[string]any{"server": sess.currentServer}
			if pid, err := strconv.Atoi(parts[1]); err == nil {
				params["pid"] = pid
			} else {
				params["filename"] = parts[1]
				if len(parts) > 2 {
					args := make([]any, len(parts)-2)
					for i, a := range parts[2:] {
						args[i] = parseScriptArg(a)
					}
					params["args"] = args
				}
			}
			sendMsg(ws, out, "killScript", params)
		}},
		{"killall", func(ws *websocket.Conn, out io.Writer, sess *session, parts []string) {
			server := sess.currentServer
			if len(parts) > 1 {
				server = parts[1]
			}
			sendMsg(ws, out, "killAllScripts", map[string]string{"server": server})
		}},
		{"run", func(ws *websocket.Conn, out io.Writer, sess *session, parts []string) {
			if len(parts) < 2 {
				fmt.Fprintln(out, "usage: run <script> [-t <threads>] [args...]")
				return
			}
			filename := parts[1]
			threads := 1
			args := []any{}
			rest := parts[2:]
			for i := 0; i < len(rest); i++ {
				if rest[i] == "-t" && i+1 < len(rest) {
					if n, err := strconv.Atoi(rest[i+1]); err == nil && n > 0 {
						threads = n
						i++
						continue
					}
				}
				args = append(args, parseScriptArg(rest[i]))
			}
			sendMsg(ws, out, "runScript", map[string]any{
				"server":   sess.currentServer,
				"filename": filename,
				"threads":  threads,
				"args":     args,
			})
		}},
		{"check", func(ws *websocket.Conn, out io.Writer, sess *session, parts []string) {
			if len(parts) < 2 {
				fmt.Fprintln(out, "usage: check <pid>  OR  check <script> [args...]")
				return
			}
			params := map[string]any{"server": sess.currentServer}
			if pid, err := strconv.Atoi(parts[1]); err == nil {
				params["pid"] = pid
			} else {
				params["filename"] = parts[1]
				if len(parts) > 2 {
					args := make([]any, len(parts)-2)
					for i, a := range parts[2:] {
						args[i] = parseScriptArg(a)
					}
					params["args"] = args
				}
			}
			raw, err := sendMsgSync(ws, "getScriptLogs", params)
			if err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
				return
			}
			var logs []string
			if err := json.Unmarshal(raw, &logs); err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
				return
			}
			for _, l := range logs {
				fmt.Fprintln(out, l)
			}
		}},
		{"tail", func(ws *websocket.Conn, out io.Writer, sess *session, parts []string) {
			if len(parts) < 2 {
				fmt.Fprintln(out, "usage: tail <pid>  OR  tail <script> [args...]")
				return
			}
			params := map[string]any{"server": sess.currentServer}
			if pid, err := strconv.Atoi(parts[1]); err == nil {
				params["pid"] = pid
			} else {
				params["filename"] = parts[1]
				if len(parts) > 2 {
					args := make([]any, len(parts)-2)
					for i, a := range parts[2:] {
						args[i] = parseScriptArg(a)
					}
					params["args"] = args
				}
			}
			raw, err := sendMsgSync(ws, "getScriptLogs", params)
			if err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
				return
			}
			var logs []string
			if err := json.Unmarshal(raw, &logs); err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
				return
			}
			for _, l := range logs {
				fmt.Fprintln(out, l)
			}
		}},

		// ── Shell utilities ───────────────────────────────────────────────────────

		{"clear", func(ws *websocket.Conn, out io.Writer, sess *session, parts []string) {
			sess.app.QueueUpdateDraw(func() { sess.outputView.Clear() })
		}},
		{"joinFaction", func(ws *websocket.Conn, out io.Writer, sess *session, parts []string) {
			if len(parts) < 2 {
				fmt.Fprintln(out, "usage: joinFaction <factionName>")
				return
			}
			name := strings.Join(parts[1:], " ")
			raw, err := sendMsgSync(ws, "joinFaction", map[string]string{"location": name})
			if err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
				return
			}
			var result string
			if json.Unmarshal(raw, &result) == nil {
				fmt.Fprintln(out, result)
			}
			// Remove from seenInvites so the invite no longer appears in the count
			if sess.seenInvites != nil {
				delete(sess.seenInvites, name)
			}
			select {
			case sess.serverRefreshCh <- struct{}{}:
			default:
			}
		}},
		{"factions", func(ws *websocket.Conn, out io.Writer, sess *session, parts []string) {
			printFactions := func(facs []factionInfoResult) {
				w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
				fmt.Fprintln(w, "Name\tRep\tFavor\tWork")
				for _, f := range facs {
					var workTypes []string
					if f.OfferHackingWork {
						workTypes = append(workTypes, "hacking")
					}
					if f.OfferFieldWork {
						workTypes = append(workTypes, "field")
					}
					if f.OfferSecurityWork {
						workTypes = append(workTypes, "security")
					}
					wt := strings.Join(workTypes, "/")
					if wt == "" {
						wt = "—"
					}
					fmt.Fprintf(w, "%s\t%s\t%.0f\t%s\n", f.Name, formatMoney(f.Rep), f.Favor, wt)
				}
				w.Flush()
			}

			fetchFactions := func(ws *websocket.Conn) ([]factionInfoResult, error) {
				raw, err := sendMsgSync(ws, "getFactions", nil)
				if err != nil {
					return nil, err
				}
				var facs []factionInfoResult
				if err := json.Unmarshal(raw, &facs); err != nil {
					return nil, err
				}
				return facs, nil
			}

			facs, err := fetchFactions(ws)
			if err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
				return
			}
			printFactions(facs)
			fmt.Fprintln(out)
			fmt.Fprintln(out, "Commands: ls  invites  join <faction>  info <faction>  work <faction> [type]  stop  share [gb]  augs <faction>  buy <faction> <aug>  exit")

			prevHandler := sess.cityHandler
			sess.setPromptStr("[cyan][[white]factions[cyan]][-] [green]>[-] ")
			sess.setSubCompleter([]string{"ls", "invites", "markread", "join", "info", "work", "stop", "share", "augs", "buy", "help", "alias", "unalias", "exit"})
			sess.currentRepl = "factions"
			inSubMode.Store(true)

			sess.setContextRefresh("Factions", func(ws *websocket.Conn) {
				f2, err2 := fetchFactions(ws)
				if err2 != nil {
					return
				}
				var sb strings.Builder
				fmt.Fprintf(&sb, " [yellow]Factions: %d[-]\n", len(f2))
				sb.WriteString(ctxSep)
				for _, f := range f2 {
					fmt.Fprintf(&sb, " [green]%s[-]\n", f.Name)
					fmt.Fprintf(&sb, "  rep   %s\n", formatMoney(f.Rep))
					fmt.Fprintf(&sb, "  favor %.0f\n", f.Favor)
				}
				text := sb.String()
				sess.app.QueueUpdateDraw(func() { sess.contextView.SetText(text) })
			})

			sess.cityHandler = func(ws *websocket.Conn, out io.Writer, sess *session, line string) bool {
				lp := strings.Fields(line)
				if len(lp) == 0 {
					return true
				}
				switch strings.ToLower(lp[0]) {
				case "exit", "quit":
					sess.cityHandler = prevHandler
					if prevHandler == nil {
						inSubMode.Store(false)
					}
					sess.currentRepl = "global"
					sess.restoreCompleter()
					sess.updatePrompt()
					sess.clearContextRefresh()

				case "ls", "list":
					f2, err2 := fetchFactions(ws)
					if err2 != nil {
						fmt.Fprintf(out, "error: %v\n", err2)
						return true
					}
					printFactions(f2)

				case "invites":
					raw, err2 := sendMsgSync(ws, "getFactionInvitations", nil)
					if err2 != nil {
						fmt.Fprintf(out, "error: %v\n", err2)
						return true
					}
					var invites []string
					if json.Unmarshal(raw, &invites) != nil || len(invites) == 0 {
						fmt.Fprintln(out, "No pending faction invitations.")
						return true
					}
					for _, name := range invites {
						if sess.readInvites[name] {
							fmt.Fprintf(out, "  %s  [darkgray](read)[-]\n", name)
						} else {
							fmt.Fprintf(out, "  %s\n", name)
						}
					}

				case "markread":
					if len(lp) < 2 {
						fmt.Fprintln(out, "usage: markread <faction>")
						return true
					}
					name := strings.Join(lp[1:], " ")
					if sess.readInvites == nil {
						sess.readInvites = make(map[string]bool)
					}
					sess.readInvites[name] = true
					sess.seenInvites[name] = true
					fmt.Fprintf(out, "Invite from '%s' marked read — badge suppressed. Use 'join %s' to accept later.\n", name, name)

				case "join":
					if len(lp) < 2 {
						fmt.Fprintln(out, "usage: join <faction>")
						return true
					}
					name := strings.Join(lp[1:], " ")
					raw, err2 := sendMsgSync(ws, "joinFaction", map[string]string{"location": name})
					if err2 != nil {
						fmt.Fprintf(out, "error: %v\n", err2)
						return true
					}
					var result string
					if json.Unmarshal(raw, &result) == nil {
						fmt.Fprintln(out, result)
					}
					if sess.seenInvites != nil {
						delete(sess.seenInvites, name)
					}
					select {
					case sess.serverRefreshCh <- struct{}{}:
					default:
					}

				case "info":
					if len(lp) < 2 {
						fmt.Fprintln(out, "usage: info <faction>")
						return true
					}
					name := strings.Join(lp[1:], " ")
					f2, err2 := fetchFactions(ws)
					if err2 != nil {
						fmt.Fprintf(out, "error: %v\n", err2)
						return true
					}
					var found *factionInfoResult
					for i := range f2 {
						if strings.EqualFold(f2[i].Name, name) {
							found = &f2[i]
							break
						}
					}
					if found == nil {
						fmt.Fprintf(out, "not a member of faction '%s'\n", name)
						return true
					}
					fmt.Fprintf(out, "%s  rep %s  favor %.0f\n", found.Name, formatMoney(found.Rep), found.Favor)
					var types []string
					if found.OfferHackingWork {
						types = append(types, "hacking")
					}
					if found.OfferFieldWork {
						types = append(types, "field")
					}
					if found.OfferSecurityWork {
						types = append(types, "security")
					}
					if len(types) > 0 {
						fmt.Fprintf(out, "Work: %s\n", strings.Join(types, ", "))
					} else {
						fmt.Fprintln(out, "Work: none")
					}
					buyable := 0
					for _, a := range found.Augmentations {
						if !a.Owned && !a.Queued {
							buyable++
						}
					}
					fmt.Fprintf(out, "Augmentations: %d total, %d buyable (use 'augs %s')\n", len(found.Augmentations), buyable, found.Name)

				case "work":
					if len(lp) < 2 {
						fmt.Fprintln(out, "usage: work <faction> [hacking|field|security]")
						return true
					}
					workType := "hacking"
					facParts := lp[1:]
					last := strings.ToLower(facParts[len(facParts)-1])
					if last == "hacking" || last == "field" || last == "security" {
						workType = last
						facParts = facParts[:len(facParts)-1]
					}
					if len(facParts) == 0 {
						fmt.Fprintln(out, "usage: work <faction> [hacking|field|security]")
						return true
					}
					facName := strings.Join(facParts, " ")
					raw, err2 := sendMsgSync(ws, "doFactionWork", map[string]string{"location": facName, "field": workType})
					if err2 != nil {
						fmt.Fprintf(out, "error: %v\n", err2)
						return true
					}
					var result string
					if json.Unmarshal(raw, &result) == nil {
						fmt.Fprintln(out, result)
					}
					select {
					case sess.serverRefreshCh <- struct{}{}:
					default:
					}

				case "stop":
					raw, err2 := sendMsgSync(ws, "stopWork", nil)
					if err2 != nil {
						fmt.Fprintf(out, "error: %v\n", err2)
						return true
					}
					var result string
					if json.Unmarshal(raw, &result) == nil {
						fmt.Fprintln(out, result)
					}
					select {
					case sess.serverRefreshCh <- struct{}{}:
					default:
					}

				case "share":
					// share [gb] — share free RAM from home for 10s rep bonus
					params := map[string]string{}
					if len(lp) >= 2 {
						params["stat"] = lp[1]
					}
					raw, err2 := sendMsgSync(ws, "shareRAM", params)
					if err2 != nil {
						fmt.Fprintf(out, "error: %v\n", err2)
						return true
					}
					var result string
					if json.Unmarshal(raw, &result) == nil {
						fmt.Fprintln(out, result)
					}

				case "augs":
					if len(lp) < 2 {
						fmt.Fprintln(out, "usage: augs <faction>")
						return true
					}
					name := strings.Join(lp[1:], " ")
					f2, err2 := fetchFactions(ws)
					if err2 != nil {
						fmt.Fprintf(out, "error: %v\n", err2)
						return true
					}
					var found *factionInfoResult
					for i := range f2 {
						if strings.EqualFold(f2[i].Name, name) {
							found = &f2[i]
							break
						}
					}
					if found == nil {
						fmt.Fprintf(out, "not a member of faction '%s'\n", name)
						return true
					}
					w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
					fmt.Fprintln(w, "Augmentation\tRep\tCost\tStatus")
					for _, a := range found.Augmentations {
						var status string
						switch {
						case a.Owned:
							status = "owned"
						case a.Queued:
							status = "queued"
						case !a.HasPrereqs:
							status = "missing prereqs"
						case !a.HasRep:
							status = fmt.Sprintf("need %s rep", formatMoney(a.RepCost))
						case !a.CanAfford:
							status = fmt.Sprintf("need %s", formatMoney(a.MoneyCost))
						default:
							status = fmt.Sprintf("buy for %s", formatMoney(a.MoneyCost))
						}
						fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", a.Name, formatMoney(a.RepCost), formatMoney(a.MoneyCost), status)
					}
					w.Flush()

				case "buy":
					if len(lp) < 3 {
						fmt.Fprintln(out, "usage: buy <faction> <augmentation>")
						return true
					}
					augName := lp[len(lp)-1]
					facName := strings.Join(lp[1:len(lp)-1], " ")
					raw, err2 := sendMsgSync(ws, "buyFactionAug", map[string]string{"location": facName, "stat": augName})
					if err2 != nil {
						fmt.Fprintf(out, "error: %v\n", err2)
						return true
					}
					var result string
					if json.Unmarshal(raw, &result) == nil {
						fmt.Fprintln(out, result)
					}
					select {
					case sess.serverRefreshCh <- struct{}{}:
					default:
					}

				case "alias":
					am := replAliases(sess.currentRepl)
					if len(lp) < 2 {
						if len(am) == 0 {
							fmt.Fprintln(out, "no aliases defined")
						} else {
							keys := make([]string, 0, len(am))
							for k := range am {
								keys = append(keys, k)
							}
							sort.Strings(keys)
							for _, k := range keys {
								fmt.Fprintf(out, "alias %s=%q\n", k, am[k])
							}
						}
						return true
					}
					name := lp[1]
					value := strings.Join(lp[2:], " ")
					if value == "" && strings.Contains(name, "=") {
						idx := strings.IndexByte(name, '=')
						value = name[idx+1:]
						name = name[:idx]
					}
					am[name] = value
					saveAliases()
					fmt.Fprintf(out, "alias %s=%q\n", name, value)

				case "unalias":
					if len(lp) < 2 {
						fmt.Fprintln(out, "usage: unalias <name>")
						return true
					}
					delete(replAliases(sess.currentRepl), lp[1])
					saveAliases()

				case "help":
					helpTable(out, "Factions", [][2]string{
						{"ls", "List joined factions with rep, favor, and work types"},
						{"invites", "List pending faction invitations"},
						{"markread <faction>", "Suppress invite badge without joining"},
						{"join <faction>", "Accept a pending faction invitation"},
						{"info <faction>", "Show faction details and augmentation count"},
						{"work <faction> [type]", "Start working for a faction (hacking/field/security)"},
						{"stop", "Stop current work"},
						{"share [gb]", "Share home RAM for 10s rep multiplier bonus (default: all free)"},
						{"augs <faction>", "List augmentations available from a faction"},
						{"buy <faction> <aug>", "Purchase an augmentation from a faction"},
					})
					helpTable(out, "Shell", [][2]string{
						{"alias [name=\"value\"]", "Define an alias"},
						{"unalias <name>", "Remove an alias"},
						{"help", "Show this help"},
						{"exit", "Return to top-level"},
					})

				default:
					fmt.Fprintf(out, "unknown command: %s\n", lp[0])
				}
				return true
			}
		}},
		{"augmentations", func(ws *websocket.Conn, out io.Writer, sess *session, parts []string) {
			// Pretty-printers for the augmentations status.
			printInstalled := func(st *augmentationsStatus) {
				if len(st.Installed) == 0 {
					fmt.Fprintln(out, "No augmentations installed.")
				} else {
					names := make([]ownedAugEntry, len(st.Installed))
					copy(names, st.Installed)
					sort.Slice(names, func(i, j int) bool { return names[i].Name < names[j].Name })
					w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
					fmt.Fprintln(w, "Augmentation\tLevel")
					for _, a := range names {
						fmt.Fprintf(w, "%s\t%d\n", a.Name, a.Level)
					}
					w.Flush()
				}
				if st.NFGLevel > 0 {
					fmt.Fprintf(out, "\nNeuroFlux Governor: level %d\n", st.NFGLevel)
				}
				if st.Entropy > 0 {
					fmt.Fprintf(out, "Entropy Virus: level %d\n", st.Entropy)
				}
			}

			printQueued := func(st *augmentationsStatus) {
				if len(st.Queued) == 0 {
					fmt.Fprintln(out, "Nothing queued for installation.")
					return
				}
				w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
				fmt.Fprintln(w, "Augmentation\tLevel")
				for _, a := range st.Queued {
					fmt.Fprintf(w, "%s\t%d\n", a.Name, a.Level)
				}
				w.Flush()
				fmt.Fprintf(out, "\n%d augmentation(s) queued. Run 'install --confirm' to apply.\n", len(st.Queued))
			}

			printMults := func(st *augmentationsStatus) {
				keys := make([]string, 0, len(st.Mults))
				for k, v := range st.Mults {
					if v != 1.0 {
						keys = append(keys, k)
					}
				}
				if len(keys) == 0 {
					fmt.Fprintln(out, "No active multipliers (all at default 1.0×).")
					return
				}
				sort.Strings(keys)
				w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
				fmt.Fprintln(w, "Multiplier\tEffect")
				for _, k := range keys {
					v := st.Mults[k]
					pct := (v - 1.0) * 100
					sign := "+"
					if pct < 0 {
						sign = ""
					}
					fmt.Fprintf(w, "%s\t%s%.1f%%\n", k, sign, pct)
				}
				w.Flush()
			}

			printSourceFiles := func(st *augmentationsStatus) {
				if len(st.SourceFiles) == 0 {
					fmt.Fprintln(out, "No source files owned.")
					return
				}
				sfs := make([]sourceFileEntry, len(st.SourceFiles))
				copy(sfs, st.SourceFiles)
				sort.Slice(sfs, func(i, j int) bool { return sfs[i].N < sfs[j].N })
				w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
				fmt.Fprintln(w, "BitNode\tLevel")
				for _, s := range sfs {
					fmt.Fprintf(w, "BN-%d\t%d\n", s.N, s.Level)
				}
				w.Flush()
			}

			fetchStatus := func(ws *websocket.Conn) (*augmentationsStatus, error) {
				raw, err := sendMsgSync(ws, "getAugmentationsStatus", nil)
				if err != nil {
					return nil, err
				}
				var st augmentationsStatus
				if err := json.Unmarshal(raw, &st); err != nil {
					return nil, err
				}
				return &st, nil
			}

			st, err := fetchStatus(ws)
			if err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
				return
			}
			printInstalled(st)
			fmt.Fprintln(out)
			fmt.Fprintln(out, "Commands: ls  queued  mults  sourcefiles  install [--confirm]  help  exit")

			prevHandler := sess.cityHandler
			sess.setPromptStr("[cyan][[white]augmentations[cyan]][-] [green]>[-] ")
			sess.setSubCompleter([]string{"ls", "queued", "mults", "sourcefiles", "install", "help", "alias", "unalias", "exit"})
			sess.currentRepl = "augmentations"
			inSubMode.Store(true)

			sess.setContextRefresh("Augmentations", func(ws *websocket.Conn) {
				st2, err2 := fetchStatus(ws)
				if err2 != nil {
					return
				}
				var sb strings.Builder
				fmt.Fprintf(&sb, " [yellow]Queued: %d aug(s)[-]\n", len(st2.Queued))
				if st2.NFGLevel > 0 {
					fmt.Fprintf(&sb, " NFG: Level %d\n", st2.NFGLevel)
				}
				if st2.Entropy > 0 {
					fmt.Fprintf(&sb, " Entropy: %d\n", st2.Entropy)
				}
				sb.WriteString(ctxSep)
				for _, a := range st2.Queued {
					fmt.Fprintf(&sb, " [green]%s[-]\n", a.Name)
				}
				text := sb.String()
				sess.app.QueueUpdateDraw(func() { sess.contextView.SetText(text) })
			})

			sess.cityHandler = func(ws *websocket.Conn, out io.Writer, sess *session, line string) bool {
				lp := strings.Fields(line)
				if len(lp) == 0 {
					return true
				}
				switch strings.ToLower(lp[0]) {
				case "exit", "quit":
					sess.cityHandler = prevHandler
					if prevHandler == nil {
						inSubMode.Store(false)
					}
					sess.currentRepl = "global"
					sess.restoreCompleter()
					sess.updatePrompt()
					sess.clearContextRefresh()

				case "ls", "list":
					st2, err2 := fetchStatus(ws)
					if err2 != nil {
						fmt.Fprintf(out, "error: %v\n", err2)
						return true
					}
					printInstalled(st2)

				case "queued":
					st2, err2 := fetchStatus(ws)
					if err2 != nil {
						fmt.Fprintf(out, "error: %v\n", err2)
						return true
					}
					printQueued(st2)

				case "mults":
					st2, err2 := fetchStatus(ws)
					if err2 != nil {
						fmt.Fprintf(out, "error: %v\n", err2)
						return true
					}
					printMults(st2)

				case "sourcefiles":
					st2, err2 := fetchStatus(ws)
					if err2 != nil {
						fmt.Fprintf(out, "error: %v\n", err2)
						return true
					}
					printSourceFiles(st2)

				case "install":
					confirmed := false
					for _, a := range lp[1:] {
						if a == "--confirm" {
							confirmed = true
						}
					}
					if !confirmed {
						fmt.Fprintln(out, "[red]╔════════════════════════════════════════════════════════════╗[-]")
						fmt.Fprintln(out, "[red]║                       ⚠  WARNING  ⚠                        ║[-]")
						fmt.Fprintln(out, "[red]╠════════════════════════════════════════════════════════════╣[-]")
						fmt.Fprintln(out, "[red]║  Installing augmentations is DESTRUCTIVE:                  ║[-]")
						fmt.Fprintln(out, "[red]║    • All running scripts will be killed.                   ║[-]")
						fmt.Fprintln(out, "[red]║    • Skills reset to baseline; multipliers reapply.        ║[-]")
						fmt.Fprintln(out, "[red]║    • Non-home server files are wiped.                      ║[-]")
						fmt.Fprintln(out, "[red]║  Run 'install --confirm' to proceed.                       ║[-]")
						fmt.Fprintln(out, "[red]╚════════════════════════════════════════════════════════════╝[-]")
						return true
					}
					raw, err2 := sendMsgSync(ws, "doInstallAugmentations", nil)
					if err2 != nil {
						fmt.Fprintf(out, "error: %v\n", err2)
						return true
					}
					var result string
					if json.Unmarshal(raw, &result) == nil {
						fmt.Fprintln(out, result)
					}
					select {
					case sess.serverRefreshCh <- struct{}{}:
					default:
					}

				case "alias":
					am := replAliases(sess.currentRepl)
					if len(lp) < 2 {
						if len(am) == 0 {
							fmt.Fprintln(out, "no aliases defined")
						} else {
							keys := make([]string, 0, len(am))
							for k := range am {
								keys = append(keys, k)
							}
							sort.Strings(keys)
							for _, k := range keys {
								fmt.Fprintf(out, "alias %s=%q\n", k, am[k])
							}
						}
						return true
					}
					name := lp[1]
					value := strings.Join(lp[2:], " ")
					if value == "" && strings.Contains(name, "=") {
						idx := strings.IndexByte(name, '=')
						value = name[idx+1:]
						name = name[:idx]
					}
					am[name] = value
					saveAliases()
					fmt.Fprintf(out, "alias %s=%q\n", name, value)

				case "unalias":
					if len(lp) < 2 {
						fmt.Fprintln(out, "usage: unalias <name>")
						return true
					}
					delete(replAliases(sess.currentRepl), lp[1])
					saveAliases()

				case "help":
					helpTable(out, "Augmentations", [][2]string{
						{"ls", "List installed augmentations and NFG/entropy levels"},
						{"queued", "Show augmentations queued for installation"},
						{"mults", "Show all non-default player multipliers"},
						{"sourcefiles", "Show owned source files and their levels"},
						{"install --confirm", "Install queued augmentations (DESTRUCTIVE — resets game)"},
					})
					helpTable(out, "Shell", [][2]string{
						{"alias [name=\"value\"]", "Define an alias"},
						{"unalias <name>", "Remove an alias"},
						{"help", "Show this help"},
						{"exit", "Return to top-level"},
					})

				default:
					fmt.Fprintf(out, "unknown command: %s\n", lp[0])
				}
				return true
			}
		}},
		{"help", func(ws *websocket.Conn, out io.Writer, sess *session, parts []string) {
			helpTable(out, "Navigation", [][2]string{
				{"connect <hostname>", "Connect to a server"},
				{"home", "Return to home server"},
				{"scan [server]", "List servers reachable from current/given server"},
				{"city", "Enter city sub-repl (travel, interact with locations)"},
				{"hacknet", "Enter hacknet sub-repl"},
				{"cloud", "Enter cloud servers sub-repl"},
				{"programs", "Enter programs sub-repl (create hacking tools)"},
				{"active_scripts", "Show all running scripts across all servers"},
				{"factions", "Enter factions sub-repl (join, work, buy augs)"},
				{"augmentations", "Enter augmentations sub-repl (install, view mults)"},
			})
			helpTable(out, "Server Info", [][2]string{
				{"hostname", "Print current server hostname"},
				{"ipaddr", "Print current server IP address"},
				{"lscpu", "Print CPU core count"},
				{"free", "Print RAM usage"},
				{"sudov", "Print root access and open port status"},
				{"analyze", "Print security, money, and hack stats"},
			})
			helpTable(out, "Files", [][2]string{
				{"ls [server]", "List files on current or given server"},
				{"cat <file>", "Print file contents"},
				{"cp <src> <dst>", "Copy file on current server"},
				{"mv <src> <dst>", "Rename/move file on current server"},
				{"rm <file>", "Delete file"},
				{"scp <file...> <dst>", "Copy file(s) to another server"},
				{"vim <file>", "Open file in browser editor"},
				{"grep <pattern> [file...]", "Search file contents"},
				{"download <file>", "Download file to local disk"},
				{"wget <url> <file>", "Fetch URL and save as .js/.ts/.txt file"},
				{"mem <script> [-t N]", "Show RAM cost breakdown for a script"},
			})
			helpTable(out, "Processes", [][2]string{
				{"ps [server]", "List running scripts"},
				{"top [server]", "List scripts sorted by RAM usage"},
				{"run <script> [-t N] [args...]", "Run a script"},
				{"kill <pid|script> [args...]", "Kill a script by PID or filename"},
				{"killall [server]", "Kill all scripts on current or given server"},
				{"check <pid|script> [args...]", "Print logs for a script"},
				{"tail <pid|script> [args...]", "Print logs for a script (alias for check)"},
			})
			helpTable(out, "Hacking", [][2]string{
				{"hack [server]", "Hack current or given server (terminal action)"},
				{"grow [server]", "Grow current or given server"},
				{"weaken [server]", "Weaken current or given server"},
				{"backdoor [server]", "Install backdoor on current or given server"},
			})
			helpTable(out, "Darkweb", [][2]string{
				{"buy -l", "List available darkweb items and prices"},
				{"buy -a", "Buy all darkweb items you can afford"},
				{"buy <item>", "Buy a specific darkweb item"},
			})
			helpTable(out, "Factions & Misc", [][2]string{
				{"expr <expression>", "Evaluate a math expression"},
				{"mkdir", "No-op (Bitburner has no real directories)"},
				{"export_game", "Export save file to current directory"},
			})
			helpTable(out, "Shell", [][2]string{
				{"alias [name=\"value\"]", "List all aliases or define a new one"},
				{"unalias <name|--all>", "Remove one or all aliases"},
				{"clear", "Clear the output pane"},
				{"help", "Show this help"},
				{"exit", "Quit bb-remote"},
			})
		}},
		{"expr", func(ws *websocket.Conn, out io.Writer, sess *session, parts []string) {
			if len(parts) < 2 {
				fmt.Fprintln(out, "usage: expr <math expression>")
				return
			}
			result, err := evalMath(strings.Join(parts[1:], ""))
			if err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
				return
			}
			if result == math.Trunc(result) && !math.IsInf(result, 0) && !math.IsNaN(result) {
				fmt.Fprintf(out, "%d\n", int64(result))
			} else {
				fmt.Fprintf(out, "%g\n", result)
			}
		}},
		{"mem", func(ws *websocket.Conn, out io.Writer, sess *session, parts []string) {
			if len(parts) < 2 {
				fmt.Fprintln(out, "usage: mem <script> [-t <threads>]")
				return
			}
			filename := parts[1]
			threads := 1
			for i := 2; i < len(parts)-1; i++ {
				if parts[i] == "-t" {
					if n, err := strconv.Atoi(parts[i+1]); err == nil && n >= 1 {
						threads = n
					}
				}
			}
			raw, err := sendMsgSync(ws, "getScriptRam", map[string]any{
				"server": sess.currentServer, "filename": filename, "threads": threads,
			})
			if err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
				return
			}
			var result scriptRamResult
			if err := json.Unmarshal(raw, &result); err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
				return
			}
			fmt.Fprintf(out, "RAM usage: %s for %d thread(s)\n", formatRAM(result.RamUsage), result.Threads)
			for _, e := range result.Entries {
				fmt.Fprintf(out, "  %8s  %-12s  %s\n", formatRAM(e.Cost), "("+e.Type+")", e.Name)
			}
		}},
		{"mkdir", func(ws *websocket.Conn, out io.Writer, sess *session, parts []string) {
			fmt.Fprintln(out, "Directories do not exist in the Bitburner filesystem. They are simply part of the file path.")
			fmt.Fprintln(out, `For example, with "/foo/bar.txt", there is no actual "/foo" directory.`)
		}},
		{"alias", func(ws *websocket.Conn, out io.Writer, sess *session, parts []string) {
			am := replAliases(sess.currentRepl)
			if len(parts) == 1 {
				if len(am) == 0 {
					fmt.Fprintf(out, "no aliases defined (scope: %s)\n", sess.currentRepl)
					return
				}
				keys := make([]string, 0, len(am))
				for k := range am {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				var sb strings.Builder
				for _, k := range keys {
					sb.WriteString(fmt.Sprintf("alias %s=%q\n", k, am[k]))
				}
				fmt.Fprint(out, sb.String())
				return
			}
			arg := strings.Join(parts[1:], " ")
			eq := strings.IndexByte(arg, '=')
			if eq < 0 {
				fmt.Fprintln(out, `usage: alias [name="value"]`)
				return
			}
			name := strings.TrimSpace(arg[:eq])
			value := strings.Trim(strings.TrimSpace(arg[eq+1:]), `"'`)
			if name == "" {
				fmt.Fprintln(out, "alias: invalid name")
				return
			}
			am[name] = value
			saveAliases()
			fmt.Fprintf(out, "alias %s=%q\n", name, value)
		}},
		{"unalias", func(ws *websocket.Conn, out io.Writer, sess *session, parts []string) {
			if len(parts) < 2 {
				fmt.Fprintln(out, "usage: unalias <name> | unalias --all")
				return
			}
			am := replAliases(sess.currentRepl)
			if parts[1] == "--all" {
				aliasStore[sess.currentRepl] = map[string]string{}
				saveAliases()
				fmt.Fprintf(out, "all aliases removed (scope: %s)\n", sess.currentRepl)
				return
			}
			if _, ok := am[parts[1]]; !ok {
				fmt.Fprintf(out, "unalias: %s: not found\n", parts[1])
				return
			}
			delete(am, parts[1])
			saveAliases()
			fmt.Fprintf(out, "removed alias %s\n", parts[1])
		}},
		{"wget", func(ws *websocket.Conn, out io.Writer, sess *session, parts []string) {
			if len(parts) != 3 {
				fmt.Fprintln(out, "usage: wget <url> <filename>")
				return
			}
			url, filename := parts[1], parts[2]
			ext := strings.ToLower(filename[strings.LastIndex(filename, ".")+1:])
			if ext != "js" && ext != "ts" && ext != "txt" {
				fmt.Fprintln(out, "error: filename must be a .js, .ts, or .txt file")
				return
			}
			resp, err := http.Get(url) //nolint:noctx
			if err != nil {
				fmt.Fprintf(out, "wget failed: %v\n", err)
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				fmt.Fprintf(out, "wget failed: HTTP %d\n", resp.StatusCode)
				return
			}
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				fmt.Fprintf(out, "wget failed: %v\n", err)
				return
			}
			if _, err := sendMsgSync(ws, "pushFile", map[string]string{
				"server": sess.currentServer, "filename": filename, "content": string(body),
			}); err != nil {
				fmt.Fprintf(out, "error saving file: %v\n", err)
				return
			}
			fmt.Fprintf(out, "saved %d bytes to %s:%s\n", len(body), sess.currentServer, filename)
			refreshFilesIfCurrent(ws, sess, sess.currentServer)
		}},

		// ── Darkweb ───────────────────────────────────────────────────────────────

		{"buy", func(ws *websocket.Conn, out io.Writer, sess *session, parts []string) {
			if len(parts) < 2 {
				fmt.Fprintln(out, "usage: buy [-l] [-a] <item>")
				return
			}
			switch parts[1] {
			case "-l", "--list":
				raw, err := sendMsgSync(ws, "getDarkwebItems", nil)
				if err != nil {
					fmt.Fprintf(out, "error: %v\n", err)
					return
				}
				var items []darkwebItem
				if err := json.Unmarshal(raw, &items); err != nil {
					fmt.Fprintf(out, "error: %v\n", err)
					return
				}
				for _, item := range items {
					owned := ""
					if item.Owned {
						owned = " \033[32m[OWNED]\033[0m"
					}
					fmt.Fprintf(out, "  %-30s  %-14s  %s%s\n", item.Name, formatMoney(item.Price), item.Description, owned)
				}
			case "-a", "--all":
				raw, err := sendMsgSync(ws, "buyAllDarkwebItems", nil)
				if err != nil {
					fmt.Fprintf(out, "error: %v\n", err)
					return
				}
				var msg string
				if err := json.Unmarshal(raw, &msg); err == nil && msg != "" {
					fmt.Fprintln(out, msg)
				}
			default:
				raw, err := sendMsgSync(ws, "buyDarkwebItem", map[string]string{"item": parts[1]})
				if err != nil {
					fmt.Fprintf(out, "error: %v\n", err)
					return
				}
				var msg string
				if err := json.Unmarshal(raw, &msg); err == nil && msg != "" {
					fmt.Fprintln(out, msg)
				}
			}
		}},

		// ── Hacking actions ───────────────────────────────────────────────────────

		{"hack", func(ws *websocket.Conn, out io.Writer, sess *session, parts []string) {
			server := sess.currentServer
			if len(parts) > 1 {
				server = parts[1]
			}
			if _, err := sendMsgSync(ws, "hackServer", map[string]string{"server": server}); err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
				return
			}
			waitForAction(ws, out)
		}},
		{"grow", func(ws *websocket.Conn, out io.Writer, sess *session, parts []string) {
			server := sess.currentServer
			if len(parts) > 1 {
				server = parts[1]
			}
			if _, err := sendMsgSync(ws, "growServer", map[string]string{"server": server}); err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
				return
			}
			waitForAction(ws, out)
		}},
		{"weaken", func(ws *websocket.Conn, out io.Writer, sess *session, parts []string) {
			server := sess.currentServer
			if len(parts) > 1 {
				server = parts[1]
			}
			if _, err := sendMsgSync(ws, "weakenServer", map[string]string{"server": server}); err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
				return
			}
			waitForAction(ws, out)
		}},
		{"backdoor", func(ws *websocket.Conn, out io.Writer, sess *session, parts []string) {
			server := sess.currentServer
			if len(parts) > 1 {
				server = parts[1]
			}
			if _, err := sendMsgSync(ws, "backdoorServer", map[string]string{"server": server}); err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
				return
			}
			waitForAction(ws, out)
		}},

		{"hacknet", func(ws *websocket.Conn, out io.Writer, sess *session, parts []string) {
			// Fetch current status to show on entry
			raw, err := sendMsgSync(ws, "hacknetInfo", nil)
			if err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
				return
			}
			var status hacknetStatusResult
			if err := json.Unmarshal(raw, &status); err != nil {
				fmt.Fprintf(out, "error parsing hacknet info: %v\n", err)
				return
			}

			nodeKind := "node"
			if status.IsServer {
				nodeKind = "server"
			}

			printHacknetStatus := func(status hacknetStatusResult) {
				if len(status.Nodes) == 0 {
					fmt.Fprintf(out, "No hacknet %ss owned. Next cost: %s\n", nodeKind, formatMoney(status.NextNodeCost))
					return
				}
				w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
				if status.IsServer {
					fmt.Fprintln(w, "#\tName\tLv\tRAM\tCores\tCache\tHashes/s\tTotal Hashes")
					for i, n := range status.Nodes {
						cache := 0
						if n.Cache != nil {
							cache = *n.Cache
						}
						fmt.Fprintf(w, "%d\t%s\t%d\t%.0fGB\t%d\t%d\t%.3f\t%.0f\n",
							i, n.Name, n.Level, n.Ram, n.Cores, cache, n.Production, n.TotalProduction)
					}
				} else {
					fmt.Fprintln(w, "#\tName\tLv\tRAM\tCores\t$/s\tTotal $")
					for i, n := range status.Nodes {
						fmt.Fprintf(w, "%d\t%s\t%d\t%.0fGB\t%d\t%s\t%s\n",
							i, n.Name, n.Level, n.Ram, n.Cores,
							formatMoney(n.Production), formatMoney(n.TotalProduction))
					}
				}
				w.Flush()
				fmt.Fprintf(out, "Next %s cost: %s\n", nodeKind, formatMoney(status.NextNodeCost))
			}

			printHacknetStatus(status)
			fmt.Fprintln(out)
			fmt.Fprintln(out, "Commands: ls  buy  upgrade <i> level|ram|core|cache [n]  hashes  spend <upgrade> [target] [count]  exit")

			prevHandler := sess.cityHandler
			sess.setPromptStr("[cyan][[white]hacknet[cyan]][-] [green]>[-] ")
			sess.setSubCompleter([]string{"ls", "list", "buy", "upgrade", "hashes", "spend", "help", "alias", "unalias", "exit"})
			sess.currentRepl = "hacknet"
			inSubMode.Store(true)

			sess.setContextRefresh("Hacknet", func(ws *websocket.Conn) {
				raw, err := sendMsgSync(ws, "hacknetInfo", nil)
				if err != nil {
					return
				}
				var st hacknetStatusResult
				if json.Unmarshal(raw, &st) != nil {
					return
				}
				nodeKind := "node"
				if st.IsServer {
					nodeKind = "server"
				}
				var sb strings.Builder
				if len(st.Nodes) == 0 {
					fmt.Fprintf(&sb, " [darkgray]no %ss owned[-]\n", nodeKind)
				} else {
					for i, n := range st.Nodes {
						fmt.Fprintf(&sb, " [yellow]#%d[-] %s\n", i, n.Name)
						fmt.Fprintf(&sb, "  Lv %-3d  %.0fGB  %dc\n", n.Level, n.Ram, n.Cores)
						if st.IsServer {
							cap := 0.0
							if n.HashCapacity != nil {
								cap = *n.HashCapacity
							}
							used := 0.0
							if n.RamUsed != nil {
								used = *n.RamUsed
							}
							sb.WriteString(ctxRAMBar(used, n.Ram))
							fmt.Fprintf(&sb, "  [cyan]%.3f H/s[-]\n", n.Production)
							fmt.Fprintf(&sb, "  Cap: %.0f / %.0f\n", n.TotalProduction, cap)
						} else {
							fmt.Fprintf(&sb, "  [cyan]%s/s[-]\n", formatMoney(n.Production))
							fmt.Fprintf(&sb, "  Total: %s\n", formatMoney(n.TotalProduction))
						}
						sb.WriteString("\n")
					}
				}
				sb.WriteString(ctxSep)
				fmt.Fprintf(&sb, " Next %s\n  %s\n", nodeKind, formatMoney(st.NextNodeCost))
				text := sb.String()
				sess.app.QueueUpdateDraw(func() { sess.contextView.SetText(text) })
			})

			sess.cityHandler = func(ws *websocket.Conn, out io.Writer, sess *session, line string) bool {
				lp := strings.Fields(line)
				if len(lp) == 0 {
					return true
				}
				switch strings.ToLower(lp[0]) {
				case "exit", "quit":
					sess.cityHandler = prevHandler
					if prevHandler == nil {
						inSubMode.Store(false)
					}
					sess.currentRepl = "global"
					sess.restoreCompleter()
					sess.updatePrompt()
					sess.clearContextRefresh()
					return true

				case "help":
					helpTable(out, "Hacknet", [][2]string{
						{"ls / list", "Show all hacknet nodes/servers"},
						{"buy", "Purchase a new hacknet node"},
						{"upgrade <i> level [n]", "Upgrade node level"},
						{"upgrade <i> ram [n]", "Upgrade node RAM"},
						{"upgrade <i> core [n]", "Upgrade node cores"},
						{"upgrade <i> cache [n]", "Upgrade node cache (server mode)"},
						{"hashes", "Show hash balance and available upgrades"},
						{"spend <upgrade> [target] [n]", "Spend hashes on an upgrade"},
						{"-", ""},
						{"alias [name=\"value\"]", "Define an alias"},
						{"unalias <name>", "Remove an alias"},
						{"help", "Show this help"},
						{"exit", "Return to top-level"},
					})
					return true

				case "ls", "list":
					raw, err := sendMsgSync(ws, "hacknetInfo", nil)
					if err != nil {
						fmt.Fprintf(out, "error: %v\n", err)
						return true
					}
					var s hacknetStatusResult
					if err := json.Unmarshal(raw, &s); err != nil {
						fmt.Fprintf(out, "error: %v\n", err)
						return true
					}
					printHacknetStatus(s)

				case "buy":
					raw, err := sendMsgSync(ws, "hacknetBuy", nil)
					if err != nil {
						fmt.Fprintf(out, "error: %v\n", err)
						return true
					}
					var msg string
					_ = json.Unmarshal(raw, &msg)
					fmt.Fprintln(out, msg)

				case "upgrade":
					// upgrade <i> level|ram|core|cache [n]
					if len(lp) < 3 {
						fmt.Fprintln(out, "usage: upgrade <nodeIndex> level|ram|core|cache [count]")
						return true
					}
					idx := lp[1]
					kind := strings.ToLower(lp[2])
					count := "1"
					if len(lp) >= 4 {
						count = lp[3]
					}
					var method string
					switch kind {
					case "level":
						method = "hacknetUpgradeLevel"
					case "ram":
						method = "hacknetUpgradeRam"
					case "core", "cores":
						method = "hacknetUpgradeCore"
					case "cache":
						method = "hacknetUpgradeCache"
					default:
						fmt.Fprintf(out, "unknown upgrade type '%s'. Use: level, ram, core, cache\n", kind)
						return true
					}
					raw, err := sendMsgSync(ws, method, map[string]any{"stat": idx, "field": count})
					if err != nil {
						fmt.Fprintf(out, "error: %v\n", err)
						return true
					}
					var msg string
					_ = json.Unmarshal(raw, &msg)
					fmt.Fprintln(out, msg)

				case "hashes":
					raw, err := sendMsgSync(ws, "hacknetHashInfo", nil)
					if err != nil {
						fmt.Fprintf(out, "error: %v\n", err)
						return true
					}
					var hs hashStatusResult
					if err := json.Unmarshal(raw, &hs); err != nil {
						var s string
						_ = json.Unmarshal(raw, &s)
						fmt.Fprintln(out, s)
						return true
					}
					fmt.Fprintf(out, "Hashes: %.2f / %.2f\n\n", hs.Hashes, hs.Capacity)
					w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
					fmt.Fprintln(w, "Upgrade\tLevel\tNext Cost")
					for _, u := range hs.Upgrades {
						costStr := formatMoney(u.Cost)
						if u.Cost >= 1e18 {
							costStr = "N/A"
						}
						fmt.Fprintf(w, "%s\t%d\t%s\n", u.Name, u.Level, costStr)
					}
					w.Flush()

				case "spend":
					// spend <upgrade name (may have spaces)> [target] [count]
					// We treat last token as count if it's a number, second-to-last as target
					// if it doesn't look like part of the upgrade name.
					// Simplest heuristic: all remaining tokens after "spend" form the upgrade name,
					// unless the last token is a number (count) or the second-to-last is a known server.
					// Use explicit flag: spend "<upgrade>" [target] [count]
					if len(lp) < 2 {
						fmt.Fprintln(out, "usage: spend <upgradeName> [target] [count]")
						fmt.Fprintln(out, "  upgradeName: use quotes if it contains spaces, e.g. \"Sell for Money\"")
						return true
					}
					// Rejoin everything after "spend" and parse: last numeric token is count,
					// the token before that (if the upgrade needs a target) is target.
					// Let the user pass: spend "Sell for Money" 5
					//                or: spend "Reduce Minimum Security" foodnstuff 2
					// We just pass all tokens as a joined upgrade name and let server validate,
					// but we need to separate count and target. Use positional heuristic:
					// - if last token is a digit, it's count
					// - if upgrade name is one of the target-requiring ones, second-to-last is target
					targetRequiring := map[string]bool{
						"Reduce Minimum Security":           true,
						"Increase Maximum Money":            true,
						"Exchange for Corporation Research": false, // no target
						"Company Favor":                     true,
					}
					remaining := lp[1:]
					count := "1"
					target := ""
					// Peel count off the end if numeric
					if len(remaining) > 0 {
						if _, err := strconv.Atoi(remaining[len(remaining)-1]); err == nil {
							count = remaining[len(remaining)-1]
							remaining = remaining[:len(remaining)-1]
						}
					}
					// If the remaining last token looks like a server/company name (not part of
					// a known multi-word upgrade), treat it as a target.
					// We detect this by checking if joining all remaining is a known upgrade name
					// after peeling the last token.
					upgName := strings.Join(remaining, " ")
					_, needsTarget := targetRequiring[upgName]
					if !needsTarget {
						// Try peeling last token as target
						if len(remaining) > 1 {
							candidate := strings.Join(remaining[:len(remaining)-1], " ")
							if _, ok2 := targetRequiring[candidate]; ok2 {
								target = remaining[len(remaining)-1]
								upgName = candidate
							}
						}
					}
					raw, err := sendMsgSync(ws, "hacknetSpendHashes", map[string]any{
						"stat":   upgName,
						"field":  target,
						"course": count,
					})
					if err != nil {
						fmt.Fprintf(out, "error: %v\n", err)
						return true
					}
					var msg string
					_ = json.Unmarshal(raw, &msg)
					fmt.Fprintln(out, msg)

				case "alias":
					am := replAliases(sess.currentRepl)
					if len(lp) < 2 {
						if len(am) == 0 {
							fmt.Fprintln(out, "no aliases defined")
						} else {
							keys := make([]string, 0, len(am))
							for k := range am {
								keys = append(keys, k)
							}
							sort.Strings(keys)
							for _, k := range keys {
								fmt.Fprintf(out, "alias %s=%q\n", k, am[k])
							}
						}
						return true
					}
					name := lp[1]
					value := strings.Join(lp[2:], " ")
					if value == "" && strings.Contains(name, "=") {
						idx := strings.IndexByte(name, '=')
						value = name[idx+1:]
						name = name[:idx]
					}
					am[name] = value
					saveAliases()
					fmt.Fprintf(out, "alias %s=%q\n", name, value)

				case "unalias":
					if len(lp) < 2 {
						fmt.Fprintln(out, "usage: unalias <name>")
						return true
					}
					delete(replAliases(sess.currentRepl), lp[1])
					saveAliases()

				default:
					fmt.Fprintf(out, "unknown command: %s\n", lp[0])
					fmt.Fprintln(out, "Commands: ls  buy  upgrade <i> level|ram|core|cache [n]  hashes  spend <upgrade> [target] [count]  alias  unalias  exit")
				}
				return true
			}
		}},

		{"cloud", func(ws *websocket.Conn, out io.Writer, sess *session, parts []string) {
			getPurchased := func() ([]string, error) {
				raw, err := sendMsgSync(ws, "getAllServers", nil)
				if err != nil {
					return nil, err
				}
				var all []rfaServer
				if err := json.Unmarshal(raw, &all); err != nil {
					return nil, err
				}
				var owned []string
				for _, s := range all {
					if s.PurchasedByPlayer {
						owned = append(owned, s.Hostname)
					}
				}
				sort.Strings(owned)
				return owned, nil
			}

			owned, err := getPurchased()
			if err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
				return
			}

			printList := func() {
				if len(owned) == 0 {
					fmt.Fprintln(out, "No purchased servers. Use 'buy <name> <ram>' to purchase one.")
					return
				}
				w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
				fmt.Fprintln(w, "  Server\tRAM used\tRAM max\t%")
				for _, h := range owned {
					raw, err := sendMsgSync(ws, "getServerInfo", map[string]string{"server": h})
					if err != nil {
						fmt.Fprintf(w, "  %s\terror\t\t\n", h)
						continue
					}
					var si serverInfo
					if json.Unmarshal(raw, &si) != nil {
						continue
					}
					pct := 0.0
					if si.MaxRam > 0 {
						pct = si.RamUsed / si.MaxRam * 100
					}
					fmt.Fprintf(w, "  %s\t%s\t%s\t%.0f%%\n", h, formatRAM(si.RamUsed), formatRAM(si.MaxRam), pct)
				}
				w.Flush()
			}

			printList()
			fmt.Fprintln(out)
			fmt.Fprintln(out, "Commands: ls  info <host>  ps [host]  run <script> <host> [threads] [args...]")
			fmt.Fprintln(out, "          kill <pid> <host>  killall <host>  buy <name> <ram>  upgrade <name> <ram>  scp <file> <host>  exit")

			prevHandler := sess.cityHandler
			sess.setPromptStr("[cyan][[white]cloud[cyan]][-] [green]>[-] ")
			sess.setSubCompleter([]string{"ls", "info", "ps", "run", "kill", "killall", "buy", "upgrade", "scp", "help", "alias", "unalias", "exit"})
			sess.currentRepl = "cloud"
			inSubMode.Store(true)

			sess.setContextRefresh("Cloud", func(ws *websocket.Conn) {
				servers, err := getPurchased()
				if err != nil {
					return
				}
				var sb strings.Builder
				if len(servers) == 0 {
					fmt.Fprintf(&sb, " [darkgray]no servers[-]\n")
				} else {
					for _, h := range servers {
						raw, err := sendMsgSync(ws, "getServerInfo", map[string]string{"server": h})
						if err != nil {
							continue
						}
						var si serverInfo
						if json.Unmarshal(raw, &si) != nil {
							continue
						}
						fmt.Fprintf(&sb, " [yellow]%s[-]\n", h)
						sb.WriteString(ctxRAMBar(si.RamUsed, si.MaxRam))
						sb.WriteString("\n")
					}
				}
				sb.WriteString(ctxSep)
				fmt.Fprintf(&sb, " Fleet: [cyan]%d[-] servers\n", len(servers))
				text := sb.String()
				sess.app.QueueUpdateDraw(func() { sess.contextView.SetText(text) })
			})

			sess.cityHandler = func(ws *websocket.Conn, out io.Writer, sess *session, line string) bool {
				lp := strings.Fields(line)
				if len(lp) == 0 {
					return true
				}
				switch lp[0] {
				case "exit", "quit":
					sess.cityHandler = prevHandler
					if prevHandler == nil {
						inSubMode.Store(false)
					}
					sess.currentRepl = "global"
					sess.restoreCompleter()
					sess.updatePrompt()
					sess.clearContextRefresh()
					return true

				case "help":
					helpTable(out, "Cloud", [][2]string{
						{"ls", "List owned servers with RAM usage"},
						{"info <host>", "Show detailed server info"},
						{"ps [host]", "List running scripts on a server"},
						{"run <script> <host> [t] [args]", "Run a script with optional thread count"},
						{"kill <pid> <host>", "Kill a script by PID"},
						{"killall <host>", "Kill all scripts on a server"},
						{"buy <name> <ram>", "Purchase a new server"},
						{"upgrade <name> <ram>", "Upgrade a server's RAM"},
						{"scp <file> <host>", "Copy a file to a server"},
						{"-", ""},
						{"alias [name=\"value\"]", "Define an alias"},
						{"unalias <name>", "Remove an alias"},
						{"help", "Show this help"},
						{"exit", "Return to top-level"},
					})
					return true

				case "ls":
					var err error
					owned, err = getPurchased()
					if err != nil {
						fmt.Fprintf(out, "error: %v\n", err)
						return true
					}
					printList()

				case "info":
					if len(lp) < 2 {
						fmt.Fprintln(out, "usage: info <hostname>")
						return true
					}
					raw, err := sendMsgSync(ws, "getServerInfo", map[string]string{"server": lp[1]})
					if err != nil {
						fmt.Fprintf(out, "error: %v\n", err)
						return true
					}
					var si serverInfo
					if err := json.Unmarshal(raw, &si); err != nil {
						fmt.Fprintf(out, "error: %v\n", err)
						return true
					}
					pct := 0.0
					if si.MaxRam > 0 {
						pct = si.RamUsed / si.MaxRam * 100
					}
					fmt.Fprintf(out, "Host:  %s\n", si.Hostname)
					fmt.Fprintf(out, "RAM:   %s / %s (%.0f%% used)\n", formatRAM(si.RamUsed), formatRAM(si.MaxRam), pct)
					fmt.Fprintf(out, "Cores: %d\n", si.CPUCores)

				case "ps":
					target := sess.currentServer
					if len(lp) >= 2 {
						target = lp[1]
					}
					raw, err := sendMsgSync(ws, "getRunningScripts", map[string]string{"server": target})
					if err != nil {
						fmt.Fprintf(out, "error: %v\n", err)
						return true
					}
					var scripts []runningScript
					if err := json.Unmarshal(raw, &scripts); err != nil {
						fmt.Fprintf(out, "error: %v\n", err)
						return true
					}
					if len(scripts) == 0 {
						fmt.Fprintf(out, "no scripts running on %s\n", target)
						return true
					}
					w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
					fmt.Fprintln(w, "PID\tScript\tThreads\tRAM")
					for _, s := range scripts {
						fmt.Fprintf(w, "%d\t%s\t%d\t%.2fGB\n", s.PID, s.Filename, s.Threads, s.RamUsage)
					}
					w.Flush()

				case "run":
					if len(lp) < 3 {
						fmt.Fprintln(out, "usage: run <script> <host> [threads] [args...]")
						return true
					}
					script := lp[1]
					host := lp[2]
					threads := 1
					scriptArgs := []any{}
					if len(lp) >= 4 {
						if n, err := strconv.Atoi(lp[3]); err == nil && n > 0 {
							threads = n
							for _, a := range lp[4:] {
								scriptArgs = append(scriptArgs, a)
							}
						} else {
							for _, a := range lp[3:] {
								scriptArgs = append(scriptArgs, a)
							}
						}
					}
					raw, err := sendMsgSync(ws, "runScript", map[string]any{
						"server": host, "filename": script, "threads": threads, "args": scriptArgs,
					})
					if err != nil {
						fmt.Fprintf(out, "error: %v\n", err)
						return true
					}
					var pid int
					json.Unmarshal(raw, &pid)
					fmt.Fprintf(out, "started %s on %s (PID %d, %d thread(s))\n", script, host, pid, threads)

				case "kill":
					if len(lp) < 3 {
						fmt.Fprintln(out, "usage: kill <pid> <host>")
						return true
					}
					pid, err := strconv.Atoi(lp[1])
					if err != nil {
						fmt.Fprintln(out, "usage: kill <pid> <host>")
						return true
					}
					_, err = sendMsgSync(ws, "killScript", map[string]any{"server": lp[2], "pid": pid})
					if err != nil {
						fmt.Fprintf(out, "error: %v\n", err)
						return true
					}
					fmt.Fprintf(out, "killed PID %d on %s\n", pid, lp[2])

				case "killall":
					if len(lp) < 2 {
						fmt.Fprintln(out, "usage: killall <host>")
						return true
					}
					_, err := sendMsgSync(ws, "killAllScripts", map[string]string{"server": lp[1]})
					if err != nil {
						fmt.Fprintf(out, "error: %v\n", err)
						return true
					}
					fmt.Fprintf(out, "killed all scripts on %s\n", lp[1])

				case "buy":
					if len(lp) < 3 {
						fmt.Fprintln(out, "usage: buy <hostname> <ram_gb>")
						return true
					}
					raw, err := sendMsgSync(ws, "buyServer", map[string]string{"location": lp[1], "stat": lp[2]})
					if err != nil {
						fmt.Fprintf(out, "error: %v\n", err)
						return true
					}
					var msg string
					json.Unmarshal(raw, &msg)
					fmt.Fprintln(out, msg)
					owned = append(owned, lp[1])
					sort.Strings(owned)

				case "upgrade":
					if len(lp) < 3 {
						fmt.Fprintln(out, "usage: upgrade <hostname> <ram_gb>")
						return true
					}
					raw, err := sendMsgSync(ws, "upgradeServer", map[string]string{"location": lp[1], "stat": lp[2]})
					if err != nil {
						fmt.Fprintf(out, "error: %v\n", err)
						return true
					}
					var msg string
					json.Unmarshal(raw, &msg)
					fmt.Fprintln(out, msg)

				case "scp":
					if len(lp) < 3 {
						fmt.Fprintln(out, "usage: scp <file> <dst_host>")
						return true
					}
					filename, dst := lp[1], lp[2]
					raw, err := sendMsgSync(ws, "getFile", map[string]string{"server": sess.currentServer, "filename": filename})
					if err != nil {
						fmt.Fprintf(out, "error reading %s: %v\n", filename, err)
						return true
					}
					var content string
					if err := json.Unmarshal(raw, &content); err != nil {
						fmt.Fprintf(out, "error: %v\n", err)
						return true
					}
					if _, err := sendMsgSync(ws, "pushFile", map[string]string{
						"server": dst, "filename": filename, "content": content,
					}); err != nil {
						fmt.Fprintf(out, "error writing to %s: %v\n", dst, err)
						return true
					}
					fmt.Fprintf(out, "%s -> %s:%s\n", filename, dst, filename)

				case "alias":
					am := replAliases(sess.currentRepl)
					if len(lp) < 2 {
						if len(am) == 0 {
							fmt.Fprintln(out, "no aliases defined")
						} else {
							keys := make([]string, 0, len(am))
							for k := range am {
								keys = append(keys, k)
							}
							sort.Strings(keys)
							for _, k := range keys {
								fmt.Fprintf(out, "alias %s=%q\n", k, am[k])
							}
						}
						return true
					}
					name := lp[1]
					value := strings.Join(lp[2:], " ")
					if value == "" && strings.Contains(name, "=") {
						idx := strings.IndexByte(name, '=')
						value = name[idx+1:]
						name = name[:idx]
					}
					am[name] = value
					saveAliases()
					fmt.Fprintf(out, "alias %s=%q\n", name, value)

				case "unalias":
					if len(lp) < 2 {
						fmt.Fprintln(out, "usage: unalias <name>")
						return true
					}
					delete(replAliases(sess.currentRepl), lp[1])
					saveAliases()

				default:
					fmt.Fprintf(out, "unknown command: %s\n", lp[0])
					fmt.Fprintln(out, "Commands: ls  info <host>  ps [host]  run <script> <host> [threads] [args...]")
					fmt.Fprintln(out, "          kill <pid> <host>  killall <host>  buy <name> <ram>  upgrade <name> <ram>  scp <file> <host>  exit")
				}
				return true
			}
		}},

		{"programs", func(ws *websocket.Conn, out io.Writer, sess *session, parts []string) {
			raw, err := sendMsgSync(ws, "listCreatablePrograms", nil)
			if err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
				return
			}
			var progs []creatableProgramInfo
			if err := json.Unmarshal(raw, &progs); err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
				return
			}

			printPrograms := func(progs []creatableProgramInfo) {
				w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
				fmt.Fprintln(w, "Program\tReq\tBase Time\tStatus")
				for _, p := range progs {
					status := "available"
					if p.Owned {
						status = "owned"
					} else if !p.ReqMet {
						status = fmt.Sprintf("need hack %d", p.ReqLevel)
					} else if p.Progress > 0 {
						status = fmt.Sprintf("%.1f%% complete (paused)", p.Progress)
					}
					fmt.Fprintf(w, "%s\t%d\t%s\t%s\n", p.Name, p.ReqLevel, formatTime(p.Time/1000), status)
				}
				w.Flush()
			}

			printPrograms(progs)
			fmt.Fprintln(out)
			fmt.Fprintln(out, "Commands: ls  create <program>  stop  help  exit")

			// Cache non-owned program names for Tab completion.
			names := make([]string, 0, len(progs))
			for _, p := range progs {
				if !p.Owned {
					names = append(names, p.Name)
				}
			}
			sess.programNames = names

			prevHandler := sess.cityHandler
			sess.setPromptStr("[cyan][[white]programs[cyan]][-] [green]>[-] ")
			sess.setSubCompleter([]string{"ls", "create", "stop", "help", "alias", "unalias", "exit"})
			sess.currentRepl = "programs"
			inSubMode.Store(true)

			sess.setContextRefresh("Programs", func(ws *websocket.Conn) {
				var sb strings.Builder

				// Current creation progress (if any).
				if rawPP, err2 := sendMsgSync(ws, "getProgramProgress", nil); err2 == nil && string(rawPP) != "null" {
					var pp programProgressResult
					if json.Unmarshal(rawPP, &pp) == nil {
						filled := int(pp.Progress / 100 * 12)
						bar := strings.Repeat("█", filled) + strings.Repeat("░", 12-filled)
						sb.WriteString(ctxHeader("Creating"))
						fmt.Fprintf(&sb, " [cyan]%s[-]\n", pp.ProgramName)
						fmt.Fprintf(&sb, " [yellow]%s[-] %.1f%%\n", bar, pp.Progress)
					}
				}

				// Program list with owned/available indicators.
				if rawList, err2 := sendMsgSync(ws, "listCreatablePrograms", nil); err2 == nil {
					var list []creatableProgramInfo
					if json.Unmarshal(rawList, &list) == nil {
						sb.WriteString(ctxHeader("Programs"))
						for _, p := range list {
							if p.Owned {
								fmt.Fprintf(&sb, " [green]✓[-] %s\n", p.Name)
							} else if !p.ReqMet {
								fmt.Fprintf(&sb, " [darkgray]· %s[-]\n", p.Name)
							} else {
								fmt.Fprintf(&sb, " [white]·[-] %s\n", p.Name)
							}
						}
					}
				}

				text := sb.String()
				sess.app.QueueUpdateDraw(func() { sess.contextView.SetText(text) })
			})

			sess.cityHandler = func(ws *websocket.Conn, out io.Writer, sess *session, line string) bool {
				lp := strings.Fields(line)
				if len(lp) == 0 {
					return true
				}
				switch strings.ToLower(lp[0]) {
				case "exit", "quit":
					sess.cityHandler = prevHandler
					if prevHandler == nil {
						inSubMode.Store(false)
					}
					sess.currentRepl = "global"
					sess.programNames = nil
					sess.restoreCompleter()
					sess.updatePrompt()
					sess.clearContextRefresh()
					return true

				case "help":
					helpTable(out, "Programs", [][2]string{
						{"ls", "List creatable programs and their status"},
						{"create <program>", "Start creating a program (background work)"},
						{"stop", "Cancel current program creation"},
						{"-", ""},
						{"alias [name=\"value\"]", "Define an alias"},
						{"unalias <name>", "Remove an alias"},
						{"help", "Show this help"},
						{"exit", "Return to top-level"},
					})
					return true

				case "ls", "list":
					raw, err := sendMsgSync(ws, "listCreatablePrograms", nil)
					if err != nil {
						fmt.Fprintf(out, "error: %v\n", err)
						return true
					}
					var list []creatableProgramInfo
					if err := json.Unmarshal(raw, &list); err != nil {
						fmt.Fprintf(out, "error: %v\n", err)
						return true
					}
					printPrograms(list)
					// Refresh autocomplete cache.
					names := make([]string, 0, len(list))
					for _, p := range list {
						if !p.Owned {
							names = append(names, p.Name)
						}
					}
					sess.programNames = names

				case "create":
					if len(lp) < 2 {
						fmt.Fprintln(out, "usage: create <program>")
						return true
					}
					progName := strings.Join(lp[1:], " ")
					raw, err := sendMsgSync(ws, "startCreateProgram", map[string]any{"stat": progName})
					if err != nil {
						fmt.Fprintf(out, "error: %v\n", err)
						return true
					}
					var msg string
					_ = json.Unmarshal(raw, &msg)
					fmt.Fprintln(out, msg)
					select {
					case sess.serverRefreshCh <- struct{}{}:
					default:
					}

				case "stop":
					raw, err := sendMsgSync(ws, "stopWork", nil)
					if err != nil {
						fmt.Fprintf(out, "error: %v\n", err)
						return true
					}
					var msg string
					_ = json.Unmarshal(raw, &msg)
					fmt.Fprintln(out, msg)
					select {
					case sess.serverRefreshCh <- struct{}{}:
					default:
					}

				case "alias":
					am := replAliases(sess.currentRepl)
					if len(lp) < 2 {
						if len(am) == 0 {
							fmt.Fprintln(out, "no aliases defined")
						} else {
							keys := make([]string, 0, len(am))
							for k := range am {
								keys = append(keys, k)
							}
							sort.Strings(keys)
							for _, k := range keys {
								fmt.Fprintf(out, "alias %s=%q\n", k, am[k])
							}
						}
						return true
					}
					name := lp[1]
					value := strings.Join(lp[2:], " ")
					if value == "" && strings.Contains(name, "=") {
						idx := strings.IndexByte(name, '=')
						value = name[idx+1:]
						name = name[:idx]
					}
					am[name] = value
					saveAliases()
					fmt.Fprintf(out, "alias %s=%q\n", name, value)

				case "unalias":
					if len(lp) < 2 {
						fmt.Fprintln(out, "usage: unalias <name>")
						return true
					}
					delete(replAliases(sess.currentRepl), lp[1])
					saveAliases()

				default:
					fmt.Fprintf(out, "unknown command: %s\n", lp[0])
				}
				return true
			}
		}},

		{"active_scripts", func(ws *websocket.Conn, out io.Writer, sess *session, parts []string) {
			raw, err := sendMsgSync(ws, "getAllRunningScripts", nil)
			if err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
				return
			}
			var groups []serverScriptGroup
			if err := json.Unmarshal(raw, &groups); err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
				return
			}

			// pidServer maps PID → hostname for kill/logs lookups.
			pidServer := map[int]string{}
			for _, g := range groups {
				for _, s := range g.Scripts {
					pidServer[s.PID] = g.Hostname
				}
			}

			printGroups := func(groups []serverScriptGroup) {
				if len(groups) == 0 {
					fmt.Fprintln(out, "no scripts running")
					return
				}
				totalScripts := 0
				for _, g := range groups {
					totalScripts += len(g.Scripts)
				}
				for _, g := range groups {
					scriptWord := "scripts"
					if len(g.Scripts) == 1 {
						scriptWord = "script"
					}
					fmt.Fprintf(out, "[%s]  RAM: %s / %s  •  %d %s\n",
						g.Hostname, formatRAM(g.RamUsed), formatRAM(g.RamMax), len(g.Scripts), scriptWord)
					w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
					fmt.Fprintln(w, "  PID\tT\tRAM\tRunning\tMoney\tScript")
					for _, s := range g.Scripts {
						args := ""
						if len(s.Args) > 0 {
							b, _ := json.Marshal(s.Args)
							args = " " + string(b)
						}
						moneyStr := ""
						if s.OnlineMoneyMade > 0 {
							moneyStr = formatMoney(s.OnlineMoneyMade)
						}
						fmt.Fprintf(w, "  %d\t%d\t%s\t%s\t%s\t%s%s\n",
							s.PID, s.Threads, formatRAM(s.RamUsage*float64(s.Threads)),
							formatTime(s.OnlineRunningTime), moneyStr, s.Filename, args)
					}
					w.Flush()
					fmt.Fprintln(out)
				}
				fmt.Fprintf(out, "Total: %d script(s) across %d server(s)\n", totalScripts, len(groups))
			}

			printGroups(groups)
			fmt.Fprintln(out, "\nCommands: ls  kill <pid>  logs <pid>  help  exit")

			prevHandler := sess.cityHandler
			sess.setPromptStr("[cyan][[white]active_scripts[cyan]][-] [green]>[-] ")
			sess.setSubCompleter([]string{"ls", "kill", "logs", "help", "alias", "unalias", "exit"})
			sess.currentRepl = "active_scripts"
			inSubMode.Store(true)

			sess.setContextRefresh("Active Scripts", func(ws *websocket.Conn) {
				rawG, err2 := sendMsgSync(ws, "getAllRunningScripts", nil)
				if err2 != nil {
					return
				}
				var gs []serverScriptGroup
				if json.Unmarshal(rawG, &gs) != nil {
					return
				}
				totalScripts := 0
				totalRamUsed := 0.0
				for _, g := range gs {
					totalScripts += len(g.Scripts)
					totalRamUsed += g.RamUsed
				}
				var sb strings.Builder
				fmt.Fprintf(&sb, " [yellow]%d[-] scripts\n", totalScripts)
				fmt.Fprintf(&sb, " [yellow]%d[-] servers\n", len(gs))
				fmt.Fprintf(&sb, " [yellow]%s[-] RAM\n", formatRAM(totalRamUsed))
				if len(gs) > 0 {
					sb.WriteString(ctxSep)
					sb.WriteString(ctxHeader("By Server"))
					for _, g := range gs {
						fmt.Fprintf(&sb, " [cyan]%s[-]\n", g.Hostname)
						fmt.Fprintf(&sb, "  %d script(s)\n", len(g.Scripts))
						sb.WriteString(ctxRAMBar(g.RamUsed, g.RamMax))
						sb.WriteString("\n")
					}
				}
				text := sb.String()
				sess.app.QueueUpdateDraw(func() { sess.contextView.SetText(text) })
			})

			sess.cityHandler = func(ws *websocket.Conn, out io.Writer, sess *session, line string) bool {
				lp := strings.Fields(line)
				if len(lp) == 0 {
					return true
				}
				switch strings.ToLower(lp[0]) {
				case "exit", "quit":
					sess.cityHandler = prevHandler
					if prevHandler == nil {
						inSubMode.Store(false)
					}
					sess.currentRepl = "global"
					sess.restoreCompleter()
					sess.updatePrompt()
					sess.clearContextRefresh()
					return true

				case "help":
					helpTable(out, "Active Scripts", [][2]string{
						{"ls", "Refresh and display all running scripts"},
						{"kill <pid>", "Kill a script by PID"},
						{"logs <pid>", "Print the last 25 log lines for a script"},
						{"-", ""},
						{"alias [name=\"value\"]", "Define an alias"},
						{"unalias <name>", "Remove an alias"},
						{"help", "Show this help"},
						{"exit", "Return to top-level"},
					})
					return true

				case "ls", "refresh":
					raw, err := sendMsgSync(ws, "getAllRunningScripts", nil)
					if err != nil {
						fmt.Fprintf(out, "error: %v\n", err)
						return true
					}
					var gs []serverScriptGroup
					if err := json.Unmarshal(raw, &gs); err != nil {
						fmt.Fprintf(out, "error: %v\n", err)
						return true
					}
					// Refresh PID→server cache.
					pidServer = map[int]string{}
					for _, g := range gs {
						for _, s := range g.Scripts {
							pidServer[s.PID] = g.Hostname
						}
					}
					groups = gs
					printGroups(gs)

				case "kill":
					if len(lp) < 2 {
						fmt.Fprintln(out, "usage: kill <pid>")
						return true
					}
					pid, err := strconv.Atoi(lp[1])
					if err != nil {
						fmt.Fprintf(out, "invalid PID: %s\n", lp[1])
						return true
					}
					hostname, ok := pidServer[pid]
					if !ok {
						fmt.Fprintf(out, "PID %d not found — run 'ls' to refresh\n", pid)
						return true
					}
					_, err = sendMsgSync(ws, "killScript", map[string]any{"server": hostname, "pid": pid})
					if err != nil {
						fmt.Fprintf(out, "error: %v\n", err)
						return true
					}
					fmt.Fprintf(out, "killed PID %d on %s\n", pid, hostname)
					delete(pidServer, pid)
					select {
					case sess.serverRefreshCh <- struct{}{}:
					default:
					}

				case "logs":
					if len(lp) < 2 {
						fmt.Fprintln(out, "usage: logs <pid>")
						return true
					}
					pid, err := strconv.Atoi(lp[1])
					if err != nil {
						fmt.Fprintf(out, "invalid PID: %s\n", lp[1])
						return true
					}
					hostname, ok := pidServer[pid]
					if !ok {
						fmt.Fprintf(out, "PID %d not found — run 'ls' to refresh\n", pid)
						return true
					}
					raw, err := sendMsgSync(ws, "getScriptLogs", map[string]any{"server": hostname, "pid": pid})
					if err != nil {
						fmt.Fprintf(out, "error: %v\n", err)
						return true
					}
					var logs []string
					if err := json.Unmarshal(raw, &logs); err != nil {
						fmt.Fprintf(out, "error: %v\n", err)
						return true
					}
					// Show last 25 lines.
					if len(logs) > 25 {
						logs = logs[len(logs)-25:]
					}
					for _, l := range logs {
						fmt.Fprintln(out, l)
					}

				case "alias":
					am := replAliases(sess.currentRepl)
					if len(lp) < 2 {
						if len(am) == 0 {
							fmt.Fprintln(out, "no aliases defined")
						} else {
							keys := make([]string, 0, len(am))
							for k := range am {
								keys = append(keys, k)
							}
							sort.Strings(keys)
							for _, k := range keys {
								fmt.Fprintf(out, "alias %s=%q\n", k, am[k])
							}
						}
						return true
					}
					name := lp[1]
					value := strings.Join(lp[2:], " ")
					if value == "" && strings.Contains(name, "=") {
						idx := strings.IndexByte(name, '=')
						value = name[idx+1:]
						name = name[:idx]
					}
					am[name] = value
					saveAliases()
					fmt.Fprintf(out, "alias %s=%q\n", name, value)

				case "unalias":
					if len(lp) < 2 {
						fmt.Fprintln(out, "usage: unalias <name>")
						return true
					}
					delete(replAliases(sess.currentRepl), lp[1])
					saveAliases()

				default:
					fmt.Fprintf(out, "unknown command: %s\n", lp[0])
				}
				return true
			}
		}},
	}
} // end init()

// ── Dispatch / completer ──────────────────────────────────────────────────────

func buildDispatch() map[string]cmdFn {
	m := make(map[string]cmdFn, len(commands))
	for _, c := range commands {
		m[c.name] = c.run
	}
	return m
}

// ── Output writer ─────────────────────────────────────────────────────────────

// queueWriter safely delivers writes to a TextView from any goroutine.
type queueWriter struct {
	rawOut io.Writer
	app    *tview.Application
	tv     *tview.TextView
}

func (q *queueWriter) Write(p []byte) (n int, err error) {
	data := make([]byte, len(p))
	copy(data, p)
	q.app.QueueUpdateDraw(func() {
		q.rawOut.Write(data) //nolint:errcheck
		q.tv.ScrollToEnd()
	})
	return len(p), nil
}

// ── Connection handler ────────────────────────────────────────────────────────

func handler(ws *websocket.Conn, sess *session) {
	defer ws.Close()
	activeWSMu.Lock()
	activeWS = ws
	activeWSMu.Unlock()
	defer func() {
		activeWSMu.Lock()
		if activeWS == ws {
			activeWS = nil
		}
		activeWSMu.Unlock()
	}()

	out := sess.out
	dispatch := buildDispatch()

	// Reset sub-repl state on each new connection.
	sess.cityHandler = nil
	sess.currentServer = "home"
	sess.currentRepl = "global"
	sess.seenInvites = nil
	inSubMode.Store(false)
	sess.updatePrompt()

	sess.app.QueueUpdateDraw(func() {
		sess.statusBar.SetText(" [green]● connected[-]")
	})
	defer sess.app.QueueUpdateDraw(func() {
		sess.statusBar.SetText(" [red]○ disconnected[-]")
	})

	// Start the receiver before any sendMsgSync call so responses are delivered.
	done := make(chan struct{})

	go func() {
		defer close(done)
		for {
			var raw string
			if err := websocket.Message.Receive(ws, &raw); err != nil {
				fmt.Fprintf(out, "bitburner disconnected: %v\n", err)
				return
			}
			var resp rpcResp
			if err := json.Unmarshal([]byte(raw), &resp); err != nil {
				fmt.Fprintf(out, "<< %s\n", raw)
				continue
			}

			mu.Lock()
			method := pending[resp.ID]
			delete(pending, resp.ID)
			ch := respChans[resp.ID]
			delete(respChans, resp.ID)
			mu.Unlock()

			// sync call — deliver result to waiting goroutine, don't print
			if ch != nil {
				if resp.Error != nil {
					ch <- rpcResult{err: unmarshalErrMsg(resp.Error)}
				} else {
					ch <- rpcResult{data: resp.Result}
				}
				continue
			}

			// async call — print to output view
			if resp.Error != nil {
				fmt.Fprintf(out, "<< [%s #%d] error: %s\n", method, resp.ID, unmarshalErrMsg(resp.Error))
			} else {
				fmt.Fprintf(out, "<< [%s #%d] %s\n", method, resp.ID, resp.Result)
			}
		}
	}()

	fmt.Fprintln(out, "bitburner connected")
	fetchAndShowFiles(ws, out, sess)

	for {
		select {
		case <-done:
			return
		case line := <-cmdChan:
			fmt.Fprintf(out, "[darkgray]> %s[-]\n", line)
			// city/hacknet sub-mode intercepts all input
			if sess.cityHandler != nil {
				// Expand aliases before the sub-repl sees the line.
				if lp := strings.Fields(line); len(lp) > 0 {
					if expanded, ok := replAliases(sess.currentRepl)[lp[0]]; ok {
						lp = append(strings.Fields(expanded), lp[1:]...)
						line = strings.Join(lp, " ")
					}
				}
				lp2 := strings.Fields(line)
				if len(lp2) > 0 && lp2[0] == "clear" {
					sess.app.QueueUpdateDraw(func() { sess.outputView.Clear() })
					continue
				}
				if len(lp2) > 0 && lp2[0] == "home" {
					sess.cityHandler = nil
					inSubMode.Store(false)
					sess.restoreCompleter()
					sess.updatePrompt()
					sess.setServer("home")
					fetchAndShowFiles(ws, out, sess)
					continue
				}
				if !sess.cityHandler(ws, out, sess, line) {
					sess.cityHandler = nil
					inSubMode.Store(false)
					sess.restoreCompleter()
					sess.updatePrompt()
				}
				continue
			}
			parts := strings.Fields(line)
			if len(parts) == 0 {
				continue
			}
			if expanded, ok := replAliases(sess.currentRepl)[parts[0]]; ok {
				parts = append(strings.Fields(expanded), parts[1:]...)
			}
			if len(parts) == 0 {
				continue
			}
			if run, ok := dispatch[parts[0]]; ok {
				run(ws, out, sess, parts)
			} else {
				fmt.Fprintf(out, "unknown command %q\n", parts[0])
			}
		}
	}
}

// ── Entry point ───────────────────────────────────────────────────────────────

func main() {
	app := tview.NewApplication()

	outputView := tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetWordWrap(false)
	outputView.SetBorder(true).SetTitle(" bb-remote ")

	statsBar := tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(false).
		SetWrap(false)
	statsBar.SetBorder(true).SetTitle(" Player ")
	statsBar.SetText("  [darkgray]waiting for connection[-]")

	contextView := tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetWordWrap(false)
	contextView.SetBorder(true).SetTitle(" Server ")

	statusBar := tview.NewTextView().
		SetDynamicColors(true)
	statusBar.SetText(" [red]○ waiting for connection[-]")

	inputField := tview.NewInputField().
		SetFieldBackgroundColor(tcell.ColorDefault)

	out := &queueWriter{
		rawOut: tview.ANSIWriter(outputView),
		app:    app,
		tv:     outputView,
	}

	sess := newSession(app, inputField, out, outputView, contextView, statusBar)
	// Set initial prompt directly — app.Run() hasn't started yet so QueueUpdateDraw would deadlock.
	inputField.SetLabel("[yellow][[white]home[yellow]][-] [green]>[-] ")

	// app.SetInputCapture fires BEFORE tview's built-in "Ctrl-C stops the app" check,
	// so this is the only place we can override that default.
	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyCtrlC:
			sess.dropdownOpen = false
			inputField.SetText("")
			sess.historyPos = -1
			sess.historyDraft = ""
			return nil // consumed — prevents tview's default Stop()
		case tcell.KeyCtrlD:
			app.Stop()
			return nil
		case tcell.KeyPgUp:
			row, col := outputView.GetScrollOffset()
			if row > 0 {
				outputView.ScrollTo(row-1, col)
			}
			return nil
		case tcell.KeyPgDn:
			row, col := outputView.GetScrollOffset()
			outputView.ScrollTo(row+1, col)
			return nil
		case tcell.KeyRune:
			switch event.Rune() {
			case '[':
				row, col := contextView.GetScrollOffset()
				if row > 0 {
					contextView.ScrollTo(row-3, col)
				}
				return nil
			case ']':
				row, col := contextView.GetScrollOffset()
				contextView.ScrollTo(row+3, col)
				return nil
			}
		}
		return event
	})

	inputField.SetAutocompleteFunc(sess.autocomplete)
	inputField.SetAutocompletedFunc(sess.autocompleteSelected)
	inputField.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		// Any regular character or backspace closes the dropdown.
		switch event.Key() {
		case tcell.KeyRune, tcell.KeyBackspace, tcell.KeyBackspace2, tcell.KeyDelete:
			sess.dropdownOpen = false
		}

		switch event.Key() {
		case tcell.KeyUp:
			if sess.dropdownOpen {
				return event // let tview navigate the dropdown list
			}
			sess.historyBack()
			return nil
		case tcell.KeyDown:
			if sess.dropdownOpen {
				return event
			}
			sess.historyForward()
			return nil
		case tcell.KeyEscape:
			sess.dropdownOpen = false
			return event
		case tcell.KeyTab:
			completions := sess.computeCompletions(inputField.GetText())
			switch len(completions) {
			case 1:
				sess.autocompleteSelected(completions[0], 0, tview.AutocompletedTab)
			default:
				if len(completions) > 1 {
					sess.tabActive = true
					sess.dropdownOpen = true
					inputField.Autocomplete()
				}
			}
			return nil
		}
		return event
	})
	inputField.SetDoneFunc(func(key tcell.Key) {
		if key != tcell.KeyEnter {
			return
		}
		line := strings.TrimSpace(inputField.GetText())
		inputField.SetText("")
		if line == "" {
			return
		}
		sess.addHistory(line)
		if line == "exit" && !inSubMode.Load() {
			if sess.currentServer != "home" {
				line = "home"
			} else {
				app.Stop()
				return
			}
		}
		select {
		case cmdChan <- line:
		default:
			// SetDoneFunc runs in the event loop goroutine — write directly, no QueueUpdateDraw.
			fmt.Fprintln(tview.ANSIWriter(outputView), "(no active connection — command dropped)")
			outputView.ScrollToEnd()
		}
	})

	mainRow := tview.NewFlex().
		AddItem(outputView, 0, 1, false).
		AddItem(contextView, 33, 0, false)

	layout := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(mainRow, 0, 1, false).
		AddItem(statsBar, 3, 0, false).
		AddItem(inputField, 1, 0, true).
		AddItem(statusBar, 1, 0, false)

	// doRefresh fetches player stats (→ top bar) and server info (→ right panel).
	doRefresh := func() {
		ws := getActiveWS()
		if ws == nil {
			return
		}

		// Player stats + faction invitations → top bar
		if rawPS, err := sendMsgSync(ws, "getPlayerStats", nil); err == nil {
			var ps playerStats
			if json.Unmarshal(rawPS, &ps) == nil {
				work := "idle"
				if ps.CurrentWork != nil {
					work = *ps.CurrentWork
				}

				// City prefix
				cityStr := ""
				if sess.currentCity != "" {
					cityStr = fmt.Sprintf(" [magenta]%s[-] ", sess.currentCity)
				}

				// Hacknet node count (always shown)
				hacknetNodes := 0
				if rawHN, err2 := sendMsgSync(ws, "getHacknetStatus", nil); err2 == nil {
					var hn hacknetStatusResult
					if json.Unmarshal(rawHN, &hn) == nil {
						hacknetNodes = hn.NumNodes
					}
				}

				// Cloud server count + rooted server count
				cloudCount, rootedCount := 0, 0
				if rawAll, err2 := sendMsgSync(ws, "getAllServers", nil); err2 == nil {
					var servers []rfaServer
					if json.Unmarshal(rawAll, &servers) == nil {
						names := make([]string, 0, len(servers))
						for _, s := range servers {
							if s.PurchasedByPlayer {
								cloudCount++
							}
							if s.HasAdminRights {
								rootedCount++
							}
							names = append(names, s.Hostname)
						}
						sess.serverNames = names
					}
				}

				line := fmt.Sprintf(
					"%s[yellow]HP[-] %.0f/%.0f  [yellow]$[-] %s  [cyan]Hack[-] %d  [cyan]Str[-] %d  [cyan]Def[-] %d  [cyan]Dex[-] %d  [cyan]Agi[-] %d  [cyan]Cha[-] %d  [darkgray]│  HN[-] %d  [darkgray]Cloud[-] %d  [darkgray]Root[-] %d  [darkgray]│[-]  [white]%s[-]",
					cityStr,
					ps.HP, ps.MaxHP, formatMoney(ps.Money),
					ps.Hacking, ps.Strength, ps.Defense, ps.Dexterity, ps.Agility, ps.Charisma,
					hacknetNodes, cloudCount, rootedCount,
					work,
				)

				// Check for new faction invitations; append count to stats bar
				if rawInv, err2 := sendMsgSync(ws, "getFactionInvitations", nil); err2 == nil {
					var invites []string
					if json.Unmarshal(rawInv, &invites) == nil {
						if sess.seenInvites == nil {
							sess.seenInvites = make(map[string]bool)
						}
						unread := 0
						for _, name := range invites {
							if !sess.seenInvites[name] {
								sess.seenInvites[name] = true
								delete(sess.readInvites, name) // new invite overrides a prior mark-read
								fmt.Fprintf(sess.out, "[yellow]*** FACTION INVITE: %s — use 'factions' then 'join %s' to accept ***[-]\n", name, name)
							}
							if !sess.readInvites[name] {
								unread++
							}
						}
						if unread > 0 {
							line += fmt.Sprintf("  [yellow]⚡ %d invite(s)[-]", unread)
						}
					}
				}

				app.QueueUpdateDraw(func() { statsBar.SetText(line) })
			}
		}

		// Right panel — delegate to sub-repl context fn if set, else show server info
		if sess.contextRefreshFn != nil {
			sess.contextRefreshFn(ws)
		} else if rawSI, err := sendMsgSync(ws, "getServerInfo", map[string]string{"server": sess.currentServer}); err == nil {
			var si serverInfo
			if json.Unmarshal(rawSI, &si) == nil {
				root := "[red]NO[-]"
				if si.HasAdminRights {
					root = "[green]YES[-]"
				}
				bdoor := "[red]NO[-]"
				if si.BackdoorInstalled {
					bdoor = "[green]YES[-]"
				}
				var sb strings.Builder
				fmt.Fprintf(&sb, " [yellow]%s[-]\n", si.Hostname)
				sb.WriteString(ctxSep)
				sb.WriteString(ctxRAMBar(si.RamUsed, si.MaxRam))
				fmt.Fprintf(&sb, " [white]Cores[-]    %d\n", si.CPUCores)
				sb.WriteString(ctxSep)
				fmt.Fprintf(&sb, " [white]Root[-]     %s\n", root)
				fmt.Fprintf(&sb, " [white]Backdoor[-] %s\n", bdoor)
				if si.MoneyMax > 0 {
					sb.WriteString(ctxSep)
					moneyPct := si.MoneyAvailable / si.MoneyMax * 100
					fmt.Fprintf(&sb, " [white]Money[-]\n  %s\n  / %s\n  [cyan]%.1f%%[-]\n",
						formatMoney(si.MoneyAvailable), formatMoney(si.MoneyMax), moneyPct)
				}
				sb.WriteString(ctxSep)
				fmt.Fprintf(&sb, " [white]Req hack[-]  %d\n", si.RequiredHacking)
				fmt.Fprintf(&sb, " [white]Ports req[-] %d\n", si.NumOpenPortsRequired)
				fmt.Fprintf(&sb, " [white]Security[-]  %.1f\n", si.HackDifficulty)
				fmt.Fprintf(&sb, " [white]Min sec[-]   %.1f\n", si.MinDifficulty)
				fmt.Fprintf(&sb, " [white]Hack %%[-]    [cyan]%.1f%%[-]\n", si.HackingChance*100)
				fmt.Fprintf(&sb, " [white]Hack time[-] %s\n", formatTime(si.HackingTime))
				// Running scripts
				if rawPS, err2 := sendMsgSync(ws, "getRunningScripts", map[string]string{"server": sess.currentServer}); err2 == nil {
					var scripts []runningScript
					if json.Unmarshal(rawPS, &scripts) == nil {
						sb.WriteString(ctxHeader("Scripts"))
						if len(scripts) == 0 {
							sb.WriteString(" [darkgray]none[-]\n")
						}
						for _, sc := range scripts {
							name := sc.Filename
							if idx := strings.LastIndex(name, "/"); idx >= 0 {
								name = name[idx+1:]
							}
							fmt.Fprintf(&sb, " [cyan]%d[-] %s\n", sc.PID, name)
							if sc.Threads > 1 {
								fmt.Fprintf(&sb, "   ×%d  %s\n", sc.Threads, formatRAM(sc.RamUsage))
							} else {
								fmt.Fprintf(&sb, "   %s\n", formatRAM(sc.RamUsage))
							}
						}
					}
				}
				// 1-hop neighbours via scanServer
				if rawScan, err2 := sendMsgSync(ws, "scanServer", map[string]string{"server": sess.currentServer}); err2 == nil {
					var neighbours []scanEntry
					if json.Unmarshal(rawScan, &neighbours) == nil {
						names := make([]string, 0, len(neighbours))
						for _, n := range neighbours {
							names = append(names, n.Hostname)
						}
						sess.neighbourNames = names
						if len(neighbours) > 0 {
							sb.WriteString(ctxHeader("Connections"))
							for _, n := range neighbours {
								indicator := "[red]·[-]"
								if n.HasAdminRights {
									indicator = "[green]·[-]"
								}
								fmt.Fprintf(&sb, " %s %s\n", indicator, n.Hostname)
							}
						}
					}
				}
				text := sb.String()
				app.QueueUpdateDraw(func() { contextView.SetText(text) })
			}
		}
	}

	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				doRefresh()
			case <-sess.serverRefreshCh:
				doRefresh()
			}
		}
	}()

	// WebSocket server.
	go func() {
		http.Handle("/", websocket.Handler(func(ws *websocket.Conn) {
			handler(ws, sess)
		}))
		addr := ":12525"
		log.Printf("listening on ws://localhost%s — waiting for Bitburner to connect", addr)
		if err := http.ListenAndServe(addr, nil); err != nil {
			log.Fatal(err)
		}
	}()

	if err := app.SetRoot(layout, true).SetFocus(inputField).Run(); err != nil {
		log.Fatal(err)
	}
}
