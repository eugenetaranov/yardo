package ui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"

	"github.com/eugenetaranov/yardo/internal/tui"
)

// The app is yardo's one program: the menu (the home and its screens),
// the board (Tools) and Connection are its views, in one alt screen. Moving between
// them keeps each one's state (the board's queue, tab, cursors and
// filter); only what needs the terminal (a question, a run) ends it.
// Every view has the same chrome: a header (who, where, the path), a
// toast line, and a footer of the keys it takes; ? lists them all.

// Key is one of a screen's keys, as its footer lists it; On draws it in
// the accent colour (the apply key, with something queued).
type Key struct {
	Key, What string
	On        bool
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
// and n pick "yes" and "no"; q and esc leave it, nothing chosen (or pick
// Q, when it's set).
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
	switch {
	case p.YN && (k == "y" || k == "n"):
		return pick(map[string]string{"y": "yes", "n": "no"}[k])
	case k == "esc" || k == "q":
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
	var out []string
	out = append(out, " "+accentBd.Render("┌ "+d.Title+" "+strings.Repeat("─", max(0, w-6-runewidth.StringWidth(d.Title)))))
	for _, l := range d.Lines {
		// An indented line stays indented, wrapped.
		body := strings.TrimLeft(l, " ")
		pad := strings.Repeat(" ", len(l)-len(body))
		if body == "" {
			out = append(out, " "+accentBd.Render("│"))
			continue
		}
		for _, wl := range wrap(body, w-8-len(pad)) {
			out = append(out, " "+accentBd.Render("│")+"  "+pad+wl)
		}
	}
	for n, o := range d.Opts {
		line := "    " + o[0]
		if n == d.Sel {
			line = "  " + rowOn.Render(padTo(" › "+o[0], w-14))
		}
		if hits != nil {
			n := n
			*hits = append(*hits, Hit{Y0: y + len(out), Y1: y + len(out), Do: func() { d.Sel = n }})
		}
		out = append(out, " "+accentBd.Render("│")+line)
	}
	return append(out, " "+accentBd.Render("└"+strings.Repeat("─", max(0, w-4))))
}

// dialogKeys are a dialog's footer keys.
func dialogKeys(d *Dialog) []Key {
	if len(d.Opts) == 0 {
		return []Key{{Key: "enter", What: "close"}}
	}
	keys := []Key{{Key: "↑↓", What: "choose"}, {Key: "enter", What: "do it"}}
	if d.YN {
		keys = append(keys, Key{Key: "y", What: "yes"}, Key{Key: "n", What: "no"})
	}
	return keys
}

// sharedKeys are the keys every screen takes, last on the keys screen.
var sharedKeys = []Key{
	{Key: "↑↓ j k", What: "move"},
	{Key: "PgUp PgDn", What: "a screenful up or down"},
	{Key: "Home g", What: "the first row"},
	{Key: "End G", What: "the last row"},
	{Key: "tab", What: "the next panel or tab"},
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
	first   bool
	toast   string
	// keys: the keys screen is up; dialog the app's own question (quit
	// with a queue).
	keys   bool
	dialog *Dialog
	width  int
	height int
	// foot are the footer's hits; click the last click, at.
	foot    []Hit
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
		case p == "add" && !i.Picked, (p == "uninstall" || p == "forget") && i.Picked, p == "update" && newer(i):
			i.Pending = p
		}
	}
	nb, nm := a.Board, a.Menu
	nb.tabID, nb.tab, nb.cursor, nb.search, nb.typing = b.tabID, b.tab, b.cursor, b.search, false
	nm.screen, nm.sel, nm.mcpSel, nm.addSel, nm.fromBoard = m.screen, m.sel, m.mcpSel, m.addSel, m.fromBoard
	a.Conn.sel = c.sel
	if nm.screen == "mcp:name" {
		nm.screen = "mcp"
	}
	if a.first && !d.First {
		a.onBoard = false
	}
	nm.normalize()
}

// Open shows view ("board", "connection", or a menu screen: home, mcp,
// settings…).
func (a *App) Open(screen string) {
	a.keys, a.dialog = false, nil
	a.onBoard, a.onConn = screen == "board", screen == "connection"
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
		case "q":
			// The home with a queue on the board: asked once.
			if !a.onBoard && a.Menu.screen == "home" && a.Board.queued() > 0 {
				n := a.Board.queued()
				a.dialog = &Dialog{Title: plural(n, "change isn't", "changes aren't") + " applied", YN: true, Q: "yes", Sel: 1,
					Lines: []string{"Tools has " + plural(n, "change", "changes") + " queued: " + a.Board.queueSay() + ".", "q again quits without " + map[bool]string{true: "it", false: "them"}[n == 1] + "."},
					Opts:  [][2]string{{"Quit without " + map[bool]string{true: "it", false: "them"}[n == 1] + " (y)", "yes"}, {"Stay (n)", "no"}, {"Go to Tools to apply", "tools"}},
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
	return a.Menu.dialog != nil
}

func (a *App) send(msg tea.Msg) tea.Cmd {
	_, cmd := a.view().Update(msg)
	return a.after(cmd)
}

// after takes what a view was left with: a hop to the other view, or a
// result that ends the program.
func (a *App) after(cmd tea.Cmd) tea.Cmd {
	m, b, c := a.Menu, a.Board, a.Conn
	// A key dismissed on Connection is off the home's panel too.
	m.info.Keys = c.keys
	switch {
	case c.left:
		// Home, on the row it was opened from.
		c.left, a.onConn = false, false
		m.screen = "home"
		for n, r := range m.rows() {
			if r.id == cmpOr(m.result.ID, "connect") && m.result.Kind == MenuConnection {
				m.sel = n
			}
		}
		m.normalize()
	case c.ended:
		c.ended = false
		r := c.result
		a.result = AppResult{Conn: &r}
		return tea.Quit
	case m.ended && m.result.Kind == MenuConnection:
		m.ended = false
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
	for _, h := range a.view().Hits() {
		if !h.has(v.X, v.Y-1) { // the body starts under the header
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
	var body string
	switch {
	case a.keys:
		path += " › Keys"
		body = keysBody(cur.Help(), w)
		keys, back = nil, "close"
	default:
		body = cur.Body(w, h-3)
	}
	toast := cmpOr(a.toast, cur.Toast())
	if a.dialog != nil {
		var dl []string
		lines := strings.Split(body, "\n")
		dl = dialogLines(a.dialog, w, 0, nil)
		cut := max(0, min(len(lines), h-3-len(dl)))
		body = strings.Join(append(lines[:cut], dl...), "\n")
		keys, back = dialogKeys(a.dialog), "stay"
	}
	var foot string
	foot, a.foot = footer(keys, back, w, h-1)
	return frame(a.Menu.info, path, body, toast, foot, w, h)
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

// frame lays out a screen w by h: the header, the body (cut or padded
// to fit), the toast line, the footer.
func frame(info HomeInfo, path, body, toast, foot string, w, h int) string {
	b := []string{header(info, path, w)}
	lines := strings.Split(body, "\n")
	if len(lines) > h-3 {
		lines = lines[:h-3]
	}
	b = append(b, lines...)
	for len(b) < h-2 {
		b = append(b, "")
	}
	t := ""
	if toast != "" {
		st := green
		if strings.HasPrefix(toast, "✗") || strings.HasPrefix(toast, "▲") {
			st = yellow
		}
		t = " " + st.Render(ansi.Truncate(toast, w-2, "…"))
	}
	return strings.Join(append(b, t, foot), "\n")
}

// header is a screen's top line: yardo, the host and the user, then
// where the screen is.
func header(info HomeInfo, path string, w int) string {
	left := " " + accentBd.Render(tui.Cur.Brand) + muted.Render("  ·  "+info.Host+"  ·  "+info.User)
	parts := strings.Split(path, " › ")
	right := ""
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
	gap := w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 2 {
		return left
	}
	return left + strings.Repeat(" ", gap) + right
}

// footer is the keys' line, at line y: each key and what it does, then
// ? and q (back says what q does), which always fit; and where each key
// is, to click.
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
		return st.Render(k.Key) + " " + wt.Render(k.What)
	}
	room := w - 2
	for _, k := range tail {
		room -= 3 + lipgloss.Width(render(k))
	}
	var shown []Key
	used := 0
	for _, k := range keys {
		pw := lipgloss.Width(render(k))
		if len(shown) > 0 {
			pw += 3
		}
		if used+pw > room {
			break
		}
		shown, used = append(shown, k), used+pw
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

// keysBody is the keys screen: the view's keys, then everyone's.
func keysBody(own []Key, w int) string {
	out := []string{""}
	col := 10
	for _, k := range append(append([]Key{}, own...), sharedKeys...) {
		col = max(col, runewidth.StringWidth(k.Key))
	}
	list := func(title string, ks []Key) {
		out = append(out, "  "+bold.Render(title))
		for _, k := range ks {
			out = append(out, "    "+keyOn.Render(padTo(k.Key, col))+"  "+runewidth.Truncate(k.What, max(10, w-col-8), "…"))
		}
		out = append(out, "")
	}
	if len(own) > 0 {
		list("This screen", own)
	}
	list("Every screen", sharedKeys)
	return strings.Join(append(out, "  "+muted.Render("q, esc or ? closes this")), "\n")
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

// standalone draws v alone, as the app would, w by h: for a view's own
// View.
func standalone(v view, info HomeInfo, w, h int) string {
	w, h = max(50, w), max(12, h)
	foot, _ := footer(v.Keys(), v.Back(), w, h-1)
	return frame(info, v.Path(), v.Body(w, h-3), v.Toast(), foot, w, h)
}
