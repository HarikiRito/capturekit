package sim

import (
	"encoding/xml"
	"fmt"
	"strings"
)

// Node is one element of the on-screen accessibility tree.
type Node struct {
	Text      string `json:"text"`
	Desc      string `json:"desc"`
	ID        string `json:"id"`
	Class     string `json:"class"`
	Clickable bool   `json:"clickable"`
	Enabled   bool   `json:"enabled"`
	Checked   bool   `json:"checked"`
	Selected  bool   `json:"selected"`
	Focused   bool   `json:"focused"`
	Password  bool   `json:"password"`
	Left      int    `json:"left"`
	Top       int    `json:"top"`
	Right     int    `json:"right"`
	Bottom    int    `json:"bottom"`
}

func (n Node) Center() (int, int) { return (n.Left + n.Right) / 2, (n.Top + n.Bottom) / 2 }

func (n Node) area() int { return (n.Right - n.Left) * (n.Bottom - n.Top) }

type xmlNode struct {
	Text      string    `xml:"text,attr"`
	Desc      string    `xml:"content-desc,attr"`
	ID        string    `xml:"resource-id,attr"`
	Class     string    `xml:"class,attr"`
	Clickable string    `xml:"clickable,attr"`
	Enabled   string    `xml:"enabled,attr"`
	Checked   string    `xml:"checked,attr"`
	Selected  string    `xml:"selected,attr"`
	Focused   string    `xml:"focused,attr"`
	Password  string    `xml:"password,attr"`
	Bounds    string    `xml:"bounds,attr"`
	Children  []xmlNode `xml:"node"`
}

// ParseUIXML flattens a uiautomator dump into nodes (document order).
func ParseUIXML(data []byte) ([]Node, error) {
	if i := strings.Index(string(data), "<?xml"); i > 0 {
		data = data[i:]
	}
	if i := strings.LastIndex(string(data), "</hierarchy>"); i >= 0 {
		data = data[:i+len("</hierarchy>")]
	}
	var h struct {
		Nodes []xmlNode `xml:"node"`
	}
	if err := xml.Unmarshal(data, &h); err != nil {
		return nil, fmt.Errorf("parse ui dump: %w", err)
	}
	var out []Node
	var walk func([]xmlNode)
	walk = func(ns []xmlNode) {
		for _, x := range ns {
			n := Node{Text: x.Text, Desc: x.Desc, ID: x.ID, Class: x.Class, Clickable: x.Clickable == "true",
				Enabled: x.Enabled != "false", Checked: x.Checked == "true", Selected: x.Selected == "true",
				Focused: x.Focused == "true", Password: x.Password == "true"}
			if _, err := fmt.Sscanf(x.Bounds, "[%d,%d][%d,%d]", &n.Left, &n.Top, &n.Right, &n.Bottom); err == nil {
				out = append(out, n)
			}
			walk(x.Children)
		}
	}
	walk(h.Nodes)
	return out, nil
}

// FindNode matches query against text and content-desc (Flutter labels live in desc).
// Ranking: exact before substring, then the preferred field (desc when byDesc, else text),
// then clickable, then smallest area. exact disables substring matching. Case-insensitive.
func FindNode(nodes []Node, query string, byDesc, exact bool) (Node, bool) {
	q := strings.ToLower(query)
	var best Node
	bestRank := -1
	for _, n := range nodes {
		if n.area() <= 0 {
			continue
		}
		primary, other := n.Text, n.Desc
		if byDesc {
			primary, other = other, primary
		}
		rank, r2 := matchRank(primary, q, exact, 0), matchRank(other, q, exact, 1)
		if rank < 0 || (r2 >= 0 && r2 < rank) {
			rank = r2
		}
		if rank < 0 {
			continue
		}
		if bestRank < 0 || rank < bestRank || (rank == bestRank && better(n, best)) {
			best, bestRank = n, rank
		}
	}
	return best, bestRank >= 0
}

// better prefers a clickable node over a plain label, then the smaller one.
func better(n, best Node) bool {
	if n.Clickable != best.Clickable {
		return n.Clickable
	}
	return n.area() < best.area()
}

// matchRank is base (exact) or base+2 (substring); -1 for no match or empty field.
func matchRank(field, q string, exact bool, base int) int {
	f := strings.ToLower(field)
	switch {
	case f == "":
		return -1
	case f == q:
		return base
	case !exact && strings.Contains(f, q):
		return base + 2
	}
	return -1
}

// FieldFor returns n if it is an input field, else the nearest input field below it
// (Flutter exposes a field's label as a separate node above the EditText).
func FieldFor(nodes []Node, n Node) Node {
	if strings.Contains(n.Class, "EditText") {
		return n
	}
	best, found := n, false
	for _, c := range nodes {
		if strings.Contains(c.Class, "EditText") && c.Top >= n.Top && (!found || c.Top < best.Top) {
			best, found = c, true
		}
	}
	return best
}

// Focused returns the focused node, preferring an input field.
func Focused(nodes []Node) (Node, bool) {
	var found Node
	ok := false
	for _, n := range nodes {
		if !n.Focused {
			continue
		}
		if strings.Contains(n.Class, "EditText") {
			return n, true
		}
		if !ok {
			found, ok = n, true
		}
	}
	return found, ok
}

// Interesting reports whether a node is worth listing (has a label or is clickable).
func (n Node) Interesting() bool { return n.Text != "" || n.Desc != "" || n.Clickable }
