package ui

import (
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"

	"github.com/eugenetaranov/yardo/internal/tui"
)

// The app is yardo's one program: the menu (the home and its screens),
// the board (Tools) and Connection (Network, Local settings) are its
// views, in one alt screen. Moving between
// them keeps each one's state (the board's queue, tab, cursors and
// filter); only what needs the terminal (a question, a run) ends it.
// Every view has the same chrome: a header (who, where, the path), a
// toast line, and a footer of the keys it takes; ? lists them all.

// Key is one of a screen's keys, as its footer lists it; On draws it in
// the accent colour (the apply key, with something queued). A narrow
// footer keeps the On and Keep ones (moving about, searching) first.
type Key struct {
	Key, What string
	On, Keep  bool
}

// MoveMsg moves a list's cursor N rows down (up when N < 0): a screenful
// (PgUp, PgDn), to an end (Home, End, g, G), or a few (the wheel).
type MoveMsg struct{ N int }

// toEnd is a move to the first or last row.
const toEnd = 1 << 20

// Hit is a place on a view's body a click acts on: lines Y0 to Y1,
// columns X0 to X1 (X1 0: to the line's end). Do selects what's there;
// a second click within doubleClick acts on it, as enter does. Key
// presses a key instead.
type Hit struct {
	Y0, Y1, X0, X1 int
	Do             func()
	Key            string
}

func (h Hit) has(x, y int) bool {
	return y >= h.Y0 && y <= h.Y1 && x >= h.X0 && (h.X1 == 0 || x < h.X1)
}

// doubleClick is how soon a second click on the same place acts.
const doubleClick = 400 * time.Millisecond

// view is one of the app's: the menu, the board or Connection.
type view interface {
	Update(tea.Msg) (tea.Model, tea.Cmd)
	// Path is where the view is, from the home: Home › Tools › Dev.
	Path() string
	// Keys are the footer's keys, and Help every key the screen takes
	// (the keys screen's); neither has ? or q, which the app adds, q
	// saying Back.
	Keys() []Key
	Help() []Key
	Back() string
	// Body is the view between the header and the toast, w by h; it
	// records Hits as it draws.
	Body(w, h int) string
	Hits() []Hit
	Toast() string
	// Typing: keys go to a text field (paging keys too).
	Typing() bool
}

// Dialog is a question over a screen, in a frame: its lines, then its
// options as rows. Enter picks the option under the cursor; with YN, y
// and n pick "yes" and "no"; esc leaves it, nothing chosen, and so does
// q, unless Q is set: then q picks it (q again quits).
type Dialog struct {
	Title string
	Lines []string
	// Opts are the choices (label, value); Sel the selected one.
	Opts [][2]string
	Sel  int
	YN   bool
	Q    string
	// On is what a choice does.
	On func(value string) tea.Cmd
	// Input: it takes a line typed (Typed), after its lines, Ask before
	// it; enter gives it to On, unless Bad says what's wrong with it:
	// then it stays, saying so, the line cleared. Enter says what enter
	// does, for the footer.
	Input bool
	Ask   string
	Typed []rune
	Bad   func(typed string) string
	Enter string
	bad   string
}

// dialogKey is key k on the dialog d points to: it closes it on an
// answer (or leaving), and returns what the answer does.
func dialogKey(d **Dialog, k string) tea.Cmd {
	p := *d
	pick := func(v string) tea.Cmd {
		*d = nil
		if p.On != nil && v != "" {
			return p.On(v)
		}
		return nil
	}
	if p.Input {
		switch {
		case k == "esc":
			return pick("")
		case k == "enter":
			t := string(p.Typed)
			if p.Bad != nil {
				if why := p.Bad(t); why != "" {
					p.bad, p.Typed = why, nil
					return nil
				}
			}
			return pick(t)
		case k == "backspace":
			if len(p.Typed) > 0 {
				p.Typed = p.Typed[:len(p.Typed)-1]
			}
		case utf8.RuneCountInString(k) == 1:
			p.Typed, p.bad = append(p.Typed, []rune(k)...), ""
		}
		return nil
	}
	switch {
	case p.YN && (k == "y" || k == "n"):
		return pick(map[string]string{"y": "yes", "n": "no"}[k])
	case k == "esc":
		return pick("")
	case k == "q":
		return pick(p.Q)
	case k == "up" || k == "k":
		p.Sel = max(0, p.Sel-1)
	case k == "down" || k == "j":
		p.Sel = min(max(0, len(p.Opts)-1), p.Sel+1)
	case k == "enter" || k == " ":
		if len(p.Opts) == 0 {
			return pick("")
		}
		return pick(p.Opts[p.Sel][1])
	}
	return nil
}

// dialogLines draw d w wide, from line y of the body: its options' hits
// are added to hits.
func dialogLines(d *Dialog, w, y int, hits *[]Hit) []string {
	// iw: inside the frame, a space each side; the frame is w-1 wide, as
	// a box is.
	iw := max(10, w-6)
	edge := func(s string) string {
		if lipgloss.Width(s) > iw {
			s = ansi.Truncate(s, iw, "…")
		}
		return fmt.Sprintf(" %s %s%s %s", accentBd.Render("│"), s, strings.Repeat(" ", iw-lipgloss.Width(s)), accentBd.Render("│"))
	}
	title := runewidth.Truncate(d.Title, iw-1, "…")
	out := []string{" " + accentBd.Render(fmt.Sprintf("┌ %s %s┐", title, strings.Repeat("─", max(0, iw-runewidth.StringWidth(title)))))}
	for _, l := range d.Lines {
		// An indented line stays indented, wrapped.
		body := strings.TrimLeft(l, " ")
		pad := strings.Repeat(" ", len(l)-len(body))
		if body == "" {
			out = append(out, edge(""))
			continue
		}
		wls := []string{body}
		if lipgloss.Width(body) > iw-1-len(pad) {
			// Wrapped only when it must be: one that fits keeps its spacing.
			wls = wrap(body, iw-1-len(pad))
		}
		for _, wl := range wls {
			out = append(out, edge(fmt.Sprintf(" %s%s", pad, wl)))
		}
	}
	if d.Input {
		out = append(out, edge(fmt.Sprintf(" %s %s%s", d.Ask, bold.Render(string(d.Typed)), accent.Render("█"))))
		out = append(out, edge(" "+red.Render(d.bad)))
	}
	for n, o := range d.Opts {
		// The label at the same place, chosen or not.
		line := "   " + o[0]
		if n == d.Sel {
			line = rowOn.Render(padTo(" › "+o[0], iw))
		}
		if hits != nil {
			n := n
			*hits = append(*hits, Hit{Y0: y + len(out), Y1: y + len(out), Do: func() { d.Sel = n }})
		}
		out = append(out, edge(line))
	}
	return append(out, " "+accentBd.Render(fmt.Sprintf("└%s┘", strings.Repeat("─", iw+2))))
}

// dialogKeys are a dialog's footer keys: what its keys do (no ? or q
// tail: q is esc's, or picks Q).
func dialogKeys(d *Dialog) []Key {
	if d.Input {
		return []Key{{Key: "enter", What: cmpOr(d.Enter, "do it"), Keep: true}, {Key: "esc", What: "back", Keep: true}}
	}
	if len(d.Opts) == 0 {
		return []Key{{Key: "enter", What: "close"}}
	}
	keys := []Key{{Key: "↑↓", What: "choose"}, {Key: "enter", What: "do it"}}
	if d.YN {
		keys = append(keys, Key{Key: "y", What: "yes"}, Key{Key: "n", What: "no"})
	}
	keys = append(keys, Key{Key: "esc", What: "cancel", Keep: true})
	for _, o := range d.Opts {
		if d.Q != "" && o[1] == d.Q {
			what, _, _ := strings.Cut(o[0], " (")
			keys = append(keys, Key{Key: "q", What: strings.ToLower(what[:1]) + what[1:], Keep: true})
		}
	}
	return keys
}

// sharedKeys are the keys every screen takes, last on the keys screen.
var sharedKeys = []Key{
	{Key: "↑↓ j k", What: "move"},
	{Key: "PgUp PgDn", What: "a screenful up or down"},
	{Key: "Home g", What: "the first row"},
	{Key: "End G", What: "the last row"},
	{Key: "←→ h l", What: "the tab along the top before or after this one"},
	{Key: "tab", What: "the next section, or Tools' next tab"},
	{Key: "1–9", What: "the tabs along the top, by number"},
	{Key: "u", What: "Tools' Updates tab (nothing there: check for updates)"},
	{Key: "enter", What: "open, or act on the row"},
	{Key: "?", What: "these keys"},
	{Key: "q", What: "back; on the home, quit"},
	{Key: "ctrl+c", What: "quit, from anywhere"},
	{Key: "mouse", What: "click selects, double-click opens, the wheel scrolls"},
}

// AppResult is what the app was left with: the menu's result, the
// board's or Connection's, or Quit.
type AppResult struct {
	Menu  *MenuResult
	Board *BoardResult
	Conn  *ConnResult
	Quit  bool
}

// AppData is what the app shows: the machine as it was read.
type AppData struct {
	Info     HomeInfo
	Sub      string
	Items    []*BoardItem
	Settings []*BoardSetting
	Conn     ConnData
	Checked  bool
	Tabs     []Tab
	// First: nothing is installed yet; there's no home, only the board.
	First bool
}

// App is the program; see NewApp.
type App struct {
	Menu  *Menu
	Board *Board
	Conn  *Connection
	// onBoard: the board is the view; onConn: Connection is.
	onBoard, onConn bool
	first           bool
	toast           string
	// keys: the keys screen is up; dialog the app's own question (quit
	// with a queue).
	keys   bool
	dialog *Dialog
	width  int
	height int
	// foot are the footer's hits; nav where the tabs along the top are
	// (by id), on line 1; top the body's first line; click the last
	// click, at.
	foot    []Hit
	nav     map[string]tabSpot
	top     int
	click   *Hit
	clickAt time.Time
	now     func() time.Time
	result  AppResult
	out     bool
	// prep sets up a new board or menu (the planner, the switcher), on
	// NewApp and Reload.
	prep func(*App)
}

// NewApp builds the app on d, the home first (the board on a first run);
// prep sets up its views, now and on each Reload.
func NewApp(d AppData, prep func(*App)) *App {
	a := &App{width: 100, height: 30, now: time.Now, prep: prep}
	a.load(d)
	a.onBoard = d.First
	return a
}

func (a *App) load(d AppData) {
	a.first = d.First
	a.Menu = NewMenu(d.Info, d.Sub, d.Items, d.Settings, d.Checked)
	a.Board = NewBoard(d.Info, d.First, d.Tabs, d.Items)
	a.Conn = NewConnection(d.Conn, d.Info.Keys)
	a.Conn.access = a.Menu.access()
	a.Menu.setups = d.Conn.Setups
	a.Board.SetFromMenu(!d.First)
	m := a.Menu
	a.Board.SetChecking(func() bool { return m.wait != nil })
	if a.prep != nil {
		a.prep(a)
	}
	a.size()
}

// Reload shows d, the machine read again after a run: each view keeps its
// screen, tab, cursors and filter, and the board its queue but what the
// run applied (an add now installed, an uninstall or a forget gone).
func (a *App) Reload(d AppData) {
	m, b, c := a.Menu, a.Board, a.Conn
	pending := map[string]string{}
	for _, i := range b.items {
		if i.Pending != "" {
			pending[i.ID] = i.Pending
		}
	}
	a.load(d)
	for _, i := range a.Board.items {
		switch p := pending[i.ID]; {
		case p == "add" && !i.Picked, (p == "uninstall" || p == "forget") && i.Picked, p == "update" && newer(i), p == "fix" && fixable(i):
			i.Pending = p
		}
	}
	nb, nm := a.Board, a.Menu
	nb.tabID, nb.tab, nb.cursor, nb.search, nb.typing = b.tabID, b.tab, b.cursor, b.search, false
	nm.screen, nm.sel, nm.need, nm.mcpSel, nm.addSel, nm.fromBoard, nm.skSel = m.screen, m.sel, m.need, m.mcpSel, m.addSel, m.fromBoard, m.skSel
	a.Conn.tab, a.Conn.sel, a.Conn.at = c.tab, c.sel, c.at
	if nm.screen == "mcp:name" {
		nm.screen = "mcp"
	}
	if a.first && !d.First {
		a.onBoard = false
	}
	nm.normalize()
}

// Open shows view ("board", "network", "local", or a menu screen: home,
// mcp, settings…).
func (a *App) Open(screen string) {
	a.keys, a.dialog = false, nil
	a.onBoard, a.onConn = screen == "board", screen == "network" || screen == "local"
	if a.onConn {
		a.Conn.Show(map[string]string{"network": NetTab, "local": LocalTab}[screen])
	}
	if a.onBoard || a.onConn {
		return
	}
	a.Menu.Open(screen, a.Menu.toast, a.Menu.fromBoard)
}

// Toast shows s on the toast line until the next key.
func (a *App) Toast(s string) { a.toast = s }

func (a *App) view() view {
	switch {
	case a.onBoard:
		return a.Board
	case a.onConn:
		return a.Conn
	}
	return a.Menu
}

func (a *App) size() {
	msg := tea.WindowSizeMsg{Width: a.width, Height: a.height}
	a.Menu.Update(msg)
	a.Board.Update(msg)
	a.Conn.Update(msg)
}

func (a *App) Init() tea.Cmd {
	a.Menu.ended, a.Board.ended, a.Conn.ended, a.Conn.left = false, false, false, false
	return tea.Batch(a.Menu.Init(), a.Conn.Init())
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = v.Width, v.Height
		a.size()
		return a, nil
	case tea.MouseMsg:
		return a, a.mouse(v)
	case tea.KeyMsg:
		return a, a.key(v)
	case MoveMsg:
		return a, a.send(v)
	case connMsg:
		// Connection's background work, whichever view is up.
		_, cmd := a.Conn.Update(msg)
		return a, cmd
	}
	// The menu's ticks and checks, whichever view is up; the board shares
	// its items, so a check's updates reach the Updates tab too.
	_, cmd := a.Menu.Update(msg)
	if _, ok := msg.(UpdatesMsg); ok && a.onBoard {
		a.toast, a.Menu.toast = a.Menu.toast, ""
		for _, i := range a.Board.items {
			if i.Pending == "update" && !newer(i) {
				i.Pending = ""
			}
		}
	}
	return a, a.after(cmd)
}

func (a *App) key(v tea.KeyMsg) tea.Cmd {
	k := v.String()
	a.toast = ""
	if k == "ctrl+c" {
		a.result = AppResult{Quit: true}
		return tea.Quit
	}
	if a.dialog != nil {
		return a.after(dialogKey(&a.dialog, k))
	}
	if a.keys {
		if k == "q" || k == "esc" || k == "?" {
			a.keys = false
		}
		return nil
	}
	cur := a.view()
	if !cur.Typing() && !a.dialogUp() {
		switch k {
		case "?":
			a.keys = true
			return nil
		case "pgup", "pgdown":
			n := max(1, a.height-8)
			if k == "pgup" {
				n = -n
			}
			return a.send(MoveMsg{N: n})
		case "home", "g":
			return a.send(MoveMsg{N: -toEnd})
		case "end", "G":
			return a.send(MoveMsg{N: toEnd})
		case "u":
			// The board's Updates tab, from anywhere but it (where u checks
			// again) and a search; with nothing there, a check.
			if !a.first && (!a.onBoard || !a.Board.onUpdates() && a.Board.search == nil) {
				return a.toUpdates()
			}
		case "left", "right", "h", "l":
			// The tabs along the top, one either way: on a tab's own
			// screen, not deeper (where ←→ picks a choice).
			if !a.first && a.onTab() {
				_, ids := a.Menu.navTabs()
				d := 1
				if k == "left" || k == "h" {
					d = len(ids) - 1
				}
				return a.goTab(ids[(max(0, a.navSel(ids))+d)%len(ids)])
			}
		case "1", "2", "3", "4", "5", "6", "7", "8", "9":
			// The tabs along the top, by number.
			if _, ids := a.Menu.navTabs(); !a.first && int(k[0]-'1') < len(ids) {
				return a.goTab(ids[k[0]-'1'])
			}
			if !a.first {
				return nil
			}
		case "q":
			// The home with a queue on the board: asked once.
			if !a.onBoard && a.Menu.screen == "home" && a.Board.queued() > 0 {
				n := a.Board.queued()
				a.dialog = &Dialog{Title: plural(n, "change isn't", "changes aren't") + " applied", YN: true, Q: "yes", Sel: 1,
					Lines: []string{fmt.Sprintf("Tools has %s queued: %s.", plural(n, "change", "changes"), a.Board.queueSay()), fmt.Sprintf("q again quits without %s.", map[bool]string{true: "it", false: "them"}[n == 1])},
					Opts:  [][2]string{{fmt.Sprintf("Quit without %s (y)", map[bool]string{true: "it", false: "them"}[n == 1]), "yes"}, {"Stay (n)", "no"}, {"Go to Tools to apply", "tools"}},
					On: func(v string) tea.Cmd {
						switch v {
						case "yes":
							a.result = AppResult{Quit: true}
							return tea.Quit
						case "tools":
							a.onBoard = true
						}
						return nil
					}}
				return nil
			}
		}
	}
	_, cmd := cur.Update(v)
	return a.after(cmd)
}

// dialogUp: the view has a question open (its keys are the dialog's).
func (a *App) dialogUp() bool {
	switch {
	case a.onBoard:
		return a.Board.dialog != nil
	case a.onConn:
		return a.Conn.dialog != nil
	}
	return a.Menu.asking()
}

// onTab is whether the view is a tab's own screen (the home, Tools,
// Network…), not one under it, where ←→ go along the tabs.
func (a *App) onTab() bool {
	switch {
	case a.onBoard:
		return a.Board.search == nil
	case a.onConn:
		return true
	}
	return slices.Contains([]string{"home", "settings", "mcp", "skills", "lastrun"}, a.Menu.screen)
}

// navSel is the tab along the top the view is on: the board is Tools,
// Connection the tab it shows; the menu, its screen's tab (Home on the
// home).
func (a *App) navSel(ids []string) int {
	id := ""
	switch {
	case a.onBoard:
		id = "tools"
	case a.onConn:
		id = a.Conn.tab
	default:
		return slices.Index(ids, cmpOr(a.Menu.tabOf(), "home"))
	}
	return slices.Index(ids, id)
}

// toUpdates opens the board's Updates tab (u, from anywhere): with
// nothing on it, it starts a check instead, as u on it does.
func (a *App) toUpdates() tea.Cmd {
	m, b := a.Menu, a.Board
	if !b.updating() {
		cmd := m.checkUpdates()
		if a.onConn || a.onBoard {
			a.toast, m.toast = m.toast, ""
		}
		return a.after(cmd)
	}
	if !a.onConn && !a.onBoard && len(m.pend)+len(m.skPend) > 0 {
		m.toast = "Apply first (a), or turn back what you changed."
		return nil
	}
	b.search, b.typing = nil, false
	b.Open(UpdatesTab)
	a.onBoard, a.onConn = true, false
	return nil
}

// goTab opens the tab along the top with id, from any view: as enter on
// it on the home does. The board stays as it is for Tools, Connection
// for its other tab; MCP servers or Skills with changes not applied
// stay, saying so.
func (a *App) goTab(id string) tea.Cmd {
	m := a.Menu
	switch {
	case a.onBoard && id == "tools", a.onConn && id == a.Conn.tab, !a.onBoard && !a.onConn && m.screen == id:
		return nil
	case a.onConn && (id == NetTab || id == LocalTab):
		a.Conn.Show(id)
		return nil
	case !a.onBoard && !a.onConn && len(m.pend)+len(m.skPend) > 0 && m.tabOf() != id:
		m.toast = "Apply first (a), or turn back what you changed."
		return nil
	}
	a.onBoard, a.onConn = false, false
	m.screen, m.fromBoard, m.dialog, m.need = "home", false, nil, -1
	if !slices.ContainsFunc(m.rows(), func(r menuRow) bool { return r.id == id }) {
		return nil
	}
	return a.after(m.openTab(id))
}

func (a *App) send(msg tea.Msg) tea.Cmd {
	_, cmd := a.view().Update(msg)
	return a.after(cmd)
}

// after takes what a view was left with: a hop to the other view, or a
// result that ends the program.
func (a *App) after(cmd tea.Cmd) tea.Cmd {
	m, b, c := a.Menu, a.Board, a.Conn
	// A key dismissed on Network is off the home's Needs you too.
	m.info.Keys = c.keys
	switch {
	case c.left:
		// Home, on its tab.
		c.left, a.onConn = false, false
		m.screen, m.need = "home", -1
		m.sel = max(0, slices.IndexFunc(m.rows(), func(r menuRow) bool { return r.id == c.tab }))
		m.normalize()
	case c.toAccess >= 0:
		// Network access is the menu's screen; q on it comes back here.
		n := c.toAccess
		c.toAccess, a.onConn = -1, false
		m.open("access")
		m.sel = n
		m.normalize()
	case c.ended:
		c.ended = false
		r := c.result
		a.result = AppResult{Conn: &r}
		return tea.Quit
	case m.ended && m.result.Kind == MenuConnection:
		// ID: the tab, and after a colon the row kind to be on (key).
		m.ended, m.screen = false, "home"
		tab, kind, _ := strings.Cut(m.result.ID, ":")
		c.Show(tab)
		if kind != "" {
			c.focus(kind)
		}
		a.onConn = true
	case b.CheckAsked():
		// u on the Updates tab: the home's check, said here.
		c := m.checkUpdates()
		if !m.ended {
			a.toast, m.toast = m.toast, ""
			return tea.Batch(cmd, c)
		}
		return a.after(c)
	case m.ended && m.result.Kind == MenuUpdatesTab:
		m.ended = false
		b.search, b.typing = nil, false
		b.Open(UpdatesTab)
		b.QueueUpdates()
		a.onBoard = true
	case m.ended && m.result.Kind == MenuFixes:
		m.ended = false
		b.search, b.typing = nil, false
		b.Focus(m.result.ID)
		a.onBoard = true
	case m.ended && m.result.Kind == MenuTools:
		m.ended = false
		if m.result.Search != "" {
			b.Search(m.result.Search)
		}
		a.onBoard = true
	case m.ended:
		m.ended = false
		r := m.result
		a.result = AppResult{Menu: &r}
		if r.Kind == MenuQuit {
			a.result = AppResult{Quit: true}
		}
		return tea.Quit
	case b.ended && b.result.Kind == BoardQuit && !a.first:
		// Home, the queue kept.
		b.ended, a.onBoard = false, false
		if m.screen != "home" && m.screen != "mcp" {
			m.Open("home", "", false)
		}
		m.screen, m.fromBoard = "home", false
		m.normalize()
	case b.ended && b.result.Kind == BoardChange && b.result.Change == "mcp":
		b.ended, a.onBoard = false, false
		m.Open("mcp", "", true)
	case b.ended:
		b.ended = false
		r := b.result
		a.result = AppResult{Board: &r}
		if r.Kind == BoardQuit {
			a.result = AppResult{Quit: true}
		}
		return tea.Quit
	}
	return cmd
}

// mouse: a click selects (a second one acts), on the footer presses its
// key; the wheel moves the list.
func (a *App) mouse(v tea.MouseMsg) tea.Cmd {
	switch {
	case v.Button == tea.MouseButtonWheelUp:
		return a.send(MoveMsg{N: -3})
	case v.Button == tea.MouseButtonWheelDown:
		return a.send(MoveMsg{N: 3})
	case v.Button != tea.MouseButtonLeft || v.Action != tea.MouseActionPress:
		return nil
	}
	for _, h := range a.foot {
		if h.has(v.X, v.Y) {
			return a.key(keyMsg(h.Key))
		}
	}
	if a.keys || a.dialog != nil {
		return nil
	}
	// The tabs along the top; not under a question, which takes clicks
	// on its options only.
	for id, s := range a.nav {
		if v.Y == 1 && v.X >= s.x0 && v.X < s.x1 && !a.dialogUp() {
			return a.goTab(id)
		}
	}
	return a.bodyClick(v)
}

// bodyClick is a click on the view's body, under the header (and the
// tabs).
func (a *App) bodyClick(v tea.MouseMsg) tea.Cmd {
	for _, h := range a.view().Hits() {
		if !h.has(v.X, v.Y-a.top) {
			continue
		}
		if h.Key != "" {
			return a.key(keyMsg(h.Key))
		}
		now := a.now()
		again := a.click != nil && a.click.Y0 == h.Y0 && a.click.X0 == h.X0 && now.Sub(a.clickAt) < doubleClick
		a.click, a.clickAt = &h, now
		if h.Do != nil {
			h.Do()
		}
		if again {
			a.click = nil
			return a.key(keyMsg("enter"))
		}
		return nil
	}
	return nil
}

// keyMsg is the key press named k, as a footer names it.
func keyMsg(k string) tea.KeyMsg {
	switch k {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "space":
		return tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
}

func (a *App) View() string {
	if a.out {
		return ""
	}
	w, h := max(50, a.width), max(12, a.height)
	cur := a.view()
	path, keys, back := cur.Path(), cur.Keys(), cur.Back()
	// ←→ along the tabs, on a tab's own screen, first.
	if !a.first && a.onTab() && !a.dialogUp() {
		keys = append([]Key{{Key: "←→", What: "tabs", Keep: true}}, keys...)
	}
	// The tabs along the top, but on a first run (no home yet).
	var nav []string
	a.nav = nil
	tab := ""
	if !a.first {
		tabs, ids := a.Menu.navTabs()
		sel := a.navSel(ids)
		var spots []tabSpot
		nav, spots = navLines(tabs, sel, w)
		a.nav = map[string]tabSpot{}
		for _, s := range spots {
			a.nav[ids[s.n]] = s
		}
		if sel > 0 {
			tab = a.Menu.rows()[sel-1].label // its name, without the count
		}
	}
	a.top = 1 + len(nav)
	bh := h - 3 - len(nav)
	var body string
	switch {
	case a.keys:
		path += " › Keys"
		body = keysBody(cur.Help(), w, bh)
		keys, back = []Key{{Key: "q", What: "close"}}, ""
	default:
		body = cur.Body(w, bh)
	}
	toast := cmpOr(a.toast, cur.Toast())
	if a.dialog != nil {
		lines := strings.Split(body, "\n")
		dl := dialogLines(a.dialog, w, 0, nil)
		cut := max(0, min(len(lines), bh-len(dl)))
		body = strings.Join(append(lines[:cut], dl...), "\n")
		keys, back = dialogKeys(a.dialog), ""
	}
	var foot string
	foot, a.foot = footer(keys, back, w, h-1)
	return frame(a.Menu.info, crumb(path, tab, len(nav) > 0), nav, body, toast, foot, w, h)
}

// crumb is the path as the header says it: under the tabs, so without
// Home, and nothing when it's only the tab the strip has selected (Home
// too).
func crumb(path, tab string, nav bool) string {
	if !nav {
		return path
	}
	p := strings.TrimPrefix(path, "Home › ")
	if p == tab || p == "Home" {
		return ""
	}
	return p
}

// cmpOr is the first of ss that isn't empty.
func cmpOr(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}

// frame lays out a screen w by h: the header, the tabs along the top
// (nav: none, or the strip and its rule), the body (cut or padded to
// fit, each line cut to w: a wrapped one would push the rest down), the
// toast line, the footer.
func frame(info HomeInfo, path string, nav []string, body, toast, foot string, w, h int) string {
	b := append([]string{header(info, path, w)}, nav...)
	room := h - 3 - len(nav)
	lines := strings.Split(body, "\n")
	if len(lines) > room {
		lines = lines[:max(0, room)]
	}
	for _, l := range lines {
		if ansi.StringWidth(l) > w {
			l = ansi.Truncate(l, w, "…")
		}
		b = append(b, l)
	}
	for len(b) < h-2 {
		b = append(b, "")
	}
	return strings.Join(append(b, toastLine(toast, w), foot), "\n")
}

// toastLine is the toast, w wide, in the colour of what it says, by its
// first mark: done (✓, or the look's OK) green, a warning (▲, or Warn)
// yellow, a failure (✗, or Down) red; anything else plain. Classic's
// marks are written as the look's.
func toastLine(t string, w int) string {
	if t == "" {
		return ""
	}
	mk := tui.Cur.Mark
	st := lipgloss.NewStyle()
	for _, s := range []struct {
		classic, own string
		st           lipgloss.Style
	}{{"✓", mk.OK, green}, {"▲", mk.Warn, yellow}, {"✗", mk.Down, red}} {
		for _, g := range []string{s.classic, s.own} {
			if rest, ok := strings.CutPrefix(t, g+" "); ok {
				t, st = fmt.Sprintf("%s %s", s.own, rest), s.st
				break
			}
		}
	}
	return " " + st.Render(ansi.Truncate(t, w-2, "…"))
}

// header is a screen's top line: yardo, the host and the user (cut
// first, when the line is short), then where the screen is.
func header(info HomeInfo, path string, w int) string {
	right := ""
	if path != "" {
		parts := strings.Split(path, " › ")
		for n, p := range parts {
			if n > 0 {
				right += muted.Render(tui.Title(" › "))
			}
			if n == len(parts)-1 {
				right += bold.Render(tui.Title(p))
			} else {
				right += muted.Render(tui.Title(p))
			}
		}
		right += " "
	}
	brand := " " + accentBd.Render(tui.Cur.Brand)
	var who []string
	for _, s := range []string{info.Host, info.User} {
		if s != "" {
			who = append(who, s)
		}
	}
	rest := ""
	if len(who) > 0 {
		rest = "  ·  " + strings.Join(who, "  ·  ")
	}
	// The path keeps its room; the host and the user are cut to the rest.
	room := w - lipgloss.Width(brand) - lipgloss.Width(right) - 2
	if room < lipgloss.Width(rest) {
		rest = ansi.Truncate(rest, max(0, room), "…")
		if room < 6 {
			rest = ""
		}
	}
	left := brand + muted.Render(rest)
	if lipgloss.Width(left)+lipgloss.Width(right)+2 > w {
		right = ansi.Truncate(right, max(0, w-lipgloss.Width(left)-2), "…")
	}
	return left + strings.Repeat(" ", max(2, w-lipgloss.Width(left)-lipgloss.Width(right))) + right
}

// tabItem is one tab of a strip: its label and, beside it, a mark in
// its style.
type tabItem struct {
	label, mark string
	st          lipgloss.Style
	// cur: the cursor is on it (the home's, along the tabs), a › before
	// it saying so, without colour too.
	cur bool
}

// tabSpot is where tab n of a strip is: columns x0 to x1.
type tabSpot struct{ n, x0, x1 int }

// tabStrip is tabs in a line w wide, sel's (none when sel < 0) in the
// accent colour and in brackets, which show without colour too. Tabs that
// don't fit scroll, sel kept in view, a ‹ or › saying some are hidden;
// spots are where the ones shown are, to click.
func tabStrip(tabs []tabItem, sel, w int) (string, []tabSpot) {
	cell := func(t tabItem) int {
		n := runewidth.StringWidth(t.label) + 2
		if t.mark != "" {
			n += 1 + runewidth.StringWidth(t.mark)
		}
		return n
	}
	// width is tabs lo to hi drawn, with the arrows they need.
	width := func(lo, hi int) int {
		n := 0
		for i := lo; i < hi; i++ {
			n += cell(tabs[i])
			if i > lo {
				n++
			}
		}
		if lo > 0 {
			n += 2
		}
		if hi < len(tabs) {
			n += 2
		}
		return n
	}
	room := w - 2
	lo, hi := 0, len(tabs)
	if width(lo, hi) > room {
		// From sel, as many before it as fit, then after.
		lo = max(0, min(sel, len(tabs)-1))
		hi = lo + 1
		for lo > 0 && width(lo-1, hi) <= room {
			lo--
		}
		for hi < len(tabs) && width(lo, hi+1) <= room {
			hi++
		}
	}
	line, x := " ", 1
	if lo > 0 {
		line += muted.Render("‹ ")
		x += 2
	}
	var spots []tabSpot
	for i := lo; i < hi; i++ {
		if i > lo {
			line += " "
			x++
		}
		t := tabs[i]
		switch {
		case i == sel:
			in := t.label
			if t.mark != "" {
				in += " " + t.mark
			}
			line += tabOn.Render("[" + in + "]")
		case t.cur && t.mark != "":
			line += fmt.Sprintf("%s %s ", accent.Render("›"+t.label), t.st.Render(t.mark))
		case t.cur:
			line += accent.Render("›"+t.label) + " "
		case t.mark != "":
			line += fmt.Sprintf(" %s %s ", t.label, t.st.Render(t.mark))
		default:
			line += fmt.Sprintf(" %s ", t.label)
		}
		spots = append(spots, tabSpot{n: i, x0: x, x1: x + cell(t)})
		x += cell(t)
	}
	if hi < len(tabs) {
		line += muted.Render(" ›")
	}
	return line, spots
}

// navLines are the tabs along the top, w wide, and a rule under them.
func navLines(tabs []tabItem, sel, w int) ([]string, []tabSpot) {
	line, spots := tabStrip(tabs, sel, w)
	return []string{line, muted.Render(strings.Repeat("─", w))}, spots
}

// footer is the keys' line, at line y: each key and what it does, then
// ? and q (back says what q does; none, nor ?, when it's ""), which
// always fit; and where each key is, to click. Keys that don't fit go,
// the On and Keep ones last.
func footer(keys []Key, back string, w, y int) (string, []Hit) {
	var tail []Key
	if back != "" {
		tail = []Key{{Key: "?", What: "keys"}, {Key: "q", What: back}}
	}
	render := func(k Key) string {
		st, wt := keyOn, lipgloss.NewStyle()
		if k.On {
			st, wt = accentBd, accentBd
		}
		return fmt.Sprintf("%s %s", st.Render(k.Key), wt.Render(k.What))
	}
	// Each key costs its width and a gap of 3.
	room := w - 2 + 3
	for _, k := range tail {
		room -= 3 + lipgloss.Width(render(k))
	}
	fits := make([]bool, len(keys))
	for _, first := range []bool{true, false} {
		for n, k := range keys {
			if (k.On || k.Keep) != first {
				continue
			}
			if c := lipgloss.Width(render(k)) + 3; c <= room {
				fits[n], room = true, room-c
			} else if !first {
				room = -1 // the rest in order: none after a gap
			}
		}
	}
	var shown []Key
	for n, k := range keys {
		if fits[n] {
			shown = append(shown, k)
		}
	}
	var hits []Hit
	line, x := " ", 1
	for n, k := range append(shown, tail...) {
		sep := ""
		if n > 0 {
			sep = "   "
		}
		part := render(k)
		pw := lipgloss.Width(sep) + lipgloss.Width(part)
		line += sep + part
		if press := footKey(k.Key); press != "" {
			hits = append(hits, Hit{Y0: y, Y1: y, X0: x + len(sep), X1: x + pw, Key: press})
		}
		x += pw
	}
	return line, hits
}

// footKey is the key a click on a footer's key presses ("" for one that
// names several, such as ↑↓).
func footKey(k string) string {
	switch k {
	case "enter", "esc", "tab", "space":
		return k
	}
	if r := []rune(k); len(r) == 1 && r[0] < 128 {
		return k
	}
	return ""
}

// keysBody is the keys screen, w by h: the view's keys, then everyone's
// (but those the view lists); cut, it says how many more.
func keysBody(own []Key, w, h int) string {
	out := []string{""}
	col := 10
	var shared []Key
	for _, k := range sharedKeys {
		if !slices.ContainsFunc(own, func(o Key) bool { return slices.Contains(strings.Fields(o.Key), k.Key) }) {
			shared = append(shared, k)
		}
	}
	for _, k := range append(append([]Key{}, own...), shared...) {
		col = max(col, runewidth.StringWidth(k.Key))
	}
	list := func(title string, ks []Key) {
		out = append(out, "  "+bold.Render(title))
		for _, k := range ks {
			out = append(out, fmt.Sprintf("    %s  %s", keyOn.Render(padTo(k.Key, col)), runewidth.Truncate(k.What, max(10, w-col-8), "…")))
		}
		out = append(out, "")
	}
	if len(own) > 0 {
		list("This screen", own)
	}
	list("Every screen", shared)
	out = append(out, "  "+muted.Render("q, esc or ? closes this"))
	if len(out) > h && h > 1 {
		out = append(out[:h-1], "  "+muted.Render(fmt.Sprintf("↓ %d more (a taller window shows them)", len(out)-h+1)))
	}
	return strings.Join(out, "\n")
}

// Run shows the app; it returns what it was left with.
func (a *App) Run() (AppResult, error) {
	a.out = false
	a.result = AppResult{}
	if _, err := tea.NewProgram(a, tea.WithAltScreen(), tea.WithMouseCellMotion()).Run(); err != nil {
		return AppResult{Quit: true}, err
	}
	a.out = true
	return a.result, nil
}

// standalone draws v alone, as the app would, w by h, with nav (the
// tabs along the top, or none; tab the name of the one it's on): for a
// view's own View.
func standalone(v view, info HomeInfo, nav []string, tab string, w, h int) string {
	w, h = max(50, w), max(12, h)
	foot, _ := footer(v.Keys(), v.Back(), w, h-1)
	return frame(info, crumb(v.Path(), tab, len(nav) > 0), nav, v.Body(w, h-3-len(nav)), v.Toast(), foot, w, h)
}
