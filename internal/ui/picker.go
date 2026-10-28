package ui

import (
	"fmt"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/eugenetaranov/yardo/internal/tui"
)

// Tab is one picker tab. A tab with Sections lists their entries (whose
// Tab is a section's ID) in order, each section under its Title.
type Tab struct {
	ID, Title string
	Sections  []Tab
}

// ids are the entry tabs a tab shows: its sections', or its own.
func (t Tab) ids() []string {
	if len(t.Sections) == 0 {
		return []string{t.ID}
	}
	var out []string
	for _, s := range t.Sections {
		out = append(out, s.ID)
	}
	return out
}

const (
	reviewTab = "__review__"
	searchTab = "__search__"
	// ExtraTab holds the extra mise tools: ids "mise:<spec>".
	ExtraTab = "__extra__"
)

// ExtraEntry is the row of an extra mise tool (a tool the catalog lacks).
func ExtraEntry(spec string, on bool) *Entry {
	return &Entry{Tab: ExtraTab, ID: "mise:" + spec, Label: spec + " (extra mise tool)", Source: "mise use -g " + spec, On: on}
}

var (
	accent   = lipgloss.NewStyle().Foreground(tui.Accent)
	bold     = lipgloss.NewStyle().Bold(true)
	muted    = lipgloss.NewStyle().Foreground(lipgloss.Color("#8A8A8A"))
	green    = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true)
	tabOn    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#1A1A1A")).Background(tui.Accent)
	accentBd = accent.Bold(true)
)

// Options shape a picker for its place in the flow.
type Options struct {
	// Review adds the Review tab, last.
	Review bool
	// Back: esc on an empty search returns Back instead of quitting
	// (ctrl-c still quits).
	Back bool
	// Next names the screen enter on the last tab goes to (key hints).
	Next string
	// Note is a muted line atop the Review tab (what the other screen
	// picked).
	Note string
	// Extra: ctrl-a in a search adds the query as an extra mise tool
	// (ExtraEntry), for a tool the catalog lacks.
	Extra bool
}

// Outcome is how a picker run ended.
type Outcome int

const (
	Quit Outcome = iota
	Done
	Back
)

// Picker is the tabbed checklist: categories as tabs, type to search every
// tab, space toggles, Review last. Only space (and ctrl-a, which adds a
// search as an extra mise tool, ticked) changes a selection.
type Picker struct {
	title   string
	tabs    []Tab
	entries []*Entry
	opt     Options

	tab            int
	query          string
	results        []*Entry
	cursor, top    map[string]int
	width, height  int
	confirmed, out bool
	back           bool
}

// NewPicker builds a picker over entries (catalog rows, then mise tools,
// which only appear in search results).
func NewPicker(title string, tabs []Tab, entries []*Entry, o Options) *Picker {
	p := &Picker{title: title, entries: entries, opt: o, cursor: map[string]int{}, top: map[string]int{}, width: 100, height: 30}
	// Tabs and sections with nothing in them are left out.
	has := func(id string) bool { return slices.ContainsFunc(entries, func(e *Entry) bool { return e.Tab == id }) }
	for _, t := range tabs {
		if len(t.Sections) > 0 {
			t.Sections = slices.DeleteFunc(slices.Clone(t.Sections), func(s Tab) bool { return !has(s.ID) })
			if len(t.Sections) > 0 {
				p.tabs = append(p.tabs, t)
			}
		} else if has(t.ID) {
			p.tabs = append(p.tabs, t)
		}
	}
	if o.Extra && slices.ContainsFunc(entries, func(e *Entry) bool { return e.Tab == ExtraTab }) {
		p.extraTab()
	}
	if o.Review {
		p.tabs = append(p.tabs, Tab{ID: reviewTab, Title: "Review"})
	}
	return p
}

// extraTab adds the Extra tools tab (before Review) if it isn't there,
// and returns its index.
func (p *Picker) extraTab() int {
	if i := slices.IndexFunc(p.tabs, func(t Tab) bool { return t.ID == ExtraTab }); i >= 0 {
		return i
	}
	i := len(p.tabs)
	if i > 0 && p.tabs[i-1].ID == reviewTab {
		i--
	}
	p.tabs = slices.Insert(p.tabs, i, Tab{ID: ExtraTab, Title: "Extra tools"})
	return i
}

// addExtra ticks the query as an extra mise tool and shows its tab.
func (p *Picker) addExtra() {
	q := p.query
	if !p.opt.Extra || !ValidExtra(q) {
		return
	}
	if i := slices.IndexFunc(p.entries, func(e *Entry) bool { return e.ID == "mise:"+q }); i >= 0 {
		p.entries[i].On = true
	} else {
		p.entries = append(p.entries, ExtraEntry(q, true))
	}
	p.setQuery("")
	p.tab = p.extraTab()
	p.cursor[ExtraTab] = len(p.items(ExtraTab)) - 1
}

// Focus opens the picker on the tab with id (a tab, or a group's
// "group:<name>"); nothing when it has none.
func (p *Picker) Focus(id string) {
	for i, t := range p.tabs {
		if t.ID == id || slices.ContainsFunc(t.Sections, func(s Tab) bool { return s.ID == id }) {
			p.tab = i
			return
		}
	}
}

// Entries are the picker's rows, to tick some before it runs.
func (p *Picker) Entries() []*Entry { return p.entries }

// Selected returns the ticked ids, in entry order.
func (p *Picker) Selected() []string {
	var out []string
	for _, e := range p.entries {
		if e.On {
			out = append(out, e.ID)
		}
	}
	return out
}

// Outcome is how the last run ended.
func (p *Picker) Outcome() Outcome {
	switch {
	case p.confirmed:
		return Done
	case p.back:
		return Back
	}
	return Quit
}

func (p *Picker) Init() tea.Cmd { return nil }

func (p *Picker) view() string {
	if p.query != "" {
		return searchTab
	}
	return p.tabs[p.tab].ID
}

func (p *Picker) items(view string) []*Entry {
	switch view {
	case searchTab:
		return p.results
	case reviewTab:
		var out []*Entry
		for _, e := range p.entries {
			if e.On {
				out = append(out, e)
			}
		}
		return out
	}
	var out []*Entry
	for _, id := range p.tabOf(view).ids() {
		for _, e := range p.entries {
			if e.Tab == id {
				out = append(out, e)
			}
		}
	}
	return out
}

// tabOf is the tab with id (or a bare one, for a view that isn't a tab).
func (p *Picker) tabOf(id string) Tab {
	for _, t := range p.tabs {
		if t.ID == id {
			return t
		}
	}
	return Tab{ID: id}
}

// heading is the section heading above items[i] of view: its section's
// title where a new section starts, else "".
func (p *Picker) heading(view string, items []*Entry, i int) string {
	t := p.tabOf(view)
	if len(t.Sections) == 0 || (i > 0 && items[i-1].Tab == items[i].Tab) {
		return ""
	}
	for _, s := range t.Sections {
		if s.ID == items[i].Tab {
			return s.Title
		}
	}
	return ""
}

func (p *Picker) setQuery(q string) {
	p.query = q
	p.results = nil
	if q != "" {
		p.results = Search(p.entries, q)
	}
	p.cursor[searchTab], p.top[searchTab] = 0, 0
}

func (p *Picker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		p.width, p.height = m.Width, m.Height
		return p, nil
	case tea.KeyMsg:
		return p, p.key(m)
	}
	return p, nil
}

func (p *Picker) key(k tea.KeyMsg) tea.Cmd {
	view := p.view()
	items := p.items(view)
	cur := p.cursor[view]
	last := len(items) - 1
	if last < 0 {
		last = 0
	}
	clamp := func(n int) int { return max(0, min(n, last)) }
	switch k.Type {
	case tea.KeyDown:
		p.cursor[view] = clamp(cur + 1)
	case tea.KeyUp:
		p.cursor[view] = clamp(cur - 1)
	case tea.KeyHome:
		p.cursor[view] = 0
	case tea.KeyEnd:
		p.cursor[view] = last
	case tea.KeyPgDown:
		p.cursor[view] = clamp(cur + 10)
	case tea.KeyPgUp:
		p.cursor[view] = clamp(cur - 10)
	case tea.KeySpace:
		if len(items) > 0 {
			items[cur].On = !items[cur].On
			if view != reviewTab {
				p.cursor[view] = clamp(cur + 1)
			}
		}
	case tea.KeyBackspace:
		if p.query != "" {
			r := []rune(p.query)
			p.setQuery(string(r[:len(r)-1]))
		}
	case tea.KeyCtrlC:
		return tea.Quit
	case tea.KeyCtrlA:
		if p.query != "" {
			p.addExtra()
		}
	case tea.KeyCtrlX:
		for _, e := range p.entries {
			e.On = false
		}
	case tea.KeyEsc:
		if p.query == "" {
			p.back = p.opt.Back
			return tea.Quit
		}
		p.setQuery("")
	case tea.KeyEnter:
		switch {
		case p.query != "":
			p.setQuery("")
			switch {
			case p.opt.Review:
				p.tab = len(p.tabs) - 1
			case len(p.tabs) == 1: // a one-list picker: done
				p.confirmed = true
				return tea.Quit
			}
		case p.tab == len(p.tabs)-1:
			p.confirmed = true
			return tea.Quit
		default:
			p.tab++
		}
	case tea.KeyRight, tea.KeyTab:
		if p.query == "" {
			p.tab = (p.tab + 1) % len(p.tabs)
		}
	case tea.KeyLeft, tea.KeyShiftTab:
		if p.query == "" {
			p.tab = (p.tab - 1 + len(p.tabs)) % len(p.tabs)
		}
	case tea.KeyRunes:
		s := string(k.Runes)
		if p.query == "" && len(s) == 1 && s[0] >= '1' && s[0] <= '9' && int(s[0]-'1') < len(p.tabs) {
			p.tab = int(s[0] - '1')
			return nil
		}
		if strings.TrimSpace(s) != "" {
			p.setQuery(p.query + s)
		}
	}
	return nil
}

// wrap splits s into lines at most w cells wide, at spaces where it can.
func wrap(s string, w int) []string {
	if w < 1 {
		w = 1
	}
	var lines []string
	line := ""
	for _, word := range strings.Fields(s) {
		for runewidth.StringWidth(word) > w {
			cut := runewidth.Truncate(word, w, "")
			if line != "" {
				lines = append(lines, line)
				line = ""
			}
			lines = append(lines, cut)
			word = word[len(cut):]
		}
		switch {
		case line == "":
			line = word
		case runewidth.StringWidth(line)+1+runewidth.StringWidth(word) <= w:
			line += " " + word
		default:
			lines = append(lines, line)
			line = word
		}
	}
	if line != "" || len(lines) == 0 {
		lines = append(lines, line)
	}
	return lines
}

// tabTitle is the title of the tab or section with id.
func (p *Picker) tabTitle(id string) string {
	for _, t := range p.tabs {
		if t.ID == id {
			return t.Title
		}
		for _, s := range t.Sections {
			if s.ID == id {
				return s.Title
			}
		}
	}
	return ""
}

func (p *Picker) View() string {
	if p.out {
		return ""
	}
	w, h := p.width, p.height
	var b strings.Builder
	chosen := len(p.Selected())
	right := fmt.Sprintf("%d selected", chosen)
	rs := muted
	if chosen > 0 {
		rs = green
	}
	gap := max(1, w-2-runewidth.StringWidth(p.title)-runewidth.StringWidth(right)-1)
	b.WriteString(" " + accentBd.Render(p.title) + strings.Repeat(" ", gap) + rs.Render(right) + "\n\n")

	view := p.view()
	if view == searchTab {
		n := len(p.results)
		plus := ""
		if n >= MaxResults {
			plus = "+"
		}
		es := "es"
		if n == 1 {
			es = ""
		}
		b.WriteString(" " + muted.Render("Search: ") + accentBd.Render(p.query+"_") + "  " + muted.Render(fmt.Sprintf("%d%s match%s", n, plus, es)) + "\n")
	} else {
		b.WriteString(p.tabBar(w, chosen) + "\n")
	}
	b.WriteString(muted.Render(strings.Repeat("─", max(1, w-1))) + "\n")

	items := p.items(view)
	listH := max(1, h-7) // the focused item's details take a line
	if view == reviewTab && p.opt.Note != "" {
		for _, l := range wrap(p.opt.Note, max(4, w-4)) {
			b.WriteString("   " + muted.Render(l) + "\n")
			listH--
		}
		listH = max(1, listH)
	}
	cur := min(p.cursor[view], max(0, len(items)-1))
	p.cursor[view] = cur

	// Labels only: the focused item's install details are on the line
	// below the list, so rows stay one line and easy to scan. Search
	// results also show their tab.
	labelW := max(14, w-10)
	srcHead := 0
	if view == searchTab {
		for _, e := range items {
			srcHead = max(srcHead, runewidth.StringWidth(p.tabTitle(e.Tab)))
		}
		srcHead = min(14, max(4, srcHead))
		labelW = max(14, w-7-srcHead-3)
	}
	srcX := 7 + labelW
	srcW := max(4, w-srcX-3)
	// A section's heading is drawn as the first line of its first row,
	// so the cursor never lands on it.
	type row struct {
		head     string
		lab, src []string
		body, h  int // lines: the label's, and with the heading (and its gap)
	}
	rows := make([]row, len(items))
	for i, e := range items {
		lab := wrap(e.Label, labelW-2)
		var src []string
		if view == searchTab {
			src = wrap(p.tabTitle(e.Tab), srcW)
		}
		head := p.heading(view, items, i)
		body := max(len(lab), len(src))
		h := body
		if head != "" {
			h += min(i, 1) + 1
		}
		rows[i] = row{head, lab, src, body, h}
	}
	top := p.top[view]
	if cur < top {
		top = cur
	}
	for top < cur {
		sum := 0
		for _, r := range rows[top : cur+1] {
			sum += r.h
		}
		if sum <= listH {
			break
		}
		top++
	}
	p.top[view] = top

	lines := 0
	if len(items) == 0 {
		msg := "(empty)"
		switch view {
		case searchTab:
			msg = "No matches — backspace to edit, esc to clear."
			if p.opt.Extra && ValidExtra(p.query) {
				msg = fmt.Sprintf("%q isn't in the catalog — ctrl-a adds it as an extra mise tool (mise use -g %s; pipx:x or github:owner/repo work too).", p.query, p.query)
			}
		case reviewTab:
			msg = "Nothing selected yet — go back with ← and tick items with space."
		}
		b.WriteString("\n")
		lines = 1
		for _, l := range wrap(msg, max(4, w-4)) {
			b.WriteString("   " + muted.Render(l) + "\n")
			lines++
		}
	}
	for i := top; i < len(items) && lines < listH; i++ {
		e, r := items[i], rows[i]
		sel := i == cur
		if r.head != "" {
			// A gap above every heading but the list's first.
			if i > top && lines < listH-1 {
				b.WriteString("\n")
				lines++
			}
			b.WriteString("   " + accentBd.Render(r.head) + "\n")
			lines++
		}
		for n := 0; n < r.body && lines < listH; n++ {
			mark, box := "  ", "    "
			if n == 0 {
				if sel {
					mark = accentBd.Render("›") + " "
				}
				box = muted.Render("[ ]") + " "
				if e.On {
					box = green.Render("[x]") + " "
				}
			}
			lab := ""
			if n < len(r.lab) {
				lab = r.lab[n]
				if sel {
					lab = bold.Render(lab)
				}
			}
			pad := max(0, labelW-runewidth.StringWidth(r.lab[min(n, len(r.lab)-1)]))
			if n >= len(r.lab) {
				pad = labelW
			}
			src := ""
			if n < len(r.src) {
				src = muted.Render(r.src[n])
			}
			b.WriteString(" " + mark + box + lab + strings.Repeat(" ", pad) + src + "\n")
			lines++
		}
	}
	for ; lines < listH; lines++ {
		b.WriteString("\n")
	}
	detail := ""
	if cur < len(items) {
		detail = runewidth.Truncate(items[cur].Source, max(4, w-5), "…")
	}
	b.WriteString("   " + muted.Render(detail) + "\n")
	esc := "esc quit"
	if p.opt.Back {
		esc = "esc back"
	}
	enter := "enter next"
	if p.tab == len(p.tabs)-1 && p.opt.Next != "" {
		enter = "enter " + p.opt.Next + " →"
	}
	keys := "type to search · ←/→ category · ↑/↓ move · space toggle · " + enter + " · " + esc
	if chosen > 0 {
		keys += " · ctrl-x untick all"
	}
	switch view {
	case searchTab:
		switch {
		case p.opt.Review:
			keys = "esc clear · ↑/↓ move · space toggle · enter review"
		case len(p.tabs) == 1:
			keys = "esc clear · ↑/↓ move · space toggle · enter confirm"
		default:
			keys = "esc clear · ↑/↓ move · space toggle · enter done"
		}
		if p.opt.Extra && ValidExtra(p.query) {
			keys += " · ctrl-a add as mise tool"
		}
	case reviewTab:
		keys = "space untick · ← back · type to search · enter confirm · " + esc
	}
	b.WriteString(" " + muted.Render(keys))
	return b.String()
}

func (p *Picker) tabBar(w, chosen int) string {
	names := make([]string, len(p.tabs))
	for i, t := range p.tabs {
		k := chosen
		if t.ID != reviewTab {
			k = 0
			for _, e := range p.items(t.ID) {
				if e.On {
					k++
				}
			}
		}
		names[i] = " " + t.Title + " "
		if k > 0 {
			names[i] = fmt.Sprintf(" %s (%d) ", t.Title, k)
		}
	}
	first := 0
	width := func(from, to int) int {
		n := 0
		for _, s := range names[from : to+1] {
			n += runewidth.StringWidth(s) + 1
		}
		return n
	}
	for width(first, p.tab) > w-6 && first < p.tab {
		first++
	}
	var b strings.Builder
	b.WriteString(" ")
	x := 1
	if first > 0 {
		b.WriteString(muted.Render("‹") + " ")
		x += 2
	}
	for i := first; i < len(names); i++ {
		if x+runewidth.StringWidth(names[i]) >= w-2 {
			b.WriteString(muted.Render("›"))
			break
		}
		if i == p.tab {
			b.WriteString(tabOn.Render(names[i]))
		} else {
			b.WriteString(muted.Render(names[i]))
		}
		b.WriteString(" ")
		x += runewidth.StringWidth(names[i]) + 1
	}
	return b.String()
}

// Run shows the picker on the terminal; it returns the ticked ids and how
// it ended. It can run again (after Back), keeping its ticks.
func (p *Picker) Run() (ids []string, o Outcome, err error) {
	p.out, p.confirmed, p.back = false, false, false
	if _, err := tea.NewProgram(p, tea.WithAltScreen()).Run(); err != nil {
		return nil, Quit, err
	}
	p.out = true
	return p.Selected(), p.Outcome(), nil
}
