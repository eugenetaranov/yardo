package ui

import (
	"bytes"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"

	"github.com/eugenetaranov/yardo/internal/tui"
)

// The findings: a held add-on's down the left (what to look at, then
// what explains it, then the notes, folded by kind: enter unfolds one);
// on the right the one under the cursor in full, where its file is on
// disk, and its code around its line. Enter opens the file in the
// editor, read only. It's the Review screen's Findings.

// Finding is one of a held add-on's findings, as the screen shows it:
// Level block, ok or note; Group its kind as the folded notes name it;
// Short its line in a group (Text when ""); By who found it.
type Finding struct {
	Level, Group string
	File         string
	Line         int
	Text, Short  string
	Next, By     string
	// Kind is what it is, in a few words ("Contacts the network"); Role
	// what its file is to the agent; Why why it matters here, a line each.
	Kind, Role string
	Why        []string
	// Look: worth your eyes; New: new since you approved it (an update).
	Look, New bool
}

// fileAt is a file the screen read: its content and its place on disk.
type fileAt struct {
	b    []byte
	path string
	ok   bool
}

type findings struct {
	fs   []Finding
	read func(file string) ([]byte, string, bool)
	// edit is the editor's command for path at line (editCmd; a seam).
	edit  func(path string, line int) *exec.Cmd
	files map[string]fileAt
	// open are the folded groups unfolded, by name.
	open map[string]bool
	// cur is the cursor, of the rows that take it; top the list's first
	// row shown; off the code's scroll.
	cur, top, off int
	// page is how many code lines showed last: a scroll's step.
	page int
	note string
}

// editedMsg is the editor done.
type editedMsg struct{ err error }

func newFindings(fs []Finding, read func(string) ([]byte, string, bool)) *findings {
	return &findings{fs: fs, read: read, edit: editCmd, files: map[string]fileAt{}, open: map[string]bool{}, page: 10}
}

// editCmd opens path at line in $VISUAL or $EDITOR (else vi): vi's kin
// read only, and with no modelines (the file isn't trusted).
func editCmd(path string, line int) *exec.Cmd {
	ed := strings.Fields(cmpOr(os.Getenv("VISUAL"), os.Getenv("EDITOR"), "vi"))
	args := ed[1:]
	switch filepath.Base(ed[0]) {
	case "vim", "nvim", "view", "nview":
		args = append(args, "-R", "--cmd", "set nomodeline")
	case "vi":
		args = append(args, "-R")
	}
	if line > 0 {
		args = append(args, fmt.Sprintf("+%d", line))
	}
	return exec.Command(ed[0], append(args, path)...)
}

// editor is the editor's name, as the keys say it.
func editor() string {
	return filepath.Base(cmpOr(strings.Fields(cmpOr(os.Getenv("VISUAL"), os.Getenv("EDITOR"), "vi"))...))
}

// file is f's file, read once.
func (m *findings) file(name string) fileAt {
	if a, ok := m.files[name]; ok {
		return a
	}
	b, p, ok := m.read(name)
	m.files[name] = fileAt{b, p, ok}
	return m.files[name]
}

// fRow is a line of the list: a section's heading, finding n, or a
// folded group (by its name).
type fRow struct {
	head, group string
	n           int
}

// explains: f explains what it can do, not worth a look of its own.
func explains(f Finding) bool { return !f.Look && len(f.Why) > 0 }

// groups are the folded notes' kinds, in the order first found, and
// each one's findings.
func (m *findings) groups() ([]string, map[string][]int) {
	var order []string
	by := map[string][]int{}
	for n, f := range m.fs {
		if f.Look || explains(f) {
			continue
		}
		g := cmpOr(f.Group, f.Kind, "Notes")
		if by[g] == nil {
			order = append(order, g)
		}
		by[g] = append(by[g], n)
	}
	return order, by
}

func (m *findings) rows() []fRow {
	var out []fRow
	section := func(head string, ok func(Finding) bool) {
		var ns []int
		for n, f := range m.fs {
			if ok(f) {
				ns = append(ns, n)
			}
		}
		if len(ns) == 0 {
			return
		}
		out = append(out, fRow{head: fmt.Sprintf("%s (%d)", head, len(ns)), n: -1})
		for _, n := range ns {
			out = append(out, fRow{n: n})
		}
	}
	section("Look at these", func(f Finding) bool { return f.Look })
	section("Context", explains)
	order, by := m.groups()
	if len(order) > 0 {
		all := 0
		for _, g := range order {
			all += len(by[g])
		}
		out = append(out, fRow{head: fmt.Sprintf("Folded notes (%d)", all), n: -1})
	}
	for _, g := range order {
		out = append(out, fRow{group: g, n: -1})
		if m.open[g] {
			for _, n := range by[g] {
				out = append(out, fRow{group: g, n: n})
			}
		}
	}
	return out
}

// picks are the rows the cursor goes to (not headings).
func (m *findings) picks() []fRow {
	return slices.DeleteFunc(m.rows(), func(r fRow) bool { return r.head != "" })
}

// at is the row under the cursor (ok false: none).
func (m *findings) at() (fRow, bool) {
	ps := m.picks()
	if len(ps) == 0 {
		return fRow{}, false
	}
	m.cur = clamp(m.cur, 0, len(ps)-1)
	return ps[m.cur], true
}

// move moves the cursor n rows.
func (m *findings) move(n int) {
	if c := clamp(m.cur+n, 0, len(m.picks())-1); c != m.cur {
		m.cur, m.off = c, 0
	}
}

// focus puts the cursor on finding n.
func (m *findings) focus(n int) {
	if i := slices.IndexFunc(m.picks(), func(r fRow) bool { return r.n == n }); i >= 0 {
		m.cur, m.off = i, 0
	}
}

// key is k on the list: ↑↓ move, J K scroll the code, enter opens the
// finding's file in the editor, or unfolds (folds) a group.
func (m *findings) key(k string) tea.Cmd {
	m.note = ""
	switch k {
	case "up", "k", "shift+tab":
		m.move(-1)
	case "down", "j", "tab":
		m.move(1)
	case "shift+up", "K":
		m.off--
	case "shift+down", "J", " ":
		m.off++
	case "ctrl+u":
		m.off -= max(1, m.page-2)
	case "ctrl+d":
		m.off += max(1, m.page-2)
	case "enter", "o":
		r, ok := m.at()
		switch {
		case !ok:
		case r.n < 0:
			m.open[r.group] = !m.open[r.group]
		case m.fs[r.n].File == "":
		default:
			f := m.fs[r.n]
			a := m.file(f.File)
			if !a.ok || a.path == "" {
				m.note = f.File + " isn't among its files now"
				break
			}
			return tea.ExecProcess(m.edit(a.path, f.Line), func(err error) tea.Msg { return editedMsg{err} })
		}
	}
	return nil
}

// edited is the editor done: a failure said.
func (m *findings) edited(err error) {
	m.note = ""
	if err != nil {
		m.note = "Couldn't open it: " + err.Error()
	}
}

// levelMark is a level's mark and its colour.
func levelMark(l string) string {
	switch l {
	case "block":
		return red.Render(tui.Cur.Mark.Down)
	case "ok":
		return yellow.Render(tui.Cur.Mark.Warn)
	}
	return muted.Render(tui.Cur.Mark.Unchecked)
}

// levelGlyph is levelMark without its colour.
func levelGlyph(l string) string {
	switch l {
	case "block":
		return tui.Cur.Mark.Down
	case "ok":
		return tui.Cur.Mark.Warn
	}
	return tui.Cur.Mark.Unchecked
}

// tail is s at most w wide, cut from the left (a path's end says most).
func tail(s string, w int) string {
	if runewidth.StringWidth(s) <= w {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && runewidth.StringWidth(string(r))+1 > w {
		r = r[1:]
	}
	return "…" + string(r)
}

// where is f's file:line, or the add-on.
func (f Finding) where() string {
	w := cmpOr(f.File, "the add-on")
	if f.Line > 0 {
		w = fmt.Sprintf("%s:%d", w, f.Line)
	}
	return w
}

// list is the list's lines, w wide and h high: the cursor's in view
// (and its heading, when it's its section's first), ↑ and ↓ say how many
// more.
func (m *findings) list(w, h int) []string {
	rows := m.rows()
	cur, _ := m.at()
	at := slices.IndexFunc(rows, func(r fRow) bool { return r.head == "" && r.n == cur.n && r.group == cur.group })
	lo := at
	if lo > 0 && rows[lo-1].head != "" {
		lo--
	}
	top, end := 0, len(rows)
	if len(rows) > h {
		top = m.top
		for {
			room := h
			if top > 0 {
				room--
			}
			if top+room < len(rows) {
				room--
			}
			end = min(len(rows), top+room)
			if lo < top {
				top = lo
				continue
			}
			if at >= end {
				top += at - end + 1
				continue
			}
			break
		}
		// No heading last, its rows all below.
		if end < len(rows) && end-1 > at && rows[end-1].head != "" {
			end--
		}
	}
	m.top = top
	count := func(rs []fRow) int {
		return len(slices.DeleteFunc(slices.Clone(rs), func(r fRow) bool { return r.head != "" }))
	}
	_, by := m.groups()
	var out []string
	if top > 0 {
		out = append(out, muted.Render(fmt.Sprintf("↑ %d more", count(rows[:top]))))
	}
	for _, r := range rows[top:end] {
		on := r.head == "" && r.n == cur.n && r.group == cur.group
		var plain, styled string
		switch {
		case r.head != "":
			out = append(out, bold.Render(runewidth.Truncate(r.head, w, "…")))
			continue
		case r.n < 0:
			fold := "▸"
			if m.open[r.group] {
				fold = "▾"
			}
			plain = fmt.Sprintf("%s %s (%d)", fold, r.group, len(by[r.group]))
			styled = muted.Render(fold) + plain[len(fold):]
		case r.group != "":
			f := m.fs[r.n]
			ww := tail(f.where(), max(12, w/2))
			plain = runewidth.Truncate(fmt.Sprintf("  %s  %s", ww, oneLine(cmpOr(f.Short, f.Text))), w-2, "…")
			styled = plain
		default:
			f := m.fs[r.n]
			what := oneLine(cmpOr(f.Kind, f.Short, f.Text))
			kw := max(10, (w-4)*11/20)
			ww := tail(filepath.Base(f.File)+strings.TrimPrefix(f.where(), f.File), max(8, w-4-kw-1))
			if f.File == "" {
				ww = "the add-on"
			}
			plain = fmt.Sprintf("%s %s %s", levelGlyph(f.Level), padTo(what, kw), ww)
			styled = fmt.Sprintf("%s %s %s", levelMark(f.Level), padTo(what, kw), keyOn.Render(ww))
		}
		if on {
			out = append(out, rowOn.Render(padTo("› "+plain, w)))
			continue
		}
		out = append(out, "  "+styled)
	}
	if end < len(rows) {
		for len(out) < h-1 {
			out = append(out, "")
		}
		out = append(out, muted.Render(fmt.Sprintf("↓ %d more", count(rows[end:]))))
	}
	return out
}

// oneLine is s with what isn't printable made a space: a list row.
func oneLine(s string) string {
	return strings.Map(func(r rune) rune {
		if r != ' ' && (!unicode.IsGraphic(r) || unicode.Is(unicode.Zs, r)) {
			return ' '
		}
		return r
	}, s)
}

// chunk is s in pieces w wide: a path, which isn't cut.
func chunk(s string, w int) []string {
	var out []string
	for runewidth.StringWidth(s) > w && w > 0 {
		c := runewidth.Truncate(s, w, "")
		if c == "" {
			c = string([]rune(s)[0])
		}
		out = append(out, c)
		s = s[len(c):]
	}
	return append(out, s)
}

// detail is the row under the cursor, w wide and h high; and what its
// box says on its right (which lines).
func (m *findings) detail(w, h int) ([]string, string) {
	r, ok := m.at()
	switch {
	case !ok:
		return []string{muted.Render("Nothing found.")}, ""
	case r.n < 0:
		return m.groupDetail(r.group, w), ""
	}
	return m.findingDetail(m.fs[r.n], w, h)
}

// groupDetail says what a folded group is: how many, where (by its
// files' roles), and that enter lists them.
func (m *findings) groupDetail(g string, w int) []string {
	_, by := m.groups()
	ns := by[g]
	out := []string{bold.Render(runewidth.Truncate(fmt.Sprintf("%s (%d)", g, len(ns)), w, "…")), ""}
	roles, files := map[string]int{}, map[string]int{}
	for _, n := range ns {
		roles[cmpOr(m.fs[n].Role, "unsorted")]++
		files[cmpOr(m.fs[n].File, "the add-on")]++
	}
	if len(roles) > 1 || roles["unsorted"] == 0 {
		var in []string
		for _, role := range append(slices.Clone(roleOrder), "unsorted") {
			if roles[role] > 0 {
				in = append(in, fmt.Sprintf("%d %s", roles[role], roleName(role)))
			}
		}
		out = append(out, wrap("In "+strings.Join(in, ", ")+".", w)...)
	}
	top := slices.SortedFunc(maps.Keys(files), func(a, b string) int { return files[b] - files[a] })
	var most []string
	for _, f := range top[:min(3, len(top))] {
		most = append(most, fmt.Sprintf("%s (%d)", f, files[f]))
	}
	out = append(out, wrap("Most in "+strings.Join(most, ", ")+".", w)...)
	out = append(out, "", muted.Render("Folded: none of them stops it."))
	say := "enter lists them one by one"
	if m.open[g] {
		say = "enter folds them again"
	}
	return append(out, keyOn.Render("enter")+muted.Render(strings.TrimPrefix(say, "enter")))
}

// findingDetail is f in full: where its file is (all of it, to open),
// what it is, why it matters, what to do and who found it; then the
// code around its line.
func (m *findings) findingDetail(f Finding, w, h int) ([]string, string) {
	var out []string
	a := fileAt{}
	if f.File == "" {
		out = append(out, muted.Render("The add-on as a whole: not one file."))
	} else {
		a = m.file(f.File)
		loc := cmpOr(a.path, f.File)
		if f.Line > 0 {
			loc = fmt.Sprintf("%s:%d", loc, f.Line)
		}
		for _, l := range chunk(loc, w) {
			out = append(out, keyOn.Render(l))
		}
	}
	out = append(out, "")
	text := f.Text
	if f.Kind != "" {
		head := levelMark(f.Level) + " " + bold.Render(runewidth.Truncate(f.Kind, w-2, "…"))
		if f.Role != "" {
			head += muted.Render("  " + roleName(f.Role))
		}
		out = append(out, head)
		out = append(out, wrap(text, w)...)
	} else {
		for n, l := range wrap(text, w-2) {
			mk := "  "
			if n == 0 {
				mk = levelMark(f.Level) + " "
			}
			out = append(out, mk+bold.Render(l))
		}
	}
	for _, y := range f.Why {
		for _, l := range wrap(y, w) {
			out = append(out, muted.Render(l))
		}
	}
	if f.Next != "" {
		out = append(out, wrap("What you can do: "+strings.TrimSuffix(f.Next, ".")+".", w)...)
	}
	if f.By != "" {
		out = append(out, muted.Render(runewidth.Truncate("("+f.By+")", w, "…)")))
	}
	if f.File == "" {
		return out, ""
	}
	out = append(out, "")
	room := h - len(out)
	switch {
	case !a.ok:
		return append(out, yellow.Render(runewidth.Truncate(f.File+" isn't among its files now.", w, "…"))), ""
	case len(a.b) == 0:
		return append(out, muted.Render("It's empty.")), ""
	case bytes.IndexByte(a.b[:min(len(a.b), 8000)], 0) >= 0:
		return append(out, muted.Render(fmt.Sprintf("A binary file (%s): no code to show.", plural(len(a.b), "byte", "bytes")))), ""
	case room < 1:
		return out, ""
	}
	lines := strings.Split(strings.TrimSuffix(string(a.b), "\n"), "\n")
	n := min(room, len(lines))
	m.page = n
	// Its line in the middle (or the file's start), moved by the scroll.
	base := 0
	if f.Line > 0 {
		base = f.Line - 1 - (n-1)/2
	}
	hi := max(0, len(lines)-n)
	start := max(0, min(hi, base+m.off))
	m.off = start - base
	nw := len(fmt.Sprint(len(lines)))
	for i := start; i < start+n; i++ {
		num := fmt.Sprintf("%*d", nw, i+1)
		if i+1 == f.Line {
			out = append(out, accent.Render("▌")+accentBd.Render(num)+" "+showLine(lines[i], w-nw-2, true))
			continue
		}
		out = append(out, " "+muted.Render(num)+" "+showLine(lines[i], w-nw-2, false))
	}
	if f.Line > len(lines) {
		out = append(out, yellow.Render(fmt.Sprintf("It has %s now: line %d isn't there.", plural(len(lines), "line", "lines"), f.Line)))
	}
	return out, fmt.Sprintf("%d–%d of %d", start+1, start+n, len(lines))
}

// showLine is a line of code at most w wide: tabs as spaces, what you
// can't see named (⟨U+200E⟩, in yellow), cut with …; on, it's in bold.
func showLine(s string, w int, on bool) string {
	s = strings.TrimSuffix(s, "\r")
	type piece struct {
		s   string
		odd bool
	}
	var ps []piece
	col := 0
	for len(s) > 0 {
		r, size := utf8.DecodeRuneInString(s)
		p := piece{s: s[:size]}
		switch {
		case r == utf8.RuneError && size == 1:
			p = piece{fmt.Sprintf("⟨0x%02X⟩", s[0]), true}
		case r == '\t':
			p.s = strings.Repeat(" ", 4-col%4)
		case r != ' ' && (!unicode.IsGraphic(r) || unicode.Is(unicode.Zs, r)):
			p = piece{fmt.Sprintf("⟨U+%04X⟩", r), true}
		}
		s = s[size:]
		col += runewidth.StringWidth(p.s)
		ps = append(ps, p)
	}
	cut := col > w
	if cut {
		w--
	}
	var b strings.Builder
	used := 0
	run := ""
	flush := func() {
		if on {
			run = bold.Render(run)
		}
		b.WriteString(run)
		run = ""
	}
	for _, p := range ps {
		pw := runewidth.StringWidth(p.s)
		if used+pw > w {
			break
		}
		used += pw
		if p.odd {
			flush()
			b.WriteString(yellow.Render(p.s))
			continue
		}
		run += p.s
	}
	flush()
	if cut {
		b.WriteString(muted.Render("…"))
	}
	return b.String()
}

// body is the list and the finding, w by h: side by side when there's
// room, else one above the other.
func (m *findings) body(w, h int) []string {
	at := ""
	if ps := m.picks(); len(ps) > 0 {
		at = fmt.Sprintf("%d of %d", m.cur+1, len(ps))
	}
	title := "Findings"
	if m.note != "" {
		title = yellow.Render(runewidth.Truncate(m.note, max(10, w/2-12), "…"))
	}
	if w >= 100 {
		// Side by side: the list two fifths, the finding the rest.
		lw := max(30, w*2/5)
		rw := w - lw - 2
		dl, dr := m.detail(rw-4, h-2)
		return beside2(box(title, at, m.list(lw-4, h-2), lw, h-2, false), box(m.where(rw/2), dr, dl, rw, h-2, true))
	}
	// One above the other: the list a third.
	lh := max(3, min(len(m.rows()), (h-4)/3))
	dl, dr := m.detail(w-4, h-4-lh)
	return append(box(title, at, m.list(w-4, lh), w, lh, false), box(m.where(w/2), dr, dl, w, h-4-lh, true)...)
}

// where is the finding box's title, at most w wide: its file's name,
// the group, or the add-on.
func (m *findings) where(w int) string {
	r, ok := m.at()
	switch {
	case !ok:
		return "The add-on"
	case r.n < 0:
		return runewidth.Truncate(r.group, w, "…")
	case m.fs[r.n].File == "":
		return "The add-on"
	}
	return runewidth.Truncate(filepath.Base(m.fs[r.n].File), w, "…")
}

// roleOrder are the roles a file can have, most weight first (as
// internal/addon has them).
var roleOrder = []string{"instructions", "script", "code", "vendored", "asset", "doc", "license"}

// roleName is a role as the screen says it.
func roleName(r string) string {
	switch r {
	case "script":
		return "script it runs"
	case "code":
		return "its own code"
	case "doc":
		return "docs"
	case "asset":
		return "assets"
	case "license":
		return "licences"
	}
	return r
}
