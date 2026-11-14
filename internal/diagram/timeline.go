package diagram

import (
	"encoding/json"
	"strings"

	"github.com/ifedan-ed/figures-go/internal/core"
)

var timelineKind = core.Kind{
	Name:   "timeline",
	Family: "diagram",
	Use:    "Events in order down a vertical line, with optional phases beside it: developmental milestones, a vaccine schedule, the course of a disease; for steps with decisions use flowchart, for a value over time use a chart.",
	Fields: "events: [{when (" + core.UpTo(tlWhenChars) + "), label (" + core.UpTo(tlLabelChars) + "), detail? (" + core.UpTo(tlDetailChars) + "), tone?}] (2 to 20), in time order. phases?: [{from, to, label (" + core.UpTo(tlPhaseChars) + "), tone?}] (at most 6), from and to being the when of an event, drawn as a bar beside the events they span.",
	Example: `{"type":"timeline","title":"Course of Kawasaki disease","alt":"Kawasaki disease: fever and mucocutaneous signs in the acute phase, peeling, high platelets and coronary aneurysms in the subacute phase, then recovery.","spec":{"events":[` +
		`{"when":"Day 1","label":"Fever begins","detail":"High, little response to antipyretics"},` +
		`{"when":"Days 2–7","label":"Mucocutaneous signs","detail":"Conjunctivitis, red lips, rash, swollen hands and feet"},` +
		`{"when":"By day 10","label":"IVIG and aspirin","detail":"Best given within 10 days of fever onset","tone":"good"},` +
		`{"when":"Weeks 2–3","label":"Peeling of fingers and toes","detail":"Platelets rise"},` +
		`{"when":"Weeks 2–4","label":"Coronary aneurysms","detail":"Highest risk of thrombosis","tone":"bad"},` +
		`{"when":"Weeks 6–8","label":"Repeat echocardiogram","detail":"Stop aspirin if the coronaries are normal"}],` +
		`"phases":[{"from":"Day 1","to":"By day 10","label":"Acute"},{"from":"Weeks 2–3","to":"Weeks 2–4","label":"Subacute","tone":"warn"},{"from":"Weeks 6–8","to":"Weeks 6–8","label":"Convalescent","tone":"good"}]}}`,
	Parse: parseTimeline,
}

type tlSpec struct {
	Events []tlEvent `json:"events"`
	Phases []tlPhase `json:"phases"`
}

type tlEvent struct {
	When   string `json:"when"`
	Label  string `json:"label"`
	Detail string `json:"detail"`
	Tone   string `json:"tone"`
}

type tlPhase struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Label string `json:"label"`
	Tone  string `json:"tone"`
}

type timeline struct {
	events []tlEvent
	phases []tlPhase
	span   [][2]int // each phase's first and last event
	lane   []int
	lanes  int
	// laid out
	when, lab, det []textBlock
	rowY, rowH     []float64
	whenX, lineX   float64
	textX          float64
	H              float64
}

const (
	tlLaneW   = 20.0
	tlWhenMax = 96.0

	tlWhenChars   = 24
	tlLabelChars  = 60
	tlDetailChars = 140
	tlPhaseChars  = 30
)

func parseTimeline(raw json.RawMessage) (core.Drawing, error) {
	var s tlSpec
	if err := core.Decode(raw, &s); err != nil {
		return nil, err
	}
	var p core.Problems
	p.Count("events", len(s.Events), 2, 20)
	p.Count("phases", len(s.Phases), 0, 6)
	t := &timeline{events: s.Events, phases: s.Phases}
	at := map[string]int{}
	var whens []string
	for i := range s.Events {
		e := &s.Events[i]
		path := core.Path("events", i)
		e.When = p.Label(core.Path(path, "when"), e.When, true, tlWhenChars)
		e.Label = p.Label(core.Path(path, "label"), e.Label, true, tlLabelChars)
		e.Detail = p.Label(core.Path(path, "detail"), e.Detail, false, tlDetailChars)
		p.Tone(core.Path(path, "tone"), e.Tone)
		if _, dup := at[e.When]; dup && e.When != "" {
			p.Add(core.Path(path, "when"), "%q is the when of an earlier event too; make each when distinct (\"Day 3\", \"Day 3, evening\"), so phases can name them", e.When)
		}
		at[e.When] = i
		whens = append(whens, e.When)
	}
	for i := range s.Phases {
		ph := &s.Phases[i]
		path := core.Path("phases", i)
		ph.Label = p.Label(core.Path(path, "label"), ph.Label, true, tlPhaseChars)
		p.Tone(core.Path(path, "tone"), ph.Tone)
		a, okA := at[strings.TrimSpace(ph.From)]
		b, okB := at[strings.TrimSpace(ph.To)]
		if !okA {
			p.Add(core.Path(path, "from"), "%q is not the when of an event; use one of %s", ph.From, idList(whens))
		}
		if !okB {
			p.Add(core.Path(path, "to"), "%q is not the when of an event; use one of %s", ph.To, idList(whens))
		}
		if okA && okB && b < a {
			p.Add(path, "ends (%q) before it starts (%q); from is the earlier event", ph.To, ph.From)
		}
		t.span = append(t.span, [2]int{a, b})
	}
	if p.Any() {
		return nil, p.Err()
	}
	// Overlapping phases take separate lanes.
	t.lane = make([]int, len(s.Phases))
	for i, sp := range t.span {
		used := map[int]bool{}
		for j := 0; j < i; j++ {
			if t.span[j][0] <= sp[1] && sp[0] <= t.span[j][1] {
				used[t.lane[j]] = true
			}
		}
		for used[t.lane[i]] {
			t.lane[i]++
		}
		t.lanes = max(t.lanes, t.lane[i]+1)
	}
	t.layout()
	if t.H > maxBody {
		tooTall(&p, "timeline", t.H, "shorten the details, or draw fewer events")
		return nil, p.Err()
	}
	return t, nil
}

func (t *timeline) layout() {
	n := len(t.events)
	t.when, t.lab, t.det = make([]textBlock, n), make([]textBlock, n), make([]textBlock, n)
	whenW := 0.0
	for i, e := range t.events {
		t.when[i] = wrapBlock(e.When, 12, tlWhenMax, true)
		whenW = max(whenW, t.when[i].w)
	}
	lanesW := float64(t.lanes) * (tlLaneW + 6)
	t.whenX = margin + lanesW + whenW
	t.lineX = t.whenX + 16
	t.textX = t.lineX + 16
	textW := core.Width - margin - t.textX
	t.rowY, t.rowH = make([]float64, n), make([]float64, n)
	for i, e := range t.events {
		t.lab[i] = wrapPara(e.Label, nodeSize, textW, true)
		h := t.lab[i].h
		if e.Detail != "" {
			t.det[i] = wrapPara(e.Detail, 11.5, textW, false)
			h += t.det[i].h + 2
		}
		t.rowH[i] = max(h, t.when[i].h) + 18
	}
	// A phase's bar must be long enough for its label, written along it.
	for i, ph := range t.phases {
		need := core.TextWidth(ph.Label, labelSize, true) + 16
		a, b := t.span[i][0], t.span[i][1]
		have := 0.0
		for k := a; k <= b; k++ {
			have += t.rowH[k]
		}
		have -= 10
		if have < need {
			extra := (need - have) / float64(b-a+1)
			for k := a; k <= b; k++ {
				t.rowH[k] += extra
			}
		}
	}
	y := 4.0
	for i := range t.events {
		t.rowY[i] = y
		y += t.rowH[i]
	}
	t.H = y
}

// dotY is where event i's dot sits: level with its label's first line.
func (t *timeline) dotY(i int) float64 { return t.rowY[i] + 9 }

func (t *timeline) Draw(c *core.Canvas) error {
	th := c.T
	n := len(t.events)
	// Phases first, behind.
	for i, ph := range t.phases {
		a, b := t.span[i][0], t.span[i][1]
		x := margin + float64(t.lane[i])*(tlLaneW+6)
		y0, y1 := t.rowY[a]+1, t.rowY[b]+t.rowH[b]-11
		stroke, fill := toneOf(th, ph.Tone, th.Series[0], th.SeriesSoft[0])
		c.Rect(x, y0, tlLaneW, y1-y0, 5, fill, stroke, 1)
		cx, cy := x+tlLaneW/2, (y0+y1)/2
		c.Open("g", "transform", "rotate(-90 "+core.F(cx)+" "+core.F(cy)+")")
		c.Text(cx, cy, ph.Label, core.TextStyle{Size: labelSize, Bold: true, Fill: th.Ink, Anchor: core.Middle})
		c.Close("g")
	}
	c.Line(t.lineX, t.dotY(0), t.lineX, t.dotY(n-1), th.Edge, 2, "")
	for i, e := range t.events {
		y := t.dotY(i)
		stroke, fill := toneOf(th, e.Tone, th.Series[0], th.SeriesSoft[0])
		c.Circle(t.lineX, y, 6, fill, stroke, 2)
		t.when[i].draw(c, t.whenX, t.rowY[i]+t.when[i].h/2+1.5, th.Muted, core.End, false)
		top := t.rowY[i] + 1.5
		t.lab[i].draw(c, t.textX, top+t.lab[i].h/2, th.Ink, core.Start, false)
		if e.Detail != "" {
			top += t.lab[i].h + 2
			t.det[i].draw(c, t.textX, top+t.det[i].h/2, th.Muted, core.Start, false)
		}
	}
	c.H = t.H
	return nil
}
